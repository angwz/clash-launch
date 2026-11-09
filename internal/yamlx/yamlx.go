// Package yamlx 是对 go.yaml.in/yaml/v3 Node API 的薄封装：带资源上限的安全解析，
// 以及构造、复制、查找节点的小工具。所有读入用户 YAML 的位置都经过 Parse，
// 保证「YAML 炸弹」与重复键在进入管线之前就被拦下（第 14.5 节）。
package yamlx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
)

// Limits 是解析时的资源上限。
type Limits struct {
	MaxBytes int // 文档体积
	MaxNodes int // 展开别名之后的节点总数
}

// DefaultLimits 适用于方案文档、原始编辑与订阅内容（订阅体积另由拉取层限制在 10 MiB）。
var DefaultLimits = Limits{MaxBytes: 10 << 20, MaxNodes: 2_000_000}

// ErrInvalid 表示文档因语法或上限无法使用；详情在诊断中。
var ErrInvalid = errors.New("yamlx: 文档无效")

// Parse 解析单个 YAML 文档：检查体积、语法、重复键，展开锚点与别名并限制展开后的节点数。
// 空文档返回 nil 节点且无错误。where 用于诊断定位（如 "raw"、"source:3"）。
func Parse(text []byte, lim Limits, where string, out *diag.List) (*yaml.Node, error) {
	if lim.MaxBytes > 0 && len(text) > lim.MaxBytes {
		out.Add(diag.YAMLTooLarge, where, "limit", strconv.Itoa(lim.MaxBytes)+" B")
		return nil, ErrInvalid
	}
	dec := yaml.NewDecoder(bytes.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		out.Add(diag.YAMLSyntax, where, "reason", err.Error())
		return nil, ErrInvalid
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		out.Add(diag.YAMLSyntax, where, "reason", "multiple documents are not supported")
		return nil, ErrInvalid
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]
	count := 0
	expanded, err := expand(root, map[*yaml.Node]bool{}, &count, lim.MaxNodes)
	if err != nil {
		out.Add(diag.YAMLTooManyNode, where, "limit", strconv.Itoa(lim.MaxNodes))
		return nil, ErrInvalid
	}
	if dups := duplicates(expanded, ""); len(dups) > 0 {
		for _, p := range dups {
			out.Add(diag.YAMLDupKey, joinWhere(where, p), "key", p)
		}
		return nil, ErrInvalid
	}
	return expanded, nil
}

func joinWhere(where, path string) string {
	if where == "" {
		return path
	}
	return where + ":" + path
}

var errTooMany = errors.New("yamlx: too many nodes")

// expand 深复制节点树并展开别名；visiting 防止自引用的别名环。
func expand(n *yaml.Node, visiting map[*yaml.Node]bool, count *int, max int) (*yaml.Node, error) {
	*count++
	if max > 0 && *count > max {
		return nil, errTooMany
	}
	if n.Kind == yaml.AliasNode {
		if n.Alias == nil || visiting[n.Alias] {
			return nil, errTooMany
		}
		visiting[n.Alias] = true
		c, err := expand(n.Alias, visiting, count, max)
		delete(visiting, n.Alias)
		return c, err
	}
	c := *n
	c.Anchor = ""
	c.Alias = nil
	c.Content = nil
	if len(n.Content) > 0 {
		c.Content = make([]*yaml.Node, len(n.Content))
		for i, ch := range n.Content {
			e, err := expand(ch, visiting, count, max)
			if err != nil {
				return nil, err
			}
			c.Content[i] = e
		}
	}
	return &c, nil
}

