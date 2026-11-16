package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/match"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/render"
	"github.com/angwz/clash-launch/internal/ruleset"
	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 分流地图、追踪与体检（docs/design/routing.md 第 6、7 节）。图按**最终配置**画（表单层与原始编辑合并之后），
// 生成阶段的结果只用来判断每一行、每个组的来源。只读：本文件不修改方案。

var (
	// ErrTraceDisabled 表示部署者用 CLASH_LAUNCH_TRACE=off 关闭了追踪与体检。
	ErrTraceDisabled = errors.New("service: 分流追踪已关闭")
	// ErrInvalidHost 表示追踪的目标不是有效的域名或 IP。
	ErrInvalidHost = errors.New("service: 追踪目标无效")
)

// builtins 是 mihomo 的内置出口（adapter/outbound 与 tunnel 中的保留名）。
var builtins = []string{"DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"}

// —— 响应结构（与 api/openapi.yaml 的 Routing* 一致）——

type GraphDTO struct {
	State           string            `json:"state"` // follow：跟随上游；own：自己管理（第 3 步）
	Editable        bool              `json:"editable"`
	ReadonlyReason  string            `json:"readonlyReason,omitempty"` // raw_override | no_rules
	Rules           []RuleDTO         `json:"rules"`
	Groups          []GroupDTO        `json:"groups"`
	Exits           []ExitDTO         `json:"exits"`
	Pending         int               `json:"pending"` // 正在后台拉取的规则集个数：大于 0 时前端稍后再取
	TraceEnabled    bool              `json:"traceEnabled"`
	Checks          []CheckDTO        `json:"checks"`                    // 期望的结果（自己管理时）
	UpstreamChanges int               `json:"upstreamChanges,omitempty"` // 自己管理后规则源的变化数
	Diagnostics     []diag.Diagnostic `json:"diagnostics"`
}

type RuleDTO struct {
	Index  int    `json:"index"`
	Raw    string `json:"raw"`
	Type   string `json:"type"`
	Target string `json:"target"`
	Origin string `json:"origin"` // routing（自己管理）| rules（跟随上游）| raw（原始编辑）
	// Entry 是这一行对应 routing.rulesets（跟随上游时为开始管理后会得到的）中的第几条；原始编辑的行没有
	Entry *int        `json:"entry,omitempty"`
	Set   *RuleSetDTO `json:"set,omitempty"`
	Flags []string    `json:"flags"`
}

type RuleSetDTO struct {
	Name        string               `json:"name"`
	Behavior    string               `json:"behavior"`
	Format      string               `json:"format"`
	URL         string               `json:"url,omitempty"`
	Count       *int                 `json:"count,omitempty"` // 内容已缓存时才有
	Samples     []SampleDTO          `json:"samples,omitempty"`
	Description *ruleset.Description `json:"description,omitempty"` // 规则集说明目录中收录的说明
	Stale       bool                 `json:"stale,omitempty"`
	Bundle      string               `json:"bundle,omitempty"` // 所属合集（决定 0007 第 9 条）
}

type SampleDTO struct {
	Host string `json:"host"`
	Hit  int    `json:"hit"` // 服务器追踪该样例得到的命中行；-1 表示没有命中
}

type GroupDTO struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Members []MemberDTO `json:"members"`
	Use     []string    `json:"use,omitempty"`
	Filter  string      `json:"filter,omitempty"`
	Params  GroupParams `json:"params"`
	Origin  string      `json:"origin"`
	Lints   []string    `json:"lints"`
}

type MemberDTO struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // group | node | builtin | provider
	// Entry 是这个成员来自组的 rule 列表中的第几条（单独写出的 []名称）；来自正则或选择器时省略
	Entry *int `json:"entry,omitempty"`
}

type GroupParams struct {
	URL       string `json:"url,omitempty"`
	Interval  *int   `json:"interval,omitempty"`
	Tolerance *int   `json:"tolerance,omitempty"`
	Lazy      *bool  `json:"lazy,omitempty"`
	Strategy  string `json:"strategy,omitempty"`
}

