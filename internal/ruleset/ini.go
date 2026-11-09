package ruleset

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
)

// ParseINI 解析 subconverter 外部配置 ini（如 ACL4SSR）的 [custom] 节（第 6.5 节）。
// 语法对照 angwz/subconverter@a33d49e src/config/binding.h 的 from_ini：
//
//	ruleset=组,来源[,间隔]          来源可带 clash-domain: 等类型前缀，或以 [] 开头的内联规则
//	custom_proxy_group=名称`类型`成员…[`测速地址`间隔,超时,容差]
//	                                url-test、fallback、load-balance 的最后两段是测速地址与时间参数；
//	                                成员中的 !!PROVIDER=a,b 写入 use
//	rename=正则@替换、emoji=正则,emoji、include_remarks=正则、exclude_remarks=正则 → 节点整理
//	enable_rule_generator=false     → 不生成规则
func ParseINI(text []byte, file string, out *diag.List) *Source {
	src := &Source{Origin: "ini", NodeOps: &node.Processing{}}
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	section := ""
	lineNo := 0
	unsupported := map[string]bool{}
	var includes, excludes []string
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), string(rune(0xFEFF))))
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		if section != "custom" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			out.Add(diag.IniInvalid, fmt.Sprintf("%s:%d", file, lineNo), "line", strconv.Itoa(lineNo), "reason", "expected key=value")
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		where := fmt.Sprintf("%s:%d", file, lineNo)
		switch key {
		case "ruleset", "surge_ruleset":
			if r, ok := iniRuleset(val, where, out); ok {
				src.Rulesets = append(src.Rulesets, r)
			}
		case "custom_proxy_group":
			if g, ok := iniGroup(val, where, out); ok {
				src.Groups = append(src.Groups, g)
			}
		case "enable_rule_generator":
			src.NoRules = strings.EqualFold(val, "false")
		case "overwrite_original_rules":
			// 本项目没有「基础配置」中的原有规则，这个开关没有作用
		case "rename":
			pat, rep, ok := strings.Cut(val, "@")
			if !ok {
				out.Add(diag.IniInvalid, where, "line", strconv.Itoa(lineNo), "reason", "rename must be pattern@replacement")
				continue
			}
			src.NodeOps.Rename = append(src.NodeOps.Rename, node.RenameRule{Pattern: pat, Replace: rep})
		case "emoji":
			pat, e, ok := strings.Cut(val, ",")
			if !ok {
				out.Add(diag.IniInvalid, where, "line", strconv.Itoa(lineNo), "reason", "emoji must be pattern,emoji")
				continue
			}
			if code, isFlag := node.FlagCode(strings.TrimSpace(e)); isFlag {
				src.NodeOps.RegionRules = append(src.NodeOps.RegionRules, node.RegionRule{Pattern: pat, Region: code})
			} else {
				out.Add(diag.IniInvalid, where, "line", strconv.Itoa(lineNo), "reason", "only flag emoji map to regions; other emoji are ignored")
			}
		case "add_emoji":
			if strings.EqualFold(val, "false") {
				src.NodeOps.Flags = node.FlagOff
			}
		case "remove_old_emoji":
			if strings.EqualFold(val, "true") && src.NodeOps.Flags != node.FlagOff {
				src.NodeOps.Flags = node.FlagReplace
			}
		case "include_remarks":
			includes = append(includes, val)
		case "exclude_remarks":
			excludes = append(excludes, val)
		default:
			unsupported[key] = true
		}
	}
	src.NodeOps.Include = strings.Join(includes, "`")
	src.NodeOps.Exclude = strings.Join(excludes, "`")
	keys := make([]string, 0, len(unsupported))
	for k := range unsupported {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Add(diag.IniUnsupported, file, "key", k)
	}
	if src.NoRules {
		src.Rulesets = nil
	}
	return src
}

var iniTypePrefixes = []string{"clash-domain:", "clash-ipcidr:", "clash-classic:", "surge:", "quanx:"}

func iniRuleset(val, where string, out *diag.List) (Ruleset, bool) {
	r := Ruleset{Where: where, Type: "surge-ruleset", Interval: 0}
	pos := strings.Index(val, ",")
	if pos < 0 {
		out.Add(diag.IniInvalid, where, "line", where, "reason", "ruleset must be group,source")
		return r, false
	}
	r.Group = val[:pos]
	rest := val[pos+1:]
	if strings.HasPrefix(rest, "[]") {
		r.Ruleset = rest
		return r, true
	}
	// 与 fork 一致：最后一个逗号之后是更新间隔
	if epos := strings.LastIndex(val, ","); epos != pos {
		r.Interval = toInt(val[epos+1:], 0)
		rest = val[pos+1 : epos]
	}
	for _, p := range iniTypePrefixes {
		if strings.HasPrefix(rest, p) {
			r.Type = map[string]string{"clash-domain:": "clash-domain", "clash-ipcidr:": "clash-ipcidr", "clash-classic:": "clash-classic", "surge:": "surge-ruleset", "quanx:": "quantumultx"}[p]
			rest = rest[len(p):]
		}
	}
	r.Ruleset = rest
	if r.Interval == 0 {
		r.Interval = 86400
	}
	return r, true
}

func iniGroup(val, where string, out *diag.List) (Group, bool) {
	parts := strings.Split(val, "`")
	if len(parts) < 3 {
		out.Add(diag.IniInvalid, where, "line", where, "reason", "custom_proxy_group needs at least name`type`member")
		return Group{}, false
	}
	g := Group{Name: parts[0], Type: parts[1], Where: where, Extra: map[string]any{}}
	upper := len(parts)
	switch g.Type {
	case "select", "relay", "ssid":
	case "url-test", "fallback", "load-balance":
		if upper < 5 {
			out.Add(diag.IniInvalid, where, "line", where, "reason", g.Type+" needs a probe URL and interval")
			return Group{}, false
		}
		upper -= 2
		g.Extra["url"] = parts[upper]
		// 时间参数：间隔,超时,容差（parseGroupTimes）
		times := strings.Split(parts[upper+1], ",")
		if len(times) > 0 && times[0] != "" {
			g.Extra["interval"] = int64(toInt(times[0], 0))
		}
		if len(times) > 1 && times[1] != "" {
			g.Extra["timeout"] = int64(toInt(times[1], 0))
		}
		if len(times) > 2 && times[2] != "" {
			g.Extra["tolerance"] = int64(toInt(times[2], 0))
		}
	default:
		out.Add(diag.IniInvalid, where, "line", where, "reason", "unknown group type "+g.Type)
		return Group{}, false
	}
	for _, m := range parts[2:upper] {
		if strings.HasPrefix(m, "!!PROVIDER=") {
			g.Use = append(g.Use, strings.Split(m[len("!!PROVIDER="):], ",")...)
			continue
		}
		g.Rule = append(g.Rule, m)
	}
	switch g.Type {
	case "relay":
		out.Add(diag.GroupTypeMihomo, where, "name", g.Name, "type", "relay", "reason", "removed from mihomo; use dialer-proxy on the proxy instead")
	case "ssid":
		out.Add(diag.GroupTypeMihomo, where, "name", g.Name, "type", "ssid", "reason", "not supported by mihomo")
	}
	return g, true
}
