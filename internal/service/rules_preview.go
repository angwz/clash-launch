package service

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 规则源编辑器的校验与策略组预览（O2，规范第 16 节「规则源」）。
//
// 用编辑中的文本（未保存）与一份配置的节点源走一遍真实的生成流程（engine.Build），
// 返回每个策略组最终包含哪些组和节点，以及诊断；诊断带上所在文件与行号，编辑器据此在行内标出。
// 不写存储。没有指定配置时用本人全部节点源（写入配置模式），回答「用我的节点，这份规则会生成什么」。

// RulesPreviewInput 是预览的输入。
type RulesPreviewInput struct {
	Kind         string `json:"kind"` // toml | ini
	GroupsText   string `json:"groupsText"`
	RulesetsText string `json:"rulesetsText"`
	INIText      string `json:"iniText"`
	Profile      string `json:"profile"` // 用这份配置的节点源；为空时用全部节点源
	// Source 是正在编辑的规则源：文本为空的部分用它已保存的内容或地址的拉取快照补上
	Source string `json:"source"`
}

// PreviewGroup 是一个策略组的展开结果。
type PreviewGroup struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Members []string `json:"members"`          // 组与节点，按输出顺序
	Use     []string `json:"use,omitempty"`    // 节点订阅（交给客户端）
	Filter  string   `json:"filter,omitempty"` // 对 use 的过滤
}

// LocatedDiagnostic 是带文件与行号的诊断（行号从 1 开始；0 表示无法定位到行）。
type LocatedDiagnostic struct {
	diag.Diagnostic
	File string `json:"file,omitempty"` // groups | rulesets | ini
	Line int    `json:"line,omitempty"`
}

// RulesPreview 是预览的结果。
type RulesPreview struct {
	Groups      []PreviewGroup      `json:"groups"`
	Rules       int                 `json:"rules"`
	Nodes       int                 `json:"nodes"`
	Diagnostics []LocatedDiagnostic `json:"diagnostics"`
	// Facets 是这批节点上出现过的属性值，供选择器标签编辑器给出候选（规范第 6.4 节）
	Facets Facets `json:"facets"`
}

// Facets 按 [custom_groups.select] 的键汇总节点属性；每项按节点数从多到少、同数按值排序。
type Facets struct {
	Region   []Facet `json:"region"`
	Source   []Facet `json:"source"`
	Protocol []Facet `json:"protocol"`
	Tags     []Facet `json:"tags"`
}

// Facet 是一个属性值与拥有它的节点数。
type Facet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// facets 汇总节点属性。地区代码为空（未识别）的节点不计入地区；标签按节点去重计数。
func facets(nodes []*node.Node) Facets {
	region, source, protocol, tags := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, n := range nodes {
		if n.Region.Code != "" {
			region[n.Region.Code]++
		}
		if n.SourceName != "" {
			source[n.SourceName]++
		}
		if n.Type != "" {
			protocol[n.Type]++
		}
		seen := map[string]bool{}
		for _, t := range n.Tags {
			if !seen[t] {
				seen[t] = true
				tags[t]++
			}
		}
	}
	return Facets{Region: facetList(region), Source: facetList(source), Protocol: facetList(protocol), Tags: facetList(tags)}
}

