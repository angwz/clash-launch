package layer

import (
	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/yamlx"
)

// Diff 是 Merge 的逆运算：给出原始编辑层 d，使 Merge(base, d) 在结构上等于 edited（反馈 #70）。
// 用于「在完整 YAML 上直接修改」：界面显示生成结果，用户改完后只把差异存为原始编辑层，
// 订阅或规则更新时，这些差异继续叠加在新的生成结果上。
//
// 选择尽量「贴着意图」的写法，使上游变化时修改仍然有意义：
//   - 映射：只写出改动的键；删掉的键写 !delete。
//   - 列表：在末尾追加 → !append；在开头插入 → !prepend；元素都是带唯一 name 的映射且
//     顺序未变 → !patch-by:name（只写改动的元素与字段）；其余情况整体替换。
//
// 返回的映射可能为空（没有任何差异）。base 与 edited 都不会被修改；结果中不带注释。
func Diff(base, edited *yaml.Node) *yaml.Node {
	if base == nil {
		base = yamlx.Map()
	}
	if edited == nil {
		edited = yamlx.Map()
	}
	return diffMap(base, edited)
}

func diffMap(b, e *yaml.Node) *yaml.Node {
	out := yamlx.Map()
	for i := 0; i+1 < len(e.Content); i += 2 {
		k, ev := e.Content[i].Value, e.Content[i+1]
		bv := yamlx.Get(b, k)
		switch {
		case bv == nil:
			yamlx.Set(out, k, bare(ev))
		case Equal(bv, ev):
		default:
			yamlx.Set(out, k, diffValue(bv, ev))
		}
	}
	for i := 0; i+1 < len(b.Content); i += 2 {
		if k := b.Content[i].Value; yamlx.Get(e, k) == nil {
			yamlx.Set(out, k, deleted())
		}
	}
	return out
}

// diffValue 在 b 与 e 不相等时给出写在映射值上的差异。
func diffValue(b, e *yaml.Node) *yaml.Node {
	switch {
	case b.Kind == yaml.MappingNode && e.Kind == yaml.MappingNode:
		return diffMap(b, e)
	case e.Kind == yaml.MappingNode:
		// 映射覆盖非映射：明确整体替换，避免被当成逐键合并
		n := bare(e)
		n.Tag = TagReplace
		return n
	case b.Kind == yaml.SequenceNode && e.Kind == yaml.SequenceNode:
		return diffSeq(b, e)
	}
	return bare(e) // 标量与列表默认替换
}

func diffSeq(b, e *yaml.Node) *yaml.Node {
	nb, ne := len(b.Content), len(e.Content)
	if ne > nb && seqEqual(b.Content, e.Content[:nb]) {
		return tagged(TagAppend, e.Content[nb:])
	}
	if ne > nb && seqEqual(b.Content, e.Content[ne-nb:]) {
		return tagged(TagPrepend, e.Content[:ne-nb])
	}
	if p := patchByName(b, e); p != nil {
		return p
	}
	return bare(e)
}

// patchByName 在两边元素都是带唯一 name 的映射、且合并后顺序与 e 一致时，给出 !patch-by:name 差异。
// patch-by 保留 base 的顺序并把新元素追加到末尾，因此只有 e 的顺序恰好如此时才能使用。
func patchByName(b, e *yaml.Node) *yaml.Node {
	bn, ok1 := names(b)
	en, ok2 := names(e)
	if !ok1 || !ok2 {
		return nil
	}
	inE := map[string]bool{}
	for _, n := range en {
		inE[n] = true
	}
	inB := map[string]int{}
	var expect []string // patch-by 合并后的顺序：base 中保留的元素，然后是新元素
	for i, n := range bn {
		inB[n] = i
		if inE[n] {
			expect = append(expect, n)
		}
	}
	for _, n := range en {
		if _, ok := inB[n]; !ok {
			expect = append(expect, n)
		}
	}
	for i := range en {
		if en[i] != expect[i] {
			return nil
		}
	}
	out := &yaml.Node{Kind: yaml.SequenceNode, Tag: TagPatchBy + "name"}
	for i, n := range en {
		ev := e.Content[i]
		j, ok := inB[n]
		if !ok {
			out.Content = append(out.Content, bare(ev))
			continue
		}
		d := diffMap(b.Content[j], ev)
		if len(d.Content) == 0 {
			continue
		}
		// 元素差异必须带上匹配用的 name，并放在第一位便于阅读
		el := yamlx.Map("name", n)
		el.Content[1] = bare(yamlx.Get(ev, "name"))
		for k := 0; k+1 < len(d.Content); k += 2 {
			if d.Content[k].Value != "name" {
				el.Content = append(el.Content, d.Content[k], d.Content[k+1])
			}
		}
		out.Content = append(out.Content, el)
	}
	for _, n := range bn {
		if !inE[n] {
			el := bare(b.Content[inB[n]])
			el.Content = []*yaml.Node{yamlx.Str("name"), bare(yamlx.Get(b.Content[inB[n]], "name"))}
			el.Tag = TagDelete
			out.Content = append(out.Content, el)
		}
	}
	return out
}

// names 返回列表中每个元素的 name；有元素不是带标量 name 的映射或 name 重复时 ok=false。
func names(s *yaml.Node) ([]string, bool) {
	seen := map[string]bool{}
	out := make([]string, 0, len(s.Content))
	for _, it := range s.Content {
		if it.Kind != yaml.MappingNode {
			return nil, false
		}
		v := yamlx.Get(it, "name")
		if v == nil || v.Kind != yaml.ScalarNode || seen[v.Value] {
			return nil, false
		}
		seen[v.Value] = true
		out = append(out, v.Value)
	}
	return out, true
}

func tagged(tag string, items []*yaml.Node) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: tag}
	for _, it := range items {
		n.Content = append(n.Content, bare(it))
	}
	return n
}

func deleted() *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: TagDelete, Value: ""}
}

// bare 复制节点并去掉注释与位置信息：差异是机器生成的，原文中的注释（例如文件头）不应混入。
func bare(n *yaml.Node) *yaml.Node {
	c := yamlx.Clone(n)
	var strip func(*yaml.Node)
	strip = func(x *yaml.Node) {
		x.HeadComment, x.LineComment, x.FootComment = "", "", ""
		x.Line, x.Column = 0, 0
		for _, ch := range x.Content {
			strip(ch)
		}
	}
	strip(c)
	return c
}

// Equal 比较两棵已展开的 YAML 树的结构与值：映射不计键的顺序，标量比较规范标签与文本，
// 不比较样式（引号、块/流）与注释。
func Equal(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.ShortTag() == b.ShortTag() && a.Value == b.Value
	case yaml.SequenceNode:
		return seqEqual(a.Content, b.Content)
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := 0; i+1 < len(a.Content); i += 2 {
			bv := yamlx.Get(b, a.Content[i].Value)
			if bv == nil || !Equal(a.Content[i+1], bv) {
				return false
			}
		}
		return true
	}
	return false
}

func seqEqual(a, b []*yaml.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
