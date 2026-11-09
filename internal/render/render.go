// Package render 把合并后的节点树序列化为最终 YAML，并对结果做格式自检（第 11 节）。
//
// 为什么不用 yaml.v3 的 Encoder：它的 is_printable 不认 4 字节 UTF-8，emoji 与国旗会被
// 强制写成 "\U0001F1F8" 这样的双引号转义；它也不会给 yes/no/0123 这类在 YAML 1.1
// 解析器中有歧义的字符串加引号，且无法在顶层分节之间留空行。这里实现一个受限的发射器：
// 只输出块映射、块序列、流式集合与三种标量写法（普通、单引号、双引号、字面块），
// 不输出锚点、别名与标签之外的任何构造。每次渲染都用 yaml.v3 重新解析输出并与
// 渲染前的树逐节点比较，保证缩进、引号与转义正确——自检失败即引擎缺陷，不发布。
package render

import (
	"bytes"
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
)

// HeaderField 是文件头的一行：# KEY: value。
type HeaderField struct{ Key, Value string }

// Options 控制渲染。
type Options struct {
	Catalog *catalog.Catalog // 键顺序的来源；nil 时保持原有顺序
	Header  []HeaderField
	// ProxiesBlock 为 true 时 proxies 中的节点用块格式；默认每个节点一行（流式映射）。
	ProxiesBlock bool
	// LongLine 超过该字符数的行给出警告；0 表示 4096。
	LongLine int
}

// Render 排序、序列化并自检。返回的文本以单个换行结尾、只含 LF、缩进 2 个空格。
// 自检失败时返回错误，诊断中给出定位（RenderSelfCheck）。
func Render(root *yaml.Node, opt Options, out *diag.List) ([]byte, error) {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("render: 顶层必须是映射")
	}
	var top *catalog.Field
	if opt.Catalog != nil {
		top = opt.Catalog.Root
	}
	tree := Order(root, top)
	e := &emitter{flowProxies: !opt.ProxiesBlock, cache: map[string]string{}}
	for _, h := range opt.Header {
		e.b.WriteString("# " + strings.ToUpper(h.Key) + ": " + oneLine(h.Value) + "\n")
	}
	if len(opt.Header) > 0 {
		e.b.WriteString("\n")
	}
	if err := e.root(tree); err != nil {
		return nil, err
	}
	text := e.b.Bytes()
	if err := SelfCheck(text, tree, out); err != nil {
		return nil, err
	}
	limit := cmp.Or(opt.LongLine, 4096)
	for i, line := range bytes.Split(text, []byte("\n")) {
		if n := utf8.RuneCount(line); n > limit {
			out.Add(diag.RenderLongLine, "", "line", strconv.Itoa(i+1), "length", strconv.Itoa(n))
		}
	}
	return text, nil
}

func oneLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }), " ")
}

// Order 返回按目录排好键序的副本：目录中的键按目录顺序，目录外的键保持出现顺序并排在末尾；
// map 类型（键是用户数据，如 hosts、rule-providers 的名称）与目录未知的位置保持原有顺序。
// 列表元素的顺序从不改变。
func Order(n *yaml.Node, f *catalog.Field) *yaml.Node {
	c := *n
	c.Content = nil
	switch n.Kind {
	case yaml.MappingNode:
		type pair struct {
			k, v *yaml.Node
			rank int
			pos  int
		}
		order := f.KeyOrder(n)
		rank := make(map[string]int, len(order))
		for i, k := range order {
			rank[k] = i
		}
		pairs := make([]pair, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			r, ok := rank[k.Value]
			if !ok {
				r = len(order) // 目录外：排在末尾，保持出现顺序
			}
			var child *catalog.Field
			if f != nil && f.Type == catalog.TMap {
				child = f.Values
			} else {
				child = f.Child(k.Value, n)
			}
			pairs = append(pairs, pair{k: k, v: Order(v, child), rank: r, pos: i})
		}
		if order != nil {
			slices.SortStableFunc(pairs, func(a, b pair) int { return cmp.Compare(a.rank, b.rank) })
		}
		for _, p := range pairs {
			kc := *p.k
			c.Content = append(c.Content, &kc, p.v)
		}
	case yaml.SequenceNode:
		elem := f.Elem()
		for _, it := range n.Content {
			c.Content = append(c.Content, Order(it, elem))
		}
	}
	return &c
}

