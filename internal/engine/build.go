package engine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/ruleset"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// SourceKind 是节点源的类型（第 5.1 节）。
type SourceKind string

const (
	KindSingle       SourceKind = "single"
	KindBatch        SourceKind = "batch"
	KindSubscription SourceKind = "subscription"
	KindFile         SourceKind = "file"
)

// SourceInput 是一个节点源在渲染时的全部输入（快照与设置），按方案中的顺序排列。
type SourceInput struct {
	Ref        string
	Name       string
	Kind       SourceKind
	Mode       profile.Mode
	Content    []byte    // 快照内容（写入配置模式使用）
	Updated    time.Time // 快照时间
	URL        string    // 订阅地址：只在「交给客户端」模式下写入配置
	HostedURL  string    // 单条、批量、文件在「交给客户端」模式下由本服务托管的地址（/p/<令牌>/<源>）
	Interval   int       // provider 的更新间隔（秒），默认 3600
	Processing node.Processing
}

// RulesInput 是规则源的快照：一对 toml，或一份外部配置 ini。
type RulesInput struct {
	Name     string
	Groups   []byte
	Rulesets []byte
	INI      []byte
	Updated  time.Time
}

// BuildInput 是生成阶段的输入。
type BuildInput struct {
	Sources []SourceInput // 已排除停用的源
	Rules   *RulesInput
	Catalog *catalog.Catalog
	Tools   *node.Tools
}

// Built 是生成阶段的结果。
type Built struct {
	Generated   *yaml.Node        // proxies、proxy-providers、proxy-groups、rule-providers、rules
	Diagnostics []diag.Diagnostic // 跳过的节点、展开策略组的问题……
	Updated     time.Time         // 输入快照中最新的时间戳
	Nodes       []*node.Node      // 写入配置的节点（界面的节点表格使用）
	Numbering   []Numbered        // 订阅编号
}

// Numbered 是一个订阅的编号。
type Numbered struct {
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	GroupID int    `json:"groupId"`
}

// Build 从源与规则源生成配置的分节。纯函数：只读输入的快照。
func Build(in BuildInput) Built {
	var out diag.List
	res := Built{Generated: yamlx.Map()}
	var ops *node.Processing
	var rs *ruleset.Source
	if in.Rules != nil {
		rs = parseRules(in.Rules, &out)
		ops = rs.NodeOps
		res.Updated = in.Rules.Updated
	}

	// 订阅按出现顺序编号 0、1、2……；单条、批量、文件不编号但保留位置（第 5.7 节）。
	gid := 0
	var providers []ruleset.ProviderSource
	providerMap := yamlx.Map()
	usedNames := map[string]bool{}
	var all []*node.Node
	for i, s := range in.Sources {
		if s.Updated.After(res.Updated) {
			res.Updated = s.Updated
		}
		id := -1
		if s.Kind == KindSubscription {
			id = gid
			gid++
			res.Numbering = append(res.Numbering, Numbered{Ref: s.Ref, Name: s.Name, GroupID: id})
		}
		where := s.Ref
		if s.Mode == profile.ModeProvider {
			url := s.URL
			if s.Kind != KindSubscription {
				url = s.HostedURL
			}
			if url != "" {
				name := uniqueName(s.Name, usedNames)
				yamlx.Set(providerMap, name, providerEntry(s, url, name, &out))
				providers = append(providers, ruleset.ProviderSource{Name: name, SourceName: s.Name, GroupID: id})
				continue
			}
			out.Add(diag.ProviderNoEquiv, where, "setting", "mode: provider (no hosted URL in this context; written inline)", "source", where)
		}
		nodes, _ := node.Parse(s.Content, where, in.Catalog, &out)
		p := mergeOps(s.Processing, ops)
		nodes = node.Process(nodes, p, in.Tools, where, &out)
		if ops != nil {
			nodes = node.FilterNames(nodes, ops.Include, ops.Exclude, where+"(rule source)", &out)
		}
		if len(nodes) == 0 {
			out.Add(diag.NodeEmptySource, where, "source", where)
		}
		for _, n := range nodes {
			n.SourceIndex, n.SourceRef, n.SourceName, n.GroupID = i, s.Ref, s.Name, id
		}
		all = append(all, nodes...)
	}
	node.Uniquify(all, &out)
	res.Nodes = all

	if len(all) > 0 {
		seq := yamlx.Seq()
		for _, n := range all {
			seq.Content = append(seq.Content, n.Mapping)
		}
		yamlx.Set(res.Generated, "proxies", seq)
	}
	if len(providerMap.Content) > 0 {
		yamlx.Set(res.Generated, "proxy-providers", providerMap)
	}
	if rs != nil {
		ruleset.CheckReferences(rs.Groups, &out)
		groups := ruleset.Expand(ruleset.ExpandInput{
			Groups: rs.Groups, Nodes: all, Providers: providers,
			GroupField: groupField(in.Catalog), Numbering: numbering(res.Numbering),
			KnownProviders: append([]string{}, yamlx.Keys(providerMap)...),
		}, &out)
		if len(groups.Content) > 0 {
			yamlx.Set(res.Generated, "proxy-groups", groups)
		}
		rp, rules := ruleset.BuildRules(rs.Rulesets, &out)
		if len(rp.Content) > 0 {
			yamlx.Set(res.Generated, "rule-providers", rp)
		}
		if len(rules) > 0 {
			yamlx.Set(res.Generated, "rules", yamlx.StrSeq(rules...))
		}
	}
	res.Diagnostics = out.Items()
	return res
}

