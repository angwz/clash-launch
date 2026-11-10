package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/service"
)

// 账号安全接口（第 14.1 节）：工作量证明、找回、安全问题、修改口令、会话管理。
// 业务规则在 internal/service/account.go；这里只做 HTTP 层的同源检查、限流与 DTO。

type powDTO struct {
	Challenge string `json:"challenge"`
	Bits      int    `json:"bits"`
}

type powProof struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
}

// powChallenge 签发工作量证明挑战。purpose 只能是 register 或 recover。
func (s *Server) powChallenge(w http.ResponseWriter, r *http.Request) {
	purpose := r.URL.Query().Get("purpose")
	if purpose != service.PoWRegister && purpose != service.PoWRecover {
		s.fail(w, badRequest("purpose must be register or recover"))
		return
	}
	ch, bits := s.Svc.PoWChallenge(purpose)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, powDTO{Challenge: ch, Bits: bits})
}

// publicLimited 是未登录的写接口共用的前置检查：同源、按 IP 限速。
func (s *Server) publicLimited(w http.ResponseWriter, r *http.Request, l *limiter) bool {
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "auth.csrf", "cross-origin request refused")
		return false
	}
	if ok, wait := l.allow(s.clientIP(r).String(), time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		problem(w, http.StatusTooManyRequests, "rate_limited", "too many attempts")
		return false
	}
	return true
}

// —— 找回 ——

type questionsDTO struct {
	Questions []auth.Question `json:"questions"`
}

// recoveryQuestions 返回找回时显示的问题；用户不存在时返回稳定的诱饵问题（不泄露用户名是否存在）。
func (s *Server) recoveryQuestions(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimited(w, r, s.recoverLimit) {
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, questionsDTO{Questions: s.Svc.RecoveryQuestions(r.Context(), req.Username)})
}

// recover 用安全问题重设口令；需要工作量证明。成功后该账号的全部会话失效。
func (s *Server) recover(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimited(w, r, s.recoverLimit) {
		return
	}
	var req struct {
		Username string   `json:"username"`
		Answers  []string `json:"answers"`
		Password string   `json:"password"`
		PoW      powProof `json:"pow"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.CheckPoW(service.PoWRecover, req.PoW.Challenge, req.PoW.Nonce); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Recover(r.Context(), req.Username, req.Answers, req.Password, s.clientIP(r).String()); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 安全问题 ——

func (s *Server) myQuestions(w http.ResponseWriter, r *http.Request, p *principal) {
	qs, err := s.Svc.MyQuestions(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if qs == nil {
		qs = []auth.Question{}
	}
	writeJSON(w, http.StatusOK, questionsDTO{Questions: qs})
}

func (s *Server) setQuestions(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Password  string          `json:"password"`
		Questions []auth.Question `json:"questions"`
		Answers   []string        `json:"answers"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.SetQuestions(r.Context(), p.User.ID, req.Password, req.Questions, req.Answers); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 口令 ——

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Current      string `json:"current"`
		Password     string `json:"password"`
		LogoutOthers bool   `json:"logoutOthers"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.ChangePassword(r.Context(), p.User.ID, req.Current, req.Password, req.LogoutOthers, p.Session.IDHash); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 会话 ——

// sessionDTO 是一个登录设备。ID 是会话哈希的前 16 位：足以在本人的会话中定位，
// 不能用来冒充会话（会话令牌原文不可从哈希反推）。
type sessionDTO struct {
	ID        string    `json:"id"`
	Device    string    `json:"device"`
	UA        string    `json:"ua"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
	LastSeen  time.Time `json:"lastSeen"`
	Current   bool      `json:"current"`
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.Svc.Store.Sessions().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]sessionDTO, 0, len(list))
	for _, x := range list {
		out = append(out, sessionDTO{ID: x.IDHash[:16], Device: auth.DeviceName(x.UA), UA: x.UA, IP: x.IPPrefix, CreatedAt: x.CreatedAt, LastSeen: x.LastSeen, Current: x.IDHash == p.Session.IDHash})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	list, err := s.Svc.Store.Sessions().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, x := range list {
		if len(id) == 16 && strings.HasPrefix(x.IDHash, id) {
			if err := s.Svc.Store.Sessions().Delete(r.Context(), p.User.ID, x.IDHash); err != nil {
				s.fail(w, err)
				return
			}
			s.audit(r.Context(), p.User.ID, "session.revoke", map[string]string{"device": auth.DeviceName(x.UA)})
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	problem(w, http.StatusNotFound, "not_found", "resource not found")
}
