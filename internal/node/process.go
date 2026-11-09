package node

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dlclark/regexp2"
	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
)

// MatchTimeout 是 regexp2 的匹配超时（第 14.5 节）：超时记为诊断并按未命中处理，不卡住渲染。
const MatchTimeout = 100 * time.Millisecond

// Compile 编译与 mihomo filter 语义一致的正则（regexp2，默认选项）。
func Compile(pattern string) (*regexp2.Regexp, error) {
	re, err := regexp2.Compile(pattern, regexp2.None)
	if err != nil {
		return nil, err
	}
	re.MatchTimeout = MatchTimeout
	return re, nil
}

// Match 在名称中查找正则；超时写入诊断并返回 false。
func Match(re *regexp2.Regexp, name, where string, out *diag.List) bool {
	ok, err := re.MatchString(name)
	if err != nil {
		out.Add(diag.RegexTimeout, where, "where", where, "name", name)
		return false
	}
	return ok
}

// Override 是对单个节点的手动指定，以节点指纹为键（机场改名后依然有效），找不到时按名称匹配。
type Override struct {
	Region string   `yaml:"region,omitempty" json:"region,omitempty"`
	Emoji  string   `yaml:"emoji,omitempty" json:"emoji,omitempty"`
	Tags   []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Rate   *float64 `yaml:"rate,omitempty" json:"rate,omitempty"`
}

// RenameRule 是一条「正则 → 替换」。替换文本中的 $1 等引用分组。
type RenameRule struct {
	Pattern string `yaml:"pattern" json:"pattern"`
	Replace string `yaml:"replace" json:"replace"`
}

// RegionRule 是源级的「正则 → 地区」规则，按顺序第一个命中为准。
type RegionRule struct {
	Pattern string `yaml:"pattern" json:"pattern"`
	Region  string `yaml:"region" json:"region"`
}

// Processing 是一个源的整理设置（第 5.5 节）。零值即默认：去掉信息节点、国旗前缀、去重、保持原序。
type Processing struct {
	Include      string              `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude      string              `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	KeepInfo     bool                `yaml:"keep-info,omitempty" json:"keepInfo,omitempty"` // 关闭「去掉信息节点」
	InfoPatterns []string            `yaml:"info-patterns,omitempty" json:"infoPatterns,omitempty"`
	Rename       []RenameRule        `yaml:"rename,omitempty" json:"rename,omitempty"`
	Prefix       string              `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Flags        FlagMode            `yaml:"flags,omitempty" json:"flags,omitempty"`
	NoDecor      bool                `yaml:"no-decor,omitempty" json:"noDecor,omitempty"` // 未识别时不加装饰 emoji
	RegionRules  []RegionRule        `yaml:"region-rules,omitempty" json:"regionRules,omitempty"`
	Overrides    map[string]Override `yaml:"overrides,omitempty" json:"overrides,omitempty"` // 键：指纹或名称
	KeepDup      bool                `yaml:"keep-duplicates,omitempty" json:"keepDuplicates,omitempty"`
	Sort         string              `yaml:"sort,omitempty" json:"sort,omitempty"` // source（默认）| name
	Tags         []string            `yaml:"tags,omitempty" json:"tags,omitempty"` // 源级标签
}

// Tools 是整理所需的数据（地区表、装饰 emoji、信息节点模式），由调用方加载一次后复用。
type Tools struct {
	Regions *Detector
	Decor   *Decor
	Info    []string
}

// LoadTools 从目录数据（catalog/regions/）加载整理工具。
func LoadTools(fsys fs.FS) (*Tools, error) {
	d, err := LoadDetector(fsys, "regions/regions.yaml")
	if err != nil {
		return nil, err
	}
	dec, err := LoadDecor(fsys, "regions/decor.yaml", nil)
	if err != nil {
		return nil, err
	}
	b, err := fs.ReadFile(fsys, "regions/info.yaml")
	if err != nil {
		return nil, err
	}
	var f struct {
		Patterns []string `yaml:"patterns"`
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &Tools{Regions: d, Decor: dec, Info: f.Patterns}, nil
}

// Process 按固定顺序整理一个源的节点：过滤 → 重命名 → 地区与国旗 → 去重 → 排序。
// 每一步去掉或改动的节点都写入诊断。返回新的切片；节点对象被就地修改。
func Process(nodes []*Node, p Processing, t *Tools, where string, out *diag.List) []*Node {
	nodes = filter(nodes, p, t, where, out)
	rename(nodes, p, where, out)
	for _, n := range nodes {
		n.Tags = appendUnique(n.Tags, p.Tags...)
		n.Rate = ParseRate(n.Name)
		if o, ok := p.override(n); ok {
			n.Tags = appendUnique(n.Tags, o.Tags...)
			if o.Rate != nil {
				n.Rate = *o.Rate
			}
		}
	}
	regions(nodes, p, t, where, out)
	if !p.KeepDup {
		nodes = dedupe(nodes, where, out)
	}
	if p.Sort == "name" {
		sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	}
	return nodes
}

func (p Processing) override(n *Node) (Override, bool) {
	if o, ok := p.Overrides[n.Fingerprint]; ok {
		return o, true
	}
	o, ok := p.Overrides[n.Name]
	return o, ok
}

func appendUnique(list []string, items ...string) []string {
	for _, it := range items {
		found := false
		for _, x := range list {
			if x == it {
				found = true
			}
		}
		if !found {
			list = append(list, it)
		}
	}
	return list
}

// splitFilter 按反引号拆分多个正则（与 mihomo filter 相同：任一命中即可）。
func splitFilter(s string) []string {
	var out []string
	for _, x := range strings.Split(s, "`") {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func compileAll(patterns []string, where string, out *diag.List) []*regexp2.Regexp {
	var res []*regexp2.Regexp
	for _, p := range patterns {
		re, err := Compile(p)
		if err != nil {
			out.Add(diag.RegexInvalid, where, "where", where, "reason", err.Error())
			continue
		}
		res = append(res, re)
	}
	return res
}

