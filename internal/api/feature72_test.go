package api

import (
	"net/http"
	"strings"
	"testing"
)

// 反馈 #72：分流规则地址的来源限制、导出、用户时区。
func TestRuleHostsExportTimezone(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	if r, b := anon.do("POST", "/api/v1/register", registerBody(t, anon, "nora", "")); r.StatusCode != 201 {
		t.Fatalf("注册：%d %s", r.StatusCode, b)
	}
	u := h.client()
	u.login("nora", "a long enough password")

	// 规则地址：普通用户只能用允许的站点；粘贴文本不受限；管理员不受限
	type hosts struct {
		Restricted bool     `json:"restricted"`
		Hosts      []string `json:"hosts"`
	}
	if hs := decode[hosts](t, u.want(200, "GET", "/api/v1/rule-hosts", nil)); !hs.Restricted || len(hs.Hosts) == 0 {
		t.Errorf("普通用户应受限：%+v", hs)
	}
	if hs := decode[hosts](t, admin.want(200, "GET", "/api/v1/rule-hosts", nil)); hs.Restricted {
		t.Errorf("管理员不应受限：%+v", hs)
	}
	if b := u.want(422, "POST", "/api/v1/rule-sources", map[string]any{"name": "自建", "kind": "ini", "iniUrl": "https://example.com/rules.ini"}); !strings.Contains(string(b), "rule.host_not_allowed") {
		t.Errorf("自建站点应被拒绝：%s", b)
	}
	u.want(422, "POST", "/api/v1/rule-sources", map[string]any{"name": "明文", "kind": "ini", "iniUrl": "http://raw.githubusercontent.com/a/b/main/x.ini"})
	u.want(201, "POST", "/api/v1/rule-sources", map[string]any{"name": "公共", "kind": "ini", "iniUrl": "https://raw.githubusercontent.com/a/b/main/x.ini"})
	u.want(201, "POST", "/api/v1/rule-sources", map[string]any{"name": "粘贴", "kind": "ini", "iniText": "[custom]\nruleset=DIRECT,[]FINAL\n"})
	admin.want(201, "POST", "/api/v1/rule-sources", map[string]any{"name": "管理员自建", "kind": "ini", "iniUrl": "https://example.com/rules.ini"})

	// 导出：作为文件下载，记为一次 export 请求
	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "导出"}))
	path := strings.TrimPrefix(l.URL, h.http.URL)
	resp, body := anon.do("GET", path+"?download=1", nil)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;") || !strings.Contains(resp.Header.Get("Content-Disposition"), ".yaml") || len(body) == 0 {
		t.Fatalf("导出：%d %q", resp.StatusCode, resp.Header.Get("Content-Disposition"))
	}
	// 带 If-None-Match 也返回完整内容
	if r2, _ := anon.do("GET", path+"?download=1", nil, "If-None-Match", resp.Header.Get("ETag")); r2.StatusCode != http.StatusOK {
		t.Errorf("导出应总是返回完整内容：%d", r2.StatusCode)
	}
	reqs := decode[list[requestDTO]](t, admin.want(200, "GET", "/api/v1/profiles/"+pid+"/links/"+l.ID+"/requests", nil))
	if len(reqs.Items) != 2 || reqs.Items[0].Kind != "export" {
		t.Errorf("请求记录应记为 export：%+v", reqs.Items)
	}

	// 时区：保存后 /me 返回；配置未指定时区时文件头按它显示；无效名称拒绝
	type tz struct {
		Timezone string `json:"timezone"`
	}
	if got := decode[tz](t, admin.want(200, "PUT", "/api/v1/account/timezone", map[string]string{"timezone": "Asia/Shanghai"})); got.Timezone != "Asia/Shanghai" {
		t.Errorf("时区：%+v", got)
	}
	for _, bad := range []string{"Mars/Olympus", "Local", "asia/shanghai/x"} {
		admin.want(422, "PUT", "/api/v1/account/timezone", map[string]string{"timezone": bad})
	}
	if me := string(admin.want(200, "GET", "/api/v1/me", nil)); !strings.Contains(me, `"timezone":"Asia/Shanghai"`) {
		t.Errorf("/me：%s", me)
	}
	p := decode[profileDTO](t, admin.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	if !strings.Contains(p.Doc, "timezone") {
		pv := decode[previewResp](t, admin.want(200, "POST", "/api/v1/profiles/"+pid+"/preview", map[string]string{"doc": p.Doc}))
		if !strings.Contains(pv.YAML, "(UTC+8)") {
			t.Errorf("文件头时间应按用户时区：\n%s", pv.YAML[:min(300, len(pv.YAML))])
		}
	}
	admin.want(200, "PUT", "/api/v1/account/timezone", map[string]string{"timezone": ""}) // 跟随浏览器
}
