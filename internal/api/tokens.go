package api

import (
	"net/http"
	"time"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 个人访问令牌（R25）与两步验证策略（R41）的接口。令牌的管理接口只接受浏览器会话（不在 tokenPaths 中）。

type tokenDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	LastIP     string     `json:"lastIp,omitempty"`
	// Token 是明文，只在创建的响应中出现一次
	Token string `json:"token,omitempty"`
}

func toToken(t *store.AccessToken) tokenDTO {
	return tokenDTO{ID: t.ID, Name: t.Name, Scope: t.Scope, CreatedAt: t.CreatedAt, ExpiresAt: tp(t.ExpiresAt), LastUsedAt: tp(t.LastUsedAt), LastIP: t.LastIP}
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request, p *principal) {
	ts, err := s.Svc.Store.Tokens().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]tokenDTO, 0, len(ts))
	for i := range ts {
		out = append(out, toToken(&ts[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type tokenReq struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Days  int    `json:"days"`
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request, p *principal) {
	var req tokenReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	t, plain, err := s.Svc.CreateToken(r.Context(), p.User, service.TokenInput{Name: req.Name, Scope: req.Scope, Days: req.Days})
	if err != nil {
		s.fail(w, err)
		return
	}
	d := toToken(t)
	d.Token = plain
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.RevokeToken(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mfaPolicyDTO struct {
	// Policy：off 不要求；admins 要求管理员；all 要求全部用户
	Policy string `json:"policy"`
}

func policyOut(p string) string {
	if p == service.MFAPolicyOff {
		return "off"
	}
	return p
}

func (s *Server) getMFAPolicy(w http.ResponseWriter, r *http.Request, _ *principal) {
	pol, err := s.Svc.MFAPolicy(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mfaPolicyDTO{Policy: policyOut(pol)})
}

func (s *Server) putMFAPolicy(w http.ResponseWriter, r *http.Request, p *principal) {
	var req mfaPolicyDTO
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	pol := req.Policy
	switch pol {
	case "off":
		pol = service.MFAPolicyOff
	case "":
		s.fail(w, service.ErrMFAPolicy) // 空串不是合法的输入：关闭要明确写 off
		return
	}
	if err := s.Svc.SetMFAPolicy(r.Context(), p.User, pol); err != nil {
		s.fail(w, err)
		return
	}
	s.getMFAPolicy(w, r, p)
}
