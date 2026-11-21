package api

import (
	"net/http"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/source"
)

// 提醒渠道（需求 R31，决定 0008）：列出、新建、修改（回到未验证）、删除、发送测试消息（成功后启用）。
// 规则在 internal/service/channels.go，发送在 internal/source/push.go。

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request, p *principal) {
	cs, err := s.Svc.ListChannels(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": cs, "available": s.Svc.Pusher != nil, "kinds": source.PushKinds, "max": service.MaxChannels})
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	var in service.ChannelInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	v, err := s.Svc.CreateChannel(r.Context(), p.User.ID, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	var in service.ChannelInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	v, err := s.Svc.UpdateChannel(r.Context(), p.User.ID, r.PathValue("id"), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteChannel(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testChannel(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := s.Svc.TestChannel(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
