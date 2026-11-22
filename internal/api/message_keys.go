package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 站内消息的加密密钥（需求 R46，决定 0009）。

type messageKeyDTO struct {
	KeyID     string          `json:"keyId"`
	PublicKey string          `json:"publicKey"`
	Active    bool            `json:"active"`
	CreatedAt time.Time       `json:"createdAt"`
	Wrapped   json.RawMessage `json:"wrapped,omitempty"`  // 只返回给本人
	Recovery  json.RawMessage `json:"recovery,omitempty"` // 只返回给本人；没有恢复密钥时省略
}

func rawOrNil(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

// myMessageKey 返回本人当前的密钥与包裹（浏览器用密码或恢复密钥解开）；还没有时返回 {"key": null}。
func (s *Server) myMessageKey(w http.ResponseWriter, r *http.Request, p *principal) {
	k, err := s.Svc.Store.MessageKeys().Active(r.Context(), p.User.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"key": nil})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": messageKeyDTO{KeyID: k.KeyID, PublicKey: k.PublicKey, Active: true, CreatedAt: k.CreatedAt, Wrapped: rawOrNil(k.Wrapped), Recovery: rawOrNil(k.Recovery)}})
}

// putMessageKey 生成（首次或 replace）或重新包裹当前密钥；需要当前密码。
func (s *Server) putMessageKey(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Password  string          `json:"password"`
		KeyID     string          `json:"keyId"`
		PublicKey string          `json:"publicKey"`
		Wrapped   json.RawMessage `json:"wrapped"`
		Recovery  json.RawMessage `json:"recovery"`
		Replace   bool            `json:"replace"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	k, err := s.Svc.SetMessageKey(r.Context(), p.User, req.Password, service.MessageKeyInput{KeyID: req.KeyID, PublicKey: req.PublicKey, Wrapped: req.Wrapped, Recovery: req.Recovery, Replace: req.Replace})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messageKeyDTO{KeyID: k.KeyID, PublicKey: k.PublicKey, Active: true, CreatedAt: k.CreatedAt})
}

// userMessageKeys 返回某个用户的全部公钥（含已不再使用的旧公钥：解开以前的消息要用）。任何登录用户可读。
func (s *Server) userMessageKeys(w http.ResponseWriter, r *http.Request, _ *principal) {
	u, err := s.Svc.Store.Users().ByUsername(r.Context(), r.PathValue("username"))
	if err != nil {
		s.fail(w, store.ErrNotFound)
		return
	}
	keys, err := s.Svc.Store.MessageKeys().All(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]messageKeyDTO, 0, len(keys))
	for _, k := range keys {
		out = append(out, messageKeyDTO{KeyID: k.KeyID, PublicKey: k.PublicKey, Active: k.Active, CreatedAt: k.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}
