package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/angwz/clash-launch/internal/events"
	"github.com/angwz/clash-launch/internal/store"
)

// 足迹（需求 R10）：只记不会重复、日后回看有意义的事。同一 (用户, 类别, 区分) 只记一次，
// 写入由数据库忽略重复，因此各触发点直接调用 Footprint，不需要先查。永久保存，随备份导出，随账号删除。
// 新记下的足迹经实时通道通知浏览器：弹出一次轻提示，指示灯播放一次庆祝效果（R11）。

// 足迹类别。前端按类别取文字（footprint.kind.*）。
const (
	FPJoined         = "joined"          // 注册（附邀请人）
	FPFirstLogin     = "first_login"     // 第一次登录
	FPFirstMFA       = "first_mfa"       // 第一次启用两步验证
	FPFirstPasskey   = "first_passkey"   // 第一次添加通行密钥
	FPFirstSource    = "first_source"    // 第一个节点源
	FPFirstProfile   = "first_profile"   // 第一个配置
	FPFirstLink      = "first_link"      // 第一条订阅链接
	FPFirstCheck     = "first_check"     // 第一次通过 mihomo 检查
	FPFirstRouting   = "first_routing"   // 第一次开始自己管理分流
	FPFirstPull      = "first_pull"      // 订阅第一次被客户端拉取
	FPClient         = "client"          // 每种客户端第一次出现（区分：客户端名称）
	FPPulls          = "pulls"           // 订阅累计被拉取（区分：阈值）
	FPStreak         = "streak"          // 订阅连续在用的天数（区分：阈值）
	FPPullCountry    = "pull_country"    // 订阅第一次从某个国家或地区被拉取（区分：国家代码）
	FPLoginCountry   = "login_country"   // 第一次从某个国家或地区登录（区分：国家代码）
	FPRename         = "rename"          // 改用户名（区分：时间，每次都记）
	FPNickname       = "nickname"        // 改昵称（区分：时间，每次都记）
	FPFirstContact   = "first_contact"   // 第一个联系人
	FPFirstMessage   = "first_message"   // 第一条消息
	FPFirstGrant     = "first_grant"     // 第一次授权给别人
	FPFirstGrantRecv = "first_grant_got" // 第一次收到授权

	// 站点（UserID 为空，只给管理员看）
	FPSiteStart = "site_start" // 网站第一天上线（第一位管理员创建）
	FPSiteFirst = "site_first" // 第一位注册用户
	FPSiteUsers = "site_users" // 用户数跨过阈值
)

var (
	pullMarks   = []int64{100, 1_000, 10_000, 100_000}
	streakMarks = []int{30, 100, 365}
	userMarks   = []int{10, 100, 1_000}
)

// auditFootprints 是可以从审计动作直接得出的「第一次」：审计记下的同时记下足迹，不必在每个触发点重复。
var auditFootprints = map[string]string{
	"login.ok":             FPFirstLogin,
	"mfa.enable":           FPFirstMFA,
	"passkey.add":          FPFirstPasskey,
	"profile.create":       FPFirstProfile,
	"link.create":          FPFirstLink,
	"grant.create":         FPFirstGrant,
	"message.source_added": FPFirstSource,
}

// Footprint 记下一条足迹；已有时忽略。失败只记日志：足迹是附加信息，不影响触发它的操作。
func (s *Service) Footprint(ctx context.Context, user, kind, subject string, detail map[string]string) {
	s.footprintAt(ctx, user, kind, subject, detail, s.now(), false)
}

func (s *Service) footprintAt(ctx context.Context, user, kind, subject string, detail map[string]string, at time.Time, backfilled bool) bool {
	var b []byte
	if len(detail) > 0 {
		b, _ = json.Marshal(detail) // map[string]string 的编码不会失败；键按字母序，输出稳定
	}
	added, err := s.Store.Footprints().Add(ctx, &store.Footprint{UserID: user, Kind: kind, Subject: subject, At: at, Detail: b, Backfilled: backfilled})
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("记录足迹失败", "kind", kind, "error", err)
		}
		return false
	}
	if added && user != "" && !backfilled && s.Events != nil {
		s.Events.Publish(user, events.Event{Type: events.Footprint, ID: kind, State: subject})
	}
	return added
}

