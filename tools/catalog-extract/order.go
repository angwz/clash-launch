package main

import (
	"os"
	"regexp"
	"strings"
)

// 官方示例 docs/config.yaml 的键顺序。示例中大量字段以注释形式出现（# port: 7890），
// 它们同样表明了官方的介绍顺序，因此也计入。有效缩进 = '#' 所在列 + '#' 后多出的空格。
// 结果是初稿：人工对照后写入目录，目录的顺序测试再以目录为准。

var keyLine = regexp.MustCompile(`^([a-z0-9][a-z0-9_.-]*|[A-Z]{3,5}):(\s|$)`)

// 子键是用户自定义名称的映射：顺序合并到 "*"。
var nameMaps = map[string]bool{
	"proxy-providers": true, "rule-providers": true, "sub-rules": true, "hosts": true,
	"dns.nameserver-policy": true, "dns.proxy-server-nameserver-policy": true,
}

// 列表项按其 type 字段区分：proxies.vmess、listeners.socks。
var typedLists = map[string]bool{"proxies": true, "proxy-groups": true, "listeners": true}

type line struct {
	indent  int
	content string
	comment bool
}

func parseLines(text string) []line {
	var out []line
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimLeft(raw, " ")
		if trimmed == "" {
			continue
		}
		indent := len(raw) - len(trimmed)
		l := line{indent: indent, content: trimmed}
		if strings.HasPrefix(trimmed, "#") {
			rest := strings.TrimLeft(trimmed, "#")
			body := strings.TrimLeft(rest, " ")
			extra := len(rest) - len(body) - 1
			if extra < 0 {
				extra = 0
			}
			l = line{indent: indent + extra, content: body, comment: true}
		}
		out = append(out, l)
	}
	return out
}

type frame struct {
	indent int
	path   string
	item   bool
	dead   bool // 被注释掉的整段列表项：其中的键不计入
}

func exampleOrder(file string) (map[string][]string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	lines := parseLines(string(b))
	orders := map[string][]string{}
	// 每个容器的「上一个记录的键」，用于把后出现的新键插在正确位置。
	stack := []frame{{indent: -1}}
	itemKeys := map[int][]string{} // 栈深度 → 当前列表项内的键序列
	flush := func(depth int) {
		f := stack[depth]
		if f.item && !f.dead {
			mergeOrder(orders, f.path, itemKeys[depth])
			delete(itemKeys, depth)
		}
	}
	pop := func(cond func(frame) bool) {
		for len(stack) > 1 && cond(stack[len(stack)-1]) {
			flush(len(stack) - 1)
			stack = stack[:len(stack)-1]
		}
	}
	addKey := func(key string, indent int) {
		parent := stack[len(stack)-1]
		if parent.dead {
			stack = append(stack, frame{indent: indent, dead: true})
			return
		}
		path := parent.path
		if parent.item {
			itemKeys[len(stack)-1] = append(itemKeys[len(stack)-1], key)
		} else {
			mergeOrder(orders, path, []string{key})
		}
		child := key
		if nameMaps[path] {
			child = "*"
		}
		full := child
		if path != "" {
			full = path + "." + child
		}
		stack = append(stack, frame{indent: indent, path: full})
	}
	for i, l := range lines {
		c := l.content
		if strings.HasPrefix(c, "- ") || c == "-" {
			if l.comment {
				// 注释中的列表项：可能是示例值，也可能是整段被注释掉的条目；其下的注释键都不计入。
				pop(func(f frame) bool { return f.indent > l.indent || (f.indent == l.indent && f.item) })
				stack = append(stack, frame{indent: l.indent, item: true, dead: true})
				continue
			}
			pop(func(f frame) bool { return f.indent > l.indent || (f.indent == l.indent && f.item) })
			parent := stack[len(stack)-1]
			path := parent.path + "[]"
			if typedLists[parent.path] {
				if t := lookaheadType(lines, i, l.indent); t != "" {
					path = parent.path + "." + t
				}
			}
			stack = append(stack, frame{indent: l.indent, path: path, item: true})
			rest := strings.TrimSpace(strings.TrimPrefix(c, "-"))
			if m := keyLine.FindStringSubmatch(rest); m != nil {
				addKey(m[1], l.indent+2)
			}
			continue
		}
		m := keyLine.FindStringSubmatch(c)
		if m == nil {
			continue
		}
		if l.comment && strings.Contains(c, " ") && !strings.Contains(c, ": ") && !strings.HasSuffix(c, ":") {
			continue
		}
		pop(func(f frame) bool { return f.indent >= l.indent })
		addKey(m[1], l.indent)
	}
	pop(func(frame) bool { return true })
	return orders, nil
}

func lookaheadType(lines []line, start, indent int) string {
	for j := start; j < len(lines); j++ {
		l := lines[j]
		if j > start && !l.comment && l.indent <= indent {
			break
		}
		if l.comment {
			continue
		}
		c := strings.TrimPrefix(l.content, "- ")
		if strings.HasPrefix(c, "type:") && (l.indent == indent+2 || (j == start)) {
			v := strings.TrimSpace(strings.TrimPrefix(c, "type:"))
			v, _, _ = strings.Cut(v, "#")
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// mergeOrder 把 keys 合并进已有顺序：已有的键不动（先出现的样例优先），
// 新键插在它在 keys 中后面第一个已知键之前；后面没有已知键时追加到末尾。
func mergeOrder(orders map[string][]string, path string, keys []string) {
	cur := orders[path]
	for i, k := range keys {
		if indexOf(cur, k) >= 0 {
			continue
		}
		pos := len(cur)
		for j := i + 1; j < len(keys); j++ {
			if p := indexOf(cur, keys[j]); p >= 0 {
				pos = p
				break
			}
		}
		cur = append(cur[:pos], append([]string{k}, cur[pos:]...)...)
	}
	orders[path] = cur
}

func indexOf(s []string, k string) int {
	for i, v := range s {
		if v == k {
			return i
		}
	}
	return -1
}
