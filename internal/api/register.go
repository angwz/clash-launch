package api

import (
	"net/http"
	"strings"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/service"
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
	// Terms 是勾选同意的用户协议版本（需求 R40）
	Terms int `json:"terms"`
	// Lang 是注册时的界面语言：选择预置昵称的词表（需求 R19）
	Lang string `json:"lang"`
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
	u, err := s.Svc.Register(r.Context(), service.RegisterInput{Username: req.Username, Password: req.Password, Invite: strings.TrimSpace(req.Invite), Questions: req.Questions, Answers: req.Answers, TermsVersion: req.Terms, Lang: langOf(r, req.Lang)})
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
