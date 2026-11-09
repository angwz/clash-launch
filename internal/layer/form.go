package layer

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Generated 是由节点与规则生成器产出的顶层分节。表单层不能直接设置它们：
// 它们的内容来自源与规则源，修改请用原始编辑（!patch-by、!prepend 等）。
var Generated = []string{"proxies", "proxy-groups", "proxy-providers", "rule-providers", "rules"}

// ValidateForm 按目录校验表单层：只保留目录中存在、类型正确的字段。
// 不合格的字段被去掉，并以 error 诊断说明原因（不静默丢弃）；渲染照常继续。
// 返回的映射是副本。
func ValidateForm(form *yaml.Node, cat *catalog.Catalog, out *diag.List) *yaml.Node {
	result := yamlx.Map()
	if form == nil {
		return result
	}
	if form.Kind != yaml.MappingNode {
		out.Add(diag.FormNotMapping, "form")
		return result
	}
	for i := 0; i+1 < len(form.Content); i += 2 {
		k, v := form.Content[i], form.Content[i+1]
		if slices.Contains(Generated, k.Value) {
			out.Add(diag.FormGenerated, k.Value, "path", k.Value)
			continue
		}
		f := cat.Root.Child(k.Value, form)
		if f == nil {
			out.Add(diag.FormUnknownField, k.Value, "path", k.Value)
			continue
		}
		if c := checkValue(v, f, k.Value, out); c != nil {
			yamlx.Set(result, k.Value, c)
		}
	}
	return result
}

// checkValue 返回校验后的副本；不合格时返回 nil 并写入诊断。对象中不合格的子字段被单独去掉。
func checkValue(v *yaml.Node, f *catalog.Field, path string, out *diag.List) *yaml.Node {
	if isCustomTag(v.Tag) {
		out.Add(diag.LayerTagPlacement, path, "tag", v.Tag)
		return nil
	}
	bad := func(want string) *yaml.Node {
		out.Add(diag.FormValueType, path, "path", path, "want", want, "got", describe(v))
		return nil
	}
	switch f.Type {
	case catalog.TAny:
		return yamlx.Clone(v)
	case catalog.TString:
		if v.Kind != yaml.ScalarNode || v.ShortTag() == "!!null" {
			return bad("string")
		}
		c := yamlx.Clone(v)
		c.Tag = "!!str" // 表单中的标量按字符串保存（例如 "1.0" 不应变成浮点数）
		return c
	case catalog.TInt:
		if v.Kind != yaml.ScalarNode || v.ShortTag() != "!!int" {
			return bad("int")
		}
	case catalog.TFloat:
		if v.Kind != yaml.ScalarNode || (v.ShortTag() != "!!float" && v.ShortTag() != "!!int") {
			return bad("float")
		}
	case catalog.TBool:
		if v.Kind != yaml.ScalarNode || v.ShortTag() != "!!bool" {
			return bad("bool")
		}
	case catalog.TDuration:
		if v.Kind != yaml.ScalarNode || (v.ShortTag() != "!!int" && v.ShortTag() != "!!str") {
			return bad("duration")
		}
	case catalog.TEnum:
		if v.Kind != yaml.ScalarNode {
			return bad("enum")
		}
		values := make([]string, len(f.Enum))
		for i, e := range f.Enum {
			values[i] = e.Value
		}
		if !slices.Contains(values, v.Value) {
			out.Add(diag.FormEnumValue, path, "path", path, "values", strings.Join(values, ", "))
			return nil
		}
		c := yamlx.Clone(v)
		c.Tag = "!!str"
		return c
	case catalog.TList:
		if v.Kind != yaml.SequenceNode {
			return bad("list")
		}
		s := yamlx.Seq()
		for i, it := range v.Content {
			if c := checkValue(it, f.Items, yamlx.Index(path, i), out); c != nil {
				s.Content = append(s.Content, c)
			}
		}
		return s
	case catalog.TMap:
		if v.Kind != yaml.MappingNode {
			return bad("map")
		}
		m := yamlx.Map()
		for i := 0; i+1 < len(v.Content); i += 2 {
			p := yamlx.Join(path, v.Content[i].Value)
			if c := checkValue(v.Content[i+1], f.Values, p, out); c != nil {
				yamlx.Set(m, v.Content[i].Value, c)
			}
		}
		return m
	case catalog.TObject, catalog.TOneOf:
		if v.Kind != yaml.MappingNode {
			return bad("object")
		}
		m := yamlx.Map()
		for i := 0; i+1 < len(v.Content); i += 2 {
			key := v.Content[i].Value
			p := yamlx.Join(path, key)
			ch := f.Child(key, v)
			if ch == nil {
				out.Add(diag.FormUnknownField, p, "path", p)
				continue
			}
			if c := checkValue(v.Content[i+1], ch, p, out); c != nil {
				yamlx.Set(m, key, c)
			}
		}
		return m
	case catalog.TAnyOf:
		for _, o := range f.Of {
			var probe diag.List
			if c := checkValue(v, o, path, &probe); c != nil && probe.Len() == 0 {
				return c
			}
		}
		return bad(anyOfName(f))
	}
	return yamlx.Clone(v)
}

func anyOfName(f *catalog.Field) string {
	names := make([]string, len(f.Of))
	for i, o := range f.Of {
		names[i] = o.Type
	}
	return strings.Join(names, " | ")
}

func describe(v *yaml.Node) string {
	if v.Kind == yaml.ScalarNode {
		return strings.TrimPrefix(v.ShortTag(), "!!")
	}
	return yamlx.KindName(v)
}
