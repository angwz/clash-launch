package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/angwz/clash-launch/internal/service"
)

// 访客试用（需求 R44）：/api/v1/demo/... 不需要登录，由示例环境（service.Demo）以示例用户的身份处理。
// 只开放编辑器与分流可视化需要的读取与预览；没有任何写入接口，示例数据不会被访客改动。
// 防滥用：预览、分流计算等 POST 只接受本站页面发起的请求（同源）；全部按 IP 限流，预览与追踪的限流也按 IP 计（示例用户是大家共用的）。

// demoPerMinute 是每个 IP 每分钟可以调用试用接口的次数（编辑时的预览、分流追踪都计入）。
const demoPerMinute = 120

// EnableDemo 开启访客试用；d 为 nil 时 /api/v1/demo/ 返回 404。在开始服务前调用。
func (s *Server) EnableDemo(d *service.Demo) {
	if d == nil {
		return
	}
	s.demo = d
	s.demoSrv = New(d.Svc, s.Cfg, s.Log)
	s.demoLimit = newLimiter(demoPerMinute, time.Minute)
}

// handler 是需要身份的处理函数（与 authed 包装的签名相同）。
type handler = func(http.ResponseWriter, *http.Request, *principal)

// demoRoute 把请求交给示例环境的处理函数 pick(s.demoSrv)，身份是示例用户。
func (s *Server) demoRoute(pick func(*Server) handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.demo == nil {
			problem(w, http.StatusNotFound, "not_found", "the demo is not available")
			return
		}
		// 来源校验只针对 POST（预览、分流计算）：浏览器对同源的 GET 不带 Origin，读取也不需要它
		if r.Method != http.MethodGet && !s.sameOrigin(r) {
			problem(w, http.StatusForbidden, "auth.csrf", "cross-origin request refused")
			return
		}
		if ok, wait := s.demoLimit.allow(s.clientIP(r).String(), time.Now()); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(wait))
			problem(w, http.StatusTooManyRequests, "rate_limited", "too many demo requests; try again shortly")
			return
		}
		u := *s.demo.User // 副本：处理函数不应改动它，即使改了也不影响下一个请求
		pick(s.demoSrv)(w, r, &principal{User: &u, limitKey: "demo:" + s.clientIP(r).String()})
	}
}

// demoInfo 返回示例配置的 ID（试用页据此打开编辑器）。
func (s *Server) demoInfo(w http.ResponseWriter, _ *http.Request) {
	if s.demo == nil {
		problem(w, http.StatusNotFound, "not_found", "the demo is not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"profileId": s.demo.ProfileID})
}
