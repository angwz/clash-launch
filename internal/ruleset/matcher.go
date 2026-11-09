package ruleset

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
)

// matchRange 移植 angwz/subconverter@a33d49e src/generator/config/subexport.cpp 的 matchRange，
// 逐项保持其行为（含其怪异之处），由 matcher_test.go 锁定：
//
//   - 逗号分隔多项，从左到右依次处理；
//   - "n"（可带负号）精确匹配，命中则置 true；
//   - "a-b" 闭区间，命中则置 true；
//   - "!n"：先置 true，若等于 n 则置 false——它会覆盖前面各项的结论，而不是「或」；
//     注意 "!-3" 中的负号被丢弃（fork 用 $1 取数字），比较的是 3；
//   - "!a-b"：先置 true，落在区间内则置 false；
//   - "n-"：target ≤ n；"n+"：target ≥ n（开区间的方向以源码为准）；
//   - 不符合以上任何写法的项被忽略。
func matchRange(rng string, target int) bool {
	match := false
	for _, x := range strings.Split(rng, ",") {
		switch {
		case reNum.MatchString(x):
			if toInt(x, math.MaxInt32) == target {
				match = true
			}
		case reRange.MatchString(x):
			m := reRange.FindStringSubmatch(x)
			if target >= toInt(m[1], math.MaxInt32) && target <= toInt(m[2], math.MinInt32) {
				match = true
			}
		case reNot.MatchString(x):
			match = true
			if toInt(reNot.FindStringSubmatch(x)[1], math.MaxInt32) == target {
				match = false
			}
		case reNotRange.MatchString(x):
			match = true
			m := reNotRange.FindStringSubmatch(x)
			if target >= toInt(m[1], math.MaxInt32) && target <= toInt(m[2], math.MinInt32) {
				match = false
			}
		case reLess.MatchString(x):
			if toInt(reLess.FindStringSubmatch(x)[1], math.MaxInt32) >= target {
				match = true
			}
		case reMore.MatchString(x):
			if toInt(reMore.FindStringSubmatch(x)[1], math.MinInt32) <= target {
				match = true
			}
		}
	}
	return match
}

// fork 的 regMatch 是整串锚定匹配（PCRE2_ANCHORED|PCRE2_ENDANCHORED）。
var (
	reNum      = regexp.MustCompile(`^-?\d+$`)
	reRange    = regexp.MustCompile(`^(\d+)-(\d+)$`)
	reNot      = regexp.MustCompile(`^!-?(\d+)$`)
	reNotRange = regexp.MustCompile(`^!(\d+)-(\d+)$`)
	reLess     = regexp.MustCompile(`^(\d+)-$`)
	reMore     = regexp.MustCompile(`^(\d+)\+$`)
	// applyMatcher 中对 GROUPID/INSERT 目标的提取：只允许数字与 - + ! ,
	reGroupID = regexp.MustCompile(`^!!(?:GROUPID|INSERT)=([\d\-+!,]+)(?:!!(.*))?$`)
)

