package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 两步验证策略（需求 R41）：管理员可以要求管理员或全部用户启用两步验证（TOTP）。
//
// 适用的用户还没有启用时，登录照常成功（密码正确即可），但 API 层只放行设置两步验证所需的接口，
// 页面先引导完成设置（见 api.mfaExempt）。只有 TOTP 才让登录多一步（通行密钥只能代替验证码，见 SecondFactors），
// 所以「已启用」按 TOTP 判断。策略适用时不能关闭两步验证。
const settingMFAPolicy = "mfa.policy"

// 策略取值。
const (
	MFAPolicyOff    = ""       // 不要求
	MFAPolicyAdmins = "admins" // 要求管理员
	MFAPolicyAll    = "all"    // 要求全部用户
)

var (
	// ErrMFAPolicy 表示策略取值无效。
	ErrMFAPolicy = errors.New("service: 两步验证策略只能是空、admins 或 all")
	// ErrMFAPolicySelf 表示管理员自己还没有启用两步验证，不能要求别人。
	ErrMFAPolicySelf = errors.New("service: 请先为自己启用两步验证，再要求其他人启用")
	// ErrMFARequired 表示策略要求两步验证，不能关闭。
	ErrMFARequired = errors.New("service: 管理员要求启用两步验证，不能关闭")
)

type policyCache struct {
	mu     sync.Mutex
	policy string
	at     time.Time
}

// MFAPolicy 返回当前策略（带短时缓存：每个已登录请求都要检查）。
func (s *Service) MFAPolicy(ctx context.Context) (string, error) {
	c := &s.mfaPolicy
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < termsCacheTTL {
		return c.policy, nil
	}
	v, err := s.Store.Settings().Get(ctx, settingMFAPolicy)
	if errors.Is(err, store.ErrNotFound) {
		v, err = MFAPolicyOff, nil
	}
	if err != nil {
		return "", err
	}
	if v != MFAPolicyOff && v != MFAPolicyAdmins && v != MFAPolicyAll {
		return "", fmt.Errorf("全局设置 %s 的值无效：%q", settingMFAPolicy, v)
	}
	c.policy, c.at = v, time.Now()
	return v, nil
}

// SetMFAPolicy 设定策略（管理员）。要求别人之前，管理员自己必须已经启用。
func (s *Service) SetMFAPolicy(ctx context.Context, admin *store.User, policy string) error {
	if policy != MFAPolicyOff && policy != MFAPolicyAdmins && policy != MFAPolicyAll {
		return ErrMFAPolicy
	}
	if policy != MFAPolicyOff {
		on, err := s.totpOn(ctx, admin.ID)
		if err != nil {
			return err
		}
		if !on {
			return ErrMFAPolicySelf
		}
	}
	if err := s.Store.Settings().Set(ctx, settingMFAPolicy, policy); err != nil {
		return err
	}
	s.mfaPolicy.mu.Lock()
	s.mfaPolicy.policy, s.mfaPolicy.at = policy, time.Now()
	s.mfaPolicy.mu.Unlock()
	shown := policy
	if shown == "" {
		shown = "off"
	}
	s.audit(ctx, admin.ID, "admin.mfa_policy", map[string]string{"policy": shown})
	return nil
}

// MFAPolicyApplies 报告策略是否要求这个用户启用两步验证。
func (s *Service) MFAPolicyApplies(ctx context.Context, u *store.User) (bool, error) {
	p, err := s.MFAPolicy(ctx)
	if err != nil {
		return false, err
	}
	return p == MFAPolicyAll || (p == MFAPolicyAdmins && u.Role == "admin"), nil
}

// MFASetupPending 报告这个用户是否需要先启用两步验证才能继续使用。
func (s *Service) MFASetupPending(ctx context.Context, u *store.User) (bool, error) {
	applies, err := s.MFAPolicyApplies(ctx, u)
	if err != nil || !applies {
		return false, err
	}
	on, err := s.totpOn(ctx, u.ID)
	return !on, err
}

func (s *Service) totpOn(ctx context.Context, userID string) (bool, error) {
	t, err := s.Store.TOTP().Get(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return t.Enabled, nil
}
