package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/angwz/clash-launch/internal/events"
	"strconv"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 人人可邀请（需求 R12）。
//
//   - 管理员不受限；其他人有名额（最近 30 天内发出的邀请数）与注册时长门槛，管理员可以调整，
//     也可以收回某个人的邀请权。
//   - 邀请可以用一次（默认）或多次（设上限），可以设有效期，可以作废未用完的邀请。
//   - 只有管理员发出的邀请会让对方成为「受邀用户」（可自选节点写法，反馈 #46）：普通用户的邀请只记录关系，
//     否则任何人都能靠互相邀请提升权限。
//   - 注册成功后以新用户的名义给邀请人发一条系统通知；消息会让双方出现在彼此的会话列表里，相当于互加好友。

const (
	settingInvitesPerMonth = "invites.per_month"
	settingInvitesMinAge   = "invites.min_age_days"
	defaultInvitesPerMonth = 5
	defaultInvitesMinAge   = 7
	inviteWindow           = 30 * 24 * time.Hour
	maxInviteUses          = 100
	maxInviteTTL           = 90 * 24 * time.Hour
)

// ErrInviteNotAllowed 表示当前不能发出邀请；Reason 说明原因。
type InviteDeniedError struct {
	Reason      string // banned | too_new | quota
	AvailableAt time.Time
}

func (e *InviteDeniedError) Error() string {
	return "service: 暂时不能发出邀请（" + e.Reason + "）"
}

// ErrInviteInput 表示邀请的参数不合规。
var ErrInviteInput = errors.New("service: 有效期最长 90 天，可用次数为 1–100")

// InvitePolicy 是管理员设定的邀请规则。
type InvitePolicy struct {
	PerMonth   int `json:"perMonth"`   // 每人最近 30 天可发出的邀请数；0 表示普通用户不能邀请
	MinAgeDays int `json:"minAgeDays"` // 注册满几天后才能邀请
}

func (s *Service) settingInt(ctx context.Context, key string, def int) (int, error) {
	v, err := s.Store.Settings().Get(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return def, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, nil
	}
	return n, nil
}

// GetInvitePolicy 返回当前的邀请规则（未设置时为默认值）。
func (s *Service) GetInvitePolicy(ctx context.Context) (InvitePolicy, error) {
	per, err := s.settingInt(ctx, settingInvitesPerMonth, defaultInvitesPerMonth)
	if err != nil {
		return InvitePolicy{}, err
	}
	age, err := s.settingInt(ctx, settingInvitesMinAge, defaultInvitesMinAge)
	return InvitePolicy{PerMonth: per, MinAgeDays: age}, err
}

// SetInvitePolicy 修改邀请规则。
func (s *Service) SetInvitePolicy(ctx context.Context, admin string, p InvitePolicy) error {
	if p.PerMonth < 0 || p.PerMonth > 1000 || p.MinAgeDays < 0 || p.MinAgeDays > 3650 {
		return ErrInviteInput
	}
	if err := s.Store.Settings().Set(ctx, settingInvitesPerMonth, strconv.Itoa(p.PerMonth)); err != nil {
		return err
	}
	if err := s.Store.Settings().Set(ctx, settingInvitesMinAge, strconv.Itoa(p.MinAgeDays)); err != nil {
		return err
	}
	s.audit(ctx, admin, "admin.invite.policy", map[string]string{"perMonth": strconv.Itoa(p.PerMonth), "minAgeDays": strconv.Itoa(p.MinAgeDays)})
	return nil
}

// InviteQuota 是某个人当前的邀请名额。管理员 Unlimited 为 true。
type InviteQuota struct {
	Unlimited   bool       `json:"unlimited"`
	Limit       int        `json:"limit"`
	Used        int        `json:"used"`
	Allowed     bool       `json:"allowed"`
	Reason      string     `json:"reason,omitempty"` // banned | too_new | quota | disabled
	AvailableAt *time.Time `json:"availableAt,omitempty"`
}

// QuotaFor 计算某人的邀请名额。
func (s *Service) QuotaFor(ctx context.Context, u *store.User) (InviteQuota, error) {
	if u.Role == "admin" {
		return InviteQuota{Unlimited: true, Allowed: true}, nil
	}
	p, err := s.GetInvitePolicy(ctx)
	if err != nil {
		return InviteQuota{}, err
	}
	used, err := s.Store.Invites().CountSince(ctx, u.ID, s.now().Add(-inviteWindow))
	if err != nil {
		return InviteQuota{}, err
	}
	q := InviteQuota{Limit: p.PerMonth, Used: used, Allowed: true}
	ready := u.CreatedAt.Add(time.Duration(p.MinAgeDays) * 24 * time.Hour)
	switch {
	case u.InviteBanned:
		q.Allowed, q.Reason = false, "banned"
	case p.PerMonth == 0:
		q.Allowed, q.Reason = false, "disabled"
	case s.now().Before(ready):
		q.Allowed, q.Reason, q.AvailableAt = false, "too_new", &ready
	case used >= p.PerMonth:
		q.Allowed, q.Reason = false, "quota"
	}
	return q, nil
}

