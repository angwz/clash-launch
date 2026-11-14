package api

import (
	"net/http"

	"github.com/angwz/clash-launch/internal/profile"
)

// getNodeMode 返回当前用户的节点写法策略：默认写法，以及能否在配置中自选（反馈 #46）。
func (s *Server) getNodeMode(w http.ResponseWriter, r *http.Request, p *principal) {
	pol, err := s.Svc.NodeModeFor(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

type nodeModeReq struct {
	Default profile.Mode `json:"default"`
}

// putNodeMode 设定全局默认写法（管理员）。
func (s *Server) putNodeMode(w http.ResponseWriter, r *http.Request, p *principal) {
	var req nodeModeReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.SetNodeModeDefault(r.Context(), p.User.ID, req.Default); err != nil {
		s.fail(w, err)
		return
	}
	s.getNodeMode(w, r, p)
}