func parseRules(r *RulesInput, out *diag.List) *ruleset.Source {
	if len(r.INI) > 0 {
		return ruleset.ParseINI(r.INI, r.Name+".ini", out)
	}
	return &ruleset.Source{
		Origin:   "toml",
		Groups:   ruleset.ParseGroups(r.Groups, "groups.toml", out),
		Rulesets: ruleset.ParseRulesets(r.Rulesets, "rulesets.toml", out),
	}
}

// mergeOps 把规则源（外部配置 ini）中的节点整理设置并入源的设置：重命名与地区规则追加在源的之后。
func mergeOps(p node.Processing, ops *node.Processing) node.Processing {
	if ops == nil {
		return p
	}
	p.Rename = append(slices.Clone(p.Rename), ops.Rename...)
	p.RegionRules = append(slices.Clone(p.RegionRules), ops.RegionRules...)
	if p.Flags == "" {
		p.Flags = ops.Flags
	}
	return p
}

func uniqueName(name string, used map[string]bool) string {
	n := name
	for i := 2; used[n]; i++ {
		n = name + " " + strconv.Itoa(i)
	}
	used[n] = true
	return n
}

// providerEntry 生成「交给客户端」模式的 proxy-provider。只有过滤能等价地改写为 filter/exclude-filter，
// 名称前缀改写为 override.additional-prefix；其余整理设置无法表达，逐项提示（第 5.6 节）。
func providerEntry(s SourceInput, url, name string, out *diag.List) *yaml.Node {
	interval := s.Interval
	if interval <= 0 {
		interval = 3600
	}
	p := yamlx.Map("type", "http", "url", url, "interval", interval, "path", "./proxies/"+fileSafe(name)+".yaml")
	pr := s.Processing
	if pr.Include != "" {
		yamlx.Set(p, "filter", yamlx.Str(pr.Include))
	}
	if pr.Exclude != "" {
		yamlx.Set(p, "exclude-filter", yamlx.Str(pr.Exclude))
	}
	if pr.Prefix != "" {
		yamlx.Set(p, "override", yamlx.Map("additional-prefix", pr.Prefix))
	}
	yamlx.Set(p, "health-check", yamlx.Map("enable", true, "url", "https://www.gstatic.com/generate_204", "interval", 600))
	var lost []string
	if pr.Flags != node.FlagOff {
		lost = append(lost, "flags")
	}
	if len(pr.Rename) > 0 {
		lost = append(lost, "rename")
	}
	if !pr.KeepInfo {
		lost = append(lost, "info-node removal")
	}
	if !pr.KeepDup {
		lost = append(lost, "deduplication")
	}
	if len(pr.RegionRules) > 0 || len(pr.Overrides) > 0 {
		lost = append(lost, "manual regions and overrides")
	}
	if pr.Sort == "name" {
		lost = append(lost, "sort")
	}
	if len(lost) > 0 {
		out.Add(diag.ProviderNoEquiv, s.Ref, "setting", strings.Join(lost, ", "), "source", s.Ref)
	}
	return p
}

// fileSafe 把名称转换为可用作文件名的形式（保留字母、数字、中日韩文字；其余替换为 _）。
func fileSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r >= 0x2E80 && r <= 0x9FFF:
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "provider"
	}
	return b.String()
}

// groupField 用目录判断某键是否是该类型策略组的合法字段。
func groupField(cat *catalog.Catalog) func(string, string) bool {
	if cat == nil {
		return nil
	}
	items := cat.Root.Child("proxy-groups", nil).Elem()
	return func(t, key string) bool {
		return items.Child(key, yamlx.Map("type", t)) != nil
	}
}

func numbering(ns []Numbered) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprintf("%d=%s", n.GroupID, n.Name)
	}
	return strings.Join(parts, ", ")
}