// InviteInput 是发出邀请的参数。
type InviteInput struct {
	TTL     time.Duration // 0 表示不过期
	Note    string
	MaxUses int // 0 视为 1
}

// NewInvite 发出邀请，返回令牌原文（只此一次）。
func (s *Service) NewInvite(ctx context.Context, u *store.User, in InviteInput) (string, *store.Invite, error) {
	if in.MaxUses == 0 {
		in.MaxUses = 1
	}
	if in.TTL < 0 || in.TTL > maxInviteTTL || in.MaxUses < 1 || in.MaxUses > maxInviteUses {
		return "", nil, ErrInviteInput
	}
	q, err := s.QuotaFor(ctx, u)
	if err != nil {
		return "", nil, err
	}
	if !q.Allowed {
		e := &InviteDeniedError{Reason: q.Reason}
		if q.AvailableAt != nil {
			e.AvailableAt = *q.AvailableAt
		}
		return "", nil, e
	}
	tok := auth.NewToken(24)
	inv := &store.Invite{TokenHash: auth.HashToken(tok), CreatedBy: u.ID, Note: in.Note, MaxUses: in.MaxUses}
	if in.TTL > 0 {
		inv.ExpiresAt = s.now().Add(in.TTL)
	}
	if err := s.Store.Invites().Create(ctx, inv); err != nil {
		return "", nil, err
	}
	action := "invite.create"
	if u.Role == "admin" {
		action = "admin.invite.create"
	}
	s.audit(ctx, u.ID, action, map[string]string{"invite": inv.ID, "uses": strconv.Itoa(inv.MaxUses)})
	return tok, inv, nil
}

// CreateInvite 是管理员发出一次性邀请的简写（保留给命令行与测试）。
func (s *Service) CreateInvite(ctx context.Context, admin string, ttl time.Duration, note string) (string, *store.Invite, error) {
	u, err := s.Store.Users().ByID(ctx, admin)
	if err != nil {
		return "", nil, err
	}
	return s.NewInvite(ctx, u, InviteInput{TTL: ttl, Note: note})
}

// InviteValid 报告邀请此刻能否使用。
func InviteValid(v *store.Invite, now time.Time) bool {
	return v.RevokedAt.IsZero() && v.Uses < v.MaxUses && (v.ExpiresAt.IsZero() || now.Before(v.ExpiresAt))
}

// InviterOf 返回有效邀请的邀请人（注册页显示「由谁邀请」）；邀请无效时返回 ErrNotFound。
func (s *Service) InviterOf(ctx context.Context, token string) (*store.User, error) {
	v, err := s.Store.Invites().ByToken(ctx, auth.HashToken(token))
	if err != nil {
		return nil, err
	}
	if !InviteValid(v, s.now()) {
		return nil, store.ErrNotFound
	}
	return s.Store.Users().ByID(ctx, v.CreatedBy)
}

// notifyInviter 告诉邀请人：有人用你的邀请注册了。失败只记日志，不影响注册。
func (s *Service) notifyInviter(ctx context.Context, invitee *store.User) {
	if invitee.InvitedBy == "" {
		return
	}
	raw, err := json.Marshal(MessageBody{Notice: &Notice{Code: "invite_joined", Name: invitee.Username}})
	if err == nil {
		err = s.Store.Messages().Create(ctx, &store.Message{SenderID: invitee.ID, RecipientID: invitee.InvitedBy, Kind: "notice", Body: raw, CreatedAt: s.now()})
	}
	if err == nil {
		s.push(invitee.InvitedBy, events.Message, invitee.ID)
	}
	if err != nil && s.Log != nil {
		s.Log.Warn("invite notice not sent", "inviter", invitee.InvitedBy, "err", err.Error())
	}
}

// inviterName 返回邀请人的当前用户名；邀请人已注销时返回快照。
func (s *Service) inviterName(ctx context.Context, u *store.User) string {
	if u.InvitedBy == "" {
		return ""
	}
	if iv, err := s.Store.Users().ByID(ctx, u.InvitedBy); err == nil {
		return iv.Username
	}
	return u.InvitedByName
}
