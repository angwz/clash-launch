package api

import (
	"net/http"
	"strconv"
	"time"
)

// 分流地图、追踪与体检（docs/design/routing.md 第 7 节）。doc 是编辑器当前的方案文档，与 preview 相同的约定；
// 方案必须属于当前用户（按所有者隔离）。只读，不保存任何东西。

type routingReq struct {
	Doc     string `json:"doc"`
	Host    string `json:"host"`
	Process string `json:"process"`
	Network string `json:"network"`
}

// routingBegin 读取请求并确认方案归当前用户所有。
func (s *Server) routingBegin(w http.ResponseWriter, r *http.Request, p *principal) (*routingReq, bool) {
	var req routingReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return nil, false
	}
	if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return nil, false
	}
	return &req, true
}

// traceAllowed 检查追踪配额（管理员不限）；超出时写出 429。
func (s *Server) traceAllowed(w http.ResponseWriter, p *principal) bool {
	if p.User.Role == "admin" {
		return true
	}
	ok, wait := s.traceLimit.allow(p.User.ID, time.Now())
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		problem(w, http.StatusTooManyRequests, "rate_limited.trace", "too many traces; try again shortly")
	}
	return ok
}

func (s *Server) routingGraph(w http.ResponseWriter, r *http.Request, p *principal) {
	req, ok := s.routingBegin(w, r, p)
	if !ok || !s.renderAllowed(w, p, false) {
		return
	}
	g, err := s.Svc.RoutingGraph(r.Context(), p.User, []byte(req.Doc))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) routingTrace(w http.ResponseWriter, r *http.Request, p *principal) {
	req, ok := s.routingBegin(w, r, p)
	if !ok || !s.traceAllowed(w, p) {
		return
	}
	t, err := s.Svc.RoutingTrace(r.Context(), p.User, []byte(req.Doc), req.Host, req.Process, req.Network)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": t})
}

func (s *Server) routingHealth(w http.ResponseWriter, r *http.Request, p *principal) {
	req, ok := s.routingBegin(w, r, p)
	if !ok || !s.traceAllowed(w, p) {
		return
	}
	items, err := s.Svc.RoutingHealth(r.Context(), p.User, []byte(req.Doc))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