// —— 发射器 ——

type emitter struct {
	b           bytes.Buffer
	flowProxies bool
	cache       map[string]string // 普通标量的解析结果缓存：值 → 规范标签
}

func (e *emitter) root(m *yaml.Node) error {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if i > 0 {
			e.b.WriteByte('\n') // 顶层分节之间空一行
		}
		k, v := m.Content[i], m.Content[i+1]
		flowItems := e.flowProxies && k.Value == "proxies" && v.Kind == yaml.SequenceNode
		if err := e.pair(k, v, 0, flowItems); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) indent(n int) { e.b.WriteString(strings.Repeat(" ", n)) }

// pair 输出一个块映射项（调用方已写好缩进）。
func (e *emitter) pair(k, v *yaml.Node, ind int, flowItems bool) error {
	key, err := e.scalarKey(k)
	if err != nil {
		return err
	}
	if len(key) > maxImplicitKey {
		// 隐式键最长 1024 个字符；更长的键改用显式写法「? 键」换行「: 值」。
		e.b.WriteString("? " + key + "\n")
		e.indent(ind)
		e.b.WriteString(":")
		return e.value(v, ind, flowItems)
	}
	e.b.WriteString(key + ":")
	return e.value(v, ind, flowItems)
}

// value 输出映射项的值，从冒号之后开始。
func (e *emitter) value(v *yaml.Node, ind int, flowItems bool) error {
	switch {
	case v.Kind == yaml.ScalarNode:
		if s, ok := e.blockScalar(v, ind+2); ok {
			e.b.WriteString(" " + s)
			return nil
		}
		s, err := e.scalar(v, false)
		if err != nil {
			return err
		}
		e.b.WriteString(" " + s + "\n")
	case len(v.Content) == 0:
		e.b.WriteString(" " + emptyOf(v) + "\n")
	case v.Kind == yaml.MappingNode:
		e.b.WriteByte('\n')
		return e.mapping(v, ind+2, false)
	case v.Kind == yaml.SequenceNode:
		e.b.WriteByte('\n')
		return e.sequence(v, ind+2, flowItems)
	default:
		return fmt.Errorf("render: 不支持的节点种类 %d", v.Kind)
	}
	return nil
}

// maxImplicitKey 留出余量：YAML 规定隐式键不超过 1024 个 Unicode 字符，这里按字节保守计算。
const maxImplicitKey = 1000

// hasLongKey 报告节点（含子孙）中是否有超出隐式键长度的键；这样的节点不用流式写法。
func hasLongKey(n *yaml.Node) bool {
	for i, c := range n.Content {
		if n.Kind == yaml.MappingNode && i%2 == 0 && len(c.Value) > maxImplicitKey-10 {
			return true
		}
		if hasLongKey(c) {
			return true
		}
	}
	return false
}

func emptyOf(n *yaml.Node) string {
	if n.Kind == yaml.MappingNode {
		return "{}"
	}
	return "[]"
}

// mapping 输出块映射；inline 为 true 时第一项紧跟在「- 」之后，不写缩进。
func (e *emitter) mapping(m *yaml.Node, ind int, inline bool) error {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if i > 0 || !inline {
			e.indent(ind)
		}
		if err := e.pair(m.Content[i], m.Content[i+1], ind, false); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) sequence(s *yaml.Node, ind int, flowItems bool) error {
	for _, it := range s.Content {
		e.indent(ind)
		e.b.WriteString("- ")
		switch {
		case it.Kind == yaml.ScalarNode:
			if bs, ok := e.blockScalar(it, ind+2); ok {
				e.b.WriteString(bs)
				continue
			}
			str, err := e.scalar(it, false)
			if err != nil {
				return err
			}
			e.b.WriteString(str + "\n")
		case len(it.Content) == 0:
			e.b.WriteString(emptyOf(it) + "\n")
		case flowItems && !hasLongKey(it):
			str, err := e.flow(it)
			if err != nil {
				return err
			}
			e.b.WriteString(str + "\n")
		case it.Kind == yaml.MappingNode:
			if err := e.mapping(it, ind+2, true); err != nil {
				return err
			}
		case it.Kind == yaml.SequenceNode:
			// 嵌套列表：紧凑写法「- - a」
			e.b.Truncate(e.b.Len() - 2)
			e.b.WriteString("-\n")
			if err := e.sequence(it, ind+2, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("render: 不支持的节点种类 %d", it.Kind)
		}
	}
	return nil
}

// flow 输出流式集合：{k: v, …} 或 [a, …]。
func (e *emitter) flow(n *yaml.Node) (string, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		return e.scalar(n, true)
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, err := e.scalarKeyFlow(n.Content[i])
			if err != nil {
				return "", err
			}
			v, err := e.flow(n.Content[i+1])
			if err != nil {
				return "", err
			}
			parts = append(parts, k+": "+v)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, it := range n.Content {
			v, err := e.flow(it)
			if err != nil {
				return "", err
			}
			parts = append(parts, v)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	}
	return "", fmt.Errorf("render: 不支持的节点种类 %d", n.Kind)
}

func (e *emitter) scalarKey(k *yaml.Node) (string, error) {
	if k.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("render: 不支持非标量的键")
	}
	if strings.ContainsAny(k.Value, "\n\r") {
		return quoteDouble(k.Value), nil
	}
	return e.scalar(k, false)
}

