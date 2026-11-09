package ruleset

import (
	"net/url"
	"path"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 规则参数：组名插在它们之前（mihomo 规则语法：类型,条件,策略[,参数…]）。
var ruleParams = map[string]bool{"no-resolve": true, "src": true}

// BuildRules 把 rulesets 转换为 rule-providers 与 rules（第 6.3 节）。顺序即 toml 顺序。
//
//	clash-domain  → behavior: domain                ；RULE-SET,<名>,<组>
//	clash-ipcidr  → behavior: ipcidr                ；RULE-SET,<名>,<组>,no-resolve（与 fork 一致）
//	clash-classic → behavior: classical
//	surge-ruleset → behavior: classical、format: text（Surge 列表与 mihomo classical text 的差异见限制）
//	quantumultx   → 不支持
//	*.mrs → format: mrs（classical 不能是 MRS）；*.txt、*.list → format: text；其他 → format: yaml
//	[]内联 → 原样写入并插入组名；[]FINAL 等同 []MATCH
func BuildRules(list []Ruleset, out *diag.List) (providers *yaml.Node, rules []string) {
	providers = yamlx.Map()
	used := map[string]bool{} // 小写的 provider 名
	surgeNoted := false
	for _, r := range list {
		if strings.HasPrefix(r.Ruleset, "[]") {
			body := r.Ruleset[2:]
			if body == "FINAL" || strings.HasPrefix(body, "FINAL,") {
				body = "MATCH" + body[5:]
			}
			rules = append(rules, InsertGroup(body, r.Group))
			continue
		}
		var behavior, format string
		switch r.Type {
		case "clash-domain":
			behavior = "domain"
		case "clash-ipcidr":
			behavior = "ipcidr"
		case "clash-classic":
			behavior = "classical"
		case "surge-ruleset":
			behavior, format = "classical", "text"
			if surgeNoted {
				break
			}
			surgeNoted = true // 每个规则源只提示一次
			out.Add(diag.RulesetLimit, r.Where, "index", r.Where, "reason", "surge-ruleset is loaded as mihomo classical text; Surge-only rule types are skipped by mihomo line by line")
		case "quantumultx":
			out.Add(diag.RulesetInvalid, r.Where, "index", r.Where, "reason", "quantumultx rule lists have no formal specification and are not supported")
			continue
		default:
			out.Add(diag.RulesetInvalid, r.Where, "index", r.Where, "reason", "unknown type "+r.Type)
			continue
		}
		file, ext := fileOf(r.Ruleset)
		switch {
		case r.Format != "":
			if r.Format == "mrs" && behavior == "classical" {
				out.Add(diag.RulesetInvalid, r.Where, "index", r.Where, "reason", "classical rule sets cannot be MRS")
				continue
			}
			format = r.Format
			if ext == "txt" && format == "yaml" {
				ext = "yaml" // 本地缓存文件名与内容一致
			}
		case ext == "mrs":
			if behavior == "classical" {
				out.Add(diag.RulesetInvalid, r.Where, "index", r.Where, "reason", "classical rule sets cannot be MRS")
				continue
			}
			format = "mrs"
		case ext == "txt" || ext == "list":
			format = "text"
		case format == "":
			format = "yaml"
		}
		if ext == "" {
			ext = map[string]string{"yaml": "yaml", "text": "txt", "mrs": "mrs"}[format]
		}
		name := file
		if name == "" {
			name = "ruleset"
		}
		base := name
		for i := 2; used[strings.ToLower(name)]; i++ {
			name = base + "_" + strconv.Itoa(i)
		}
		if name != base {
			out.Add(diag.RulesetRenamed, r.Where, "name", base, "new", name)
		}
		used[strings.ToLower(name)] = true
		p := yamlx.Map("type", typeOf(r.Ruleset), "behavior", behavior)
		if typeOf(r.Ruleset) == "http" {
			yamlx.Set(p, "url", yamlx.Str(r.Ruleset))
		}
		yamlx.Set(p, "path", yamlx.Str(localPath(r.Ruleset, name, ext)))
		yamlx.Set(p, "interval", yamlx.Int(int64(r.Interval)))
		yamlx.Set(p, "format", yamlx.Str(format))
		yamlx.Set(providers, name, p)
		line := "RULE-SET," + name + "," + r.Group
		if behavior == "ipcidr" {
			line += ",no-resolve"
		}
		rules = append(rules, line)
	}
	return providers, rules
}

// typeOf：http(s) 地址为 http，其余视为本地文件。
func typeOf(src string) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return "http"
	}
	return "file"
}

func localPath(src, name, ext string) string {
	if typeOf(src) == "file" {
		return src
	}
	return "./ruleset/" + name + "." + ext
}

// fileOf 与 fork 的 findFileName 一致：取最后一个路径分隔符之后、最后一个点之前的部分；同时返回小写扩展名。
func fileOf(src string) (name, ext string) {
	p := src
	if u, err := url.Parse(src); err == nil && u.Path != "" {
		p = u.Path
	}
	base := path.Base(strings.ReplaceAll(p, `\`, "/"))
	if i := strings.LastIndex(base, "."); i > 0 {
		return base[:i], strings.ToLower(base[i+1:])
	}
	return base, ""
}

// InsertGroup 把组名插入内联规则：放在规则参数（no-resolve、src）之前；
// 逻辑规则（AND/OR/NOT/SUB-RULE）的条件在括号内，组名放在括号之后。
//
// fork 的做法是先在末尾追加组名，再对含三个及以上逗号的结果套用 ^(.*?,.*?)(,.*)(,.*)$ → $1$3$2，
// 对于「GEOIP,CN,no-resolve」两者结果相同；对于带括号的逻辑规则 fork 会把组名插进括号内，这里不会。
func InsertGroup(rule, group string) string {
	parts := splitRule(rule)
	if len(parts) == 0 {
		return group
	}
	i := len(parts)
	for i > 1 && ruleParams[strings.TrimSpace(parts[i-1])] {
		i--
	}
	out := append(append(append([]string{}, parts[:i]...), group), parts[i:]...)
	return strings.Join(out, ",")
}

// splitRule 按顶层逗号拆分规则（括号内的逗号不拆）。
func splitRule(rule string) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range rule {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, rule[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, rule[start:])
}
