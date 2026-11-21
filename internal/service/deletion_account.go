package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 注销账户（需求 R20）。
//
// 用户本人发起：需要当前密码（启用了两步验证时还要验证码）；进入 7 天冷静期，所有会话与访问令牌立即失效，
// 冷静期内再次登录即撤销。订阅在冷静期内照常可用。到期后由后台任务注销。
// 管理员注销普通用户：立即执行，记入审计。最后一个可用的管理员不能被注销（包括自己）。
//
// 注销 = 按 R2 的删除路径删除全部业务数据（授权收回并通知对方）+ 把用户行改为墓碑：
// 登录凭据与只属于本人的记录全部删除，用户名改为 deleted-…（原用户名按改名规则保留 180 天，防止被冒用），
// 对话、好友、授权记录与邀请关系保留，对方看到的是「已注销的用户」。

// DeletionCooling 是注销的冷静期。
const DeletionCooling = 7 * 24 * time.Hour

// DeletedPrefix 是已注销账号的用户名前缀；注册与改名不能使用。
const DeletedPrefix = "deleted-"

var (
	// ErrLastAdmin 表示这是最后一个可用的管理员，不能注销。
	ErrLastAdmin = errors.New("service: 这是最后一个管理员，不能注销")
	// ErrDeleted 表示账号已经注销。
	ErrDeleted = errors.New("service: 账号已注销")
)

// RequestDeletion 发起注销，返回计划注销的时间。
func (s *Service) RequestDeletion(ctx context.Context, u *store.User, password, code string) (time.Time, error) {
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return time.Time{}, ErrWrongPassword
	}
	if st, err := s.TOTPStatus(ctx, u.ID); err != nil {
		return time.Time{}, err
	} else if st.Enabled && !s.checkSecondFactor(ctx, u.ID, code, s.now()) {
		return time.Time{}, ErrMFA
	}
	if err := s.guardLastAdmin(ctx, u); err != nil {
		return time.Time{}, err
	}
	at := s.now().Add(DeletionCooling)
	if err := s.Store.Users().SetDeletion(ctx, u.ID, at); err != nil {
		return time.Time{}, err
	}
	if err := s.Store.Sessions().DeleteAll(ctx, u.ID); err != nil {
		return time.Time{}, err
	}
	if err := s.Store.Tokens().DeleteAll(ctx, u.ID); err != nil {
		return time.Time{}, err
	}
	s.audit(ctx, u.ID, "account.delete_request", map[string]string{"at": at.UTC().Format(time.RFC3339)})
	s.notify(ctx, u.ID, AlertDeletion, map[string]string{"when": at.UTC().Format("2006-01-02 15:04 MST")})
	return at, nil
}

// CancelDeletionOnLogin 在冷静期内登录时撤销注销；返回是否撤销了。
func (s *Service) CancelDeletionOnLogin(ctx context.Context, u *store.User) bool {
	if u.DeletionAt.IsZero() || !u.DeletedAt.IsZero() {
		return false
	}
	if err := s.Store.Users().SetDeletion(ctx, u.ID, time.Time{}); err != nil {
		if s.Log != nil {
			s.Log.Warn("撤销注销失败", "user", u.ID, "error", err)
		}
		return false
	}
	u.DeletionAt = time.Time{}
	s.audit(ctx, u.ID, "account.delete_cancel", nil)
	return true
}

// AdminDeleteUser 由管理员立即注销一个账号。
func (s *Service) AdminDeleteUser(ctx context.Context, admin *store.User, id string) error {
	u, err := s.Store.Users().ByID(ctx, id)
	if err != nil {
		return err
	}
	if !u.DeletedAt.IsZero() {
		return ErrDeleted
	}
	if err := s.guardLastAdmin(ctx, u); err != nil {
		return err
	}
	s.audit(ctx, admin.ID, "admin.user.delete", map[string]string{"user": u.Username})
	return s.PurgeAccount(ctx, u)
}

