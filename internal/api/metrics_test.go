package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/metrics"
)

// 运行指标与后台任务面板（R26、R36）：
//   - /metrics 直接来自本机且不带转发头时可读，带转发头（经反向代理）时 404；设置令牌后只凭令牌；
//   - 请求按路由模式统计，抓取本身不计入；
//   - 后台任务的运行记录只对管理员可见。
func TestMetricsAndJobs(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	c.want(200, "GET", "/api/v1/profiles/"+pid, nil)

	anon := h.client()
	r, b := anon.do("GET", "/metrics", nil)
	out := string(b)
	if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("本机读取：%d %s", r.StatusCode, out)
	}
	for _, want := range []string{`clash_launch_http_requests_total{route="GET /api/v1/profiles/{id}",code="2xx"}`, "clash_launch_render_duration_seconds_count", "go_goroutines"} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %s", want)
		}
	}
	if strings.Contains(out, `route="GET /metrics"`) {
		t.Error("抓取本身不应计入")
	}
	if r, _ := anon.do("GET", "/metrics", nil, "X-Forwarded-For", "203.0.113.9"); r.StatusCode != 404 {
		t.Errorf("经反向代理的请求应 404：%d", r.StatusCode)
	}
	h.srv.Cfg.MetricsToken = "example-metrics-token"
	if r, _ := anon.do("GET", "/metrics", nil); r.StatusCode != 404 {
		t.Errorf("设置令牌后，本机请求也要令牌：%d", r.StatusCode)
	}
	if r, _ := anon.do("GET", "/metrics", nil, "Authorization", "Bearer example-metrics-token", "X-Forwarded-For", "203.0.113.9"); r.StatusCode != 200 {
		t.Errorf("凭令牌可以经反向代理读取：%d", r.StatusCode)
	}
	if r, _ := anon.do("GET", "/metrics", nil, "Authorization", "Bearer wrong"); r.StatusCode != 404 {
		t.Errorf("令牌错误：%d", r.StatusCode)
	}

	// 跑一轮后台任务
	ctx, cancel := context.WithCancel(context.Background())
	go h.svc.Run(ctx, time.Hour)
	deadline := time.Now().Add(10 * time.Second)
	for len(metrics.Default.Jobs()) < 5 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	jobs := string(c.want(200, "GET", "/api/v1/admin/jobs", nil))
	for _, want := range []string{`"name":"fetch"`, `"name":"prune_audit"`, `"name":"purge_backups"`, `"lastMs"`, `"fetches"`} {
		if !strings.Contains(jobs, want) {
			t.Errorf("任务面板缺少 %s：%s", want, jobs)
		}
	}
	c.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(404, "GET", "/api/v1/admin/jobs", nil)
}