// Audit 写入审计日志，并记下由该动作得出的足迹（API 层与服务层共用）。
func (s *Service) Audit(ctx context.Context, owner, action string, detail map[string]string) {
	s.audit(ctx, owner, action, detail)
}

func (s *Service) footprintFromAudit(ctx context.Context, owner, action string, detail map[string]string) {
	if owner == "" {
		return
	}
	// 用户名与昵称的历次变更（也用于防冒充）：每次都记，以「时间:新值」区分（同一毫秒内改成不同的值也各记一条）
	switch action {
	case "account.username":
		s.Footprint(ctx, owner, FPRename, strconv.FormatInt(s.now().UnixMilli(), 10)+":"+detail["to"], map[string]string{"from": detail["from"], "to": detail["to"]})
		return
	case "account.nickname":
		s.Footprint(ctx, owner, FPNickname, strconv.FormatInt(s.now().UnixMilli(), 10)+":"+detail["nickname"], map[string]string{"nickname": detail["nickname"]})
		return
	}
	kind := auditFootprints[action]
	if kind == "" {
		return
	}
	d := map[string]string{}
	for _, k := range []string{"profile", "to", "name"} { // 只留回看时有意义的；登录的 IP 与设备不进足迹
		if v := detail[k]; v != "" {
			d[k] = v
		}
	}
	s.Footprint(ctx, owner, kind, "", d)
}

// pullFootprints 从刚写入的订阅请求得出足迹：第一次被拉取、新客户端、新国家；再检查累计次数与连续天数。
func (s *Service) pullFootprints(ctx context.Context, reqs []store.SubRequest) {
	owners := map[string]bool{}
	for _, q := range reqs {
		if q.LinkID == "" || q.OwnerID == "" || q.Status >= 400 {
			continue
		}
		owners[q.OwnerID] = true
		s.footprintAt(ctx, q.OwnerID, FPFirstPull, "", map[string]string{"client": q.Client}, q.At, false)
		if q.Client != "" {
			s.footprintAt(ctx, q.OwnerID, FPClient, q.Client, nil, q.At, false)
		}
		if q.Country != "" {
			s.footprintAt(ctx, q.OwnerID, FPPullCountry, q.Country, nil, q.At, false)
		}
	}
	for owner := range owners {
		if err := s.pullMilestones(ctx, owner, s.now()); err != nil && s.Log != nil {
			s.Log.Warn("检查订阅足迹失败", "user", owner, "error", err)
		}
	}
}

// pullMilestones 检查累计拉取次数与连续在用天数（按每天是否被拉取，UTC 日期）。
func (s *Service) pullMilestones(ctx context.Context, owner string, now time.Time) error {
	n, err := s.Store.Stats().OwnerPulls(ctx, owner)
	if err != nil {
		return err
	}
	for _, m := range pullMarks {
		if n >= m {
			s.footprintAt(ctx, owner, FPPulls, strconv.FormatInt(m, 10), nil, now, false)
		}
	}
	days, err := s.Store.Stats().OwnerDays(ctx, owner, Day(now.AddDate(0, 0, -streakMarks[len(streakMarks)-1])))
	if err != nil {
		return err
	}
	run := streak(days, now)
	for _, m := range streakMarks {
		if run >= m {
			s.footprintAt(ctx, owner, FPStreak, strconv.Itoa(m), nil, now, false)
		}
	}
	return nil
}

// streak 返回截至 now 当天（含）连续有记录的天数；days 为升序的 yyyymmdd。
func streak(days []int, now time.Time) int {
	have := make(map[int]bool, len(days))
	for _, d := range days {
		have[d] = true
	}
	n := 0
	for t := now.UTC(); have[Day(t)]; t = t.AddDate(0, 0, -1) {
		n++
	}
	return n
}