func (e *emitter) scalarKeyFlow(k *yaml.Node) (string, error) {
	if k.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("render: 不支持非标量的键")
	}
	return e.scalar(k, true)
}

// scalar 选择标量的写法：能用普通写法就不加引号；需要引号时优先单引号；
// 含控制字符或换行（在流式上下文中）时用双引号。
func (e *emitter) scalar(n *yaml.Node, inFlow bool) (string, error) {
	tag := n.ShortTag()
	v := n.Value
	switch tag {
	case "!!str":
		if plainSafe(v, inFlow) && e.resolve(v) == "!!str" && !yaml11Ambiguous(v) {
			return v, nil
		}
		if singleQuotable(v) {
			return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
		}
		return quoteDouble(v), nil
	case "!!null":
		if v == "" {
			return "null", nil
		}
		fallthrough
	default:
		// 非字符串标量（数字、布尔、null、时间戳……）：原文能以普通写法解析回同一类型才写原文，
		// 否则显式写出标签。
		if plainSafe(v, inFlow) && e.resolve(v) == tag {
			return v, nil
		}
		return tag + " " + quoteDouble(v), nil
	}
}

// blockScalar 对多行字符串使用字面块（|、|-）。不使用 |+：保留尾随空行的写法会吞掉
// 分节之间的空行，此类值改用双引号。
func (e *emitter) blockScalar(n *yaml.Node, ind int) (string, bool) {
	v := n.Value
	if n.ShortTag() != "!!str" || !strings.Contains(v, "\n") {
		return "", false
	}
	body := v
	chomp := "-"
	switch {
	case strings.HasSuffix(v, "\n\n"):
		return "", false
	case strings.HasSuffix(v, "\n"):
		chomp = ""
		body = strings.TrimSuffix(v, "\n")
	}
	if strings.Trim(body, "\n") == "" {
		return "", false // 只有换行的字符串无法用字面块无损表达
	}
	for _, r := range body {
		if r != '\n' && r != '\t' && !safeRune(r) {
			return "", false
		}
	}
	lines := strings.Split(body, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "\t") {
			return "", false // 行首的制表符会紧贴缩进出现，规范要求输出中不出现制表符缩进
		}
	}
	indicator := ""
	if strings.HasPrefix(lines[0], " ") || (lines[0] == "" && len(lines) > 1) {
		indicator = "2" // 首行以空格开头时必须显式给出缩进量
	}
	var b strings.Builder
	b.WriteString("|" + indicator + chomp + "\n")
	pad := strings.Repeat(" ", ind)
	for _, l := range lines {
		if l == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(pad + l + "\n")
	}
	return b.String(), true
}

// resolve 返回 v 以普通写法出现时 yaml.v3 推断的标签（与 mihomo 使用同一解析器）。
func (e *emitter) resolve(v string) string {
	if t, ok := e.cache[v]; ok {
		return t
	}
	var n yaml.Node
	t := "?"
	if err := yaml.Unmarshal([]byte("k: "+v), &n); err == nil && len(n.Content) == 1 && len(n.Content[0].Content) == 2 {
		val := n.Content[0].Content[1]
		if val.Kind == yaml.ScalarNode && val.Value == v {
			t = val.ShortTag()
		}
	}
	e.cache[v] = t
	return t
}

