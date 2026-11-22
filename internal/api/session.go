package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

type ctxKey int

const userKey ctxKey = 1

// principal 是已认证的请求者。
type principal struct {
	User    *store.User
	Session *store.Session
	// Token 非空表示请求用个人访问令牌认证（需求 R25）；此时 Session 是空值，没有 CSRF 令牌
	Token *store.AccessToken
}

func (s *Server) cookieName(r *http.Request) string {
	if s.secure(r) {
		return "__Host-cl_session" // __Host- 前缀要求 Secure、Path=/、无 Domain
	}
	return "cl_session"
}

// authed 包装需要登录的处理器；写操作额外检查 CSRF 与 Origin（第 14.2 节）。
// 带 Authorization: Bearer 的请求按个人访问令牌认证（需求 R25），不看 Cookie：令牌不是浏览器自动附带的凭据，
// 不存在跨站请求伪造，所以不检查 CSRF；它能访问的范围由 gate 限制。
func (s *Server) authed(h func(http.ResponseWriter, *http.Request, *principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			t, u, err := s.Svc.TokenPrincipal(r.Context(), strings.TrimSpace(bearer), s.clientIP(r).String())
			if err != nil {
				problem(w, http.StatusUnauthorized, "auth.token", "the access token is invalid, expired or revoked")
				return
			}
			p := &principal{User: u, Session: &store.Session{}, Token: t}
			if s.gate(w, r, p) {
				h(w, r.WithContext(context.WithValue(r.Context(), userKey, p)), p)
			}
			return
		}
		p, err := s.principal(r)
		if err != nil {
			problem(w, http.StatusUnauthorized, "auth.required", "login required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !s.sameOrigin(r) || !auth.Equal(r.Header.Get("X-CSRF-Token"), p.Session.CSRF) {
				problem(w, http.StatusForbidden, "auth.csrf", "missing or invalid CSRF token")
				return
			}
		}
		if !s.gate(w, r, p) {
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), userKey, p)), p)
	}
}

func (s *Server) principal(r *http.Request) (*principal, error) {
	c, err := r.Cookie(s.cookieName(r))
	if err != nil || c.Value == "" {
		return nil, errors.New("no session")
	}
	ctx := r.Context()
	sess, err := s.Svc.Store.Sessions().ByHash(ctx, auth.HashToken(c.Value))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeen) > s.Cfg.SessionIdle {
		_ = s.Svc.Store.Sessions().Delete(ctx, sess.UserID, sess.IDHash)
		return nil, errors.New("session expired")
	}
	// 定时退出到点（需求 R8）：与过期一样删除会话，并记入审计
	if !sess.LogoutAt.IsZero() && now.After(sess.LogoutAt) {
		_ = s.Svc.Store.Sessions().Delete(ctx, sess.UserID, sess.IDHash)
		s.audit(ctx, sess.UserID, "session.scheduled_logout", map[string]string{"device": auth.DeviceName(sess.UA)})
		return nil, errors.New("session ended as scheduled")
	}
	u, err := s.Svc.Store.Users().ByID(ctx, sess.UserID)
	if err != nil || u.Disabled {
		return nil, errors.New("user unavailable")
	}
	if now.Sub(sess.LastSeen) > time.Minute {
		_ = s.Svc.Store.Sessions().Touch(ctx, sess.IDHash, now)
	}
	return &principal{User: u, Session: sess}, nil
}

// sameOrigin 检查 Origin（没有 Origin 时检查 Referer 不可用——浏览器的写请求总会带 Origin）。
func (s *Server) sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return false
	}
	u, err := url.Parse(o)
	if err != nil {
		return false
	}
	// 配置了对外地址时，Origin 必须等于这个请求所用的那一个（Host 不在列表中时是主地址，必然对不上）
	if p := s.publicFor(r); p != "" {
		pu, err := url.Parse(p)
		return err == nil && strings.EqualFold(pu.Host, u.Host) && strings.EqualFold(pu.Host, r.Host) && pu.Scheme == u.Scheme
	}
	return strings.EqualFold(u.Host, r.Host)
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type sessionResp struct {
	User userDTO `json:"user"`
	CSRF string  `json:"csrf"`
	// LogoutAt 是当前设备的定时退出时间（需求 R8），页面顶部据此提示
	LogoutAt *time.Time `json:"logoutAt,omitempty"`
	// Terms 是用户协议的同意状态（需求 R40）；pending 为 true 时页面先显示协议
	Terms termsState `json:"terms"`
	// MFASetup 为 true 表示两步验证策略要求本账号启用而尚未启用（需求 R41）；页面先引导设置
	MFASetup bool `json:"mfaSetup"`
	// DeletionCanceled 为 true 表示这次登录撤销了冷静期内的注销（需求 R20），页面据此提示
	DeletionCanceled bool `json:"deletionCanceled,omitempty"`
}

type userDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Lang     string `json:"lang"`
	Timezone string `json:"timezone"`
	// 需求 R4、R5、R12
	Nickname     string     `json:"nickname,omitempty"`
	NextRenameAt *time.Time `json:"nextRenameAt,omitempty"` // 下次可以改用户名的时间；为空表示现在就可以
	InvitedBy    string     `json:"invitedBy,omitempty"`    // 邀请人的用户名
}

func toUser(u *store.User) userDTO {
	d := userDTO{ID: u.ID, Username: u.Username, Role: u.Role, Lang: u.Lang, Timezone: u.Timezone, Nickname: u.Nickname, InvitedBy: u.InvitedByName}
	if next := service.NextRename(u); next.After(time.Now()) {
		d.NextRenameAt = &next
	}
	return d
}

// login 校验密码并建立新会话（登录后总是新的会话 ID，防会话固定）。
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "auth.csrf", "cross-origin login refused")
		return
	}
	var req loginReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	ctx := r.Context()
	u, err := s.Svc.Login(ctx, req.Username, req.Password, s.clientIP(r).String())
	if err != nil {
		s.fail(w, err)
		return
	}
	// 启用了第二因素：不建立会话，返回 5 分钟有效的凭证，由第二步完成登录
	if factors := s.Svc.SecondFactors(ctx, u.ID); len(factors) > 0 {
		writeJSON(w, http.StatusOK, mfaResp{MFA: factors, Ticket: s.Svc.MFATicket(u.ID)})
		return
	}
	s.startSession(w, r, u)
}

// mfaResp 是需要第二因素时的登录响应。
type mfaResp struct {
	MFA    []string `json:"mfa"`
	Ticket string   `json:"ticket"`
}

// startSession 建立新会话（总是新的会话 ID，防会话固定），设置 Cookie，记录登录与设备。
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) {
	ctx := r.Context()
	s.Svc.Stats().Login(time.Now()) // 登录次数（反馈 #66），密码、两步验证与通行密钥登录都经过这里
	if old, err := r.Cookie(s.cookieName(r)); err == nil {
		_ = s.Svc.Store.Sessions().Delete(ctx, u.ID, auth.HashToken(old.Value))
	}
	tok := auth.NewToken(32)
	now := time.Now()
	addr := s.clientIP(r)
	sess := &store.Session{IDHash: auth.HashToken(tok), UserID: u.ID, CSRF: auth.NewToken(24), CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(s.Cfg.SessionMax), UA: truncate(r.UserAgent(), 200), IPPrefix: ipPrefix(addr), Location: placeJSON(s.Svc.Geo.Place(addr))}
	if err := s.Svc.Store.Sessions().Create(ctx, sess); err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(r), Value: tok, Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: int(s.Cfg.SessionMax.Seconds())}) //nolint:gosec // Secure 在 HTTPS 部署下为 true；明文的本地开发中浏览器不会发送 Secure Cookie
	s.Svc.LoginSucceeded(ctx, u, sess.IPPrefix, sess.UA, s.Svc.Geo.Lookup(addr).Country)
	canceled := s.Svc.CancelDeletionOnLogin(ctx, u) // 冷静期内登录即撤销注销（需求 R20）
	terms, err := s.termsState(ctx, u)
	if err != nil {
		s.fail(w, err)
		return
	}
	mfa, err := s.Svc.MFASetupPending(ctx, u)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionResp{User: toUser(u), CSRF: sess.CSRF, Terms: terms, MFASetup: mfa, DeletionCanceled: canceled})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, p *principal) {
	_ = s.Svc.Store.Sessions().Delete(r.Context(), p.User.ID, p.Session.IDHash)
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(r), Value: "", Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1}) //nolint:gosec // 同上
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, p *principal) {
	terms, err := s.termsState(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	mfa, err := s.Svc.MFASetupPending(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionResp{User: s.userWithInviter(r.Context(), p.User), CSRF: p.Session.CSRF, LogoutAt: tp(p.Session.LogoutAt), Terms: terms, MFASetup: mfa})
}

type setupReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// setupStatus 报告是否还需要创建首个管理员（登录页据此跳到首次注册）。
func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	needed, err := s.Svc.NeedsSetup(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needed": needed})
}

// setupAdmin 在还没有任何账号时创建首个管理员；之后一律 403。
// 检查与创建在同一把锁内完成，并发的第二个请求会看到已有账号。
func (s *Server) setupAdmin(w http.ResponseWriter, r *http.Request) {
	var req setupReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	needed, err := s.Svc.NeedsSetup(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if !needed {
		problem(w, http.StatusForbidden, "setup.done", "an administrator already exists")
		return
	}
	u, err := s.Svc.CreateAdmin(r.Context(), strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUser(u))
}

func (s *Server) audit(ctx context.Context, owner, action string, detail map[string]string) {
	// 经服务层写入：同时记下由该动作得出的足迹（R10）
	s.Svc.Audit(ctx, owner, action, detail)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// hashPassword 供测试与管理命令使用。
func hashPassword(pw string) (string, error) { return auth.HashPassword(pw) }
