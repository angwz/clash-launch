// Package ruleset 解析 subconverter 语义的规则源——rulesets.toml、groups.toml 与外部配置 ini（如 ACL4SSR）——
// 转换为同一个模型，并展开为 mihomo 的 proxy-groups、rule-providers 与 rules（第 6 节）。
//
// 语义基准是 angwz/subconverter@a33d49e；与它不同的地方都列在 docs/design/subconverter-parity.md。
package ruleset

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
)

// Builtin 是可以在 []名称 中直接引用的内置策略（与 domainrouter_v2 groups.py 一致）。
var Builtin = []string{"DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"}

// subconverter 认识的组类型；其中 relay 已被 mihomo 移除，ssid 从未被 mihomo 支持。
var groupTypes = []string{"select", "url-test", "load-balance", "fallback", "relay", "ssid"}
var needsProbe = []string{"url-test", "fallback", "load-balance"}

// Group 是一个策略组定义（[[custom_groups]] 或 ini 的 custom_proxy_group）。
type Group struct {
	Name   string
	Type   string
	Rule   []string       // 成员条目：[]名称、正则、!!GROUPID= 等匹配器、"@select"
	Use    []string       // 直接引用的 proxy-provider
	Select *Selector      // 现代选择器（第 6.4 节）
	Extra  map[string]any // 其余键：url、interval、tolerance……，按键名排序输出
	Where  string         // 定位：groups.toml#3
}

// Ruleset 是一条规则集定义（[[rulesets]] 或 ini 的 ruleset=）。
type Ruleset struct {
	Group    string
	Ruleset  string // URL、文件路径或以 [] 开头的内联规则
	Type     string // clash-domain | clash-ipcidr | clash-classic | surge-ruleset | quantumultx
	Interval int
	// Format 是本项目的扩展键（subconverter 忽略它）：扩展名不能说明格式时显式指定 yaml、text 或 mrs，
	// 例如 Loyalsoldier/clash-rules 的 .txt 文件内容是 YAML payload。
	Format string
	Where  string
}

// Source 是转换后的规则源。
type Source struct {
	Groups   []Group
	Rulesets []Ruleset
	NodeOps  *node.Processing // 外部配置 ini 中的 rename、emoji、include/exclude_remarks
	NoRules  bool             // ini 中 enable_rule_generator=false
	Origin   string           // toml | ini
}

// ParseGroups 解析并校验 groups.toml（结构为 [[custom_groups]] 数组）。
// 校验移植 domainrouter_v2@2c812f9 src/domainrouter_v2/groups.py 的全部检查，并加上 mihomo 的组类型限制。
func ParseGroups(text []byte, file string, out *diag.List) []Group {
	var doc struct {
		CustomGroups []map[string]any `toml:"custom_groups"`
	}
	if err := toml.Unmarshal(text, &doc); err != nil {
		out.Add(diag.RulesSyntax, file, "file", file, "reason", tomlError(err))
		return nil
	}
	if len(doc.CustomGroups) == 0 {
		out.Add(diag.RulesSyntax, file, "file", file, "reason", "missing [[custom_groups]]")
		return nil
	}
	var groups []Group
	for i, raw := range doc.CustomGroups {
		where := fmt.Sprintf("%s#%d", file, i+1)
		g, ok := groupFromMap(raw, where, out)
		if ok {
			groups = append(groups, g)
		}
	}
	return groups
}

func tomlError(err error) string {
	var de *toml.DecodeError
	if ok := asDecodeError(err, &de); ok {
		row, col := de.Position()
		return fmt.Sprintf("line %d, column %d: %s", row, col, de.Error())
	}
	return err.Error()
}

func asDecodeError(err error, target **toml.DecodeError) bool {
	de, ok := err.(*toml.DecodeError) //nolint:errorlint // go-toml 直接返回该类型
	if ok {
		*target = de
	}
	return ok
}

// groupFromMap 转换并校验一个组；结构性错误（缺 name）时丢弃该组并报告。
func groupFromMap(raw map[string]any, where string, out *diag.List) (Group, bool) {
	name, _ := raw["name"].(string)
	if name == "" {
		out.Add(diag.GroupInvalid, where, "name", where, "reason", "missing name")
		return Group{}, false
	}
	g := Group{Name: name, Where: where, Extra: map[string]any{}}
	g.Type, _ = raw["type"].(string)
	if strings.ContainsAny(name, ",\n\r") || name != strings.TrimSpace(name) {
		out.Add(diag.GroupInvalid, where, "name", name, "reason", "name contains a comma, newline or surrounding whitespace")
	}
	if !slices.Contains(groupTypes, g.Type) {
		out.Add(diag.GroupInvalid, where, "name", name, "reason", "invalid type "+strconv.Quote(g.Type))
	} else if slices.Contains(needsProbe, g.Type) {
		_, hasURL := raw["url"].(string)
		_, hasInterval := raw["interval"].(int64)
		if !hasURL || !hasInterval {
			out.Add(diag.GroupInvalid, where, "name", name, "reason", g.Type+" requires url and interval")
		}
	}
	switch g.Type {
	case "relay":
		out.Add(diag.GroupTypeMihomo, where, "name", name, "type", "relay", "reason", "removed from mihomo; use dialer-proxy on the proxy instead")
	case "ssid":
		out.Add(diag.GroupTypeMihomo, where, "name", name, "type", "ssid", "reason", "not supported by mihomo")
	}
	rule, hasRule := raw["rule"]
	use, hasUse := raw["use"]
	if !hasRule && !hasUse {
		out.Add(diag.GroupInvalid, where, "name", name, "reason", "missing rule or use")
	}
	if hasRule {
		list, ok := stringList(rule)
		if !ok {
			out.Add(diag.GroupInvalid, where, "name", name, "reason", "rule must be an array of strings")
		}
		g.Rule = list
		if len(list) == 0 && !hasUse {
			out.Add(diag.GroupInvalid, where, "name", name, "reason", "rule is empty")
		}
	}
	if hasUse {
		list, ok := stringList(use)
		if !ok {
			out.Add(diag.GroupInvalid, where, "name", name, "reason", "use must be an array of strings")
		}
		g.Use = list
	}
	if s, ok := raw["select"]; ok {
		m, ok := s.(map[string]any)
		if !ok {
			out.Add(diag.SelectInvalid, where, "name", name, "reason", "select must be a table")
		} else if sel := parseSelector(m, name, where, out); sel != nil {
			g.Select = sel
		}
	}
	for k, v := range raw {
		switch k {
		case "name", "type", "rule", "use", "select":
		default:
			g.Extra[k] = v
		}
	}
	return g, true
}

