package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 账号安全（第 14.1 节）：登录退避、安全问题与找回、工作量证明、修改口令。
// 退避与工作量证明的状态在进程内（单实例假设，见 docs/decisions/0002 #6）。

var (
	// ErrInvalidLogin 表示用户名或口令错误（不区分两者，不泄露用户是否存在）。
	ErrInvalidLogin = errors.New("service: 用户名或口令错误")
	// ErrRecoveryFailed 表示找回失败：用户不存在、没有设置安全问题，或答对的少于 2 个（不区分）。
	ErrRecoveryFailed = errors.New("service: 找回失败")
	// ErrWrongPassword 表示需要当前口令的操作中口令错误。
	ErrWrongPassword = errors.New("service: 当前口令错误")
)

// WaitError 表示需要等待一段时间后再试（退避或锁定）。
type WaitError struct{ Wait time.Duration }

func (e *WaitError) Error() string {
	return fmt.Sprintf("service: 请在 %s 后重试", e.Wait.Round(time.Second))
}

// guards 是账号安全的进程内状态，第一次使用时初始化（零值 Service 也能工作，便于测试与命令行）。
type guards struct {
	pow         *auth.PoW
	loginUser   *auth.Backoff // 按账号：前 3 次不等待，之后 1 s 起指数增长，最长 15 分钟
	loginIP     *auth.Backoff // 按 IP：前 10 次不等待（同一出口下的多个用户）
	recoverUser *auth.Backoff // 按账号：连续失败 5 次后锁定 15 分钟
	recoverIP   *auth.Backoff // 按 IP：连续失败 10 次后锁定 15 分钟
	decoy       []byte        // 生成诱饵问题的密钥
}

func (s *Service) g() *guards {
	s.guardsOnce.Do(func() {
		s.guards = &guards{
			pow:         auth.NewPoW(s.PoWBits),
			loginUser:   auth.NewBackoff(3, time.Second, 15*time.Minute),
			loginIP:     auth.NewBackoff(10, time.Second, 15*time.Minute),
			recoverUser: auth.NewBackoff(4, 15*time.Minute, 15*time.Minute),
			recoverIP:   auth.NewBackoff(9, 15*time.Minute, 15*time.Minute),
			decoy:       []byte(auth.NewToken(32)),
		}
	})
	return s.guards
}

// guardsState 嵌入 Service（见 service.go）。
type guardsState struct {
	guardsOnce  sync.Once
	guards      *guards
	ticketsOnce sync.Once
	ticketsVal  *auth.Tickets
}

// —— 工作量证明 ——

// PoW 用途。
const (
	PoWRegister = "register"
	PoWRecover  = "recover"
)

// PoWChallenge 签发一个挑战，返回挑战与要求的位数。
func (s *Service) PoWChallenge(purpose string) (string, int) {
	g := s.g()
	return g.pow.Challenge(purpose, s.now()), g.pow.Bits
}

// CheckPoW 校验工作量证明；无效时返回 auth.ErrPoW。
func (s *Service) CheckPoW(purpose, challenge, nonce string) error {
	return s.g().pow.Verify(purpose, challenge, nonce, s.now())
}

// —— 登录 ——

func userKey(username string) string { return "u:" + store.NormalizeUsername(username) }

// Login 校验用户名与口令。失败按账号与 IP 退避；等待期内直接返回 *WaitError，不再校验口令。
func (s *Service) Login(ctx context.Context, username, password, ip string) (*store.User, error) {
	g, now := s.g(), s.now()
	if w := max(g.loginUser.Wait(userKey(username), now), g.loginIP.Wait("ip:"+ip, now)); w > 0 {
		return nil, &WaitError{Wait: w}
	}
	u, err := s.Store.Users().ByUsername(ctx, username)
	if err != nil {
		auth.DummyVerify(password) // 与口令错误同样耗时，不泄露用户是否存在
	}
	if err != nil || u.Disabled || !auth.VerifyPassword(u.PasswordHash, password) {
		g.loginUser.Fail(userKey(username), now)
		g.loginIP.Fail("ip:"+ip, now)
		owner := ""
		if u != nil {
			owner = u.ID
		}
		s.audit(ctx, owner, "login.failed", map[string]string{"ip": ip})
		return nil, ErrInvalidLogin
	}
	g.loginUser.Reset(userKey(username))
	g.loginIP.Reset("ip:" + ip)
	return u, nil
}

// —— 安全问题 ——

// hashQuestions 规范化并哈希答案，生成存储形式。调用前已经过 auth.CheckQuestions。
func hashQuestions(qs []auth.Question, answers []string) ([]store.Question, error) {
	out := make([]store.Question, len(qs))
	for i, q := range qs {
		h, err := auth.HashPassword(auth.NormalizeAnswer(answers[i]))
		if err != nil {
			return nil, err
		}
		out[i] = store.Question{Idx: i, Question: q.Encode(), AnswerHash: h}
	}
	return out, nil
}

// SetQuestions 替换安全问题；需要当前口令。
func (s *Service) SetQuestions(ctx context.Context, userID, password string, qs []auth.Question, answers []string) error {
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return ErrWrongPassword
	}
	if err := auth.CheckQuestions(qs, answers); err != nil {
		return err
	}
	stored, err := hashQuestions(qs, answers)
	if err != nil {
		return err
	}
	if err := s.Store.Questions().Set(ctx, userID, stored); err != nil {
		return err
	}
	s.audit(ctx, userID, "account.questions", nil)
	s.notify(ctx, userID, AlertQuestions, nil)
	return nil
}

