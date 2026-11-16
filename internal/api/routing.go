package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/angwz/clash-launch/internal/diag"
)

// 分流地图、追踪与体检（docs/design/routing.md 第 7 节）。doc 是编辑器当前的方案文档，与 preview 相同的约定；
// 方案必须属于当前用户（按所有者隔离）。只读，不保存任何东西。

type routingReq struct {
	Doc     string `json:"doc"`
	Draft   string `json:"draft"` // 草稿的 routing 段文本（第 3 步）
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
	t, after, err := s.Svc.RoutingTrace(r.Context(), p.User, []byte(req.Doc), req.Draft, req.Host, req.Process, req.Network)
	if err != nil {
		s.fail(w, err)
		return
	}
	res := map[string]any{"current": t}
	if after != nil {
		res["draft"] = after
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) routingHealth(w http.ResponseWriter, r *http.Request, p *principal) {
	req, ok := s.routingBegin(w, r, p)
	if !ok || !s.traceAllowed(w, p) {
		return
	}
	items, err := s.Svc.RoutingHealth(r.Context(), p.User, []byte(req.Doc), req.Draft)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// routingFork 返回开始自己管理时的 routing 段文本（不保存，前端放进草稿，应用时写入方案文档）。
func (s *Server) routingFork(w http.ResponseWriter, r *http.Request, p *principal) {
	req, ok := s.routingBegin(w, r, p)
	if !ok || !s.renderAllowed(w, p, false) {
		return
	}
	text, ds, err := s.Svc.RoutingFork(r.Context(), p.User, []byte(req.Doc))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"routing": text, "diagnostics": diag.Sorted(ds)})
}
