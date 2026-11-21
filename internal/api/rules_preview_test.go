package api

import (
	"strings"
	"testing"
)

// 规则源预览（O2）：用编辑中的文本与本人的节点展开策略组；错误带文件与行号；可以指定配置的节点源；不写存储。
func TestRuleSourcePreview(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	type res struct {
		Groups []struct {
			Name    string   `json:"name"`
			Members []string `json:"members"`
		} `json:"groups"`
		Rules       int `json:"rules"`
		Nodes       int `json:"nodes"`
		Diagnostics []struct {
			Code string `json:"code"`
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"diagnostics"`
	}
	groups := groupsToml + "[[custom_groups]]\nname = \"香港\"\ntype = \"select\"\nrule = [\"港\"]\n"
	r := decode[res](t, c.want(200, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "toml", "groupsText": groups, "rulesetsText": rulesetsToml}))
	if len(r.Groups) != 2 || r.Rules != 2 || r.Nodes != 2 {
		t.Fatalf("预览：%+v", r)
	}
	if got := strings.Join(r.Groups[0].Members, ","); !strings.Contains(got, "DIRECT") || !strings.Contains(got, "香港 01") || !strings.Contains(got, "日本 02") {
		t.Errorf("PROXY 的成员：%s", got)
	}
	if got := strings.Join(r.Groups[1].Members, ","); !strings.Contains(got, "香港 01") || strings.Contains(got, "日本") {
		t.Errorf("香港 的成员：%s", got)
	}
	// 指定配置：用它的节点源
	byProfile := decode[res](t, c.want(200, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "toml", "groupsText": groups, "rulesetsText": rulesetsToml, "profile": pid}))
	if byProfile.Nodes != 2 {
		t.Errorf("按配置预览：%+v", byProfile)
	}
	c.want(404, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "toml", "groupsText": groups, "profile": "missing"})
	// 文本为空：用已保存的规则源补上
	rss := decode[struct {
		Items []ruleSourceDTO `json:"items"`
	}](t, c.want(200, "GET", "/api/v1/rule-sources", nil)).Items
	saved := decode[res](t, c.want(200, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "toml", "source": rss[0].ID}))
	if len(saved.Groups) != 1 || saved.Rules != 2 {
		t.Errorf("用已保存的内容预览：%+v", saved)
	}

	// 第 2 个组缺少 name：定位到第二个 [[custom_groups]] 所在的行
	bad := groupsToml + "\n[[custom_groups]]\ntype = \"select\"\nrule = [\".*\"]\n"
	r = decode[res](t, c.want(200, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "toml", "groupsText": bad, "rulesetsText": rulesetsToml}))
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "group.invalid" && d.File == "groups" && d.Line == 6 {
			found = true
		}
	}
	if !found {
		t.Errorf("应定位到 groups 第 6 行：%+v", r.Diagnostics)
	}
	// TOML 语法错误：行号来自解析器
	r = decode[res](t, c.want(200, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "toml", "groupsText": "[[custom_groups]]\nname = \n", "rulesetsText": rulesetsToml}))
	found = false
	for _, d := range r.Diagnostics {
		if d.Code == "rules.syntax" && d.File == "groups" && d.Line == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("语法错误应定位到第 2 行：%+v", r.Diagnostics)
	}
	// ini：行号来自 rules.ini:N
	r = decode[res](t, c.want(200, "POST", "/api/v1/rule-sources/preview", map[string]any{"kind": "ini", "iniText": "[custom]\nnot a key value line\ncustom_proxy_group=全部`select`.*\nruleset=全部,[]FINAL\n"}))
	found = false
	for _, d := range r.Diagnostics {
		if d.File == "ini" && d.Line == 2 {
			found = true
		}
	}
	if !found || len(r.Groups) != 1 {
		t.Errorf("ini：%+v", r)
	}
}
