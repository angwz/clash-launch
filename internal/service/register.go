package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 注册（docs/decisions/0003）：管理员开关「允许注册」，默认关闭。关闭时只能凭一次性邀请注册：
// 管理员生成邀请链接（可选有效期），发给别人；邀请用过一次即作废。
// 安全问题与工作量证明在 M5 加入注册流程。

const settingRegistration = "registration.open"

// ErrRegistrationClosed 表示注册关闭且没有有效的邀请。
var ErrRegistrationClosed = errors.New("service: 注册已关闭，需要有效的邀请")

// ErrUsername 表示用户名不合规。
var ErrUsername = errors.New("service: 用户名需为 3–32 个字符，不含空白与控制字符")

// RegistrationOpen 报告是否允许无邀请注册。
func (s *Service) RegistrationOpen(ctx context.Context) (bool, error) {
	v, err := s.Store.Settings().Get(ctx, settingRegistration)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return v == "true", err
}

// SetRegistration 打开或关闭无邀请注册。
func (s *Service) SetRegistration(ctx context.Context, admin string, open bool) error {
	v := "false"
	if open {
		v = "true"
	}
	if err := s.Store.Settings().Set(ctx, settingRegistration, v); err != nil {
		return err
	}
	return s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: admin, Action: "admin.registration", Detail: []byte(`{"open":` + v + `}`)})
}

// CreateInvite 生成一次性邀请，返回令牌原文（只此一次）。ttl 为 0 表示不过期。
func (s *Service) CreateInvite(ctx context.Context, admin string, ttl time.Duration, note string) (string, *store.Invite, error) {
	tok := auth.NewToken(24)
	inv := &store.Invite{TokenHash: auth.HashToken(tok), CreatedBy: admin, Note: note}
	if ttl > 0 {
		inv.ExpiresAt = s.now().Add(ttl)
	}
	if err := s.Store.Invites().Create(ctx, inv); err != nil {
		return "", nil, err
	}
	_ = s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: admin, Action: "admin.invite.create", Detail: []byte(`{"invite":"` + inv.ID + `"}`)})
	return tok, inv, nil
}

// CheckUsername 校验用户名：1–32 个字符，首尾无空白，不含控制字符（反馈 #17：用户名可以简单）。
func CheckUsername(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 32 || strings.TrimSpace(name) != name {
		return ErrUsername
	}
	for _, r := range name {
		if r <= ' ' || r == 0x7F || (r >= 0x80 && r <= 0x9F) || r == 0x2028 || r == 0x2029 {
			return ErrUsername
		}
	}
	return nil
}

// RegisterInput 是注册的输入。安全问题必填（第 14.1 节：找回口令的唯一途径）。
type RegisterInput struct {
	Username  string
	Password  string
	Invite    string
	Questions []auth.Question
	Answers   []string
}

// Register 创建普通用户。注册开放时不需要邀请；关闭时邀请必须有效。
// 账号、安全问题与邀请的消费在同一事务中完成。工作量证明由调用方（HTTP 层）先行校验。
func (s *Service) Register(ctx context.Context, in RegisterInput) (*store.User, error) {
	username, password, invite := in.Username, in.Password, in.Invite
	if err := CheckUsername(username); err != nil {
		return nil, err
	}
	if err := auth.CheckPassword(password); err != nil {
		return nil, err
	}
	if err := auth.CheckQuestions(in.Questions, in.Answers); err != nil {
		return nil, err
	}
	open, err := s.RegistrationOpen(ctx)
	if err != nil {
		return nil, err
	}
	if !open && invite == "" {
		return nil, ErrRegistrationClosed
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	stored, err := hashQuestions(in.Questions, in.Answers)
	if err != nil {
		return nil, err
	}
	u := &store.User{Username: username, PasswordHash: hash, Role: "user"}
	err = s.Store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := tx.Questions().Set(ctx, u.ID, stored); err != nil {
			return err
		}
		if invite != "" {
			// 邀请成功消费即记为受邀用户（可自选节点写法，反馈 #46）；开放注册时邀请可选，
			// 无效或过期的邀请不挡住注册，只是不算受邀。
			_, err := tx.Invites().Use(ctx, auth.HashToken(invite), u.ID, s.now())
			switch {
			case err == nil:
				u.Invited = true
				if err := tx.Users().SetInvited(ctx, u.ID, true); err != nil {
					return err
				}
			case !errors.Is(err, store.ErrNotFound):
				return err
			case !open:
				return ErrRegistrationClosed
			}
		}
		return tx.Audit().Add(ctx, &store.AuditEntry{OwnerID: u.ID, Action: "register"})
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}