func toInt(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// typeNames 是 fork 中 !!TYPE= 比较的协议名（大写）。fork 只认识这些；
// 其余 mihomo 协议按其 type 的大写形式参与比较（本项目的扩展，见 subconverter-parity）。
var typeNames = map[string]string{
	"ss": "SS", "ssr": "SSR", "vmess": "VMESS", "trojan": "TROJAN", "snell": "SNELL",
	"socks5": "SOCKS5", "wireguard": "WIREGUARD", "vless": "VLESS", "hysteria": "HYSTERIA", "hysteria2": "HYSTERIA2",
}

func typeName(n *node.Node) string {
	if n.Type == "http" {
		var v map[string]any
		if n.Mapping.Decode(&v) == nil && v["tls"] == true {
			return "HTTPS"
		}
		return "HTTP"
	}
	if t, ok := typeNames[n.Type]; ok {
		return t
	}
	return strings.ToUpper(n.Type)
}

// matcher 是解析后的一个 rule 条目。
type matcher struct {
	kind   string // ref | select | regex | groupid | insert | group | type | port | server | script
	target string // 引用名、范围、正则
	extra  string // 匹配器之后的 !!正则
	raw    string
}

func parseMatcher(rule string) matcher {
	m := matcher{raw: rule}
	cut := func(prefix string) (string, string) {
		rest := rule[len(prefix):]
		// 与 fork 的 (.+?)(?:!!(.*))? 一致：目标是第一个 "!!" 之前的部分
		if i := strings.Index(rest, "!!"); i >= 0 {
			return rest[:i], rest[i+2:]
		}
		return rest, ""
	}
	switch {
	case strings.HasPrefix(rule, "[]"):
		m.kind, m.target = "ref", rule[2:]
	case rule == "@select":
		m.kind = "select"
	case strings.HasPrefix(rule, "script:"):
		m.kind, m.target = "script", rule[7:]
	case strings.HasPrefix(rule, "!!GROUPID="), strings.HasPrefix(rule, "!!INSERT="):
		m.kind = "groupid"
		if strings.HasPrefix(rule, "!!INSERT=") {
			m.kind = "insert"
		}
		if sm := reGroupID.FindStringSubmatch(rule); sm != nil {
			m.target, m.extra = sm[1], sm[2]
		} else {
			m.target = "\x00invalid" // fork 中目标为空串，matchRange("") 恒为 false
		}
	case strings.HasPrefix(rule, "!!GROUP="):
		m.kind = "group"
		m.target, m.extra = cut("!!GROUP=")
	case strings.HasPrefix(rule, "!!TYPE="):
		m.kind = "type"
		m.target, m.extra = cut("!!TYPE=")
	case strings.HasPrefix(rule, "!!PORT="):
		m.kind = "port"
		m.target, m.extra = cut("!!PORT=")
	case strings.HasPrefix(rule, "!!SERVER="):
		m.kind = "server"
		m.target, m.extra = cut("!!SERVER=")
	default:
		m.kind, m.target = "regex", rule
	}
	return m
}

// pick 返回匹配该条目的节点（保持节点顺序）。regex 与 extra 按名称搜索（regexp2，与 mihomo filter 一致）。
func (m matcher) pick(nodes []*node.Node, group, where string, out *diag.List) []*node.Node {
	compile := func(p string) *regexpCache {
		if p == "" {
			return nil
		}
		re, err := node.Compile(p)
		if err != nil {
			out.Add(diag.RegexInvalid, where, "where", where+" ("+group+")", "reason", err.Error())
			return &regexpCache{bad: true}
		}
		return &regexpCache{re: re}
	}
	extra := compile(m.extra)
	var target *regexpCache
	switch m.kind {
	case "regex", "group", "server":
		target = compile(m.target)
	case "type":
		// fork 用整串匹配比较协议名
		target = compile("^(?:" + m.target + ")$")
	}
	if (target != nil && target.bad) || (extra != nil && extra.bad) {
		return nil
	}
	var res []*node.Node
	for _, n := range nodes {
		ok := false
		switch m.kind {
		case "regex":
			ok = target.match(n.Name, where, out)
		case "groupid":
			ok = n.GroupID >= 0 && matchRange(m.target, n.GroupID)
		case "insert":
			ok = false // 本项目没有 insert 源（见 Expand 中的说明）
		case "group":
			ok = target.match(n.SourceName, where, out)
		case "type":
			ok = target.match(typeName(n), where, out)
		case "port":
			ok = matchRange(m.target, n.Port)
		case "server":
			ok = target.match(n.Server, where, out)
		}
		if ok && extra != nil && !extra.match(n.Name, where, out) {
			ok = false
		}
		if ok {
			res = append(res, n)
		}
	}
	return res
}

type regexpCache struct {
	re  interface{ MatchString(string) (bool, error) }
	bad bool
}

func (c *regexpCache) match(s, where string, out *diag.List) bool {
	ok, err := c.re.MatchString(s)
	if err != nil {
		out.Add(diag.RegexTimeout, where, "where", where, "name", s)
		return false
	}
	return ok
}