// siteFootprints 在新用户注册后检查站点的里程碑：第一位注册用户、用户数跨过阈值。
func (s *Service) siteFootprints(ctx context.Context, u *store.User) {
	if u.Role != "admin" {
		s.Footprint(ctx, "", FPSiteFirst, "", map[string]string{"user": u.Username})
	}
	n, err := s.Store.Users().Count(ctx)
	if err != nil {
		return
	}
	for _, m := range userMarks {
		if n >= m {
			s.Footprint(ctx, "", FPSiteUsers, strconv.Itoa(m), nil)
		}
	}
}

// Footprints 返回用户的足迹；管理员另附站点的足迹。
func (s *Service) Footprints(ctx context.Context, u *store.User) (mine, site []store.Footprint, err error) {
	if mine, err = s.Store.Footprints().List(ctx, u.ID); err != nil {
		return nil, nil, err
	}
	if u.Role == "admin" {
		if site, err = s.Store.Footprints().List(ctx, ""); err != nil {
			return nil, nil, err
		}
	}
	return mine, site, nil
}

const settingFootprintsBackfilled = "footprints.backfilled"

// BackfillFootprints 在足迹上线后补记一次能从已有数据推算的事件，并标为「补记」：
// 注册、第一个节点源、第一个配置、订阅第一次被拉取（统计里最早的一天）、站点上线日与第一位注册用户。
// 只运行一次（settings 中留有标记）；之后的事件由各触发点实时记下。
func (s *Service) BackfillFootprints(ctx context.Context) error {
	if _, err := s.Store.Settings().Get(ctx, settingFootprintsBackfilled); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	var earliest *store.User
	for after := ""; ; {
		users, err := s.Store.Users().List(ctx, store.Page{After: after, Limit: 500})
		if err != nil {
			return err
		}
		for i := range users {
			u := &users[i]
			if !u.DeletedAt.IsZero() {
				continue
			}
			if err := s.backfillUser(ctx, u); err != nil {
				return err
			}
			if earliest == nil || u.CreatedAt.Before(earliest.CreatedAt) {
				earliest = u
			}
		}
		if len(users) < 500 {
			break
		}
		after = users[len(users)-1].ID
	}
	if earliest != nil {
		s.footprintAt(ctx, "", FPSiteStart, "", nil, earliest.CreatedAt, true)
	}
	return s.Store.Settings().Set(ctx, settingFootprintsBackfilled, s.now().UTC().Format(time.RFC3339))
}

func (s *Service) backfillUser(ctx context.Context, u *store.User) error {
	detail := map[string]string{}
	if u.InvitedByName != "" {
		detail["inviter"] = u.InvitedByName
	}
	s.footprintAt(ctx, u.ID, FPJoined, "", detail, u.CreatedAt, true)
	page := store.Page{Limit: searchListCap}
	if srcs, err := s.Store.Sources().List(ctx, u.ID, page); err != nil {
		return err
	} else if first := earliestAt(len(srcs), func(i int) time.Time { return srcs[i].CreatedAt }); !first.IsZero() {
		s.footprintAt(ctx, u.ID, FPFirstSource, "", nil, first, true)
	}
	if ps, err := s.Store.Profiles().List(ctx, u.ID, page); err != nil {
		return err
	} else if first := earliestAt(len(ps), func(i int) time.Time { return ps[i].CreatedAt }); !first.IsZero() {
		s.footprintAt(ctx, u.ID, FPFirstProfile, "", nil, first, true)
	}
	days, err := s.Store.Stats().OwnerDays(ctx, u.ID, 0)
	if err != nil {
		return err
	}
	if len(days) > 0 {
		d := days[0]
		s.footprintAt(ctx, u.ID, FPFirstPull, "", nil, time.Date(d/10000, time.Month(d/100%100), d%100, 0, 0, 0, 0, time.UTC), true)
	}
	return nil
}

func earliestAt(n int, at func(int) time.Time) time.Time {
	var t time.Time
	for i := range n {
		if a := at(i); t.IsZero() || a.Before(t) {
			t = a
		}
	}
	return t
}
