package service

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 通行密钥（第 14.1 节，可选；决定 0005）：
//   - 作为免密码登录：凭通行密钥（要求用户验证）直接登录，视为已完成两步验证；
//   - 作为第二步：启用了 TOTP 的账号在第二步也可以用通行密钥代替验证码；
//   - 不会让口令登录额外要求通行密钥（丢失设备时不至于被锁在门外）。
// 挑战是签名凭证中的随机数（5 分钟有效），只能使用一次。依赖方由对外地址决定，没有配置时不可用。

var (
	// ErrPasskey 表示通行密钥验证失败。
	ErrPasskey = errors.New("service: 通行密钥验证失败")
	// ErrPasskeyUnavailable 表示没有配置对外地址（WebAuthn 需要确定的域名与来源）。
	ErrPasskeyUnavailable = errors.New("service: 未配置对外地址，通行密钥不可用")
)

const (
	ticketPasskeyReg   = "pk.reg"
	ticketPasskeyLogin = "pk.login"
	passkeyTTL         = 5 * time.Minute
	maxPasskeys        = 10
)

// RelyingParty 由对外地址推出：RP ID 为主机名（不含端口），来源为 scheme://host[:port]。
func (s *Service) RelyingParty() (auth.RelyingParty, error) {
	u, err := url.Parse(s.PublicURL)
	if s.PublicURL == "" || err != nil || u.Host == "" {
		return auth.RelyingParty{}, ErrPasskeyUnavailable
	}
	return auth.RelyingParty{ID: u.Hostname(), Name: totpIssuer, Origin: u.Scheme + "://" + u.Host}, nil
}

func (s *Service) passkeyOnce() *auth.Once {
	s.onceOnce.Do(func() { s.onceVal = auth.NewOnce() })
	return s.onceVal
}

func newChallenge() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// openTicket 校验通行密钥的凭证，返回其中的主体与挑战；挑战只能使用一次。
func (s *Service) openTicket(purpose, ticket string) (subject string, challenge []byte, err error) {
	sub, err := s.tickets().Verify(purpose, ticket, s.now())
	if err != nil {
		return "", nil, ErrPasskey
	}
	subject, enc, ok := strings.Cut(sub, "|")
	if !ok {
		return "", nil, ErrPasskey
	}
	if challenge, err = auth.UnB64(enc); err != nil {
		return "", nil, ErrPasskey
	}
	if !s.passkeyOnce().Use(purpose+"|"+enc, s.now().Add(passkeyTTL), s.now()) {
		return "", nil, ErrPasskey
	}
	return subject, challenge, nil
}

