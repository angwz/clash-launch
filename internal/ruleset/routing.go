package ruleset

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 方案文档的 routing 段与规则源模型之间的转换（docs/design/routing.md 第 4 节，决定 0007 第 6 条）。
// routing 沿用 groups.toml 与 rulesets.toml 的键，所以校验、选择器、展开与规则生成都复用规则源的代码；
// 这里只处理两件规则源没有的事：YAML 的整数类型与 bundle 合集。

// FromRouting 把 routing 段的 groups 与 rulesets 转换为规则源模型。定位写成 routing.groups#3、routing.rulesets#5。
func FromRouting(groups, rulesets []map[string]any, out *diag.List) *Source {
	src := &Source{Origin: "routing"}
	for i, raw := range groups {
		if g, ok := groupFromMap(normalize(raw).(map[string]any), fmt.Sprintf("routing.groups#%d", i+1), out); ok {
			src.Groups = append(src.Groups, g)
		}
	}
	for i, raw := range rulesets {
		if r, ok := rulesetFromMap(normalize(raw).(map[string]any), fmt.Sprintf("routing.rulesets#%d", i+1), true, out); ok {
			src.Rulesets = append(src.Rulesets, r)
		}
	}
	checkBundles(src.Rulesets, out)
	return src
}

// normalize 把 YAML 解出的整数统一为 int64：TOML 解出的就是 int64，groupFromMap 等判断只认它。
func normalize(v any) any {
	switch x := v.(type) {
	case int:
		return int64(x)
	case uint64:
		return int64(x) //nolint:gosec // 规则源里的整数是间隔、容差等小数值
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = normalize(e)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, e := range x {
			l[i] = normalize(e)
		}
		return l
	}
	if v == nil {
		return map[string]any{}
	}
	return v
}

// checkBundles：同一 bundle 的条目必须相邻且 group 相同，否则只是不按合集显示，输出照原顺序。
func checkBundles(list []Ruleset, out *diag.List) {
	seen := map[string]int{} // bundle → 最后一次出现的位置
	for i, r := range list {
		if r.Bundle == "" {
			continue
		}
		if last, ok := seen[r.Bundle]; ok && (last != i-1 || list[last].Group != r.Group) {
			out.Add(diag.RoutingBundleSplit, r.Where, "bundle", r.Bundle, "where", r.Where)
		}
		seen[r.Bundle] = i
	}
}

// nodeOpKeys 是 ini 中影响节点的键；开始自己管理时原样保留这些行（routing.ini-node-ops）。
var nodeOpKeys = []string{"rename", "emoji", "add_emoji", "remove_old_emoji", "include_remarks", "exclude_remarks"}

// ParseNodeOps 解析 routing.ini-node-ops 中的行，与 ParseINI 的节点整理语义相同。其他键报告为不支持。
func ParseNodeOps(lines []string, file string, out *diag.List) *node.Processing {
	if len(lines) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("[custom]\n")
	for _, l := range lines {
		k, _, _ := strings.Cut(l, "=")
		if !slices.Contains(nodeOpKeys, strings.TrimSpace(k)) {
			out.Add(diag.IniUnsupported, file, "key", strings.TrimSpace(k))
			continue
		}
		b.WriteString(strings.ReplaceAll(l, "\n", " ") + "\n")
	}
	return ParseINI([]byte(b.String()), file, out).NodeOps
}

// NodeOpLines 取出 ini [custom] 节中影响节点的行（原文，顺序不变），供开始自己管理时写入 routing.ini-node-ops。
func NodeOpLines(ini []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(ini))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	section := ""
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), string(rune(0xFEFF))))
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if section == "custom" && ok && slices.Contains(nodeOpKeys, strings.TrimSpace(k)) {
			out = append(out, line)
		}
	}
	return out
}

// 组的键顺序：常用的在前，其余按键名追加。不用 map 序列化（会按键名排序，打乱阅读顺序）。
var groupKeyOrder = []string{"name", "type", "rule", "select", "use", "url", "interval", "tolerance", "lazy", "strategy"}

