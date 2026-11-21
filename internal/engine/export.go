package engine

import (
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/ruleset"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 导出（需求 R17）：生成与订阅链接同源的配置，可选去掉节点或规则集，得到可以分享的「骨架」。
// 在合并与适配之后、渲染之前裁剪，因此原始编辑层写入的节点与规则集同样被去掉，
// 输出仍经同一个渲染器与格式自检。去掉与改写的内容逐项进入诊断，不静默删除。

// Export 是导出的选项；零值即完整导出（与订阅链接相同）。
type Export struct {
	NoNodes bool // 去掉 proxies 与 proxy-providers
	NoRules bool // 去掉 rule-providers 与使用它们的规则
}

// stripExport 按选项裁剪合并后的配置。
func stripExport(root *yaml.Node, ex Export, out *diag.List) {
	if ex.NoNodes {
		stripNodes(root, out)
	}
	if ex.NoRules {
		stripRuleSets(root, out)
	}
}

// stripNodes 去掉全部节点与节点 provider。策略组里对节点的引用与 use 一并去掉；
// 去掉后既没有成员、也不会自动收录节点的组写入 DIRECT 作为占位（mihomo 不接受空组）；
// 直接以节点为目标的规则改为 DIRECT。
func stripNodes(root *yaml.Node, out *diag.List) {
	removed := map[string]bool{}
	nProxies, nProviders := 0, 0
	if seq := yamlx.Get(root, "proxies"); seq != nil && seq.Kind == yaml.SequenceNode {
		for _, p := range seq.Content {
			if n := yamlx.Get(p, "name"); n != nil {
				removed[n.Value] = true
			}
		}
		nProxies = len(seq.Content)
	}
	if m := yamlx.Get(root, "proxy-providers"); m != nil && m.Kind == yaml.MappingNode {
		nProviders = len(m.Content) / 2
	}
	yamlx.Delete(root, "proxies")
	yamlx.Delete(root, "proxy-providers")
	if nProxies == 0 && nProviders == 0 {
		return
	}
	out.Add(diag.ExportNodesRemoved, "proxies", "proxies", strconv.Itoa(nProxies), "providers", strconv.Itoa(nProviders))

	if groups := yamlx.Get(root, "proxy-groups"); groups != nil && groups.Kind == yaml.SequenceNode {
		for i, g := range groups.Content {
			name := ""
			if n := yamlx.Get(g, "name"); n != nil {
				name = n.Value
			}
			yamlx.Delete(g, "use")
			if ps := yamlx.Get(g, "proxies"); ps != nil && ps.Kind == yaml.SequenceNode {
				kept := ps.Content[:0]
				for _, p := range ps.Content {
					if !removed[p.Value] {
						kept = append(kept, p)
					}
				}
				ps.Content = kept
			}
			if !groupHasMembers(g) {
				yamlx.Set(g, "proxies", yamlx.StrSeq("DIRECT"))
				out.Add(diag.ExportGroupPlaceholder, yamlx.Index("proxy-groups", i), "name", name)
			}
		}
	}
	if rules := yamlx.Get(root, "rules"); rules != nil && rules.Kind == yaml.SequenceNode {
		for i, r := range rules.Content {
			target, at, parts := ruleset.Target(r.Value)
			if at < 0 || !removed[target] {
				continue
			}
			parts[at] = "DIRECT"
			was := r.Value
			r.Value = strings.Join(parts, ",")
			out.Add(diag.ExportRuleTarget, yamlx.Index("rules", i), "rule", was, "target", target)
		}
	}
}

// groupHasMembers 报告组是否还有成员，或会自动收录节点（include-all 系列）。
func groupHasMembers(g *yaml.Node) bool {
	if ps := yamlx.Get(g, "proxies"); ps != nil && len(ps.Content) > 0 {
		return true
	}
	for _, k := range []string{"include-all", "include-all-proxies", "include-all-providers"} {
		if v := yamlx.Get(g, k); v != nil && v.Value == "true" {
			return true
		}
	}
	return false
}

// stripRuleSets 去掉 rule-providers 与引用它们的规则（含条件中引用规则集的逻辑规则），
// 其余规则（手写规则、GEOIP、兜底 MATCH）原样保留。
func stripRuleSets(root *yaml.Node, out *diag.List) {
	var names []string
	if m := yamlx.Get(root, "rule-providers"); m != nil && m.Kind == yaml.MappingNode {
		names = yamlx.Keys(m)
	}
	yamlx.Delete(root, "rule-providers")
	n := 0
	if rules := yamlx.Get(root, "rules"); rules != nil && rules.Kind == yaml.SequenceNode {
		kept := rules.Content[:0]
		for _, r := range rules.Content {
			if usesRuleSet(r.Value) {
				n++
				continue
			}
			kept = append(kept, r)
		}
		rules.Content = kept
	}
	if len(names) > 0 || n > 0 {
		out.Add(diag.ExportRulesRemoved, "rules", "providers", strconv.Itoa(len(names)), "rules", strconv.Itoa(n))
	}
}

// usesRuleSet 报告规则本身或其条件（逻辑规则的括号内）是否为 RULE-SET。
func usesRuleSet(rule string) bool {
	for _, f := range strings.FieldsFunc(rule, func(r rune) bool { return r == ',' || r == '(' || r == ')' }) {
		if strings.EqualFold(strings.TrimSpace(f), "RULE-SET") {
			return true
		}
	}
	return false
}
