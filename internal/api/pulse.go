package api

import (
	"net/http"

	"github.com/angwz/clash-launch/internal/version"
)

// pulseDTO 是在线指示灯（R11）的数据。实时通道连不上（代理不支持 WebSocket 升级、浏览器不支持）时，
// 浏览器每 15 秒轮询 GET /api/v1/pulse 取同样的数据；连得上时这些数据随 hello、pong 推送。
type pulseDTO struct {
	Version string `json:"version"`
	Online  int    `json:"online,omitempty"` // 只发给有权看的人
	Unread  int    `json:"unread"`
	Kernel  bool   `json:"kernel"` // 服务端的 mihomo 内核检查是否可用；不可用时指示灯降级
}

func (s *Server) pulse(w http.ResponseWriter, r *http.Request, p *principal) {
	n, err := s.Svc.Store.Messages().Unread(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pulseDTO{Version: version.Version, Online: s.Svc.OnlineFor(r.Context(), p.User), Unread: n, Kernel: s.Svc.Kernel != nil})
}

type pulseSettings struct {
	ShowOnline bool `json:"showOnline"`
}

// getPulseSettings 返回是否向普通用户显示在线人数（管理员）。
func (s *Server) getPulseSettings(w http.ResponseWriter, r *http.Request, _ *principal) {
	show, err := s.Svc.ShowOnlineToUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pulseSettings{ShowOnline: show})
}

// putPulseSettings 设定是否向普通用户显示在线人数（管理员）。
func (s *Server) putPulseSettings(w http.ResponseWriter, r *http.Request, p *principal) {
	var req pulseSettings
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.SetShowOnlineToUsers(r.Context(), p.User.ID, req.ShowOnline); err != nil {
		s.fail(w, err)
		return
	}
	s.getPulseSettings(w, r, p)
}