type ExitDTO struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"` // node | builtin | provider
	Region     string            `json:"region,omitempty"`
	RegionName map[string]string `json:"regionName,omitempty"` // 地区的中英文名（catalog/regions），出口按地区归并时显示
	Source     string            `json:"source,omitempty"`
	Protocol   string            `json:"protocol,omitempty"`
	Via        string            `json:"via,omitempty"`
	Filter     string            `json:"filter,omitempty"`
}

type TraceDTO struct {
	Host    string      `json:"host"`
	IP      string      `json:"ip,omitempty"`
	Steps   []StepDTO   `json:"steps"`
	Hit     int         `json:"hit"`
	Chain   []ChainStep `json:"chain"`
	Exit    TraceExit   `json:"exit"`
	NeedsIP *NeedsIPDTO `json:"needsIp,omitempty"`
	Stale   bool        `json:"stale,omitempty"`
}

type StepDTO struct {
	Index    int    `json:"index"`
	Result   string `json:"result"`
	Evidence string `json:"evidence,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type ChainStep struct {
	Group      string   `json:"group"`
	Type       string   `json:"type"`
	Pick       string   `json:"pick"`
	Candidates []string `json:"candidates"`
}

type TraceExit struct {
	Kind   string `json:"kind"` // node | builtin | provider | unknown
	Name   string `json:"name"`
	Region string `json:"region,omitempty"`
	Via    string `json:"via,omitempty"`
}

type NeedsIPDTO struct {
	At         int   `json:"at"`
	Candidates []int `json:"candidates"`
}

type HealthItem struct {
	Host      string    `json:"host"`
	Category  string    `json:"category"`
	Hit       int       `json:"hit"`
	Rule      string    `json:"rule"`
	Exit      TraceExit `json:"exit"`
	Uncertain bool      `json:"uncertain"` // 命中之前有无法判断的行（取决于应用的除外）
}

// —— 最终配置 ——

type finalGroup struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"`
	Proxies   []string `yaml:"proxies"`
	Use       []string `yaml:"use"`
	Filter    string   `yaml:"filter"`
	URL       string   `yaml:"url"`
	Interval  *int     `yaml:"interval"`
	Tolerance *int     `yaml:"tolerance"`
	Lazy      *bool    `yaml:"lazy"`
	Strategy  string   `yaml:"strategy"`
}

type finalConfig struct {
	Rules          []string                  `yaml:"rules"`
	ProxyGroups    []finalGroup              `yaml:"proxy-groups"`
	Proxies        []map[string]any          `yaml:"proxies"`
	ProxyProviders map[string]map[string]any `yaml:"proxy-providers"`
	RuleProviders  map[string]map[string]any `yaml:"rule-providers"`
	GeodataMode    bool                      `yaml:"geodata-mode"`
}

// routingInput 是一次地图或追踪需要的全部数据。
type routingInput struct {
	res     *Result
	prof    *profile.Profile
	cfg     finalConfig
	specs   map[string]ruleSpec // rule-provider 名 → 描述
	lines   []match.Line
	env     match.Env
	hosts   source.Hosts
	groups  map[string]*finalGroup
	proxies map[string]bool
	nodes   map[string]*node.Node
	via     map[string]string
}

// routingInput 渲染方案并解析最终配置。渲染有致命错误时返回的诊断说明原因，cfg 为空。
func (s *Service) routingInput(ctx context.Context, user *store.User, doc []byte) (*routingInput, error) {
	res, err := s.Render(ctx, user.ID, doc, time.Time{}, false)
	if err != nil {
		return nil, err
	}
	in := &routingInput{res: res, specs: map[string]ruleSpec{}, groups: map[string]*finalGroup{}, proxies: map[string]bool{}, nodes: map[string]*node.Node{}, via: map[string]string{}, hosts: s.RuleHostsFor(user)}
	var ds diag.List
	in.prof, _ = profile.Parse(doc, &ds)
	if res.Fatal != "" || len(res.YAML) == 0 {
		return in, nil
	}
	if err := yaml.Unmarshal(res.YAML, &in.cfg); err != nil {
		return nil, fmt.Errorf("解析最终配置: %w", err)
	}
	for i := range in.cfg.ProxyGroups {
		g := &in.cfg.ProxyGroups[i]
		in.groups[g.Name] = g
	}
	for _, p := range in.cfg.Proxies {
		if name, ok := p["name"].(string); ok {
			in.proxies[name] = true
		}
	}
	for _, n := range res.Nodes {
		in.nodes[n.Name] = n
	}
	for _, c := range res.Chains {
		for _, n := range c.Nodes {
			in.via[n] = c.Via
		}
	}
	for name, p := range in.cfg.RuleProviders {
		in.specs[name] = specOf(name, p)
	}
	for i, raw := range in.cfg.Rules {
		in.lines = append(in.lines, match.ParseLine(i, raw))
	}
	in.env = match.Env{Geo: s.Trace.Geo, GeodataMode: in.cfg.GeodataMode}
	return in, nil
}