func anyMatch(res []*regexp2.Regexp, name, where string, out *diag.List) bool {
	for _, re := range res {
		if Match(re, name, where, out) {
			return true
		}
	}
	return false
}

func filter(nodes []*Node, p Processing, t *Tools, where string, out *diag.List) []*Node {
	var info []*regexp2.Regexp
	if !p.KeepInfo {
		pats := t.Info
		if len(p.InfoPatterns) > 0 {
			pats = p.InfoPatterns
		}
		for _, x := range pats {
			re, err := regexp2.Compile(x, regexp2.IgnoreCase)
			if err != nil {
				out.Add(diag.RegexInvalid, where+".info-patterns", "where", where+".info-patterns", "reason", err.Error())
				continue
			}
			re.MatchTimeout = MatchTimeout
			info = append(info, re)
		}
	}
	inc := compileAll(splitFilter(p.Include), where+".include", out)
	exc := compileAll(splitFilter(p.Exclude), where+".exclude", out)
	var kept []*Node
	for _, n := range nodes {
		switch {
		case len(info) > 0 && anyMatch(info, n.Name, where+".info-patterns", out):
			out.Add(diag.NodeInfoRemoved, n.Origin, "name", n.Name, "source", where)
		case len(inc) > 0 && !anyMatch(inc, n.Name, where+".include", out):
			out.Add(diag.NodeFiltered, n.Origin, "name", n.Name, "source", where, "rule", "include")
		case len(exc) > 0 && anyMatch(exc, n.Name, where+".exclude", out):
			out.Add(diag.NodeFiltered, n.Origin, "name", n.Name, "source", where, "rule", "exclude")
		default:
			kept = append(kept, n)
		}
	}
	return kept
}

func rename(nodes []*Node, p Processing, where string, out *diag.List) {
	type rule struct {
		re  *regexp2.Regexp
		rep string
	}
	var rules []rule
	for i, r := range p.Rename {
		w := fmt.Sprintf("%s.rename[%d]", where, i)
		re, err := Compile(r.Pattern)
		if err != nil {
			out.Add(diag.RegexInvalid, w, "where", w, "reason", err.Error())
			continue
		}
		rules = append(rules, rule{re, r.Replace})
	}
	for _, n := range nodes {
		name := n.Name
		for i, r := range rules {
			s, err := r.re.Replace(name, r.rep, -1, -1)
			if err != nil {
				out.Add(diag.RegexTimeout, fmt.Sprintf("%s.rename[%d]", where, i), "where", fmt.Sprintf("%s.rename[%d]", where, i), "name", name)
				continue
			}
			name = s
		}
		name = strings.TrimSpace(p.Prefix + name)
		if name != n.Name && name != "" {
			n.SetName(name)
		}
	}
}