// duplicates 返回所有重复键的路径。
func duplicates(n *yaml.Node, path string) []string {
	var out []string
	switch n.Kind {
	case yaml.MappingNode:
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			p := Join(path, k)
			if n.Content[i].Kind == yaml.ScalarNode {
				if seen[k] {
					out = append(out, p)
				}
				seen[k] = true
			}
			out = append(out, duplicates(n.Content[i+1], p)...)
		}
	case yaml.SequenceNode:
		for i, ch := range n.Content {
			out = append(out, duplicates(ch, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}

// Join 拼接配置路径：Join("dns", "nameserver") = "dns.nameserver"。
func Join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Index 拼接列表下标：Index("proxies", 2) = "proxies[2]"。
func Index(path string, i int) string { return fmt.Sprintf("%s[%d]", path, i) }

// —— 构造 ——

// Str 构造字符串标量（Tag 明确为 !!str，渲染时按需加引号）。
func Str(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// Int 构造整数标量。
func Int(i int64) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(i, 10)}
}

// Bool 构造布尔标量。
func Bool(b bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(b)}
}

// Null 构造 null。
func Null() *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"} }

// Map 构造映射；kv 依次为键、值。
func Map(kv ...any) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(kv); i += 2 {
		Set(m, kv[i].(string), toNode(kv[i+1]))
	}
	return m
}

// Seq 构造序列。
func Seq(items ...*yaml.Node) *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
}

// StrSeq 构造字符串序列。
func StrSeq(items ...string) *yaml.Node {
	s := Seq()
	for _, it := range items {
		s.Content = append(s.Content, Str(it))
	}
	return s
}

func toNode(v any) *yaml.Node {
	switch v := v.(type) {
	case *yaml.Node:
		return v
	case string:
		return Str(v)
	case int:
		return Int(int64(v))
	case int64:
		return Int(v)
	case bool:
		return Bool(v)
	case []string:
		return StrSeq(v...)
	case nil:
		return Null()
	}
	panic(fmt.Sprintf("yamlx: 不支持的值类型 %T", v))
}

// FromAny 把解码得到的 Go 值（map[string]any 等）转换为节点树。map 的键按字典序排列——
// 调用方随后会按目录排序，这里只需保证确定性。
func FromAny(v any) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	clearStyle(&n)
	return &n, nil
}

func clearStyle(n *yaml.Node) {
	n.Style = 0
	n.Line, n.Column = 0, 0
	for _, c := range n.Content {
		clearStyle(c)
	}
}

// —— 查找与修改 ——

// Get 返回映射中 key 对应的值节点。
func Get(m *yaml.Node, key string) *yaml.Node {
	if i := KeyIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

// KeyIndex 返回键在 Content 中的下标；不存在返回 -1。
func KeyIndex(m *yaml.Node, key string) int {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// Set 设置映射中的键；已存在时原位替换值，否则追加在末尾。
func Set(m *yaml.Node, key string, v *yaml.Node) {
	if i := KeyIndex(m, key); i >= 0 {
		m.Content[i+1] = v
		return
	}
	m.Content = append(m.Content, Str(key), v)
}

// Delete 删除映射中的键，返回是否删除了内容。
func Delete(m *yaml.Node, key string) bool {
	i := KeyIndex(m, key)
	if i < 0 {
		return false
	}
	m.Content = append(m.Content[:i], m.Content[i+2:]...)
	return true
}

// Keys 返回映射的键（按出现顺序）。
func Keys(m *yaml.Node) []string {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]string, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		out = append(out, m.Content[i].Value)
	}
	return out
}

// Clone 深复制节点（不含别名，调用方保证树已展开）。
func Clone(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	if len(n.Content) > 0 {
		c.Content = make([]*yaml.Node, len(n.Content))
		for i, ch := range n.Content {
			c.Content[i] = Clone(ch)
		}
	}
	return &c
}

// ShortTag 返回节点的规范标签（!!str、!!int、!!map……）。显式的自定义标签原样返回。
func ShortTag(n *yaml.Node) string { return n.ShortTag() }

// KindName 返回节点种类的英文名，用于诊断参数。
func KindName(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	case yaml.DocumentNode:
		return "document"
	}
	return "unknown"
}