func specOf(name string, p map[string]any) ruleSpec {
	str := func(k string) string { v, _ := p[k].(string); return v }
	spec := ruleSpec{Name: name, Type: str("type"), Behavior: str("behavior"), Format: str("format"), URL: str("url")}
	if spec.Format == "" {
		spec.Format = "yaml" // 内核默认（constant/provider.ParseRuleFormat）
	}
	if n, ok := p["interval"].(int); ok && n > 0 {
		spec.Interval = time.Duration(n) * time.Second
	}
	if list, ok := p["payload"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				spec.Payload = append(spec.Payload, s)
			}
		}
	}
	return spec
}

// usedSpecs 按 rules 中的顺序返回被引用的规则集。
func (in *routingInput) usedSpecs() []ruleSpec {
	var out []ruleSpec
	seen := map[string]bool{}
	for _, l := range in.lines {
		if l.Type != "RULE-SET" || seen[l.Payload] {
			continue
		}
		seen[l.Payload] = true
		if spec, ok := in.specs[l.Payload]; ok {
			out = append(out, spec)
		}
	}
	return out
}

func (in *routingInput) kindOf(name string) string {
	switch {
	case in.groups[name] != nil:
		return "group"
	case slices.Contains(builtins, name):
		return "builtin"
	}
	return "node"
}

// —— 地图 ——