func regions(nodes []*Node, p Processing, t *Tools, where string, out *diag.List) {
	type rr struct {
		re   *regexp2.Regexp
		code string
		pat  string
	}
	var rules []rr
	for i, r := range p.RegionRules {
		w := fmt.Sprintf("%s.region-rules[%d]", where, i)
		re, err := Compile(r.Pattern)
		if err != nil {
			out.Add(diag.RegexInvalid, w, "where", w, "reason", err.Error())
			continue
		}
		rules = append(rules, rr{re, strings.ToUpper(r.Region), r.Pattern})
	}
	mode := p.Flags
	if mode == "" {
		mode = FlagPrefix
	}
	prevEmoji := ""
	for _, n := range nodes {
		o, hasO := p.override(n)
		switch {
		case hasO && o.Region != "":
			n.Region = Region{Code: strings.ToUpper(o.Region), Kind: RegionManual}
			if !t.Regions.Known(n.Region.Code) {
				out.Add(diag.NodeRegionManual, n.Origin, "code", o.Region, "name", n.Name)
			}
		default:
			matched := false
			for _, r := range rules {
				if Match(r.re, n.Name, where+".region-rules", out) {
					n.Region = Region{Code: r.code, Kind: RegionBySource, Match: r.pat}
					matched = true
					break
				}
			}
			if !matched {
				n.Region = t.Regions.Detect(n.Name)
			}
		}
		if n.Region.Inferred {
			out.Add(diag.NodeRegionInfer, n.Origin, "name", n.Name, "code", n.Region.Code, "reason", n.Region.Explain())
		}
		if mode == FlagOff {
			continue
		}
		base := n.Name
		if mode == FlagReplace {
			base = StripFlags(base)
		}
		_, hasFlag := FlagCode(base)
		switch {
		case n.Region.Code != "" && !hasFlag:
			n.SetName(Flag(n.Region.Code) + " " + base)
		case n.Region.Code == "" && !p.NoDecor && !hasFlag:
			e := o.Emoji
			if e == "" {
				e = t.Decor.Pick(n.Fingerprint, prevEmoji)
			}
			n.Emoji = e
			prevEmoji = e
			n.SetName(e + " " + base)
		default:
			if base != n.Name {
				n.SetName(base)
			}
		}
	}
}

func dedupe(nodes []*Node, where string, out *diag.List) []*Node {
	seen := map[string]*Node{}
	var kept []*Node
	for _, n := range nodes {
		if first, ok := seen[n.DedupKey]; ok {
			out.Add(diag.NodeDuplicate, n.Origin, "name", n.Name, "source", where, "first", first.Name)
			continue
		}
		seen[n.DedupKey] = n
		kept = append(kept, n)
	}
	return kept
}

// Uniquify 处理跨源同名：后出现的追加「 2」「 3」……（与 subconverter 的做法一致）。
func Uniquify(nodes []*Node, out *diag.List) {
	used := map[string]bool{}
	for _, n := range nodes {
		if !used[n.Name] {
			used[n.Name] = true
			continue
		}
		for i := 2; ; i++ {
			cand := n.Name + " " + strconv.Itoa(i)
			if !used[cand] {
				out.Add(diag.NodeRenamed, n.Origin, "name", n.Name, "new", cand)
				n.SetName(cand)
				used[cand] = true
				break
			}
		}
	}
}

var rateRe = []*regexp.Regexp{
	// 「2x」「0.5×」「2倍」：数字与符号之间不允许空格，否则「04 x2」中的「04 x」会被误读为 4 倍
	regexp.MustCompile(`(?i)(?:^|[^0-9.])(\d+(?:\.\d+)?)(?:x|×|倍)`),
	regexp.MustCompile(`(?i)(?:x|×)\s*(\d+(?:\.\d+)?)`),
	regexp.MustCompile(`倍率\s*[:：]?\s*(\d+(?:\.\d+)?)`),
}

// ParseRate 从名称解析倍率：x2、2倍、0.5×、×1.5、倍率 3；识别不到时为 1。
func ParseRate(name string) float64 {
	for _, re := range rateRe {
		if m := re.FindStringSubmatch(name); m != nil {
			if f, err := strconv.ParseFloat(m[1], 64); err == nil && f > 0 && f < 1000 {
				return f
			}
		}
	}
	return 1
}

// FilterNames 按包含、排除正则过滤节点（外部配置 ini 的 include_remarks、exclude_remarks）。
func FilterNames(nodes []*Node, include, exclude, where string, out *diag.List) []*Node {
	if include == "" && exclude == "" {
		return nodes
	}
	return filter(nodes, Processing{Include: include, Exclude: exclude, KeepInfo: true}, nil, where, out)
}
