package api

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/service"
)

// 通行密钥接口（第 14.1 节；决定 0005）。二进制字段一律为 base64url。
// 规则在 internal/service/passkey.go，验证在 internal/auth/webauthn.go。

type passkeyDTO struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"createdAt"`
	LastUsed  *time.Time `json:"lastUsed,omitempty"`
}

type beginResp struct {
	Options map[string]any `json:"options"`
	Ticket  string         `json:"ticket"`
}

func (s *Server) listPasskeys(w http.ResponseWriter, r *http.Request, p *principal) {
	keys, err := s.Svc.Store.Passkeys().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]passkeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, passkeyDTO{ID: k.ID, Name: k.Name, CreatedAt: k.CreatedAt, LastUsed: tp(k.LastUsed)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "available": s.passkeysAvailable()})
}

func (s *Server) passkeysAvailable() bool {
	_, err := s.Svc.RelyingParty()
	return err == nil
}

func (s *Server) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request, p *principal) {
	opts, ticket, err := s.Svc.PasskeyRegisterBegin(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, beginResp{Options: opts, Ticket: ticket})
}

func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Ticket     string `json:"ticket"`
		Name       string `json:"name"`
		Credential struct {
			ClientDataJSON    string `json:"clientDataJSON"`
			AttestationObject string `json:"attestationObject"`
		} `json:"credential"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	cd, err1 := auth.UnB64(req.Credential.ClientDataJSON)
	ao, err2 := auth.UnB64(req.Credential.AttestationObject)
	if err1 != nil || err2 != nil {
		s.fail(w, service.ErrPasskey)
		return
	}
	k, err := s.Svc.PasskeyRegisterFinish(r.Context(), p.User, req.Ticket, req.Name, cd, ao)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, passkeyDTO{ID: k.ID, Name: k.Name, CreatedAt: k.CreatedAt})
}

func (s *Server) renamePasskey(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 60 {
		s.fail(w, badRequest("name must be 1–60 characters"))
		return
	}
	if err := s.Svc.Store.Passkeys().Rename(r.Context(), p.User.ID, r.PathValue("id"), name); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deletePasskey(w http.ResponseWriter, r *http.Request, p *principal) {
	var req passwordReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.PasskeyDelete(r.Context(), p.User.ID, r.PathValue("id"), req.Password); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sessionPasskeyBegin 开始通行密钥登录。带 ticket（口令登录返回的第二步凭证）时用作第二步。
func (s *Server) sessionPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimited(w, r, s.passkeyLimit) {
		return
	}
	var req struct {
		Ticket string `json:"ticket"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	opts, ticket, err := s.Svc.PasskeyLoginBegin(r.Context(), req.Ticket)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, beginResp{Options: opts, Ticket: ticket})
}

// sessionPasskey 用通行密钥完成登录并建立会话。
func (s *Server) sessionPasskey(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "auth.csrf", "cross-origin login refused")
		return
	}
	var req struct {
		Ticket     string `json:"ticket"`
		Credential struct {
			ID                string `json:"id"`
			ClientDataJSON    string `json:"clientDataJSON"`
			AuthenticatorData string `json:"authenticatorData"`
			Signature         string `json:"signature"`
			UserHandle        string `json:"userHandle"`
		} `json:"credential"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	c := req.Credential
	cd, e1 := auth.UnB64(c.ClientDataJSON)
	ad, e2 := auth.UnB64(c.AuthenticatorData)
	sig, e3 := auth.UnB64(c.Signature)
	uh, e4 := auth.UnB64(c.UserHandle)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		s.fail(w, service.ErrPasskey)
		return
	}
	u, err := s.Svc.PasskeyLoginFinish(r.Context(), req.Ticket, service.PasskeyAssertion{CredentialID: c.ID, ClientDataJSON: cd, AuthenticatorData: ad, Signature: sig, UserHandle: uh}, s.clientIP(r).String())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.startSession(w, r, u)
}