// guardLastAdmin 拒绝注销最后一个可用的管理员。
func (s *Service) guardLastAdmin(ctx context.Context, u *store.User) error {
	if u.Role != "admin" {
		return nil
	}
	n := 0
	after := ""
	for {
		us, err := s.Store.Users().List(ctx, store.Page{After: after, Limit: 200})
		if err != nil {
			return err
		}
		for _, x := range us {
			if x.Role == "admin" && !x.Disabled && x.DeletedAt.IsZero() && x.DeletionAt.IsZero() && x.ID != u.ID {
				n++
			}
		}
		if len(us) < 200 {
			break
		}
		after = us[len(us)-1].ID
	}
	if n == 0 {
		return ErrLastAdmin
	}
	return nil
}

// PurgeAccount 注销账号：删除全部业务数据（R2：先收回授权，事后通知对方），再改为墓碑。
// 配置的删除保护（R55）只防误删，不阻止注销。
func (s *Service) PurgeAccount(ctx context.Context, u *store.User) error {
	st := s.Store
	profs, err := listPaged(func(after string) ([]store.Profile, error) {
		return st.Profiles().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Profile) string { return x.ID })
	if err != nil {
		return err
	}
	for _, p := range profs {
		links, err := st.Links().List(ctx, u.ID, p.ID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(links))
		for _, l := range links {
			ids = append(ids, l.ID)
		}
		pid := p.ID
		if err := s.deleteWithShorts(ctx, u, ids, func() error { return st.Profiles().Delete(ctx, u.ID, pid) }); err != nil {
			return err
		}
	}
	srcs, err := listPaged(func(after string) ([]store.Source, error) {
		return st.Sources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.Source) string { return x.ID })
	if err != nil {
		return err
	}
	for _, x := range srcs {
		if err := s.DeleteSource(ctx, u, x.ID); err != nil {
			return err
		}
	}
	shorts, err := st.ShortLinks().List(ctx, u.ID)
	if err != nil {
		return err
	}
	for _, x := range shorts {
		if err := s.DeleteShortLink(ctx, u, x.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	rss, err := listPaged(func(after string) ([]store.RuleSource, error) {
		return st.RuleSources().List(ctx, u.ID, store.Page{After: after, Limit: 200})
	}, func(x store.RuleSource) string { return x.ID })
	if err != nil {
		return err
	}
	for _, x := range rss {
		if err := st.RuleSources().Delete(ctx, u.ID, x.ID); err != nil {
			return err
		}
	}
	tpls, err := st.Templates().List(ctx, u.ID)
	if err != nil {
		return err
	}
	for _, x := range tpls {
		if err := st.Templates().Delete(ctx, u.ID, x.ID); err != nil {
			return err
		}
	}
	placeholder := DeletedPrefix + tombstoneSuffix(u.ID)
	if err := st.Users().Tombstone(ctx, u.ID, placeholder, s.now()); err != nil {
		return err
	}
	s.audit(ctx, u.ID, "account.deleted", nil)
	return nil
}

// tombstoneSuffix 取 ID 中随机的末尾部分（UUIDv7 的开头是时间，同一时刻注册的账号会重复）。
func tombstoneSuffix(id string) string {
	h := strings.ReplaceAll(id, "-", "")
	if len(h) > 12 {
		h = h[len(h)-12:]
	}
	return h
}

// RunDueDeletions 注销冷静期已满的账号（后台任务调用）。失败只记日志，下次再试。
func (s *Service) RunDueDeletions(ctx context.Context) {
	us, err := s.Store.Users().DueDeletions(ctx, s.now())
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("读取待注销的账号失败", "error", err)
		}
		jobFailed(ctx, err)
		return
	}
	for i := range us {
		if err := s.PurgeAccount(ctx, &us[i]); err != nil {
			jobFailed(ctx, err)
			if s.Log != nil {
				s.Log.Warn("注销账号失败", "user", us[i].ID, "error", err)
			}
		}
	}
}
