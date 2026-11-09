// Package node 负责节点输入：内容识别、分享链接与 Clash YAML 的解析与校验、整理
// （过滤、重命名、地区与国旗、去重、排序）、标签与倍率（第 5 节）。
//
// 本包是纯函数：输入是源的原文快照，输出是节点列表与诊断，不联网、不读写数据库。
// 分享链接解析与节点校验调用 mihomo 的 Go 库（internal/kernel），与内核行为一致。
package node

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Node 是一个已解析、已校验的节点。
type Node struct {
	Mapping *yaml.Node // 节点字段（映射）；name 键与 Name 保持一致
	Name    string
	Type    string
	Server  string
	Port    int

	// 来源与编号（第 5.7 节）
	SourceIndex int    // 源在方案中的位置
	SourceRef   string // 源的引用 ID
	SourceName  string // 源的名称（供 !!GROUP= 与现代选择器的 source 使用）
	GroupID     int    // 订阅编号；非订阅类型的源为 -1
	Origin      string // 输入位置，如 "src_a:3"

	// 属性
	Fingerprint string // 协议 + 服务器 + 端口 + 凭据哈希：手动指定、标签、倍率的键，机场改名后依然有效
	DedupKey    string // 去重键：除 name 外全部字段
	Region      Region
	Emoji       string // 未识别地区时使用的装饰 emoji
	Tags        []string
	Rate        float64
}

// Format 是识别出的内容格式。
type Format string

const (
	FormatClash   Format = "clash-yaml"
	FormatSingbox Format = "sing-box"
	FormatBase64  Format = "base64"
	FormatLinks   Format = "links"
	FormatUnknown Format = "unknown"
)

// credentialKeys 参与指纹计算的凭据字段。
var credentialKeys = []string{"uuid", "password", "psk", "auth-str", "auth", "token", "private-key", "username", "pre-shared-key", "obfs-password", "public-key"}

// Parse 识别并解析一个源的原文，返回校验通过的节点。where 是源的标识（诊断用，不含 URL）。
// cat 用于把分享链接产出的字段规范为目录类型（例如端口从字符串转为整数）。
func Parse(content []byte, where string, cat *catalog.Catalog, out *diag.List) ([]*Node, Format) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, FormatUnknown
	}
	// 1. Clash/Mihomo YAML；2. sing-box JSON（JSON 也是 YAML）
	var probe diag.List
	if root, err := yamlx.Parse(content, yamlx.DefaultLimits, where, &probe); err == nil && root != nil {
		switch {
		case root.Kind == yaml.MappingNode && yamlx.Get(root, "proxies") != nil:
			return fromYAMLList(yamlx.Get(root, "proxies"), where, out), FormatClash
		case root.Kind == yaml.SequenceNode && looksLikeProxyList(root):
			return fromYAMLList(root, where, out), FormatClash
		case root.Kind == yaml.MappingNode && yamlx.Get(root, "type") != nil && yamlx.Get(root, "name") != nil:
			return fromYAMLList(yamlx.Seq(root), where, out), FormatClash
		case root.Kind == yaml.MappingNode && yamlx.Get(root, "outbounds") != nil:
			return fromSingbox(yamlx.Get(root, "outbounds"), where, cat, out), FormatSingbox
		}
	}
	// 3. base64：整体解码后按行重新识别
	if dec, ok := decodeBase64(content); ok {
		nodes := fromLinks(dec, where, cat, out)
		if len(nodes) > 0 || bytes.Contains(dec, []byte("://")) {
			return nodes, FormatBase64
		}
	}
	// 4. 分享链接
	if bytes.Contains(content, []byte("://")) {
		return fromLinks(content, where, cat, out), FormatLinks
	}
	out.Add(diag.NodeUnrecognized, where, "source", where, "preview", Preview(content))
	return nil, FormatUnknown
}

func looksLikeProxyList(n *yaml.Node) bool {
	for _, it := range n.Content {
		if it.Kind == yaml.MappingNode && yamlx.Get(it, "type") != nil {
			return true
		}
	}
	return false
}

// decodeBase64 依次尝试标准与 URL-safe 字母表，容忍缺失的填充与换行。解码结果必须是 UTF-8 文本。
func decodeBase64(b []byte) ([]byte, bool) {
	s := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, string(b))
	if s == "" || strings.Contains(s, "://") {
		return nil, false
	}
	s = strings.TrimRight(s, "=")
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if dec, err := enc.DecodeString(s); err == nil && utf8.Valid(dec) {
			return dec, true
		}
	}
	return nil, false
}

var (
	previewSecret = regexp.MustCompile(`[A-Za-z0-9+/_=-]{16,}`)
	previewUser   = regexp.MustCompile(`://[^\s/@]*@`)
	previewQuery  = regexp.MustCompile(`\?[^\s#]*`)
)

// Preview 返回开头 200 字节的脱敏预览：去掉 URL 中的用户信息与查询串、形似密钥的长串。
func Preview(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
		for !utf8.ValidString(s) && len(s) > 0 {
			s = s[:len(s)-1]
		}
	}
	s = previewUser.ReplaceAllString(s, "://***@")
	s = previewQuery.ReplaceAllString(s, "?***")
	s = previewSecret.ReplaceAllString(s, "***")
	return strings.Join(strings.Fields(s), " ")
}

