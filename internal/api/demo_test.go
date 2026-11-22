package api

import (
	"context"
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/service"
)

// 访客试用（R44）：未登录可以读取示例配置、节点与规则，预览与分流图可以计算；只接受同源请求；
// 没有写入接口；示例环境与正式数据隔离；示例配置生成时没有错误。
func TestDemo(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup() // 正式数据里有管理员与「我的 VPS」
	d, err := service.NewDemo(context.Background(), h.svc, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	anon := h.client()
	anon.want(404, "GET", "/api/v1/demo", nil) // 未开启
	h.srv.EnableDemo(d)

	pid := decode[map[string]string](t, anon.want(200, "GET", "/api/v1/demo", nil))["profileId"]
	p := decode[profileDTO](t, anon.want(200, "GET", "/api/v1/demo/profiles/"+pid, nil))
	if !strings.Contains(p.Doc, "示例配置") {
		t.Fatalf("示例配置：%+v", p)
	}
	srcs := string(anon.want(200, "GET", "/api/v1/demo/sources", nil))
	if !strings.Contains(srcs, "示例节点") || strings.Contains(srcs, "我的 VPS") {
		t.Errorf("示例节点源应与正式数据隔离：%s", srcs)
	}
	anon.want(200, "GET", "/api/v1/demo/rule-sources", nil)
	anon.want(404, "GET", "/api/v1/demo/profiles/not-the-demo", nil)

	pv := decode[previewResp](t, anon.want(200, "POST", "/api/v1/demo/profiles/"+pid+"/preview", map[string]string{"doc": p.Doc}))
	if !strings.Contains(pv.YAML, "🚀 节点选择") || !strings.Contains(pv.YAML, "192.0.2.11") || pv.Fatal != "" {
		t.Fatalf("预览：%s %s", pv.Fatal, pv.YAML)
	}
	for _, x := range pv.Diagnostics {
		if x.Severity == "error" {
			t.Errorf("示例配置不应有错误：%+v", x)
		}
	}
	if b := string(anon.want(200, "POST", "/api/v1/demo/profiles/"+pid+"/routing/graph", map[string]string{"doc": p.Doc})); !strings.Contains(b, "🎬 流媒体") {
		t.Errorf("分流图：%.300s", b)
	}
	anon.want(200, "POST", "/api/v1/demo/profiles/"+pid+"/routing/trace", map[string]string{"doc": p.Doc, "host": "www.example.com"})

	// 跨站请求被拒；没有写入接口
	anon.want(403, "POST", "/api/v1/demo/profiles/"+pid+"/preview", map[string]string{"doc": p.Doc}, "Origin", "https://evil.example.com")
	if r, _ := anon.do("PUT", "/api/v1/demo/profiles/"+pid, map[string]string{"doc": p.Doc}); r.StatusCode < 400 {
		t.Errorf("试用不应能保存：%d", r.StatusCode)
	}
	// 示例用户不能登录
	anon.want(401, "POST", "/api/v1/session", map[string]string{"username": "demo", "password": "anything long enough"})
	_ = admin
}
