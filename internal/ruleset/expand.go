package ruleset

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// ProviderSource 是「交给客户端」模式的源：最终配置中是一个 proxy-provider，节点由客户端自己拉取。
type ProviderSource struct {
	Name       string // proxy-provider 名
	SourceName string
	GroupID    int // 订阅编号；非订阅为 -1
}

// ExpandInput 是策略组展开的输入。
type ExpandInput struct {
	Groups    []Group
	Nodes     []*node.Node     // 「写入配置」模式的节点，按源顺序
	Providers []ProviderSource // 「交给客户端」模式的源，按源顺序
	// GroupField 报告 key 是否是该类型策略组的目录字段（用于透传检查）。
	GroupField func(groupType, key string) bool
	// KnownProviders 是已生成的 proxy-provider 名；为 nil 时不检查 use 引用。
	KnownProviders []string
	// Numbering 是订阅编号的说明（"0=机场 A, 1=机场 B"），用于 GROUPID 的提示。
	Numbering string
}

// subconverter 中有、但不能原样用于 mihomo 的键。
var ignoredKeys = map[string]string{
	"timeout":             "subconverter measures it in seconds and does not write it to Clash configs; mihomo's timeout is in milliseconds — set it in raw edit if needed",
	"evaluate-before-use": "Surge-only option",
	"persistent":          "Surge-only option",
}

// Expand 展开策略组（第 6.2、6.4 节）：
//   - 条目顺序即组内顺序；多个条目命中同一节点时只出现一次，位置取首次命中；
//   - "@select" 标出现代选择器结果的位置，没有写时追加在全部条目之后；
//   - 正则未命中任何节点、也没有 use 的组写入 empty-fallback（默认 COMPATIBLE）并警告；
//   - 「交给客户端」的源改写为 use 与 filter，无法等价表达的条件逐条警告。
func Expand(in ExpandInput, out *diag.List) *yaml.Node {
	seq := yamlx.Seq()
	for _, g := range in.Groups {
		seq.Content = append(seq.Content, expandGroup(g, in, out))
	}
	return seq
}

type providerPlan struct {
	use      []string
	filters  []string
	excludes []string
	exTypes  []string
}

func (p *providerPlan) add(ps []ProviderSource) {
	for _, x := range ps {
		if !slices.Contains(p.use, x.Name) {
			p.use = append(p.use, x.Name)
		}
	}
}

func (p *providerPlan) filter(pattern string) {
	if pattern != "" && !slices.Contains(p.filters, pattern) {
		p.filters = append(p.filters, pattern)
	}
}

func expandGroup(g Group, in ExpandInput, out *diag.List) *yaml.Node {
	m := yamlx.Map("name", g.Name, "type", g.Type)
	keys := make([]string, 0, len(g.Extra))
	for k := range g.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if reason, ok := ignoredKeys[k]; ok {
			out.Add(diag.GroupKeyIgnored, g.Where, "name", g.Name, "key", k, "reason", reason)
			continue
		}
		if in.GroupField != nil && !in.GroupField(g.Type, k) {
			out.Add(diag.GroupOffCatalog, g.Where, "name", g.Name, "key", k)
		}
		v, err := yamlx.FromAny(g.Extra[k])
		if err != nil {
			continue
		}
		yamlx.Set(m, k, v)
	}

	var members []string
	seen := map[string]bool{}
	addName := func(n string) {
		if !seen[n] {
			seen[n] = true
			members = append(members, n)
		}
	}
	var plan providerPlan
	selected := false
	usedGroupID := false
	// 现代选择器的结果在「@select」处就地加入；同一节点只出现一次，位置取首次出现处。
	pickSelect := func() {
		if g.Select == nil || selected {
			return
		}
		selected = true
		for _, n := range g.Select.Pick(in.Nodes, g.Where, out) {
			addName(n.Name)
		}
		providerSelect(g, in.Providers, &plan, out)
	}
	for _, rule := range g.Rule {
		mt := parseMatcher(rule)
		switch mt.kind {
		case "ref":
			addName(mt.target)
			continue
		case "select":
			pickSelect()
			continue
		case "script":
			out.Add(diag.GroupMatcher, g.Where, "name", g.Name, "matcher", "script:", "reason", "subconverter scripts need its JavaScript runtime")
			continue
		case "insert":
			out.Add(diag.GroupMatcherNote, g.Where, "name", g.Name, "matcher", rule, "reason", "this project has no insert sources, so !!INSERT= matches nothing; order sources instead")
			continue
		case "groupid":
			usedGroupID = true
		}
		for _, n := range mt.pick(in.Nodes, g.Name, g.Where, out) {
			addName(n.Name)
		}
		providerRule(mt, g, in.Providers, &plan, out)
	}
	if usedGroupID {
		out.Add(diag.GroupIDShift, g.Where, "name", g.Name, "range", groupIDRanges(g.Rule), "numbering", in.Numbering)
	}
	pickSelect() // 没有写「@select」时追加在全部条目之后

	use := append([]string{}, g.Use...)
	if in.KnownProviders != nil {
		for _, u := range g.Use {
			if !slices.Contains(in.KnownProviders, u) {
				out.Add(diag.GroupProviderRef, g.Where, "name", g.Name, "ref", u)
			}
		}
	}
	for _, u := range plan.use {
		if !slices.Contains(use, u) {
			use = append(use, u)
		}
	}
	if len(plan.use) > 0 {
		if len(plan.filters) > 0 {
			yamlx.Set(m, "filter", yamlx.Str(strings.Join(plan.filters, "`")))
		}
		if len(plan.excludes) > 0 {
			yamlx.Set(m, "exclude-filter", yamlx.Str(strings.Join(plan.excludes, "`")))
		}
		if len(plan.exTypes) > 0 {
			yamlx.Set(m, "exclude-type", yamlx.Str(strings.Join(plan.exTypes, "|")))
		}
	}
	if len(members) == 0 && len(use) == 0 {
		fb := "COMPATIBLE"
		if v, ok := g.Extra["empty-fallback"].(string); ok && v != "" {
			fb = v
		}
		out.Add(diag.GroupEmpty, g.Where, "name", g.Name, "fallback", fb)
		members = []string{fb}
	}
	if len(members) > 0 {
		yamlx.Set(m, "proxies", yamlx.StrSeq(members...))
	}
	if len(use) > 0 {
		yamlx.Set(m, "use", yamlx.StrSeq(use...))
	}
	return m
}

