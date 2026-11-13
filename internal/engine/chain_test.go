package engine

import (
	"slices"
	"testing"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/yamlx"
)

const chainAirport = `proxies:
  - {name: node-a1, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pw}
  - {name: node-a2, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pw}
`
const chainVPS = `proxies:
  - {name: node-vps, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example-pw}
`

// 两个源：机场（订阅）与node-vps（手动）；可选第三个「交给客户端」的订阅。
func chainBuild(t *testing.T, groups string, provider bool, chains ...profile.Chain) (Built, *diag.List) {
	t.Helper()
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := node.LoadTools(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	in := BuildInput{Catalog: cat, Tools: tools, Chains: chains, Sources: []SourceInput{
		{Ref: "a", Name: "机场", Kind: KindSubscription, Mode: profile.ModeInline, Content: []byte(chainAirport), Processing: node.Processing{Flags: node.FlagOff}},
		{Ref: "v", Name: "VPS", Kind: KindBatch, Mode: profile.ModeInline, Content: []byte(chainVPS), Processing: node.Processing{Flags: node.FlagOff}},
	}}
	if provider {
		in.Sources = append(in.Sources, SourceInput{Ref: "b", Name: "机场 B", Kind: KindSubscription, Mode: profile.ModeProvider, URL: "https://example.com/sub"})
	}
	if groups != "" {
		in.Rules = &RulesInput{Name: "r", Groups: []byte(groups), Rulesets: []byte("[[rulesets]]\ngroup = \"DIRECT\"\nruleset = \"[]MATCH\"\n")}
	}
	b := Build(in)
	var l diag.List
	l.Append(b.Diagnostics...)
	return b, &l
}

func dialerOf(t *testing.T, b Built, name string) string {
	t.Helper()
	for _, n := range b.Nodes {
		if n.Name == name {
			if d := yamlx.Get(n.Mapping, "dialer-proxy"); d != nil {
				return d.Value
			}
			return ""
		}
	}
	t.Fatalf("没有节点 %s", name)
	return ""
}

func TestChainBySource(t *testing.T) {
	b, l := chainBuild(t, "", false, profile.Chain{Via: "node-vps", Select: map[string]any{"source": []any{"机场"}}})
	if dialerOf(t, b, "node-a1") != "node-vps" || dialerOf(t, b, "node-a2") != "node-vps" || dialerOf(t, b, "node-vps") != "" {
		t.Fatalf("应只给机场的节点设置前置：%v", l.Items())
	}
	if len(b.Chains) != 1 || !slices.Equal(b.Chains[0].Nodes, []string{"node-a1", "node-a2"}) {
		t.Errorf("预览结果：%+v", b.Chains)
	}
	if !slices.Contains(b.Outbounds, "node-vps") {
		t.Errorf("前置候选应包含节点：%v", b.Outbounds)
	}
}

func TestChainSelfSkippedAndLaterOverrides(t *testing.T) {
	b, l := chainBuild(t, "", false,
		profile.Chain{Via: "node-vps"},
		profile.Chain{Via: "node-a2", Select: map[string]any{"name": "a1"}},
	)
	if !l.Has(diag.ChainSelf) || !l.Has(diag.ChainOverride) {
		t.Errorf("应提示跳过自身与覆盖：%v", l.Items())
	}
	if dialerOf(t, b, "node-a1") != "node-a2" {
		t.Error("后面的规则应覆盖前面的")
	}
}

func TestChainDirectCycleRejected(t *testing.T) {
	// 香港 → VPS，再让 VPS → 香港：必然循环，第二条不套用
	b, l := chainBuild(t, "", false,
		profile.Chain{Via: "node-vps", Select: map[string]any{"name": "a1"}},
		profile.Chain{Via: "node-a1", Select: map[string]any{"name": "vps"}},
	)
	if !l.Has(diag.ChainCycle) || dialerOf(t, b, "node-vps") != "" {
		t.Fatalf("应拒绝直接循环：%v", l.Items())
	}
}

func TestChainLoopThroughGroup(t *testing.T) {
	groups := "[[custom_groups]]\nname = \"全部\"\ntype = \"select\"\nrule = [\".*\"]\n"
	b, l := chainBuild(t, groups, false, profile.Chain{Via: "全部", Select: map[string]any{"name": "a1"}})
	if !l.Has(diag.ChainLoopRisk) {
		t.Errorf("经策略组绕回自身应给出警告：%v", l.Items())
	}
	if dialerOf(t, b, "node-a1") != "全部" {
		t.Error("循环风险只警告，仍然套用")
	}
	if !slices.Contains(b.Outbounds, "全部") {
		t.Errorf("前置候选应包含策略组：%v", b.Outbounds)
	}
}

func TestChainProviders(t *testing.T) {
	b, l := chainBuild(t, "", true, profile.Chain{Via: "node-vps", Select: map[string]any{"source": []any{"机场 B"}}})
	p := yamlx.Get(yamlx.Get(b.Generated, "proxy-providers"), "机场 B")
	if p == nil || yamlx.Get(p, "dialer-proxy") == nil || yamlx.Get(p, "dialer-proxy").Value != "node-vps" {
		t.Fatalf("provider 应整体设置 dialer-proxy：%v", l.Items())
	}
	b, l = chainBuild(t, "", true, profile.Chain{Via: "node-vps", Select: map[string]any{"region": []any{"HK"}}})
	if !l.Has(diag.ChainProvider) || yamlx.Get(yamlx.Get(yamlx.Get(b.Generated, "proxy-providers"), "机场 B"), "dialer-proxy") != nil {
		t.Errorf("按地区选择时 provider 不应套用并应警告：%v", l.Items())
	}
}

func TestChainInvalidInputs(t *testing.T) {
	_, l := chainBuild(t, "", false, profile.Chain{Via: ""}, profile.Chain{Via: "不存在", Select: map[string]any{"name": "a1"}}, profile.Chain{Via: "node-vps", Select: map[string]any{"bogus": 1}})
	for _, c := range []diag.Code{diag.ChainInvalid, diag.ChainVia, diag.SelectInvalid} {
		if !l.Has(c) {
			t.Errorf("缺少诊断 %s：%v", c, l.Items())
		}
	}
	_, l = chainBuild(t, "", false, profile.Chain{Via: "node-vps", Select: map[string]any{"name": "zz"}})
	if !l.Has(diag.ChainEmpty) {
		t.Error("没有选中节点时应警告")
	}
}