// RoutingGraph 返回分流地图的数据。规则内容只取已缓存的；未缓存的在后台拉取，Pending 告诉前端稍后再取。
func (s *Service) RoutingGraph(ctx context.Context, user *store.User, doc []byte) (*GraphDTO, error) {
	in, err := s.routingInput(ctx, user, doc)
	if err != nil {
		return nil, err
	}
	g := &GraphDTO{State: "follow", Rules: []RuleDTO{}, Groups: []GroupDTO{}, Exits: []ExitDTO{}, Checks: []CheckDTO{}, TraceEnabled: !s.Trace.Disabled, Diagnostics: append([]diag.Diagnostic{}, diag.Sorted(in.res.Diagnostics)...)}
	if in.res.Fatal != "" {
		g.ReadonlyReason = "no_rules"
		return g, nil
	}
	switch {
	case in.prof == nil || (in.prof.Rules == nil && in.prof.Routing == nil):
		g.ReadonlyReason = "no_rules"
	case rawOverrides(in.prof.Raw):
		g.ReadonlyReason = "raw_override"
	}
	g.Editable = g.ReadonlyReason == ""

	genRules, genGroups := generatedParts(in.res.Generated)
	// 生成的行与组的来源：自己管理时为 routing，跟随上游时为 rules
	srcOrigin := "rules"
	if in.prof != nil && in.prof.Routing != nil {
		g.State, srcOrigin = "own", "routing"
	}
	// routing 模型（跟随上游时为开始管理后会得到的）：用来标出每一行、每个成员对应 routing 中的第几条
	model := s.routingModel(ctx, user.ID, in.prof)
	var mapEntries bool
	if model != nil {
		var gen finalConfig
		if in.res.Generated != nil && in.res.Generated.Decode(&gen) == nil {
			mapEntries = len(gen.Rules) == len(model.Rulesets) && !model.NoRules
		}
	}
	entry := 0
	descs := s.descriptions()

	// 规则
	sets := map[string]*match.Set{}
	stale := map[string]bool{}
	if !s.Trace.Disabled {
		for name, spec := range in.specs {
			if set, st := s.cachedSet(spec, in.env); set != nil {
				sets[name], stale[name] = set, st
			}
		}
		g.Pending = s.prefetch(in.usedSpecs(), in.env, in.hosts)
	}
	for _, l := range in.lines {
		r := RuleDTO{Index: l.Index, Raw: l.Raw, Type: l.Type, Target: l.Target, Origin: "raw", Flags: ruleFlags(l)}
		if genRules[l.Raw] > 0 {
			genRules[l.Raw]--
			r.Origin = srcOrigin
			if mapEntries {
				k := entry
				r.Entry = &k
			}
			entry++
		}
		if l.Type == "RULE-SET" {
			if spec, ok := in.specs[l.Payload]; ok {
				rs := &RuleSetDTO{Name: spec.Name, Behavior: spec.Behavior, Format: spec.Format, URL: spec.URL}
				if spec.Type == "inline" {
					rs.Format = "inline"
				}
				if descs != nil && spec.URL != "" {
					rs.Description = descs.Describe(spec.URL)
				}
				if set := sets[spec.Name]; set != nil {
					n := set.Count
					rs.Count, rs.Stale = &n, stale[spec.Name]
					for _, h := range set.Samples(8) {
						req, err := match.NewRequest(h, "", "")
						if err != nil {
							continue
						}
						out := match.Trace(in.lines, sets, req, in.env)
						rs.Samples = append(rs.Samples, SampleDTO{Host: h, Hit: out.Hit})
					}
				}
				r.Set = rs
			}
		}
		if r.Entry != nil && r.Set != nil {
			r.Set.Bundle = model.Rulesets[*r.Entry].Bundle
		}
		g.Rules = append(g.Rules, r)
	}

	// 期望与上游变化（自己管理时，第 4 步）：期望只用已缓存的规则内容判断，不为此等待拉取
	var cl diag.List
	g.Checks = s.routingChecks(in, sets, stale, &cl)
	if g.Checks == nil {
		g.Checks = []CheckDTO{}
	}
	if g.State == "own" && in.prof.Rules != nil {
		if up, err := s.RoutingUpstream(ctx, user, doc); err == nil && len(up.Changes) > 0 {
			g.UpstreamChanges = len(up.Changes)
			cl.Add(diag.RoutingUpstreamChanged, "routing.from", "n", strconv.Itoa(len(up.Changes)))
		}
	}
	g.Diagnostics = append(g.Diagnostics, cl.Items()...)

	// 策略组
	referenced := map[string]bool{}
	for _, l := range in.lines {
		referenced[l.Target] = true
	}
	for _, fg := range in.cfg.ProxyGroups {
		for _, m := range fg.Proxies {
			referenced[m] = true
		}
	}
	for _, fg := range in.cfg.ProxyGroups {
		gd := GroupDTO{Name: fg.Name, Type: fg.Type, Use: fg.Use, Filter: fg.Filter, Origin: "raw", Lints: []string{},
			Params: GroupParams{URL: fg.URL, Interval: fg.Interval, Tolerance: fg.Tolerance, Lazy: fg.Lazy, Strategy: fg.Strategy}}
		if gen, ok := genGroups[fg.Name]; ok && sameGroup(gen, fg) {
			gd.Origin = srcOrigin
		}
		for _, m := range fg.Proxies {
			md := MemberDTO{Name: m, Kind: in.kindOf(m)}
			if mg := modelGroup(model, fg.Name); mg != nil {
				if i := slices.Index(mg.Rule, "[]"+m); i >= 0 {
					md.Entry = &i
				}
			}
			gd.Members = append(gd.Members, md)
		}
		for _, p := range fg.Use {
			gd.Members = append(gd.Members, MemberDTO{Name: p, Kind: "provider"})
		}
		if gd.Members == nil {
			gd.Members = []MemberDTO{}
		}
		gd.Lints = groupLints(fg, referenced[fg.Name])
		g.Groups = append(g.Groups, gd)
	}

	// 出口：节点（最终配置中的顺序）、用到的内置出口、provider
	used := map[string]bool{}
	for _, l := range in.lines {
		used[l.Target] = true
	}
	for _, fg := range in.cfg.ProxyGroups {
		for _, m := range fg.Proxies {
			used[m] = true
		}
	}
	for _, p := range in.cfg.Proxies {
		name, _ := p["name"].(string)
		e := ExitDTO{Name: name, Kind: "node", Via: in.via[name]}
		e.Protocol, _ = p["type"].(string)
		if dp, ok := p["dialer-proxy"].(string); ok && e.Via == "" {
			e.Via = dp
		}
		if n := in.nodes[name]; n != nil {
			e.Region, e.Source = n.Region.Code, n.SourceName
			if s.Tools != nil && s.Tools.Regions != nil && e.Region != "" {
				if zh, en := s.Tools.Regions.Name(e.Region); zh != "" || en != "" {
					e.RegionName = map[string]string{"zh": zh, "en": en}
				}
			}
		}
		g.Exits = append(g.Exits, e)
	}
	for _, b := range builtins {
		if used[b] {
			g.Exits = append(g.Exits, ExitDTO{Name: b, Kind: "builtin"})
		}
	}
	names := make([]string, 0, len(in.cfg.ProxyProviders))
	for name := range in.cfg.ProxyProviders {
		names = append(names, name)
	}
	slices.Sort(names) // 映射的键没有顺序：按名称排序保证结果确定
	for _, name := range names {
		f, _ := in.cfg.ProxyProviders[name]["filter"].(string)
		g.Exits = append(g.Exits, ExitDTO{Name: name, Kind: "provider", Filter: f})
	}
	return g, nil
}