// providerRule 把一个条目改写为 provider 模式的 use 与 filter。
func providerRule(mt matcher, g Group, ps []ProviderSource, plan *providerPlan, out *diag.List) {
	if len(ps) == 0 {
		return
	}
	switch mt.kind {
	case "regex":
		plan.add(ps)
		plan.filter(mt.target)
	case "groupid":
		var sel []ProviderSource
		for _, p := range ps {
			if p.GroupID >= 0 && matchRange(mt.target, p.GroupID) {
				sel = append(sel, p)
			}
		}
		plan.add(sel)
		if mt.extra != "" && len(sel) > 0 {
			plan.filter(mt.extra)
			out.Add(diag.GroupProviderNote, g.Where, "name", g.Name, "condition", "the regex after !!GROUPID applies to every provider of this group")
		}
	case "group":
		var sel []ProviderSource
		re, err := node.Compile(mt.target)
		if err == nil {
			for _, p := range ps {
				if ok, _ := re.MatchString(p.SourceName); ok {
					sel = append(sel, p)
				}
			}
		}
		plan.add(sel)
		if mt.extra != "" && len(sel) > 0 {
			plan.filter(mt.extra)
		}
	case "type", "port", "server":
		out.Add(diag.GroupProviderNote, g.Where, "name", g.Name, "condition", mt.raw)
	}
}

// providerSelect 把现代选择器改写为 provider 模式：source → use，name → filter，
// exclude.name → exclude-filter，protocol → exclude-type（取补集）；region、tags、rate 无法表达。
func providerSelect(g Group, ps []ProviderSource, plan *providerPlan, out *diag.List) {
	s := g.Select
	if len(ps) == 0 {
		return
	}
	var sel []ProviderSource
	for _, p := range ps {
		if len(s.Source) == 0 || sourceMatchProvider(s.Source, p) {
			sel = append(sel, p)
		}
	}
	plan.add(sel)
	if s.NameText != "" {
		plan.filter(s.NameText)
	}
	if s.Exclude != nil && s.Exclude.NameText != "" {
		plan.excludes = append(plan.excludes, s.Exclude.NameText)
	}
	if len(s.Protocol) > 0 {
		for _, t := range knownTypes {
			if !slices.Contains(s.Protocol, t) && !slices.Contains(plan.exTypes, t) {
				plan.exTypes = append(plan.exTypes, t)
			}
		}
	}
	conds := []struct {
		name string
		on   bool
	}{{"region", len(s.Region) > 0}, {"tags", len(s.Tags) > 0}, {"rate", s.RateMin != nil || s.RateMax != nil}}
	for _, c := range conds { // 固定顺序：诊断的顺序也必须确定
		if c.on {
			out.Add(diag.GroupProviderNote, g.Where, "name", g.Name, "condition", "select."+c.name)
		}
	}
}

// knownTypes 是 mihomo v1.19.33 adapter.ParseProxy 支持的协议（exclude-type 取补集时使用）。
var knownTypes = []string{"ss", "ssr", "socks5", "http", "vmess", "vless", "snell", "trojan", "hysteria", "hysteria2", "wireguard", "tuic", "shadowquic", "gost-relay", "direct", "dns", "reject", "rematch", "ssh", "mieru", "anytls", "sudoku", "masque", "trusttunnel", "openvpn", "tailscale", "zerotier", "easytier"}

func sourceMatchProvider(want []string, p ProviderSource) bool {
	for _, w := range want {
		if strings.HasPrefix(w, "#") && fmt.Sprint(p.GroupID) == w[1:] && p.GroupID >= 0 {
			return true
		}
		if w == p.SourceName {
			return true
		}
	}
	return false
}

func groupIDRanges(rules []string) string {
	var out []string
	for _, r := range rules {
		if strings.HasPrefix(r, "!!GROUPID=") {
			out = append(out, parseMatcher(r).target)
		}
	}
	return strings.Join(out, " ")
}
