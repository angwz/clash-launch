package api

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/service"
)

var updateFixtures = flag.Bool("update", false, "重写 web/src/lib/routing/fixtures/（差异必须人工审阅）")

// TestRoutingFixtures 生成前端布局与智能视图测试用的图数据夹具（routing 第 13 节第 2 步），并校验与当前代码一致。
// 夹具来自真实的渲染路径；除 tiny 外在关闭追踪时生成（不联网，因此没有条数与样例）。
// 诊断与随机 ID 不进入夹具。
func TestRoutingFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "web", "src", "lib", "routing", "fixtures")
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	vps := "- {name: 香港 01, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pw}\n- {name: 日本 02, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pw}\n- {name: 美国 03, type: ss, server: 192.0.2.3, port: 8388, cipher: aes-128-gcm, password: example-pw}\n"
	type src struct{ name, content, mode string }
	cases := []struct {
		name    string
		sources []src
		rules   map[string]any
		raw     string
		trace   bool
	}{
		{name: "tiny", sources: []src{{"我的 VPS", vps, ""}}, trace: true,
			rules: map[string]any{"kind": "toml", "groupsText": groupsToml, "rulesetsText": rulesetsToml},
			raw:   "rule-providers:\n  Ads: {type: inline, behavior: domain, payload: ['+.ads.example.com', 'track.example.net']}\n  Lan: {type: inline, behavior: ipcidr, payload: ['192.0.2.0/24']}\nrules: !prepend\n  - RULE-SET,Ads,REJECT\n  - RULE-SET,Lan,DIRECT,no-resolve\n"},
		{name: "domainrouter", sources: []src{{"我的 VPS", vps, ""}},
			rules: map[string]any{"kind": "toml", "groupsText": read("testdata/rules/domainrouter/groups.toml"), "rulesetsText": read("testdata/rules/domainrouter/rulesets.toml")}},
		{name: "acl4ssr", sources: []src{{"机场 A", read("testdata/workspace/snapshots/airport-a.txt"), ""}, {"我的 VPS", vps, ""}, {"机场 B", read("testdata/workspace/snapshots/airport-b.yaml"), "provider"}},
			rules: map[string]any{"kind": "ini", "iniText": read("testdata/rules/sample.ini")}},
		{name: "provider", sources: []src{{"机场 B", read("testdata/workspace/snapshots/airport-b.yaml"), "provider"}},
			rules: map[string]any{"kind": "toml", "groupsText": groupsToml, "rulesetsText": rulesetsToml}},
		{name: "raw-override", sources: []src{{"我的 VPS", vps, ""}},
			rules: map[string]any{"kind": "toml", "groupsText": groupsToml, "rulesetsText": rulesetsToml},
			raw:   "rules:\n  - DOMAIN-SUFFIX,example.com,PROXY\n  - MATCH,DIRECT\n"},
		{name: "deep8", sources: []src{{"我的 VPS", vps, ""}},
			rules: map[string]any{"kind": "toml", "groupsText": deepGroups(8), "rulesetsText": "[[rulesets]]\ngroup = \"L1\"\nruleset = \"[]DOMAIN-SUFFIX,example.com\"\n[[rulesets]]\ngroup = \"L4\"\nruleset = \"[]DOMAIN-SUFFIX,example.net\"\n[[rulesets]]\ngroup = \"DIRECT\"\nruleset = \"[]MATCH\"\n"}},
	}
	h := newHarness(t, nil)
	c, _ := h.setup()
	uuid := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	for _, tc := range cases {
		h.svc.Trace.Disabled = !tc.trace
		doc := "clash-launch/profile: v1\nname: " + tc.name + "\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n"
		for _, s := range tc.sources {
			sd := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{"name": s.name, "kind": "batch", "content": s.content}))
			doc += "  - ref: " + sd.ID + "\n"
			if s.mode != "" {
				doc += "    mode: " + s.mode + "\n"
			}
		}
		rs := decode[ruleSourceDTO](t, c.want(201, "POST", "/api/v1/rule-sources", mergeMap(map[string]any{"name": tc.name}, tc.rules)))
		doc += "rules: {ref: " + rs.ID + "}\noutput: {empty-groups: keep}\n"
		if tc.raw != "" {
			doc += "raw: |\n  " + strings.ReplaceAll(strings.TrimSuffix(tc.raw, "\n"), "\n", "\n  ") + "\n"
		}
		p := decode[profileDTO](t, c.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
		g := decode[service.GraphDTO](t, c.want(200, "POST", "/api/v1/profiles/"+p.ID+"/routing/graph", map[string]string{"doc": doc}))
		g.Diagnostics = []diag.Diagnostic{}
		g.Pending = 0
		b, _ := json.MarshalIndent(g, "", "  ")
		b = append(b, '\n')
		if uuid.Match(b) || bytes.Contains(b, []byte(h.http.URL)) {
			t.Fatalf("%s：夹具中有随机 ID 或测试服务器地址", tc.name)
		}
		path := filepath.Join(dir, tc.name+".json")
		if *updateFixtures {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, b, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), b) {
			t.Errorf("%s 与当前代码不一致（运行 go test ./internal/api/ -run TestRoutingFixtures -update 并审阅差异）", tc.name)
		}
	}
}

// deepGroups 生成 n 层的默认路径：L1 → L2 → … → Ln → 节点，另有一条跨层的候选。
func deepGroups(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("[[custom_groups]]\nname = \"L" + itoa(i) + "\"\ntype = \"select\"\n")
		if i < n {
			b.WriteString("rule = [\"[]L" + itoa(i+1) + "\", \"[]DIRECT\"]\n\n")
		} else {
			b.WriteString("rule = [\".*\"]\n\n")
		}
	}
	return b.String()
}

func mergeMap(a, b map[string]any) map[string]any {
	for k, v := range b {
		a[k] = v
	}
	return a
}
