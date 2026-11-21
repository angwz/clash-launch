package api

import (
	"bufio"
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/angwz/clash-launch/internal/metrics"
)

// 运行指标（需求 R26）：/metrics 以 Prometheus 文本格式输出请求数与耗时（按路由模式）、配置生成耗时、
// 数据库语句耗时、拉取结果、后台任务与实时连接数。指标不含任何用户数据，但会暴露路由的使用情况，所以：
//   - 设置了 CLASH_LAUNCH_METRICS_TOKEN 时，凭 Authorization: Bearer <令牌> 访问；
//   - 否则只接受直接来自本机或内网、且不带转发头的请求（经反向代理转来的公网请求不算，即使代理在本机）；
//   - 都不满足时返回 404，不暴露端点存在。

// recordMetrics 记录每个请求的路由模式、状态码与耗时。路由模式在 ServeMux 匹配后写回同一个 *Request。
func (s *Server) recordMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &metricsWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.Pattern == "GET /metrics" {
			return // 抓取本身不计入
		}
		metrics.Default.Request(r.Pattern, sw.status, time.Since(start))
	})
}

// metricsWriter 记下状态码；Unwrap 与 Hijack 让 http.ResponseController 与 WebSocket 照常工作。
type metricsWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *metricsWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status, w.wrote = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *metricsWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func (w *metricsWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *metricsWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.status = http.StatusSwitchingProtocols
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *metricsWriter) Flush() { _ = http.NewResponseController(w.ResponseWriter).Flush() }

func (s *Server) serveMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.metricsAllowed(r) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = metrics.Default.WriteText(w)
}

func (s *Server) metricsAllowed(r *http.Request) bool {
	if tok := s.Cfg.MetricsToken; tok != "" {
		got := r.Header.Get("Authorization")
		return subtle.ConstantTimeCompare([]byte(got), []byte("Bearer "+tok)) == 1
	}
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Real-IP") != "" {
		return false
	}
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	a := ap.Addr().Unmap()
	return a.IsLoopback() || a.IsPrivate()
}

// adminJobs 返回后台任务的运行情况（需求 R36）：每个任务最近一次的开始时间、耗时、失败原因与累计次数，
// 以及进程启动以来的拉取次数。数据在内存中，重启后从零开始。
func (s *Server) adminJobs(w http.ResponseWriter, _ *http.Request, _ *principal) {
	writeJSON(w, http.StatusOK, map[string]any{"jobs": metrics.Default.Jobs(), "fetches": metrics.Default.FetchCounts(), "since": metrics.Default.Started()})
}
