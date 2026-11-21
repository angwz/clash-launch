package api

import (
	"strings"
	"testing"
)

// 导出配置（R17）：与订阅链接同源；不含节点时去掉节点并给出诊断；文件名标明裁剪方式；
// 写入审计；别人的配置 404；指定不存在的订阅链接 404。
func TestExportConfig(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	type exp struct {
		YAML        string `json:"yaml"`
		Filename    string `json:"filename"`
		Fatal       string `json:"fatal"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	full := decode[exp](t, c.want(200, "POST", "/api/v1/profiles/"+pid+"/export-config", map[string]any{"nodes": true, "rules": true}))
	if full.Fatal != "" || !strings.Contains(full.YAML, "香港 01") || !strings.Contains(full.YAML, "proxy-groups:") || !strings.HasSuffix(full.Filename, ".yaml") || strings.Contains(full.Filename, ".skeleton") {
		t.Fatalf("完整导出：%+v", full)
	}
	// 与订阅链接下发的内容相同
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]any{"note": "导出"}))
	sub := h.client()
	r, body := sub.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	if r.StatusCode != 200 || string(body) != full.YAML {
		t.Errorf("完整导出应与订阅内容相同（%d）", r.StatusCode)
	}

	bare := decode[exp](t, c.want(200, "POST", "/api/v1/profiles/"+pid+"/export-config", map[string]any{"nodes": false, "rules": false}))
	if strings.Contains(bare.YAML, "香港 01") || strings.Contains(bare.YAML, "\nproxies:") || !strings.Contains(bare.YAML, "proxy-groups:") || !strings.HasSuffix(bare.Filename, ".skeleton.yaml") {
		t.Errorf("骨架：%s\n%s", bare.Filename, bare.YAML)
	}
	found := false
	for _, d := range bare.Diagnostics {
		found = found || d.Code == "export.nodes_removed"
	}
	if !found {
		t.Errorf("缺少 export.nodes_removed：%+v", bare.Diagnostics)
	}
	if !strings.Contains(string(c.want(200, "GET", "/api/v1/account/audit", nil)), "profile.export") {
		t.Error("审计缺少 profile.export")
	}
	c.want(404, "POST", "/api/v1/profiles/"+pid+"/export-config", map[string]any{"nodes": true, "rules": true, "link": "missing"})

	c.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(404, "POST", "/api/v1/profiles/"+pid+"/export-config", map[string]any{"nodes": true, "rules": true})
}
