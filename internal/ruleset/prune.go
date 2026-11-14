package ruleset

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 空策略组的处理（反馈 #45）。
//
// 背景（mihomo v1.19.33 adapter/outboundgroup/parser.go、groupbase.go）：组内没有任何节点时，mihomo 使用
// empty-fallback，默认是 COMPATIBLE——一个 Direct 类型的出站。也就是说，空组会让指向它的流量悄悄直连；
// empty-fallback 也不允许填策略组。subconverter 的做法是写入 DIRECT，同样悄悄直连。
//
// 三种模式（方案 output.empty-groups）：
//   - prune（默认）：空组先从所有组的成员中移除（因此变空的组同样处理）；没有被规则直接引用的删除，
//     被规则引用的保留，成员改为 MATCH 规则的目标（这份配置自己的「其余流量」去向），不改写规则；
//   - fallback：同样从成员中移除，但所有空组都保留，成员改为 MATCH 的目标；
//   - keep：保持 mihomo 的默认行为（COMPATIBLE，直连），只报告。
//
// MATCH 目标本身为空、或没有 MATCH 规则时，无法确定兜底去向：保留 COMPATIBLE 并报告，不猜。
const (
	EmptyPrune    = "prune"
	EmptyFallback = "fallback"
	EmptyKeep     = "keep"
)

// PruneEmpty 按 mode 处理 Expand 记录的空组。groups 是 proxy-groups 序列（原地修改），rules 是生成的规则。
// 返回被删除的组名（按原顺序）。
func PruneEmpty(groups *yaml.Node, rules []string, empty map[string]bool, mode string, where map[string]string, out *diag.List) []string {
	if len(empty) == 0 {
		return nil
	}
	if mode == "" {
		mode = EmptyPrune
	}
	referenced := ruleTargets(rules)
	matchTarget := ""
	for _, r := range rules {
		if f := strings.Split(r, ","); len(f) >= 2 && strings.TrimSpace(f[0]) == "MATCH" {
			matchTarget = strings.TrimSpace(f[1])
		}
	}

	byName := map[string]*yaml.Node{}
	var order []string
	for _, g := range groups.Content {
		if n := yamlx.Get(g, "name"); n != nil {
			byName[n.Value] = g
			order = append(order, n.Value)
		}
	}

	// 空组从所有组的成员中移除（它不是有意义的选项；指向 MATCH 目标后若仍留在目标组里会形成环，
	// mihomo 拒绝策略组之间的环）。因此清空且没有 use 的组同样成为空组，反复直到稳定。
	removed := map[string]bool{}
	if mode != EmptyKeep {
		for changed := true; changed; {
			changed = false
			for _, name := range order {
				if empty[name] {
					continue
				}
				g := byName[name]
				px := yamlx.Get(g, "proxies")
				if px == nil {
					continue
				}
				before := len(px.Content)
				px.Content = slices.DeleteFunc(px.Content, func(n *yaml.Node) bool { return empty[n.Value] })
				if len(px.Content) < before && len(px.Content) == 0 && yamlx.Get(g, "use") == nil {
					empty[name] = true
					changed = true
				}
			}
		}
		if mode == EmptyPrune {
			for _, name := range order {
				if empty[name] && !referenced[name] {
					removed[name] = true
				}
			}
		}
	}

	var gone []string
	kept := groups.Content[:0]
	for _, g := range groups.Content {
		name := yamlx.Get(g, "name").Value
		if removed[name] {
			gone = append(gone, name)
			out.Add(diag.GroupPruned, where[name], "name", name)
			continue
		}
		kept = append(kept, g)
		if !empty[name] {
			continue
		}
		switch {
		case mode == EmptyKeep:
			out.Add(diag.GroupEmpty, where[name], "name", name, "fallback", "COMPATIBLE")
		case matchTarget != "" && matchTarget != name && !empty[matchTarget]:
			yamlx.Set(g, "proxies", yamlx.StrSeq(matchTarget))
			out.Add(diag.GroupEmptyFallback, where[name], "name", name, "target", matchTarget)
		default:
			// 被规则引用、但找不到可用的兜底：保持 COMPATIBLE（mihomo 默认），并明确报告会直连
			px := yamlx.Get(g, "proxies")
			if px == nil || len(px.Content) == 0 {
				yamlx.Set(g, "proxies", yamlx.StrSeq("COMPATIBLE"))
			}
			out.Add(diag.GroupEmpty, where[name], "name", name, "fallback", "COMPATIBLE")
		}
	}
	groups.Content = kept
	return gone
}

// ruleTargets 返回规则中出现的全部字段（除规则类型外）。保守地把任何字段都视作可能的目标：
// 宁可多保留一个组，也不删掉规则真正引用的组。
func ruleTargets(rules []string) map[string]bool {
	out := map[string]bool{}
	for _, r := range rules {
		for i, f := range strings.Split(r, ",") {
			if i > 0 {
				out[strings.Trim(strings.TrimSpace(f), "()")] = true
			}
		}
	}
	return out
}
