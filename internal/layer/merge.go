// Package layer 实现两层编辑的合并：表单层的结果之上，叠加原始编辑层（后备隐藏能源）。
//
// 合并规则（第 9.2 节）：映射递归合并；标量与列表替换；null 设为 null。
// 合并标签改变行为，且只能出现在映射的值上与 !patch-by 列表的元素上：
//
//	!replace         映射、列表   整体替换，不递归
//	!append          列表         追加到原列表末尾
//	!prepend         列表         插入到原列表开头
//	!delete          任意         删除该键（值被忽略）
//	!patch-by:<键>   映射的列表   按键找到元素并递归合并，找不到则追加；元素带 !delete 时删除该元素
//
// 合并是纯函数：输入不被修改，相同输入得到相同输出。新键追加在映射末尾；
// 最终的键顺序由渲染器按目录统一排列（目录外的键保持出现顺序，排在末尾）。
package layer

import (
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 合并标签。
const (
	TagReplace = "!replace"
	TagAppend  = "!append"
	TagPrepend = "!prepend"
	TagDelete  = "!delete"
	TagPatchBy = "!patch-by:"
)

// Result 是合并的结果。
type Result struct {
	Node *yaml.Node // 合并后的顶层映射
	// Touched 是原始编辑写入或修改过的配置路径（界面用「⚡」标记）。按产生顺序，不重复。
	Touched []string
}

type merger struct {
	cat     *catalog.Catalog
	out     *diag.List
	touched []string
	seen    map[string]bool
}

// Merge 把原始编辑层 raw（顶层映射，可为 nil）合并到 base（顶层映射）之上。
// base 与 raw 都不会被修改。
func Merge(base, raw *yaml.Node, cat *catalog.Catalog, out *diag.List) Result {
	m := &merger{cat: cat, out: out, seen: map[string]bool{}}
	result := yamlx.Clone(base)
	if result == nil {
		result = yamlx.Map()
	}
	if raw == nil {
		return Result{Node: result}
	}
	if raw.Kind != yaml.MappingNode {
		out.Add(diag.YAMLNotMapping, "raw")
		return Result{Node: result}
	}
	if isCustomTag(raw.Tag) {
		out.Add(diag.LayerTagPlacement, "raw", "tag", raw.Tag)
		return Result{Node: result}
	}
	var root *catalog.Field
	if cat != nil {
		root = cat.Root
	}
	m.mapping(result, raw, root, "")
	return Result{Node: result, Touched: m.touched}
}

func (m *merger) touch(path string) {
	if !m.seen[path] {
		m.seen[path] = true
		m.touched = append(m.touched, path)
	}
}

// isCustomTag 区分显式的自定义标签（!replace）与 YAML 标准标签（!!str）及解析器推断的标签。
func isCustomTag(tag string) bool {
	return strings.HasPrefix(tag, "!") && !strings.HasPrefix(tag, "!!")
}

// opOf 解析值上的合并标签；未知标签返回 ok=false。
func opOf(tag string) (op, arg string, ok bool) {
	switch {
	case !isCustomTag(tag):
		return "", "", true
	case tag == TagReplace, tag == TagAppend, tag == TagPrepend, tag == TagDelete:
		return tag, "", true
	case strings.HasPrefix(tag, TagPatchBy) && len(tag) > len(TagPatchBy):
		return TagPatchBy, tag[len(TagPatchBy):], true
	}
	return "", "", false
}

// mapping 把 patch 映射合并进 dst（就地修改 dst，dst 是副本）。
func (m *merger) mapping(dst, patch *yaml.Node, field *catalog.Field, path string) {
	for i := 0; i+1 < len(patch.Content); i += 2 {
		k, v := patch.Content[i], patch.Content[i+1]
		if isCustomTag(k.Tag) {
			m.out.Add(diag.LayerTagPlacement, yamlx.Join(path, k.Value), "tag", k.Tag)
			continue
		}
		key := k.Value
		p := yamlx.Join(path, key)
		child := field.Child(key, dst)
		if field != nil && child == nil && field.Type != catalog.TMap && field.Type != catalog.TAny && field.KeyOrder(dst) != nil {
			m.out.Add(diag.LayerOffCatalog, p, "path", p)
		}
		if field != nil && field.Type == catalog.TMap {
			child = field.Values
		}
		op, arg, ok := opOf(v.Tag)
		if !ok {
			m.out.Add(diag.LayerUnknownTag, p, "tag", v.Tag)
			continue
		}
		existing := yamlx.Get(dst, key)
		switch op {
		case TagDelete:
			if !yamlx.Delete(dst, key) {
				m.out.Add(diag.LayerDeleteMiss, p, "path", p)
			}
			m.touch(p)
		case TagReplace:
			if v.Kind != yaml.MappingNode && v.Kind != yaml.SequenceNode {
				m.out.Add(diag.LayerTagKind, p, "tag", op, "kind", yamlx.KindName(v))
				continue
			}
			yamlx.Set(dst, key, m.fresh(v, child, p))
			m.touch(p)
		case TagAppend, TagPrepend:
			if v.Kind != yaml.SequenceNode {
				m.out.Add(diag.LayerTagKind, p, "tag", op, "kind", yamlx.KindName(v))
				continue
			}
			if existing != nil && existing.Kind != yaml.SequenceNode && !isNull(existing) {
				m.out.Add(diag.LayerTagKind, p, "tag", op, "kind", yamlx.KindName(existing))
				continue
			}
			items := m.fresh(v, child, p).Content
			merged := yamlx.Seq()
			if existing != nil && existing.Kind == yaml.SequenceNode {
				merged = yamlx.Clone(existing)
			}
			if op == TagAppend {
				merged.Content = append(merged.Content, items...)
			} else {
				merged.Content = append(append([]*yaml.Node{}, items...), merged.Content...)
			}
			yamlx.Set(dst, key, merged)
			m.touch(p)
		case TagPatchBy:
			if v.Kind != yaml.SequenceNode {
				m.out.Add(diag.LayerPatchTarget, p, "key", arg)
				continue
			}
			if existing != nil && existing.Kind != yaml.SequenceNode && !isNull(existing) {
				m.out.Add(diag.LayerPatchTarget, p, "key", arg)
				continue
			}
			base := yamlx.Seq()
			if existing != nil && existing.Kind == yaml.SequenceNode {
				base = existing
			}
			yamlx.Set(dst, key, m.patchBy(base, v, arg, child.Elem(), p))
			m.touch(p)
		default:
			if v.Kind == yaml.MappingNode && existing != nil && existing.Kind == yaml.MappingNode {
				m.mapping(existing, v, child, p)
				continue
			}
			yamlx.Set(dst, key, m.fresh(v, child, p))
			m.touch(p)
		}
	}
}

func isNull(n *yaml.Node) bool { return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" }

// fresh 返回 v 作为新值写入时的结果：等价于把 v 合并到「空」之上。
// 这样新插入的映射内部的合并标签同样生效（例如 !append 到不存在的列表 = 直接设为该列表），
// 列表元素与映射键上的标签则按位置规则报错。
func (m *merger) fresh(v *yaml.Node, field *catalog.Field, path string) *yaml.Node {
	switch v.Kind {
	case yaml.MappingNode:
		dst := yamlx.Map()
		dst.Style = v.Style &^ yaml.TaggedStyle
		m.mapping(dst, v, field, path)
		return dst
	case yaml.SequenceNode:
		s := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: v.Style &^ yaml.TaggedStyle}
		elem := field.Elem()
		for i, it := range v.Content {
			ip := yamlx.Index(path, i)
			if isCustomTag(it.Tag) {
				m.out.Add(diag.LayerTagPlacement, ip, "tag", it.Tag)
				continue
			}
			s.Content = append(s.Content, m.fresh(it, elem, ip))
		}
		return s
	default:
		c := yamlx.Clone(v)
		if isCustomTag(c.Tag) {
			// 标量上的合并标签已在 mapping 中处理；能走到这里说明标签位置不对。
			m.out.Add(diag.LayerTagPlacement, path, "tag", c.Tag)
			c.Tag = ""
		}
		c.Style &^= yaml.TaggedStyle
		return c
	}
}

// patchBy 实现 !patch-by:<键>：按键匹配元素并递归合并；元素带 !delete 时删除。
func (m *merger) patchBy(base, patch *yaml.Node, key string, elem *catalog.Field, path string) *yaml.Node {
	out := yamlx.Clone(base)
	for _, it := range out.Content {
		if it.Kind != yaml.MappingNode {
			m.out.Add(diag.LayerPatchTarget, path, "key", key)
			return out
		}
	}
	for i, e := range patch.Content {
		ip := yamlx.Index(path, i)
		op, _, ok := opOf(e.Tag)
		if !ok || (op != "" && op != TagDelete) {
			m.out.Add(diag.LayerTagPlacement, ip, "tag", e.Tag)
			continue
		}
		if e.Kind != yaml.MappingNode {
			m.out.Add(diag.LayerPatchKey, ip, "key", key)
			continue
		}
		id := yamlx.Get(e, key)
		if id == nil || id.Kind != yaml.ScalarNode {
			m.out.Add(diag.LayerPatchKey, ip, "key", key)
			continue
		}
		at := -1
		for j, it := range out.Content {
			if v := yamlx.Get(it, key); v != nil && v.Kind == yaml.ScalarNode && v.Value == id.Value {
				at = j
				break
			}
		}
		ep := path + "[" + key + "=" + id.Value + "]"
		switch {
		case op == TagDelete && at >= 0:
			out.Content = append(out.Content[:at], out.Content[at+1:]...)
			m.touch(ep)
		case op == TagDelete:
			m.out.Add(diag.LayerDeleteMiss, ep, "path", ep)
		case at >= 0:
			m.mapping(out.Content[at], e, elem, ep)
			m.touch(ep)
		default:
			out.Content = append(out.Content, m.fresh(e, elem, ep))
			m.touch(ep)
		}
	}
	return out
}
