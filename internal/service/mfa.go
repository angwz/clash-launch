package service

import (
	"context"
	"errors"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 两步验证（第 14.1 节，可选）：TOTP 与一次性恢复码；通行密钥也可作为第二因素（见 passkey.go）。
// 登录流程：口令正确 → 若启用了第二因素，签发 5 分钟有效的凭证（不建立会话）→ 提交验证码 → 建立会话。

var (
	// ErrMFA 表示第二因素验证失败（验证码错误、已用过或凭证过期）。
	ErrMFA = errors.New("service: 两步验证失败")
	// ErrMFAState 表示两步验证的状态不允许该操作（如未启用时关闭、没有待确认的密钥）。
	ErrMFAState = errors.New("service: 两步验证状态不允许该操作")
)

const (
	ticketMFA      = "mfa"
	mfaTicketTTL   = 5 * time.Minute
	totpIssuer     = "Clash, Launch!"
	recoveryCodesN = 10
)

func (s *Service) tickets() *auth.Tickets {
	s.ticketsOnce.Do(func() { s.ticketsVal = auth.NewTickets() })
	return s.ticketsVal
}

// SecondFactors 返回用户启用的第二因素：totp、passkey。为空表示只需要口令。
func (s *Service) SecondFactors(ctx context.Context, userID string) []string {
	var out []string
	if t, err := s.Store.TOTP().Get(ctx, userID); err == nil && t.Enabled {
		out = append(out, "totp")
	}
	if ks, err := s.Store.Passkeys().List(ctx, userID); err == nil && len(ks) > 0 {
		out = append(out, "passkey")
	}
	return out
}

// MFATicket 为已通过口令验证的用户签发第二步的凭证。
func (s *Service) MFATicket(userID string) string {
	return s.tickets().Issue(ticketMFA, userID, mfaTicketTTL, s.now())
}

// MFAUser 校验凭证，返回对应的用户（不消耗凭证；第二因素本身防重放）。
func (s *Service) MFAUser(ctx context.Context, ticket string) (*store.User, error) {
	id, err := s.tickets().Verify(ticketMFA, ticket, s.now())
	if err != nil {
		return nil, ErrMFA
	}
	u, err := s.Store.Users().ByID(ctx, id)
	if err != nil || u.Disabled {
		return nil, ErrMFA
	}
	return u, nil
}

// VerifyTOTPLogin 用 TOTP 验证码或恢复码完成第二步。失败计入该账号的登录退避。
func (s *Service) VerifyTOTPLogin(ctx context.Context, ticket, code, ip string) (*store.User, error) {
	u, err := s.MFAUser(ctx, ticket)
	if err != nil {
		return nil, err
	}
	g, now := s.g(), s.now()
	key := userKey(u.Username)
	if w := g.loginUser.Wait(key, now); w > 0 {
		return nil, &WaitError{Wait: w}
	}
	if s.checkSecondFactor(ctx, u.ID, code, now) {
		g.loginUser.Reset(key)
		return u, nil
	}
	g.loginUser.Fail(key, now)
	s.audit(ctx, u.ID, "mfa.failed", map[string]string{"ip": ip})
	return nil, ErrMFA
}

// checkSecondFactor 依次尝试 TOTP 与恢复码。TOTP 的时间步与恢复码都只能使用一次。
func (s *Service) checkSecondFactor(ctx context.Context, userID, code string, now time.Time) bool {
	t, err := s.Store.TOTP().Get(ctx, userID)
	if err != nil || !t.Enabled {
		return false
	}
	if step, ok := auth.VerifyTOTP(t.Secret, code, now); ok {
		return s.Store.TOTP().Advance(ctx, userID, step) == nil
	}
	if err := s.Store.TOTP().UseCode(ctx, userID, auth.HashRecoveryCode(code), now); err == nil {
		s.audit(ctx, userID, "mfa.recovery_code", nil)
		return true
	}
	return false
}

// TOTPStatus 是两步验证的状态。
type TOTPStatus struct {
	Enabled        bool `json:"enabled"`
	RemainingCodes int  `json:"remainingCodes"`
}

func (s *Service) TOTPStatus(ctx context.Context, userID string) (TOTPStatus, error) {
	t, err := s.Store.TOTP().Get(ctx, userID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !t.Enabled) {
		return TOTPStatus{}, nil
	}
	if err != nil {
		return TOTPStatus{}, err
	}
	n, err := s.Store.TOTP().RemainingCodes(ctx, userID)
	return TOTPStatus{Enabled: true, RemainingCodes: n}, err
}

// TOTPSetup 生成待确认的密钥（已启用时拒绝，需先关闭）。返回密钥与 otpauth 地址。
func (s *Service) TOTPSetup(ctx context.Context, u *store.User) (secret, uri string, err error) {
	if t, err := s.Store.TOTP().Get(ctx, u.ID); err == nil && t.Enabled {
		return "", "", ErrMFAState
	}
	secret = auth.NewTOTPSecret()
	if err := s.Store.TOTP().Put(ctx, &store.TOTP{UserID: u.ID, Secret: secret}); err != nil {
		return "", "", err
	}
	return secret, auth.TOTPURI(secret, totpIssuer, u.Username), nil
}

// TOTPEnable 用一个有效验证码确认待启用的密钥，返回恢复码（只显示这一次）。
func (s *Service) TOTPEnable(ctx context.Context, userID, code string) ([]string, error) {
	t, err := s.Store.TOTP().Get(ctx, userID)
	if err != nil || t.Enabled {
		return nil, ErrMFAState
	}
	step, ok := auth.VerifyTOTP(t.Secret, code, s.now())
	if !ok {
		return nil, ErrMFA
	}
	t.Enabled, t.LastStep = true, step
	codes, hashes := auth.NewRecoveryCodes(recoveryCodesN)
	err = s.Store.Tx(ctx, func(tx store.Store) error {
		if err := tx.TOTP().Put(ctx, t); err != nil {
			return err
		}
		return tx.TOTP().ReplaceCodes(ctx, userID, hashes)
	})
	if err != nil {
		return nil, err
	}
	s.audit(ctx, userID, "mfa.enable", nil)
	s.notify(ctx, userID, AlertMFA, map[string]string{"change": "enabled"})
	return codes, nil
}

// TOTPDisable 关闭两步验证；需要口令。
func (s *Service) TOTPDisable(ctx context.Context, userID, password string) error {
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return err
	}
	if err := s.Store.TOTP().Delete(ctx, userID); err != nil {
		return err
	}
	s.audit(ctx, userID, "mfa.disable", nil)
	s.notify(ctx, userID, AlertMFA, map[string]string{"change": "disabled"})
	return nil
}

// RegenerateRecoveryCodes 生成新的恢复码（旧码全部作废）；需要口令且两步验证已启用。
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, password string) ([]string, error) {
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return nil, err
	}
	if st, err := s.TOTPStatus(ctx, userID); err != nil || !st.Enabled {
		return nil, ErrMFAState
	}
	codes, hashes := auth.NewRecoveryCodes(recoveryCodesN)
	if err := s.Store.TOTP().ReplaceCodes(ctx, userID, hashes); err != nil {
		return nil, err
	}
	s.audit(ctx, userID, "mfa.recovery_codes", nil)
	s.notify(ctx, userID, AlertMFA, map[string]string{"change": "recovery codes regenerated"})
	return codes, nil
}

func (s *Service) checkPassword(ctx context.Context, userID, password string) error {
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return ErrWrongPassword
	}
	return nil
}