// rawOverrides 报告原始编辑是否整段替换了 rules、proxy-groups 或 rule-providers（决定 0007 #17：这时地图只读）。
// 合并语义见 docs/design/merge.md：列表默认整体替换，!replace、!delete 同样是整段；
// !append、!prepend、!patch-by 与映射的递归合并只是局部改动，对应的行与组标「原始编辑」即可。
func rawOverrides(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	var root yaml.Node
	if yaml.Unmarshal([]byte(raw), &root) != nil || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return false
	}
	m := root.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if !slices.Contains([]string{"rules", "proxy-groups", "rule-providers"}, m.Content[i].Value) {
			continue
		}
		v := m.Content[i+1]
		switch {
		case v.Tag == "!replace" || v.Tag == "!delete":
			return true
		case v.Tag == "!append" || v.Tag == "!prepend" || strings.HasPrefix(v.Tag, "!patch-by"):
		case v.Kind == yaml.SequenceNode:
			return true
		}
	}
	return false
}

// generatedParts 返回生成阶段的规则（多重集合）与策略组（按名称）。
func generatedParts(gen *yaml.Node) (map[string]int, map[string]finalGroup) {
	rules, groups := map[string]int{}, map[string]finalGroup{}
	if gen == nil {
		return rules, groups
	}
	var cfg finalConfig
	if err := gen.Decode(&cfg); err != nil {
		return rules, groups
	}
	for _, r := range cfg.Rules {
		rules[r]++
	}
	for _, g := range cfg.ProxyGroups {
		groups[g.Name] = g
	}
	return rules, groups
}

func sameGroup(a, b finalGroup) bool {
	ab, _ := yaml.Marshal(a)
	bb, _ := yaml.Marshal(b)
	return string(ab) == string(bb)
}

func ruleFlags(l match.Line) []string {
	flags := []string{}
	switch {
	case l.Type == "AND" || l.Type == "OR" || l.Type == "NOT":
		flags = append(flags, "logic")
	case l.Type == "SUB-RULE":
		flags = append(flags, "sub-rule")
	case strings.HasPrefix(l.Type, "PROCESS-"):
		flags = append(flags, "process")
	}
	if slices.Contains(l.Params, "no-resolve") {
		flags = append(flags, "no-resolve")
	}
	return flags
}

// groupLints 按 routing 第 11.3 节计算组的提醒。依据见决定 0007 #16（mihomo v1.19.33 adapter/outboundgroup）。
func groupLints(g finalGroup, referenced bool) []string {
	lints := []string{}
	if g.Type != "select" && len(g.Use) == 0 && len(g.Proxies) < 2 {
		lints = append(lints, "group.single_member")
	}
	if g.Type == "url-test" {
		if g.Tolerance == nil || *g.Tolerance == 0 {
			lints = append(lints, "group.tolerance_zero")
		}
		if g.Lazy == nil || *g.Lazy {
			lints = append(lints, "group.lazy_stale")
		}
		lints = append(lints, "group.latency_only")
	}
	if g.Type == "load-balance" && g.Strategy == "round-robin" {
		lints = append(lints, "group.round_robin")
	}
	if (g.Type == "url-test" || g.Type == "fallback" || g.Type == "load-balance") && strings.HasPrefix(g.URL, "http://") {
		lints = append(lints, "group.http_probe")
	}
	if !referenced {
		lints = append(lints, "group.unused")
	}
	return lints
}