func stringList(v any) ([]string, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		s, ok := x.(string)
		if !ok {
			return out, false
		}
		out = append(out, s)
	}
	return out, true
}

// CheckReferences 检查 []名称 引用的组或内置策略存在，且组之间没有循环引用。
func CheckReferences(groups []Group, out *diag.List) {
	refs := map[string][]string{}
	where := map[string]string{}
	var order []string
	for _, g := range groups {
		_, dup := refs[g.Name]
		if dup {
			out.Add(diag.GroupInvalid, g.Where, "name", g.Name, "reason", "duplicate group name")
		}
		var rs []string
		for _, r := range g.Rule {
			if strings.HasPrefix(r, "[]") {
				rs = append(rs, r[2:])
			}
		}
		refs[g.Name] = rs // 与 groups.py 一致：重名时以后出现的定义为准
		where[g.Name] = g.Where
		if !dup {
			order = append(order, g.Name)
		}
	}
	for _, name := range order {
		for _, r := range refs[name] {
			if _, ok := refs[r]; !ok && !slices.Contains(Builtin, r) {
				out.Add(diag.GroupReference, where[name], "name", name, "ref", r)
			}
		}
	}
	if cyc := findCycle(order, refs); len(cyc) > 0 {
		out.Add(diag.GroupCycle, "", "cycle", strings.Join(cyc, " → "))
	}
}

func findCycle(order []string, refs map[string][]string) []string {
	state := map[string]int{} // 0 未访问 / 1 访问中 / 2 完成
	var path []string
	var visit func(string) []string
	visit = func(n string) []string {
		state[n] = 1
		path = append(path, n)
		for _, r := range refs[n] {
			if _, ok := refs[r]; !ok {
				continue
			}
			switch state[r] {
			case 1:
				i := slices.Index(path, r)
				return append(slices.Clone(path[i:]), r)
			case 0:
				if c := visit(r); c != nil {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[n] = 2
		return nil
	}
	for _, n := range order {
		if state[n] == 0 {
			if c := visit(n); c != nil {
				return c
			}
		}
	}
	return nil
}

// ParseRulesets 解析 rulesets.toml（结构为 [[rulesets]] 数组，键 group、ruleset、type、interval）。
func ParseRulesets(text []byte, file string, out *diag.List) []Ruleset {
	var doc struct {
		Rulesets []map[string]any `toml:"rulesets"`
	}
	if err := toml.Unmarshal(text, &doc); err != nil {
		out.Add(diag.RulesSyntax, file, "file", file, "reason", tomlError(err))
		return nil
	}
	var list []Ruleset
	for i, raw := range doc.Rulesets {
		where := fmt.Sprintf("%s#%d", file, i+1)
		r := Ruleset{Where: where, Interval: 86400, Type: "surge-ruleset"} // 缺省值与 fork 的 binding.h 一致
		r.Group, _ = raw["group"].(string)
		r.Ruleset, _ = raw["ruleset"].(string)
		if t, ok := raw["type"].(string); ok {
			r.Type = t
		}
		if iv, ok := raw["interval"].(int64); ok {
			r.Interval = int(iv)
		}
		if f, ok := raw["format"].(string); ok {
			if f != "yaml" && f != "text" && f != "mrs" {
				out.Add(diag.RulesetInvalid, where, "index", where, "reason", "format must be yaml, text or mrs")
				continue
			}
			r.Format = f
		}
		if r.Group == "" || r.Ruleset == "" {
			out.Add(diag.RulesetInvalid, where, "index", where, "reason", "group and ruleset are required")
			continue
		}
		var unknown []string
		for k := range raw {
			if !slices.Contains([]string{"group", "ruleset", "type", "interval", "format"}, k) {
				unknown = append(unknown, k)
			}
		}
		sort.Strings(unknown)
		for _, k := range unknown {
			out.Add(diag.RulesetLimit, where, "index", where, "reason", "unknown key "+k+" ignored")
		}
		list = append(list, r)
	}
	return list
}
