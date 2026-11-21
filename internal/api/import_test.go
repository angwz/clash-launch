package api

import (
	"strings"
	"testing"
)

// 导入（R17）：预览不写存储；按计划经已有接口创建节点源与配置后，订阅输出原样保留节点名、策略组与规则；
// 订阅更新间隔低于配额最小值时被提高并报告；不是配置时 ok=false。
func TestImportPreview(t *testing.T) {
	h := newHarness(t, nil)
	c, _ := h.setup()
	text := "mixed-port: 7890\nsecret: example-secret\nproxies:\n  - {name: 东京 01, type: ss, server: 192.0.2.7, port: 8388, cipher: aes-128-gcm, password: example-pw}\n" +
		"proxy-providers:\n  机场: {type: http, url: https://example.com/sub, interval: 60}\n" +
		"proxy-groups:\n  - {name: 我的选择, type: select, proxies: [东京 01, DIRECT], use: [机场]}\n" +
		"rules:\n  - DOMAIN,example.com,东京 01\n  - MATCH,我的选择\n"
	type src struct {
		Key        string         `json:"key"`
		Kind       string         `json:"kind"`
		Name       string         `json:"name"`
		URL        string         `json:"url"`
		Content    string         `json:"content"`
		Interval   int            `json:"interval"`
		Processing map[string]any `json:"processing"`
		Mode       string         `json:"mode"`
	}
	type plan struct {
		OK          bool   `json:"ok"`
		Doc         string `json:"doc"`
		Sources     []src  `json:"sources"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	before := len(decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, c.want(200, "GET", "/api/v1/sources", nil)).Items)
	pl := decode[plan](t, c.want(200, "POST", "/api/v1/import/preview", map[string]any{"text": text, "name": "导入测试"}))
	if !pl.OK || len(pl.Sources) != 2 || strings.Contains(pl.Doc, "example-secret") {
		t.Fatalf("计划：%+v", pl)
	}
	// 管理员没有更新间隔下限：原样保留
	if pl.Sources[1].Interval != 60 {
		t.Errorf("管理员的更新间隔不应改动：%+v", pl.Sources[1])
	}
	// 预览不写存储
	after := len(decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, c.want(200, "GET", "/api/v1/sources", nil)).Items)
	if after != before {
		t.Errorf("预览不应创建节点源：%d → %d", before, after)
	}

	// 照界面的做法创建
	doc := pl.Doc
	for _, s := range pl.Sources {
		body := map[string]any{"name": s.Name, "kind": s.Kind, "processing": s.Processing}
		if s.Kind == "subscription" {
			body["url"], body["interval"] = s.URL, s.Interval
		} else {
			body["content"] = s.Content
		}
		created := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", body))
		doc = strings.ReplaceAll(doc, "@"+s.Key, created.ID)
	}
	p := decode[profileDTO](t, c.want(201, "POST", "/api/v1/profiles", map[string]any{"doc": doc}))
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+p.ID+"/links", map[string]any{"note": "导入"}))
	r, body := h.client().do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	y := string(body)
	if r.StatusCode != 200 {
		t.Fatalf("订阅：%d %s", r.StatusCode, y)
	}
	for _, want := range []string{"name: 东京 01", "name: 我的选择", "- 东京 01", "机场:", "url: https://example.com/sub", "DOMAIN,example.com,东京 01", "MATCH,我的选择", "mixed-port: 7890"} {
		if !strings.Contains(y, want) {
			t.Errorf("订阅输出缺少 %q：\n%s", want, y)
		}
	}
	if strings.Contains(y, "🇯🇵") || strings.Contains(y, "example-secret") {
		t.Errorf("节点名不应被改写，也不应带入面板密钥：\n%s", y)
	}

	// 普通用户：更新间隔提高到配额最小值并报告
	c.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bp := decode[plan](t, bob.want(200, "POST", "/api/v1/import/preview", map[string]any{"text": text}))
	raised := false
	for _, d := range bp.Diagnostics {
		raised = raised || d.Code == "import.interval_raised"
	}
	if !raised || bp.Sources[1].Interval < 1800 {
		t.Errorf("更新间隔应提高到配额最小值：%+v %+v", bp.Sources[1], bp.Diagnostics)
	}

	bad := decode[plan](t, c.want(200, "POST", "/api/v1/import/preview", map[string]any{"text": "- a\n- b\n"}))
	if bad.OK || len(bad.Diagnostics) == 0 {
		t.Errorf("不是配置时应 ok=false：%+v", bad)
	}
}
