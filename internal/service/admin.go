package service

import (
	"context"
	"errors"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 管理（第 14.1、14.7 节）：管理员可以查看用户的元数据与用量、停用账号、重置口令、调整配额；
// 不能查看他人的节点凭据、订阅地址、链接或方案内容——这里只读取 users、user_quotas 与用量统计。

// ErrSelf 表示管理员试图停用自己或取消自己的管理员角色。
var ErrSelf = errors.New("service: 不能对自己执行此操作")

// AdminUser 是管理页显示的用户元数据。
type AdminUser struct {
	ID        string      `json:"id"`
	Username  string      `json:"username"`
	Role      string      `json:"role"`
	Disabled  bool        `json:"disabled"`
	CreatedAt time.Time   `json:"createdAt"`
	MFA       bool        `json:"mfa"`
	Usage     store.Usage `json:"usage"`
	Quota     store.Quota `json:"quota"` // -1 表示使用默认值
}

// AdminUsers 列出用户（只含元数据）。
func (s *Service) AdminUsers(ctx context.Context, p store.Page) ([]AdminUser, error) {
	users, err := s.Store.Users().List(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]AdminUser, 0, len(users))
	for _, u := range users {
		usage, err := s.Store.Quotas().Usage(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		q, err := s.Store.Quotas().Get(ctx, u.ID)
		if err != nil {
			return nil, err
		}
		st, _ := s.TOTPStatus(ctx, u.ID)
		out = append(out, AdminUser{ID: u.ID, Username: u.Username, Role: u.Role, Disabled: u.Disabled, CreatedAt: u.CreatedAt, MFA: st.Enabled, Usage: *usage, Quota: *q})
	}
	return out, nil
}

// AdminSetDisabled 停用或启用账号；停用时注销该账号的全部会话。
func (s *Service) AdminSetDisabled(ctx context.Context, admin, id string, disabled bool) error {
	if admin == id {
		return ErrSelf
	}
	u, err := s.Store.Users().ByID(ctx, id)
	if err != nil {
		return err
	}
	u.Disabled = disabled
	if err := s.Store.Users().Update(ctx, u); err != nil {
		return err
	}
	if disabled {
		if err := s.Store.Sessions().DeleteAll(ctx, id); err != nil {
			return err
		}
	}
	action := "admin.user.enable"
	if disabled {
		action = "admin.user.disable"
	}
	s.audit(ctx, admin, action, map[string]string{"user": u.Username})
	return nil
}

// AdminResetPassword 为用户设置新口令，并注销其全部会话。
func (s *Service) AdminResetPassword(ctx context.Context, admin, id, password string) error {
	if err := auth.CheckPassword(password, CommonPasswords); err != nil {
		return err
	}
	u, err := s.Store.Users().ByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.setPassword(ctx, u, password); err != nil {
		return err
	}
	if err := s.Store.Sessions().DeleteAll(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, admin, "admin.user.password", map[string]string{"user": u.Username})
	s.audit(ctx, id, "account.password_reset_by_admin", nil)
	s.notify(ctx, id, AlertPassword, map[string]string{"change": "reset by administrator"})
	return nil
}

// AdminSetQuota 设置用户的配额（-1 表示使用默认值）。
func (s *Service) AdminSetQuota(ctx context.Context, admin string, q store.Quota) error {
	u, err := s.Store.Users().ByID(ctx, q.UserID)
	if err != nil {
		return err
	}
	if err := s.Store.Quotas().Put(ctx, &q); err != nil {
		return err
	}
	s.audit(ctx, admin, "admin.user.quota", map[string]string{"user": u.Username})
	return nil
}

// QuotaDefaultsView 返回生效的全局默认配额（管理页显示）。
func (s *Service) QuotaDefaultsView() Limits {
	d := s.defaults()
	return Limits{Sources: d.Sources, Profiles: d.Profiles, Links: d.Links, MinInterval: int(d.MinInterval.Seconds()), Storage: d.Storage}
}

// PruneAudit 删除超过保留期的审计记录；保留期为 0 时不删除。
func (s *Service) PruneAudit(ctx context.Context) {
	if s.AuditRetention <= 0 {
		return
	}
	n, err := s.Store.Audit().Prune(ctx, s.now().Add(-s.AuditRetention))
	if err != nil && s.Log != nil {
		s.Log.Error("清理审计日志失败", "error", err)
	} else if n > 0 && s.Log != nil {
		s.Log.Info("已清理过期的审计日志", "rows", n)
	}
}
