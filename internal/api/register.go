package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// adminOnly 包装只有管理员能用的处理器；普通用户得到 404（不暴露管理接口的存在）。
func (s *Server) adminOnly(h func(http.ResponseWriter, *http.Request, *principal)) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, p *principal) {
		if p.User.Role != "admin" {
			problem(w, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		h(w, r, p)
	})
}

type registrationDTO struct {
	Open bool `json:"open"`
}

// registrationState 是公开接口：注册页据此决定是否要求邀请。
func (s *Server) registrationState(w http.ResponseWriter, r *http.Request) {
	open, err := s.Svc.RegistrationOpen(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, registrationDTO{Open: open})
}

type registerReq struct {
	Username  string          `json:"username"`
	Password  string          `json:"password"`
	Invite    string          `json:"invite"`
	Questions []auth.Question `json:"questions"`
	Answers   []string        `json:"answers"`
	PoW       powProof        `json:"pow"`
}

// register 创建普通用户：注册开放时不需要邀请；关闭时需要有效的一次性邀请。
// 需要工作量证明与 3 个安全问题；按 IP 限速。
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimited(w, r, s.regLimit) {
		return
	}
	var req registerReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.CheckPoW(service.PoWRegister, req.PoW.Challenge, req.PoW.Nonce); err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.Svc.Register(r.Context(), service.RegisterInput{Username: req.Username, Password: req.Password, Invite: strings.TrimSpace(req.Invite), Questions: req.Questions, Answers: req.Answers})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUser(u))
}

func (s *Server) getRegistration(w http.ResponseWriter, r *http.Request, _ *principal) {
	s.registrationState(w, r)
}

func (s *Server) putRegistration(w http.ResponseWriter, r *http.Request, p *principal) {
	var req registrationDTO
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.SetRegistration(r.Context(), p.User.ID, req.Open); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

type inviteDTO struct {
	ID        string     `json:"id"`
	URL       string     `json:"url,omitempty"` // 只在创建时返回：令牌只以哈希存储
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	Status    string     `json:"status"` // active | used | expired
}

func inviteStatus(v *store.Invite, now time.Time) string {
	switch {
	case !v.UsedAt.IsZero():
		return "used"
	case !v.ExpiresAt.IsZero() && now.After(v.ExpiresAt):
		return "expired"
	}
	return "active"
}

func toInvite(v *store.Invite) inviteDTO {
	return inviteDTO{ID: v.ID, Note: v.Note, CreatedAt: v.CreatedAt, ExpiresAt: tp(v.ExpiresAt), UsedAt: tp(v.UsedAt), Status: inviteStatus(v, time.Now())}
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request, _ *principal) {
	items, err := s.Svc.Store.Invites().List(r.Context(), page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[inviteDTO]{Items: []inviteDTO{}}
	for i := range items {
		out.Items = append(out.Items, toInvite(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// createInvite 生成一次性邀请链接；hours 为有效期（小时），0 表示不过期，最长 90 天。
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Hours int    `json:"hours"`
		Note  string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if req.Hours < 0 || req.Hours > 90*24 {
		s.fail(w, badRequest("hours must be between 0 (never expires) and 2160"))
		return
	}
	tok, inv, err := s.Svc.CreateInvite(r.Context(), p.User.ID, time.Duration(req.Hours)*time.Hour, truncate(req.Note, 200))
	if err != nil {
		s.fail(w, err)
		return
	}
	d := toInvite(inv)
	d.URL = s.baseURL(r) + "/register#" + tok // 令牌放在片段中：不会出现在反向代理的访问日志里
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Invites().Delete(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "admin.invite.delete", map[string]string{"invite": r.PathValue("id")})
	w.WriteHeader(http.StatusNoContent)
}