// PasskeyRegisterBegin 返回 navigator.credentials.create() 的选项（二进制字段为 base64url）与凭证。
func (s *Service) PasskeyRegisterBegin(ctx context.Context, u *store.User) (map[string]any, string, error) {
	rp, err := s.RelyingParty()
	if err != nil {
		return nil, "", err
	}
	existing, err := s.Store.Passkeys().List(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	if len(existing) >= maxPasskeys {
		return nil, "", ErrQuota
	}
	exclude := make([]map[string]any, 0, len(existing))
	for _, k := range existing {
		exclude = append(exclude, map[string]any{"type": "public-key", "id": k.CredentialID})
	}
	ch := newChallenge()
	opts := map[string]any{
		"rp":        map[string]any{"id": rp.ID, "name": rp.Name},
		"user":      map[string]any{"id": auth.B64([]byte(u.ID)), "name": u.Username, "displayName": u.Username},
		"challenge": auth.B64(ch),
		"pubKeyCredParams": []map[string]any{
			{"type": "public-key", "alg": auth.AlgEdDSA},
			{"type": "public-key", "alg": auth.AlgES256},
			{"type": "public-key", "alg": auth.AlgRS256},
		},
		"authenticatorSelection": map[string]any{"residentKey": "required", "requireResidentKey": true, "userVerification": "required"},
		"attestation":            "none",
		"excludeCredentials":     exclude,
		"timeout":                120000,
	}
	return opts, s.tickets().Issue(ticketPasskeyReg, u.ID+"|"+auth.B64(ch), passkeyTTL, s.now()), nil
}

// PasskeyRegisterFinish 验证注册结果并保存凭据。
func (s *Service) PasskeyRegisterFinish(ctx context.Context, u *store.User, ticket, name string, clientDataJSON, attestationObject []byte) (*store.Passkey, error) {
	rp, err := s.RelyingParty()
	if err != nil {
		return nil, err
	}
	sub, ch, err := s.openTicket(ticketPasskeyReg, ticket)
	if err != nil || sub != u.ID {
		return nil, ErrPasskey
	}
	cred, err := rp.VerifyRegistration(ch, clientDataJSON, attestationObject)
	if err != nil {
		if s.Log != nil {
			s.Log.Info("通行密钥注册失败", "user", u.ID, "error", err)
		}
		return nil, ErrPasskey
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		name = "Passkey"
	}
	k := &store.Passkey{UserID: u.ID, CredentialID: auth.B64(cred.ID), PublicKey: cred.PublicKey, SignCount: cred.SignCount, Name: name}
	if err := s.Store.Passkeys().Create(ctx, k); err != nil {
		return nil, err
	}
	s.audit(ctx, u.ID, "passkey.add", map[string]string{"name": name})
	s.notify(ctx, u.ID, AlertPasskey, map[string]string{"change": "added", "name": name})
	return k, nil
}

// PasskeyLoginBegin 返回 navigator.credentials.get() 的选项与凭证。
// mfaTicket 非空时用于第二步：只允许该用户的凭据；为空时是免密码登录（可发现凭据，不需要用户名）。
func (s *Service) PasskeyLoginBegin(ctx context.Context, mfaTicket string) (map[string]any, string, error) {
	rp, err := s.RelyingParty()
	if err != nil {
		return nil, "", err
	}
	subject := ""
	allow := []map[string]any{}
	if mfaTicket != "" {
		u, err := s.MFAUser(ctx, mfaTicket)
		if err != nil {
			return nil, "", err
		}
		subject = u.ID
		keys, err := s.Store.Passkeys().List(ctx, u.ID)
		if err != nil {
			return nil, "", err
		}
		for _, k := range keys {
			allow = append(allow, map[string]any{"type": "public-key", "id": k.CredentialID})
		}
	}
	ch := newChallenge()
	opts := map[string]any{"challenge": auth.B64(ch), "rpId": rp.ID, "userVerification": "required", "allowCredentials": allow, "timeout": 120000}
	return opts, s.tickets().Issue(ticketPasskeyLogin, subject+"|"+auth.B64(ch), passkeyTTL, s.now()), nil
}

// PasskeyAssertion 是 navigator.credentials.get() 的结果（二进制字段已解码）。
type PasskeyAssertion struct {
	CredentialID      string // base64url
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
	UserHandle        []byte
}

// PasskeyLoginFinish 验证断言，返回登录的用户。失败计入 IP 的登录退避。
func (s *Service) PasskeyLoginFinish(ctx context.Context, ticket string, a PasskeyAssertion, ip string) (*store.User, error) {
	g, now := s.g(), s.now()
	if w := g.loginIP.Wait("ip:"+ip, now); w > 0 {
		return nil, &WaitError{Wait: w}
	}
	u, err := s.passkeyLogin(ctx, ticket, a)
	if err != nil {
		g.loginIP.Fail("ip:"+ip, now)
		s.audit(ctx, "", "passkey.login_failed", map[string]string{"ip": ip})
		return nil, ErrPasskey
	}
	g.loginIP.Reset("ip:" + ip)
	g.loginUser.Reset(userKey(u.Username))
	return u, nil
}

func (s *Service) passkeyLogin(ctx context.Context, ticket string, a PasskeyAssertion) (*store.User, error) {
	rp, err := s.RelyingParty()
	if err != nil {
		return nil, err
	}
	subject, ch, err := s.openTicket(ticketPasskeyLogin, ticket)
	if err != nil {
		return nil, err
	}
	k, err := s.Store.Passkeys().ByCredential(ctx, a.CredentialID)
	if err != nil {
		return nil, ErrPasskey
	}
	// 第二步时凭据必须属于口令验证过的那个用户；免密码登录时 userHandle 必须与凭据的所有者一致
	if subject != "" && k.UserID != subject {
		return nil, ErrPasskey
	}
	if len(a.UserHandle) > 0 && string(a.UserHandle) != k.UserID {
		return nil, ErrPasskey
	}
	count, err := rp.VerifyAssertion(ch, k.PublicKey, k.SignCount, a.ClientDataJSON, a.AuthenticatorData, a.Signature)
	if err != nil {
		if s.Log != nil {
			s.Log.Info("通行密钥登录失败", "user", k.UserID, "error", err)
		}
		return nil, ErrPasskey
	}
	if err := s.Store.Passkeys().Used(ctx, k.ID, count, s.now()); err != nil {
		return nil, err
	}
	u, err := s.Store.Users().ByID(ctx, k.UserID)
	if err != nil || u.Disabled {
		return nil, ErrPasskey
	}
	return u, nil
}

// PasskeyDelete 删除一个通行密钥；需要口令。
func (s *Service) PasskeyDelete(ctx context.Context, userID, id, password string) error {
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return err
	}
	if err := s.Store.Passkeys().Delete(ctx, userID, id); err != nil {
		return err
	}
	s.audit(ctx, userID, "passkey.delete", map[string]string{"id": id})
	s.notify(ctx, userID, AlertPasskey, map[string]string{"change": "removed"})
	return nil
}
