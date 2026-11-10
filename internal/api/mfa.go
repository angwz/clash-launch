package api

import "net/http"

// 两步验证接口（第 14.1 节）：登录的第二步，以及本人启用、关闭 TOTP 与重新生成恢复码。
// 规则在 internal/service/mfa.go。

// sessionTOTP 用 TOTP 验证码或恢复码完成登录的第二步。
func (s *Server) sessionTOTP(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "auth.csrf", "cross-origin login refused")
		return
	}
	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.Svc.VerifyTOTPLogin(r.Context(), req.Ticket, req.Code, s.clientIP(r).String())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.startSession(w, r, u)
}

func (s *Server) totpStatus(w http.ResponseWriter, r *http.Request, p *principal) {
	st, err := s.Svc.TOTPStatus(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// totpSetup 生成待确认的密钥；客户端显示二维码与密钥文本，用户在认证器中添加后提交一个验证码确认。
func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request, p *principal) {
	secret, uri, err := s.Svc.TOTPSetup(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
}

type codesResp struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}

func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	codes, err := s.Svc.TOTPEnable(r.Context(), p.User.ID, req.Code)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, codesResp{RecoveryCodes: codes})
}

type passwordReq struct {
	Password string `json:"password"`
}

func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request, p *principal) {
	var req passwordReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.TOTPDisable(r.Context(), p.User.ID, req.Password); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) totpCodes(w http.ResponseWriter, r *http.Request, p *principal) {
	var req passwordReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	codes, err := s.Svc.RegenerateRecoveryCodes(r.Context(), p.User.ID, req.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, codesResp{RecoveryCodes: codes})
}