func fromYAMLList(list *yaml.Node, where string, out *diag.List) []*Node {
	if list.Kind != yaml.SequenceNode {
		out.Add(diag.NodeNotProxy, where, "source", where, "index", "proxies")
		return nil
	}
	var nodes []*Node
	for i, it := range list.Content {
		if it.Kind != yaml.MappingNode || yamlx.Get(it, "name") == nil || yamlx.Get(it, "type") == nil {
			out.Add(diag.NodeNotProxy, where, "source", where, "index", strconv.Itoa(i))
			continue
		}
		if n := build(yamlx.Clone(it), where, fmt.Sprintf("%s:proxies[%d]", where, i), out); n != nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func fromLinks(b []byte, where string, cat *catalog.Catalog, out *diag.List) []*Node {
	var nodes []*Node
	for i, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		res := kernel.ParseLink(line, i+1)
		if res.Err != "" {
			scheme := res.Scheme
			if scheme == "" {
				scheme = "-"
			}
			out.Add(diag.NodeLineSkipped, fmt.Sprintf("%s:%d", where, i+1), "source", where, "line", strconv.Itoa(i+1), "reason", res.Err, "scheme", scheme)
			continue
		}
		m, err := yamlx.FromAny(res.Mapping)
		if err != nil {
			out.Add(diag.NodeLineSkipped, fmt.Sprintf("%s:%d", where, i+1), "source", where, "line", strconv.Itoa(i+1), "reason", "unencodable fields", "scheme", res.Scheme)
			continue
		}
		Coerce(m, proxyField(cat))
		if n := build(m, where, fmt.Sprintf("%s:%d", where, i+1), out); n != nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func proxyField(cat *catalog.Catalog) *catalog.Field {
	if cat == nil {
		return nil
	}
	return cat.Root.Child("proxies", nil).Elem()
}

// build 校验节点并计算属性。校验用 mihomo 的 adapter.ParseProxy；失败的节点进入跳过清单。
func build(m *yaml.Node, where, origin string, out *diag.List) *Node {
	var v map[string]any
	if err := m.Decode(&v); err != nil {
		out.Add(diag.NodeNotProxy, origin, "source", where, "index", origin)
		return nil
	}
	name := fmt.Sprint(v["name"])
	if err := kernel.ValidateProxy(v); err != nil {
		out.Add(diag.NodeInvalid, origin, "source", where, "name", name, "reason", redactWith(kernel.Redact(err.Error()), v))
		return nil
	}
	n := &Node{Mapping: m, Name: name, Type: fmt.Sprint(v["type"]), Server: fmt.Sprint(v["server"]), Origin: origin, GroupID: -1, Rate: 1}
	if p, err := strconv.Atoi(fmt.Sprint(v["port"])); err == nil {
		n.Port = p
	}
	n.Fingerprint, n.DedupKey = fingerprints(v)
	return n
}

// redactWith 把节点的凭据值从错误信息中去掉。
func redactWith(msg string, v map[string]any) string {
	for _, k := range credentialKeys {
		if s, ok := v[k].(string); ok && len(s) >= 3 {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	return msg
}

func fingerprints(v map[string]any) (fp, dedup string) {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%v|%v|%v", v["type"], v["server"], v["port"]) // hash.Hash 的写入不会失败
	for _, k := range credentialKeys {
		if x, ok := v[k]; ok {
			_, _ = fmt.Fprintf(h, "|%s=%v", k, x)
		}
	}
	fp = hex.EncodeToString(h.Sum(nil))[:16]
	d := sha256.New()
	canonical(d, v, "name")
	dedup = hex.EncodeToString(d.Sum(nil))[:16]
	return fp, dedup
}

// canonical 以排序后的键写出值的规范形式（去重键用；与 map 迭代顺序无关）。
func canonical(w interface{ Write([]byte) (int, error) }, v any, skip string) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			if k != skip {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		_, _ = w.Write([]byte("{"))
		for _, k := range keys {
			_, _ = fmt.Fprintf(w, "%q:", k)
			canonical(w, x[k], "")
			_, _ = w.Write([]byte(","))
		}
		_, _ = w.Write([]byte("}"))
	case []any:
		_, _ = w.Write([]byte("["))
		for _, it := range x {
			canonical(w, it, "")
			_, _ = w.Write([]byte(","))
		}
		_, _ = w.Write([]byte("]"))
	default:
		_, _ = fmt.Fprintf(w, "%T:%v", v, v)
	}
}

// Coerce 按目录类型修正字符串形式的数字与布尔（分享链接解析器把端口等写成字符串）。
// 只用于由程序构造的节点；用户提供的 YAML 按原样保留。
func Coerce(n *yaml.Node, f *catalog.Field) {
	if n == nil || f == nil {
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			var ch *catalog.Field
			if f.Type == catalog.TMap {
				ch = f.Values
			} else {
				ch = f.Child(n.Content[i].Value, n)
			}
			Coerce(n.Content[i+1], ch)
		}
	case yaml.SequenceNode:
		for _, it := range n.Content {
			Coerce(it, f.Elem())
		}
	case yaml.ScalarNode:
		if n.ShortTag() != "!!str" {
			return
		}
		switch f.Type {
		case catalog.TInt:
			if _, err := strconv.ParseInt(n.Value, 10, 64); err == nil {
				n.Tag, n.Style = "!!int", 0
			}
		case catalog.TBool:
			if n.Value == "true" || n.Value == "false" {
				n.Tag, n.Style = "!!bool", 0
			}
		}
	}
}

// SetName 修改节点名称（同时更新映射中的 name 键）。
func (n *Node) SetName(name string) {
	n.Name = name
	yamlx.Set(n.Mapping, "name", yamlx.Str(name))
}