// —— 追踪 ——

// RoutingTrace 追踪一个域名或 IP 在当前配置下的走向。draft 非空时它是草稿的 routing 段，
// 另外给出应用草稿后的走向，供界面对比（routing 第 7 节）。
func (s *Service) RoutingTrace(ctx context.Context, user *store.User, doc []byte, draft, host, process, network string) (current, after *TraceDTO, err error) {
	if s.Trace.Disabled {
		return nil, nil, ErrTraceDisabled
	}
	req, err := match.NewRequest(host, process, network)
	if err != nil {
		return nil, nil, fmt.Errorf("%w：%w", ErrInvalidHost, err)
	}
	run := func(d []byte) (*TraceDTO, error) {
		in, err := s.routingInput(ctx, user, d)
		if err != nil {
			return nil, err
		}
		sets, stale := s.loadSets(ctx, in)
		return in.trace(req, sets, stale), nil
	}
	if current, err = run(doc); err != nil || draft == "" {
		return current, nil, err
	}
	d, err := withRouting(doc, draft)
	if err != nil {
		return nil, nil, err
	}
	after, err = run(d)
	return current, after, err
}

// loadSets 取得 rules 引用的全部规则集（并发、有上限）。拿不到的规则集不放进结果，追踪中记为 no-content。
func (s *Service) loadSets(ctx context.Context, in *routingInput) (map[string]*match.Set, map[string]bool) {
	specs := in.usedSpecs()
	sets, stale := map[string]*match.Set{}, map[string]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, spec := range specs {
		wg.Add(1)
		go func(spec ruleSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			set, st, err := s.loadSet(ctx, spec, in.env, in.hosts)
			if err != nil {
				return
			}
			mu.Lock()
			sets[spec.Name], stale[spec.Name] = set, st
			mu.Unlock()
		}(spec)
	}
	wg.Wait()
	return sets, stale
}

// trace 逐行判断，并按内核 tunnel.match 跳过目标不存在或为 PASS 的命中行；随后沿默认路径到出口。
// 节点是否支持 UDP 取决于客户端实际选中的成员，服务器不判断（见 routing 第 5.2 节）。
func (in *routingInput) trace(req match.Request, sets map[string]*match.Set, stale map[string]bool) *TraceDTO {
	t := &TraceDTO{Host: req.Host, Steps: []StepDTO{}, Hit: -1, Chain: []ChainStep{}}
	if req.IP.IsValid() {
		t.IP = req.IP.String()
	}
	needsAt, cands := -1, []int{}
	for start := 0; start < len(in.lines); {
		out := match.TraceFrom(in.lines, start, sets, req, in.env)
		for _, st := range out.Steps {
			t.Steps = append(t.Steps, StepDTO{Index: st.Index, Result: st.Result, Evidence: st.Evidence, Reason: st.Reason})
			if l := in.lines[st.Index]; l.Type == "RULE-SET" && stale[l.Payload] {
				t.Stale = true
			}
		}
		if needsAt < 0 {
			needsAt = out.NeedsIPAt
		}
		cands = append(cands, out.IPCandidates...)
		if out.Hit < 0 {
			break
		}
		target := in.lines[out.Hit].Target
		reason := ""
		switch {
		case target == "PASS":
			reason = "pass"
		case in.kindOf(target) == "node" && !in.proxies[target]:
			reason = "no-target"
		}
		if reason == "" {
			t.Hit = out.Hit
			break
		}
		last := &t.Steps[len(t.Steps)-1]
		last.Result, last.Reason = match.ResultMiss, reason
		start = out.Hit + 1
	}
	if needsAt >= 0 {
		t.NeedsIP = &NeedsIPDTO{At: needsAt, Candidates: cands}
	}
	if t.Hit < 0 {
		t.Exit = TraceExit{Kind: "builtin", Name: "DIRECT"} // 内核：没有任何规则命中时直连（tunnel.go）
		return t
	}
	t.Chain, t.Exit = in.follow(in.lines[t.Hit].Target)
	return t
}