func facetList(m map[string]int) []Facet {
	out := make([]Facet, 0, len(m))
	for v, n := range m {
		out = append(out, Facet{Value: v, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return out
}

// PreviewRules 预览一份（可能尚未保存的）规则源。
func (s *Service) PreviewRules(ctx context.Context, owner string, in RulesPreviewInput) (*RulesPreview, error) {
	var p *profile.Profile
	if in.Profile != "" {
		x, err := s.Store.Profiles().Get(ctx, owner, in.Profile)
		if err != nil {
			return nil, err
		}
		var ds diag.List
		if p, err = profile.Parse(x.Doc, &ds); err != nil {
			return nil, err
		}
	} else {
		p = &profile.Profile{Name: "preview"}
		srcs, err := listPaged(func(after string) ([]store.Source, error) {
			return s.Store.Sources().List(ctx, owner, store.Page{After: after, Limit: 200})
		}, func(x store.Source) string { return x.ID })
		if err != nil {
			return nil, err
		}
		for _, x := range srcs {
			p.Sources = append(p.Sources, profile.SourceRef{Ref: x.ID, Mode: profile.ModeInline})
		}
	}
	p.Rules, p.Routing = nil, nil // 规则只来自编辑中的文本
	bi, _, _, _, ads, err := s.assemble(ctx, owner, p, time.Time{})
	if err != nil {
		return nil, err
	}
	if in.Source != "" {
		saved, err := s.rulesInput(ctx, owner, in.Source)
		if err != nil {
			return nil, err
		}
		fill := func(dst *string, b []byte) {
			if strings.TrimSpace(*dst) == "" {
				*dst = string(b)
			}
		}
		fill(&in.GroupsText, saved.Groups)
		fill(&in.RulesetsText, saved.Rulesets)
		fill(&in.INIText, saved.INI)
	}
	ri := &engine.RulesInput{Name: "rules"}
	if in.Kind == "ini" {
		ri.INI = []byte(in.INIText)
	} else {
		ri.Groups, ri.Rulesets = []byte(in.GroupsText), []byte(in.RulesetsText)
	}
	bi.Rules = ri
	built := engine.Build(bi)

	res := &RulesPreview{Groups: []PreviewGroup{}, Nodes: len(built.Nodes), Diagnostics: []LocatedDiagnostic{}, Facets: facets(built.Nodes)}
	if gs := yamlx.Get(built.Generated, "proxy-groups"); gs != nil {
		for _, g := range gs.Content {
			pg := PreviewGroup{Name: scalar(g, "name"), Type: scalar(g, "type"), Members: strs(yamlx.Get(g, "proxies")), Filter: scalar(g, "filter")}
			pg.Use = strs(yamlx.Get(g, "use"))
			res.Groups = append(res.Groups, pg)
		}
	}
	if rs := yamlx.Get(built.Generated, "rules"); rs != nil {
		res.Rules = len(rs.Content)
	}
	texts := map[string]string{"groups": in.GroupsText, "rulesets": in.RulesetsText, "ini": in.INIText}
	for _, d := range append(ads.Items(), built.Diagnostics...) {
		res.Diagnostics = append(res.Diagnostics, locate(d, texts))
	}
	return res, nil
}

func scalar(m *yaml.Node, k string) string {
	if v := yamlx.Get(m, k); v != nil {
		return v.Value
	}
	return ""
}

func strs(n *yaml.Node) []string {
	out := []string{}
	if n == nil {
		return out
	}
	for _, x := range n.Content {
		out = append(out, x.Value)
	}
	return out
}

var reTomlLine = regexp.MustCompile(`line (\d+)`)

// locate 把诊断的位置（groups.toml#3、rules.ini:12、或 TOML 语法错误中的 line N）换算为文件与行号。
func locate(d diag.Diagnostic, texts map[string]string) LocatedDiagnostic {
	ld := LocatedDiagnostic{Diagnostic: d}
	path := d.Path
	switch {
	case strings.HasPrefix(path, "groups.toml"):
		ld.File = "groups"
		ld.Line = entryLine(texts["groups"], "custom_groups", path)
	case strings.HasPrefix(path, "rulesets.toml"):
		ld.File = "rulesets"
		ld.Line = entryLine(texts["rulesets"], "rulesets", path)
	case strings.HasPrefix(path, "rules.ini"):
		ld.File = "ini"
		if i := strings.LastIndexByte(path, ':'); i >= 0 {
			ld.Line, _ = strconv.Atoi(path[i+1:])
		}
	}
	// TOML 语法错误：位置只在原因里
	if ld.File != "" && ld.Line == 0 {
		if m := reTomlLine.FindStringSubmatch(d.Args["reason"]); m != nil {
			ld.Line, _ = strconv.Atoi(m[1])
		}
	}
	return ld
}

// entryLine 找到 path 中 #n 所指的第 n 个 [[table]] 所在的行。
func entryLine(text, table, path string) int {
	i := strings.LastIndexByte(path, '#')
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(path[i+1:])
	if err != nil || n < 1 {
		return 0
	}
	header := "[[" + table + "]]"
	seen := 0
	for no, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), header) {
			seen++
			if seen == n {
				return no + 1
			}
		}
	}
	return 0
}
