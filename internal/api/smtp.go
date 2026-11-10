package api

import (
	"net/http"

	"github.com/angwz/clash-launch/internal/service"
)

// 个人 SMTP 提醒（第 14.1 节）：读取、保存（未验证）、删除、发送测试邮件（成功后启用）。
// 规则在 internal/service/smtp.go，发送在 internal/source/smtp.go。

func (s *Server) getSMTP(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := s.Svc.GetSMTP(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": v, "available": s.Svc.Mailer != nil})
}

func (s *Server) putSMTP(w http.ResponseWriter, r *http.Request, p *principal) {
	var in service.SMTPInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.PutSMTP(r.Context(), p.User.ID, in); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteSMTP(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteSMTP(r.Context(), p.User.ID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testSMTP(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.TestSMTP(r.Context(), p.User.ID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