// follow 沿默认路径（每层第一个成员）从 name 走到出口。
func (in *routingInput) follow(name string) ([]ChainStep, TraceExit) {
	chain := []ChainStep{}
	seen := map[string]bool{}
	for {
		g := in.groups[name]
		if g == nil {
			break
		}
		if seen[name] { // 组之间成环：内核会拒绝这份配置，这里只是不陷入死循环
			return chain, TraceExit{Kind: "unknown", Name: name}
		}
		seen[name] = true
		members := append(append([]string{}, g.Proxies...), g.Use...)
		if len(members) == 0 {
			return chain, TraceExit{Kind: "unknown", Name: name}
		}
		chain = append(chain, ChainStep{Group: g.Name, Type: g.Type, Pick: members[0], Candidates: append([]string{}, members[1:]...)})
		if len(g.Proxies) == 0 { // 只有 provider：由客户端在其中选择（决定 0007 #18）
			return chain, TraceExit{Kind: "provider", Name: g.Use[0]}
		}
		name = members[0]
	}
	switch {
	case slices.Contains(builtins, name):
		return chain, TraceExit{Kind: "builtin", Name: name}
	case in.proxies[name]:
		e := TraceExit{Kind: "node", Name: name, Via: in.via[name]}
		if n := in.nodes[name]; n != nil {
			e.Region = n.Region.Code
		}
		return chain, e
	}
	return chain, TraceExit{Kind: "unknown", Name: name}
}

// —— 体检 ——

// HealthSite 是常用网站列表（catalog/routing/health.yaml）中的一项。
type HealthSite struct {
	Host     string `yaml:"host" json:"host"`
	Category string `yaml:"category" json:"category"`
}

var (
	healthOnce  sync.Once
	healthSites []HealthSite
	healthErr   error
)