// MyQuestions 返回用户自己的安全问题（不含答案）。
func (s *Service) MyQuestions(ctx context.Context, userID string) ([]auth.Question, error) {
	stored, err := s.Store.Questions().List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]auth.Question, len(stored))
	for i, q := range stored {
		out[i] = auth.DecodeQuestion(q.Question)
	}
	return out, nil
}

// RecoveryQuestions 返回找回时显示的问题。用户不存在或没有设置问题时返回一组稳定的诱饵问题，
// 响应形状相同，不能据此判断用户名是否存在。
func (s *Service) RecoveryQuestions(ctx context.Context, username string) []auth.Question {
	if u, err := s.Store.Users().ByUsername(ctx, username); err == nil && !u.Disabled {
		if qs, err := s.MyQuestions(ctx, u.ID); err == nil && len(qs) == auth.QuestionCount {
			return qs
		}
	}
	return auth.DecoyQuestions(s.g().decoy, username)
}

// Recover 用安全问题重设口令：答对至少 2 个时设置新口令，并注销该账号的全部会话。
// 失败按账号与 IP 计数，连续失败后锁定 15 分钟。
func (s *Service) Recover(ctx context.Context, username string, answers []string, newPassword, ip string) error {
	g, now := s.g(), s.now()
	if w := max(g.recoverUser.Wait(userKey(username), now), g.recoverIP.Wait("ip:"+ip, now)); w > 0 {
		return &WaitError{Wait: w}
	}
	if err := auth.CheckPassword(newPassword, CommonPasswords); err != nil {
		return err
	}
	if len(answers) != auth.QuestionCount {
		return ErrRecoveryFailed
	}
	u, err := s.Store.Users().ByUsername(ctx, username)
	var stored []store.Question
	if err == nil && !u.Disabled {
		stored, _ = s.Store.Questions().List(ctx, u.ID)
	}
	correct := 0
	for i, a := range answers {
		norm := auth.NormalizeAnswer(a)
		if i < len(stored) {
			if auth.VerifyPassword(stored[i].AnswerHash, norm) {
				correct++
			}
		} else {
			auth.DummyVerify(norm) // 不存在的用户同样做三次哈希，耗时一致
		}
	}
	if len(stored) != auth.QuestionCount || correct < auth.RecoverThreshold {
		g.recoverUser.Fail(userKey(username), now)
		g.recoverIP.Fail("ip:"+ip, now)
		owner := ""
		if u != nil {
			owner = u.ID
		}
		s.audit(ctx, owner, "recovery.failed", map[string]string{"ip": ip})
		return ErrRecoveryFailed
	}
	if err := s.setPassword(ctx, u, newPassword); err != nil {
		return err
	}
	if err := s.Store.Sessions().DeleteAll(ctx, u.ID); err != nil {
		return err
	}
	g.recoverUser.Reset(userKey(username))
	g.loginUser.Reset(userKey(username))
	s.audit(ctx, u.ID, "recovery.ok", map[string]string{"ip": ip})
	s.notify(ctx, u.ID, AlertRecovered, map[string]string{"ip": ip})
	return nil
}

// —— 口令 ——

func (s *Service) setPassword(ctx context.Context, u *store.User, pw string) error {
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	return s.Store.Users().Update(ctx, u)
}

// ChangePassword 修改口令（需要当前口令）。logoutOthers 为 true 时注销除 keepSession 外的全部会话。
func (s *Service) ChangePassword(ctx context.Context, userID, current, next string, logoutOthers bool, keepSession string) error {
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(u.PasswordHash, current) {
		return ErrWrongPassword
	}
	if err := auth.CheckPassword(next, CommonPasswords); err != nil {
		return err
	}
	if err := s.setPassword(ctx, u, next); err != nil {
		return err
	}
	if logoutOthers {
		sessions, err := s.Store.Sessions().List(ctx, userID)
		if err != nil {
			return err
		}
		for _, x := range sessions {
			if x.IDHash != keepSession {
				_ = s.Store.Sessions().Delete(ctx, userID, x.IDHash)
			}
		}
	}
	s.audit(ctx, userID, "account.password", map[string]string{"logoutOthers": fmt.Sprint(logoutOthers)})
	s.notify(ctx, userID, AlertPassword, nil)
	return nil
}

// —— 审计 ——

func (s *Service) audit(ctx context.Context, owner, action string, detail map[string]string) {
	var b []byte
	if len(detail) > 0 {
		parts := make([]string, 0, len(detail))
		for k, v := range detail {
			parts = append(parts, fmt.Sprintf("%q:%q", k, v))
		}
		// 键序固定，便于比较与检索
		slices.Sort(parts)
		b = []byte("{" + strings.Join(parts, ",") + "}")
	}
	if err := s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: owner, Action: action, Detail: b}); err != nil && s.Log != nil {
		s.Log.Warn("写入审计日志失败", "action", action, "error", err)
	}
}

// LoginSucceeded 记录一次成功登录：审计与已知设备。设备第一次出现时发送新设备提醒；
// 账号创建后一分钟内的登录（注册后的自动登录）不提醒。
func (s *Service) LoginSucceeded(ctx context.Context, u *store.User, ipPrefix, ua string) {
	device := auth.DeviceName(ua)
	s.audit(ctx, u.ID, "login.ok", map[string]string{"ip": ipPrefix, "device": device})
	isNew, err := s.Store.Devices().Seen(ctx, u.ID, auth.DeviceFingerprint(ipPrefix, ua), s.now())
	if err == nil && isNew && s.now().Sub(u.CreatedAt) > time.Minute {
		s.notify(ctx, u.ID, AlertNewDevice, map[string]string{"ip": ipPrefix, "device": device})
	}
}