// ToRouting 把规则源模型写回 routing 段的两个 YAML 序列（展开之前的模型）。相同输入逐字节相同。
func ToRouting(src *Source) (groups, rulesets *yaml.Node) {
	groups = yamlx.Seq()
	for _, g := range src.Groups {
		m := yamlx.Map("name", g.Name, "type", g.Type)
		vals := map[string]any{}
		for k, v := range g.Extra {
			vals[k] = v
		}
		if g.Rule != nil {
			vals["rule"] = g.Rule
		}
		if g.SelectRaw != nil {
			vals["select"] = g.SelectRaw
		}
		if g.Use != nil {
			vals["use"] = g.Use
		}
		keys := make([]string, 0, len(vals))
		for k := range vals {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			oi, oj := slices.Index(groupKeyOrder, keys[i]), slices.Index(groupKeyOrder, keys[j])
			switch {
			case oi >= 0 && oj >= 0:
				return oi < oj
			case oi >= 0:
				return true
			case oj >= 0:
				return false
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			yamlx.Set(m, k, valueNode(vals[k]))
		}
		groups.Content = append(groups.Content, m)
	}
	rulesets = yamlx.Seq()
	for _, r := range src.Rulesets {
		m := yamlx.Map("group", r.Group, "ruleset", r.Ruleset)
		if !strings.HasPrefix(r.Ruleset, "[]") { // 内联规则没有类型与间隔
			yamlx.Set(m, "type", yamlx.Str(r.Type))
			yamlx.Set(m, "interval", valueNode(int64(r.Interval)))
		}
		if r.Format != "" {
			yamlx.Set(m, "format", yamlx.Str(r.Format))
		}
		if r.Bundle != "" {
			yamlx.Set(m, "bundle", yamlx.Str(r.Bundle))
		}
		m.Style = yaml.FlowStyle // 每条规则集一行，便于阅读与比较
		rulesets.Content = append(rulesets.Content, m)
	}
	return groups, rulesets
}

// valueNode 把 TOML 解出的值（string、int64、float64、bool、[]any、[]string、map[string]any）转换为 YAML 节点；
// 映射的键按名称排序，保证结果确定。
func valueNode(v any) *yaml.Node {
	switch x := v.(type) {
	case []string:
		n := yamlx.StrSeq(x...)
		n.Style = yaml.FlowStyle
		return n
	case []any:
		n := yamlx.Seq()
		n.Style = yaml.FlowStyle
		for _, e := range x {
			n.Content = append(n.Content, valueNode(e))
		}
		return n
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := yamlx.Map()
		m.Style = yaml.FlowStyle
		for _, k := range keys {
			yamlx.Set(m, k, valueNode(x[k]))
		}
		return m
	}
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return yamlx.Str(fmt.Sprint(v))
	}
	return &n
}

// DetectBundles 自动识别合集（决定 0007 第 9 条）：相邻、走向相同、文件名分别为 X.*、X-ipcidr.*、X-classic.*，
// 且路径分别含 clash-domain/、clash-ipcidr/、clash-classic/ 的条目记为合集 X（domainrouter_v2 的发布约定）。
// 其他相邻且走向相同的规则集不自动合并。
func DetectBundles(list []Ruleset) {
	base := func(r Ruleset) (name, kind string) {
		u := r.Ruleset
		for _, k := range []string{"clash-domain/", "clash-ipcidr/", "clash-classic/"} {
			if strings.Contains(u, k) {
				kind = k
			}
		}
		if kind == "" {
			return "", ""
		}
		file := path.Base(u)
		name = strings.TrimSuffix(file, path.Ext(file))
		switch kind {
		case "clash-ipcidr/":
			name = strings.TrimSuffix(name, "-ipcidr")
		case "clash-classic/":
			name = strings.TrimSuffix(name, "-classic")
		}
		return name, kind
	}
	for i := 0; i < len(list); {
		name, kind := base(list[i])
		j := i + 1
		kinds := []string{kind}
		for name != "" && j < len(list) {
			n2, k2 := base(list[j])
			if n2 != name || list[j].Group != list[i].Group || slices.Contains(kinds, k2) {
				break
			}
			kinds = append(kinds, k2)
			j++
		}
		if name != "" && j-i >= 2 {
			for k := i; k < j; k++ {
				list[k].Bundle = name
			}
		}
		i = j
	}
}

// Digest 是上游 groups 与 rulesets 的规范化 JSON 的 sha256，用于判断上游是否变化（routing.from.digest）。
func Digest(groups, rulesets *yaml.Node) string {
	var g, r any
	_ = groups.Decode(&g)
	_ = rulesets.Decode(&r)
	b, _ := json.Marshal(map[string]any{"groups": g, "rulesets": r}) // json 对映射的键排序，结果确定
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Fork 从规则源的当前内容生成 routing 段（开始自己管理，决定 0007 第 7 条）：from 记录上游与摘要，
// groups、rulesets 是上游的规范化副本（合集自动识别），base 与之相同，作为之后上游同步的比对基准。
// 上游为 ini 时，影响节点的行原样写入 ini-node-ops。at 由调用方给出（纯函数不读当前时间）。
func Fork(groupsText, rulesetsText, ini []byte, name, ref, at string, out *diag.List) *yaml.Node {
	var src *Source
	var nodeOps []string
	if len(bytes.TrimSpace(ini)) > 0 {
		src = ParseINI(ini, name+".ini", out)
		nodeOps = NodeOpLines(ini)
	} else {
		src = &Source{Groups: ParseGroups(groupsText, "groups.toml", out), Rulesets: ParseRulesets(rulesetsText, "rulesets.toml", out)}
	}
	DetectBundles(src.Rulesets)
	groups, rulesets := ToRouting(src)
	from := yamlx.Map("rules", ref, "digest", Digest(groups, rulesets), "at", at)
	r := yamlx.Map("from", from)
	if len(nodeOps) > 0 {
		yamlx.Set(r, "ini-node-ops", yamlx.StrSeq(nodeOps...))
	}
	yamlx.Set(r, "groups", groups)
	yamlx.Set(r, "rulesets", rulesets)
	yamlx.Set(r, "base", yamlx.Map("groups", yamlx.Clone(groups), "rulesets", yamlx.Clone(rulesets)))
	return r
}