// LoadHealthSites 读取体检用的常用网站列表。
func LoadHealthSites(fsys fs.FS) ([]HealthSite, error) {
	b, err := fs.ReadFile(fsys, "routing/health.yaml")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Sites []HealthSite `yaml:"sites"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析体检网站列表: %w", err)
	}
	for _, site := range doc.Sites {
		if _, err := match.NewRequest(site.Host, "", ""); err != nil || site.Category == "" {
			return nil, fmt.Errorf("体检网站 %q 无效", site.Host)
		}
	}
	return doc.Sites, nil
}

// RoutingHealth 用常用网站批量追踪当前配置（按 443 端口、TCP）。
func (s *Service) RoutingHealth(ctx context.Context, user *store.User, doc []byte, draft string) ([]HealthItem, error) {
	if s.Trace.Disabled {
		return nil, ErrTraceDisabled
	}
	healthOnce.Do(func() { healthSites, healthErr = LoadHealthSites(s.catalogFS()) })
	if healthErr != nil {
		return nil, healthErr
	}
	if draft != "" { // 有草稿时按草稿体检
		d, err := withRouting(doc, draft)
		if err != nil {
			return nil, err
		}
		doc = d
	}
	in, err := s.routingInput(ctx, user, doc)
	if err != nil {
		return nil, err
	}
	sets, stale := s.loadSets(ctx, in)
	items := make([]HealthItem, 0, len(healthSites))
	for _, site := range healthSites {
		req, _ := match.NewRequest(site.Host+":443", "", "")
		t := in.trace(req, sets, stale)
		item := HealthItem{Host: site.Host, Category: site.Category, Hit: t.Hit, Exit: t.Exit}
		if t.Hit >= 0 {
			item.Rule = in.lines[t.Hit].Raw
		}
		// 「不确定」只标真正可能改变结果、且用户无从补充的情况：网站体检不针对某个应用，
		// 是否经过 DNS 解析在完整追踪里单独说明；这两类在几乎每份真实配置里都会出现，标出来没有区分度。
		for _, st := range t.Steps {
			if st.Result == match.ResultUnknown && st.Reason != match.ReasonNeedsProcess {
				item.Uncertain = true
			}
		}
		items = append(items, item)
	}
	return items, nil
}

var (
	descOnce sync.Once
	descs    *ruleset.Descriptions
)

// descriptions 返回规则集说明目录；目录无效时返回 nil（TestRulesetDescriptions 保证不会发生）。
func (s *Service) descriptions() *ruleset.Descriptions {
	descOnce.Do(func() {
		d, err := ruleset.LoadDescriptions(catalogdata.FS)
		if err != nil {
			if s.Log != nil {
				s.Log.Error("规则集说明目录无效", "error", err)
			}
			return
		}
		descs = d
	})
	return descs
}

func (s *Service) catalogFS() fs.FS { return catalogdata.FS }

// routingModel 返回方案的 routing 模型：自己管理时就是 routing 段；跟随上游时是规则源的当前内容（合集自动识别），
// 即开始管理后会得到的 routing。没有规则时返回 nil。诊断不在这里报告（预览已报告过）。
func (s *Service) routingModel(ctx context.Context, owner string, p *profile.Profile) *ruleset.Source {
	var l diag.List
	switch {
	case p == nil:
		return nil
	case p.Routing != nil:
		return ruleset.FromRouting(p.Routing.Groups, p.Routing.Rulesets, &l)
	case p.Rules != nil:
		ri, err := s.rulesInput(ctx, owner, p.Rules.Ref)
		if err != nil {
			return nil
		}
		var src *ruleset.Source
		if len(bytes.TrimSpace(ri.INI)) > 0 {
			src = ruleset.ParseINI(ri.INI, ri.Name+".ini", &l)
		} else {
			src = &ruleset.Source{Groups: ruleset.ParseGroups(ri.Groups, "groups.toml", &l), Rulesets: ruleset.ParseRulesets(ri.Rulesets, "rulesets.toml", &l)}
		}
		ruleset.DetectBundles(src.Rulesets)
		return src
	}
	return nil
}

func modelGroup(m *ruleset.Source, name string) *ruleset.Group {
	if m == nil {
		return nil
	}
	for i := range m.Groups {
		if m.Groups[i].Name == name {
			return &m.Groups[i]
		}
	}
	return nil
}

// ErrNoRules 表示方案没有选择规则源，无法开始自己管理分流。
var ErrNoRules = errors.New("service: 方案没有选择分流规则")

// RoutingFork 从规则源的当前内容生成 routing 段文本（开始自己管理，决定 0007 第 7 条）。不保存：
// 前端把它放进草稿，用户点「应用」时才写入方案文档。已经自己管理时返回当前的 routing 段。
func (s *Service) RoutingFork(ctx context.Context, user *store.User, doc []byte) (string, []diag.Diagnostic, error) {
	var l diag.List
	p, err := profile.Parse(doc, &l)
	if err != nil {
		return "", l.Items(), err
	}
	if p.Rules == nil {
		return "", nil, ErrNoRules
	}
	ri, err := s.rulesInput(ctx, user.ID, p.Rules.Ref)
	if err != nil {
		return "", nil, err
	}
	n := ruleset.Fork(ri.Groups, ri.Rulesets, ri.INI, ri.Name, p.Rules.Ref, s.now().UTC().Format(time.RFC3339), &l)
	text, err := render.Render(n, render.Options{}, &l)
	if err != nil {
		return "", l.Items(), fmt.Errorf("输出 routing 段: %w", err)
	}
	return string(text), l.Items(), nil
}

// withRouting 把方案文档中的 routing 段替换为 draft（草稿的 routing 段文本），用于草稿与已应用的对比追踪。
func withRouting(doc []byte, draft string) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(doc, &root); err != nil || len(root.Content) == 0 {
		return nil, fmt.Errorf("%w：方案文档无效", ErrBadDraft)
	}
	var d yaml.Node
	if err := yaml.Unmarshal([]byte(draft), &d); err != nil || len(d.Content) == 0 || d.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w：routing 草稿必须是映射", ErrBadDraft)
	}
	yamlx.Set(root.Content[0], "routing", d.Content[0])
	return yaml.Marshal(&root)
}

// ErrBadDraft 表示 routing 草稿或方案文档无法解析。
var ErrBadDraft = errors.New("service: 草稿无效")