const indicators = "-?:,[]{}#&*!|>'\"%@`"

// plainSafe 是普通写法的保守判定：宁可多加引号，也不冒险。
func plainSafe(v string, inFlow bool) bool {
	if v == "" || v != strings.TrimSpace(v) || v == "<<" || v == "=" {
		return false
	}
	if strings.ContainsRune(indicators, rune(v[0])) {
		return false
	}
	if strings.Contains(v, ": ") || strings.Contains(v, " #") || strings.HasSuffix(v, ":") {
		return false
	}
	// 流式上下文中 libyaml 的限制比规范更多（如 ':'、'?' 后接分隔符），这里改用白名单：
	// ASCII 只允许字母、数字、空格与少量安全符号；非 ASCII 的可打印字符照常允许。
	if inFlow {
		for _, r := range v {
			if r < 0x80 && !flowSafe(r) {
				return false
			}
		}
	}
	for _, r := range v {
		if !safeRune(r) {
			return false
		}
	}
	return true
}

func flowSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(" ._/+@()=^~$;!-", r)
}

// singleQuotable \u62A5\u544A\u5B57\u7B26\u4E32\u80FD\u5426\u5199\u6210\u5355\u5F15\u53F7\u6807\u91CF\uFF1A\u5355\u5F15\u53F7\u5185\u6CA1\u6709\u8F6C\u4E49\uFF0C\u53EA\u80FD\u627F\u8F7D\u53EF\u6253\u5370\u5B57\u7B26\u3002
func singleQuotable(v string) bool {
	if !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if !safeRune(r) {
			return false
		}
	}
	return true
}

// safeRune \u62A5\u544A r \u80FD\u5426\u4E0D\u7ECF\u8F6C\u4E49\u51FA\u73B0\u5728\u666E\u901A\u3001\u5355\u5F15\u53F7\u6216\u5B57\u9762\u5757\u6807\u91CF\u4E2D\uFF1A\u53D6 YAML 1.2 \u7684\u53EF\u6253\u5370\u5B57\u7B26\u96C6\uFF0C
// \u53BB\u6389\u5236\u8868\u7B26\u3001BOM\uFF080xFEFF\uFF09\uFF0C\u4EE5\u53CA\u4F1A\u88AB libyaml \u5F53\u4F5C\u6362\u884C\u7684 0x2028/0x2029\uFF1BNEL\uFF080x85\uFF09\u4E0D\u5728\u533A\u95F4\u5185\u3002
func safeRune(r rune) bool {
	switch {
	case r >= 0x20 && r <= 0x7E:
		return true
	case r >= 0xA0 && r <= 0xD7FF:
		return r != 0x2028 && r != 0x2029
	case r >= 0xE000 && r <= 0xFFFD:
		return r != 0xFEFF
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	}
	return false
}

var (
	yaml11Bool = regexp.MustCompile(`^(y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF|null|Null|NULL|~)$`)
	yaml11Num  = regexp.MustCompile(`^[-+]?(0b[01_]+|0x[0-9a-fA-F_]+|0o?[0-7_]+|[0-9][0-9_]*(:[0-5]?[0-9])+(\.[0-9_]*)?|[0-9][0-9_]*(\.[0-9_]*)?([eE][-+]?[0-9]+)?|\.[0-9_]+([eE][-+]?[0-9]+)?|\.(inf|Inf|INF)|\.(nan|NaN|NAN))$`)
	yaml11Time = regexp.MustCompile(`^[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}`)
)

// yaml11Ambiguous 报告字符串在 YAML 1.1 解析器（部分旧客户端）中可能被当作布尔、数字或时间。
func yaml11Ambiguous(v string) bool {
	return yaml11Bool.MatchString(v) || yaml11Num.MatchString(v) || yaml11Time.MatchString(v)
}

func quoteDouble(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case 0:
			b.WriteString(`\0`)
		default:
			// 转义前缀用拼接生成：源码中不出现反斜杠紧跟 u 的写法，避免被工具链提前解码。
			const esc = "\\"
			switch {
			case safeRune(r):
				b.WriteRune(r)
			case r <= 0xFFFF:
				b.WriteString(esc + "u" + fmt.Sprintf("%04X", r))
			default:
				b.WriteString(esc + "U" + fmt.Sprintf("%08X", r))
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
