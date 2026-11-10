package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

type ctxKey int

const userKey ctxKey = 1

// principal 是已认证的请求者。
type principal struct {
	User    *store.User
	Session *store.Session
}

func (s *Server) cookieName() string {
	if s.secure() {
		return "__Host-cl_session" // __Host- 前缀要求 Secure、Path=/、无 Domain
	}
	return "cl_session"
}

// authed 包装需要登录的处理器；写操作额外检查 CSRF 与 Origin（第 14.2 节）。
func (s *Server) authed(h func(http.ResponseWriter, *http.Request, *principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		h(w, r.WithContext(context.WithValue(r.Context(), userKey, p)), p)
	}
}

func (s *Server) principal(r *http.Request) (*principal, error) {
	c, err := r.Cookie(s.cookieName())
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
	if s.Cfg.PublicURL != "" {
		pu, err := url.Parse(s.Cfg.PublicURL)
		return err == nil && strings.EqualFold(pu.Host, u.Host) && pu.Scheme == u.Scheme
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
}

type userDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Lang     string `json:"lang"`
	Timezone string `json:"timezone"`
}

func toUser(u *store.User) userDTO {
	return userDTO{ID: u.ID, Username: u.Username, Role: u.Role, Lang: u.Lang, Timezone: u.Timezone}
}

// login 校验口令并建立新会话（登录后总是新的会话 ID，防会话固定）。
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
	if old, err := r.Cookie(s.cookieName()); err == nil {
		_ = s.Svc.Store.Sessions().Delete(ctx, u.ID, auth.HashToken(old.Value))
	}
	tok := auth.NewToken(32)
	now := time.Now()
	sess := &store.Session{IDHash: auth.HashToken(tok), UserID: u.ID, CSRF: auth.NewToken(24), CreatedAt: now, LastSeen: now, ExpiresAt: now.Add(s.Cfg.SessionMax), UA: truncate(r.UserAgent(), 200), IPPrefix: ipPrefix(s.clientIP(r))}
	if err := s.Svc.Store.Sessions().Create(ctx, sess); err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: tok, Path: "/", HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode, MaxAge: int(s.Cfg.SessionMax.Seconds())}) //nolint:gosec // Secure 在 HTTPS 部署下为 true；明文的本地开发中浏览器不会发送 Secure Cookie
	s.Svc.LoginSucceeded(ctx, u, sess.IPPrefix, sess.UA)
	writeJSON(w, http.StatusOK, sessionResp{User: toUser(u), CSRF: sess.CSRF})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, p *principal) {
	_ = s.Svc.Store.Sessions().Delete(r.Context(), p.User.ID, p.Session.IDHash)
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", HttpOnly: true, Secure: s.secure(), SameSite: http.SameSiteLaxMode, MaxAge: -1}) //nolint:gosec // 同上
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, p *principal) {
	writeJSON(w, http.StatusOK, sessionResp{User: toUser(p.User), CSRF: p.Session.CSRF})
}

type setupReq struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// setupAdmin 用首次启动时打印的一次性令牌创建首个管理员（15 分钟有效，用后作废）。
func (s *Server) setupAdmin(w http.ResponseWriter, r *http.Request) {
	var req setupReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	s.mu.Lock()
	st := s.setup
	s.mu.Unlock()
	if st == nil || time.Now().After(st.Expires) || !auth.Equal(req.Token, st.Token) {
		problem(w, http.StatusForbidden, "setup.invalid", "setup link is invalid or expired")
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		s.fail(w, badRequest("username is required"))
		return
	}
	u, err := s.Svc.CreateAdmin(r.Context(), strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.SetSetup(nil)
	writeJSON(w, http.StatusCreated, toUser(u))
}

func (s *Server) audit(ctx context.Context, owner, action string, detail map[string]string) {
	var b []byte
	if detail != nil {
		b = []byte(jsonString(detail))
	}
	_ = s.Svc.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: owner, Action: action, Detail: b})
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// hashPassword 供测试与管理命令使用。
func hashPassword(pw string) (string, error) { return auth.HashPassword(pw) }
