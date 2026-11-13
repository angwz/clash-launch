package engine

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/ruleset"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 链式代理（反馈 #24）。依据 mihomo v1.19.33 源码：
//   - relay 组已移除（adapter/outboundgroup/parser.go），dialer-proxy 是唯一的链式写法；
//   - dialer-proxy 写在节点上，值为节点名或策略组名；写在组上无效（parser.go 只记录错误）；
//   - proxy-provider 的 dialer-proxy 套用到其中每个节点，override.dialer-proxy 在其后生效
//     （adapter/provider/provider.go）；
//   - 内核只检查目标存在与「节点 → 节点」的直接循环（config/utils.go validateDialerProxies），
//     经策略组绕回自身的循环要到运行时才出现，这里一并检查。

// ChainResult 是一条链式代理规则的实际效果，供界面预览。
type ChainResult struct {
	Via       string   `json:"via"`
	Nodes     []string `json:"nodes"`     // 写入配置、已设置 dialer-proxy 的节点
	Providers []string `json:"providers"` // 整体设置了 dialer-proxy 的 provider
}

// applyChains 按顺序套用链式代理规则：后面的规则覆盖前面的。gen 中已有 proxies、proxy-providers、proxy-groups。
// 返回每条规则的效果，以及可作前置的名字（节点与策略组，按输出顺序）。
func applyChains(chains []profile.Chain, gen *yaml.Node, nodes []*node.Node, providers []ruleset.ProviderSource, out *diag.List) ([]ChainResult, []string) {
	g := newChainGraph(gen, nodes)
	outbounds := append(append([]string{}, g.nodeOrder...), g.groupOrder...)
	var results []ChainResult
	for i, c := range chains {
		where := yamlx.Index("chains", i)
		idx := fmt.Sprint(i + 1)
		res := ChainResult{Via: c.Via, Nodes: []string{}, Providers: []string{}}
		if c.Disabled {
			results = append(results, res)
			continue
		}
		if strings.TrimSpace(c.Via) == "" {
			out.Add(diag.ChainInvalid, where, "index", idx, "reason", "via is required")
			results = append(results, res)
			continue
		}
		if !g.isNode[c.Via] && g.members[c.Via] == nil {
			out.Add(diag.ChainVia, where+".via", "index", idx, "via", c.Via)
		}
		var sel *ruleset.Selector
		if len(c.Select) > 0 {
			if sel = ruleset.ParseSelector(c.Select, "chains["+fmt.Sprint(i)+"]", where+".select", out); sel == nil {
				results = append(results, res)
				continue
			}
		}

		picked := nodes
		if sel != nil {
			picked = sel.Pick(nodes, where, out)
		}
		for _, n := range picked {
			if n.Name == c.Via {
				out.Add(diag.ChainSelf, where, "index", idx, "name", n.Name)
				continue
			}
			if cycle, viaGroup := g.reach(c.Via, n.Name); cycle != nil {
				path := strings.Join(append([]string{n.Name}, cycle...), " → ")
				if !viaGroup {
					out.Add(diag.ChainCycle, where, "index", idx, "name", n.Name, "via", c.Via, "cycle", path)
					continue
				}
				out.Add(diag.ChainLoopRisk, where, "index", idx, "name", n.Name, "via", c.Via, "cycle", path)
			}
			if old := yamlx.Get(n.Mapping, "dialer-proxy"); old != nil && old.Value != c.Via {
				out.Add(diag.ChainOverride, where, "index", idx, "name", n.Name, "old", old.Value, "via", c.Via)
			}
			yamlx.Set(n.Mapping, "dialer-proxy", yamlx.Str(c.Via))
			g.dialer[n.Name] = c.Via
			res.Nodes = append(res.Nodes, n.Name)
		}

		pm := yamlx.Get(gen, "proxy-providers")
		for _, p := range providers {
			match, exact := true, true
			if sel != nil {
				match, exact = sel.MatchesProvider(p.SourceName, p.GroupID)
			}
			if !match {
				continue
			}
			if !exact {
				out.Add(diag.ChainProvider, where, "index", idx, "provider", p.Name, "condition", sel.String())
				continue
			}
			entry := yamlx.Get(pm, p.Name)
			if entry == nil {
				continue
			}
			if old := yamlx.Get(entry, "dialer-proxy"); old != nil && old.Value != c.Via {
				out.Add(diag.ChainOverride, where, "index", idx, "name", p.Name, "old", old.Value, "via", c.Via)
			}
			yamlx.Set(entry, "dialer-proxy", yamlx.Str(c.Via))
			res.Providers = append(res.Providers, p.Name)
		}
		if len(res.Nodes) == 0 && len(res.Providers) == 0 {
			out.Add(diag.ChainEmpty, where, "index", idx, "via", c.Via)
		}
		results = append(results, res)
	}
	return results, outbounds
}

// chainGraph 是拨号依赖图：节点 → 它的 dialer-proxy；策略组 → 它的静态成员。
type chainGraph struct {
	isNode     map[string]bool
	dialer     map[string]string
	members    map[string][]string
	nodeOrder  []string
	groupOrder []string
}

func newChainGraph(gen *yaml.Node, nodes []*node.Node) *chainGraph {
	g := &chainGraph{isNode: map[string]bool{}, dialer: map[string]string{}, members: map[string][]string{}}
	for _, n := range nodes {
		g.isNode[n.Name] = true
		g.nodeOrder = append(g.nodeOrder, n.Name)
		if d := yamlx.Get(n.Mapping, "dialer-proxy"); d != nil && d.Value != "" {
			g.dialer[n.Name] = d.Value
		}
	}
	if groups := yamlx.Get(gen, "proxy-groups"); groups != nil {
		for _, grp := range groups.Content {
			name := yamlx.Get(grp, "name")
			if name == nil {
				continue
			}
			ms := []string{}
			if ps := yamlx.Get(grp, "proxies"); ps != nil {
				for _, m := range ps.Content {
					ms = append(ms, m.Value)
				}
			}
			g.members[name.Value] = ms
			g.groupOrder = append(g.groupOrder, name.Value)
		}
	}
	return g
}

// reach 从 from 出发沿拨号依赖寻找 target：返回路径（from … target），以及路径是否经过策略组。
// 只经过节点的路径是必然的循环（内核拒绝加载）；经过策略组的路径只在组选中相应成员时循环。
// 深度优先，按成员顺序遍历，结果确定。
func (g *chainGraph) reach(from, target string) ([]string, bool) {
	seen := map[string]bool{}
	var path []string
	var viaGroup bool
	var dfs func(cur string, group bool) bool
	dfs = func(cur string, group bool) bool {
		if seen[cur] {
			return false
		}
		seen[cur] = true
		path = append(path, cur)
		if cur == target {
			viaGroup = group
			return true
		}
		if d, ok := g.dialer[cur]; ok && dfs(d, group) {
			return true
		}
		for _, m := range g.members[cur] {
			if dfs(m, true) {
				return true
			}
		}
		path = path[:len(path)-1]
		return false
	}
	if dfs(from, false) {
		return path, viaGroup
	}
	return nil, false
}
