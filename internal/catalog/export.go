package catalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// 目录导出（第 8.4 节）：同一份目录生成前端表单元数据、JSON Schema 与 i18n 源消息。
// 导出物由构建生成，不手工编辑。说明文字只出现在消息里，元数据中以消息键引用。

// MetaField 是表单元数据中的一个字段。
type MetaField struct {
	Key        string            `json:"key"`
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Importance string            `json:"importance,omitempty"`
	Stability  string            `json:"stability,omitempty"`
	Since      string            `json:"since,omitempty"`
	Removed    string            `json:"removed,omitempty"`
	Replace    string            `json:"replacement,omitempty"`
	Default    any               `json:"default,omitempty"`
	Enum       []string          `json:"enum,omitempty"`
	Examples   []any             `json:"examples,omitempty"`
	Related    []string          `json:"related,omitempty"`
	Doc        string            `json:"doc,omitempty"`
	Sensitive  bool              `json:"sensitive,omitempty"`
	UI         map[string]string `json:"ui,omitempty"`
	Targets    map[string]string `json:"targets,omitempty"`
	Fields     []*MetaField      `json:"fields,omitempty"`
	Items      *MetaField        `json:"items,omitempty"`
	Values     *MetaField        `json:"values,omitempty"`
	Of         []*MetaField      `json:"of,omitempty"`
	Disc       string            `json:"discriminator,omitempty"`
	Variants   []*MetaVariant    `json:"variants,omitempty"`
	HasDetail  bool              `json:"hasDetail,omitempty"`
}

// MetaVariant 是 oneOf 的一个分支。
type MetaVariant struct {
	Value  string       `json:"value"`
	ID     string       `json:"id"`
	Doc    string       `json:"doc,omitempty"`
	Fields []*MetaField `json:"fields"`
}

// Sections 返回按顶层键拆分的表单元数据（供前端按分节懒加载），顺序即渲染顺序。
func (c *Catalog) Sections() []*MetaField {
	out := make([]*MetaField, 0, len(c.Root.Fields))
	for _, f := range c.Root.Fields {
		out = append(out, meta(f))
	}
	return out
}

func meta(f *Field) *MetaField {
	if f == nil {
		return nil
	}
	m := &MetaField{
		Key: f.Key, ID: f.ID, Type: f.Type, Importance: f.Importance, Stability: f.Stability,
		Since: f.Since, Removed: f.Removed, Replace: f.Replacement, Related: f.Related, Doc: f.DocURL,
		Sensitive: f.Sensitive, UI: f.UI, Targets: f.Targets, Disc: f.Discriminator, HasDetail: len(f.DetailText) > 0,
	}
	if f.Stability == "stable" {
		m.Stability = ""
	}
	if f.Default.Kind != 0 {
		m.Default = nodeValue(&f.Default)
	}
	for _, e := range f.Enum {
		m.Enum = append(m.Enum, e.Value)
	}
	for i := range f.Examples {
		m.Examples = append(m.Examples, nodeValue(&f.Examples[i].Value))
	}
	for _, ch := range f.Fields {
		m.Fields = append(m.Fields, meta(ch))
	}
	m.Items, m.Values = meta(f.Items), meta(f.Values)
	for _, o := range f.Of {
		m.Of = append(m.Of, meta(o))
	}
	for _, v := range f.Variants {
		mv := &MetaVariant{Value: v.Value, ID: v.ID, Doc: v.DocURL}
		for _, ch := range v.Fields {
			mv.Fields = append(mv.Fields, meta(ch))
		}
		m.Variants = append(m.Variants, mv)
	}
	return m
}

func nodeValue(n *yaml.Node) any {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil
	}
	return jsonable(v)
}

// jsonable 把 yaml 解码出的 map[string]any 等转换为可被 encoding/json 编码的形式。
func jsonable(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = jsonable(x)
		}
		return out
	case []any:
		for i := range v {
			v[i] = jsonable(v[i])
		}
	}
	return v
}

// Messages 返回 i18n 源消息：键 → 文本。lang 为 "en" 或 "zh-CN"。
// 键的形式：catalog.<id>.title / .summary / .detail / .enum.<值> / .example.<序号>。
func (c *Catalog) Messages(lang string) map[string]string {
	out := map[string]string{}
	pick := func(f *Field) []string {
		if lang == "zh-CN" {
			return f.ZHText
		}
		return f.ENText
	}
	for _, f := range c.all {
		base := "catalog." + f.ID
		t := pick(f)
		out[base+".title"] = t[0]
		out[base+".summary"] = t[1]
		if d := f.DetailText[map[bool]string{true: "zh", false: "en"}[lang == "zh-CN"]]; d != "" {
			out[base+".detail"] = d
		}
		for _, e := range f.Enum {
			txt := e.EN
			if lang == "zh-CN" {
				txt = e.ZH
			}
			out[base+".enum."+e.Value] = txt
		}
		for i, e := range f.Examples {
			txt := e.EN
			if lang == "zh-CN" {
				txt = e.ZH
			}
			out[base+".example."+strconv.Itoa(i)] = txt
		}
		c.variantMessages(f, lang, out)
	}
	return out
}

func (c *Catalog) variantMessages(f *Field, lang string, out map[string]string) {
	walk := func(x *Field) {
		if x == nil || x.Type != TOneOf {
			return
		}
		for _, v := range x.Variants {
			t := v.ENText
			if lang == "zh-CN" {
				t = v.ZHText
			}
			out["catalog."+v.ID+".title"] = t[0]
		}
	}
	walk(f)
	walk(f.Items)
	walk(f.Values)
}

// JSONSchema 返回 draft 2020-12 的 JSON Schema。目录外的字段是允许的（原始编辑可以写任何字段），
// 因此对象不设 additionalProperties: false；编辑器用它做补全、悬浮说明与类型检查。
func (c *Catalog) JSONSchema(lang string) map[string]any {
	msgs := c.Messages(lang)
	root := schemaOf(c.Root, msgs)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "https://github.com/angwz/clash-launch/catalog/" + c.Pack.ID + ".schema.json"
	root["title"] = c.Pack.Label + " configuration (" + c.Pack.CoreMax + ")"
	return root
}

func schemaOf(f *Field, msgs map[string]string) map[string]any {
	s := map[string]any{}
	if f.ID != "" {
		if t := msgs["catalog."+f.ID+".title"]; t != "" {
			s["title"] = t
		}
		if d := msgs["catalog."+f.ID+".summary"]; d != "" {
			s["description"] = d
		}
	}
	if f.Stability == "deprecated" {
		s["deprecated"] = true
	}
	switch f.Type {
	case TString:
		s["type"] = "string"
	case TInt:
		s["type"] = "integer"
	case TFloat:
		s["type"] = "number"
	case TBool:
		s["type"] = "boolean"
	case TDuration:
		s["type"] = []string{"string", "integer"}
	case TEnum:
		vals := make([]string, len(f.Enum))
		for i, e := range f.Enum {
			vals[i] = e.Value
		}
		s["enum"] = vals
	case TList:
		s["type"] = "array"
		s["items"] = schemaOf(f.Items, msgs)
	case TMap:
		s["type"] = "object"
		s["additionalProperties"] = schemaOf(f.Values, msgs)
	case TObject:
		s["type"] = "object"
		props := map[string]any{}
		for _, ch := range f.Fields {
			props[ch.Key] = schemaOf(ch, msgs)
		}
		s["properties"] = props
	case TAnyOf:
		var of []any
		for _, o := range f.Of {
			of = append(of, schemaOf(o, msgs))
		}
		s["anyOf"] = of
	case TOneOf:
		s["type"] = "object"
		var all []any
		for _, v := range f.Variants {
			props := map[string]any{}
			for _, ch := range v.Fields {
				props[ch.Key] = schemaOf(ch, msgs)
			}
			all = append(all, map[string]any{
				"if":   map[string]any{"properties": map[string]any{f.Discriminator: map[string]any{"const": v.Value}}, "required": []string{f.Discriminator}},
				"then": map[string]any{"properties": props},
			})
		}
		s["allOf"] = all
	}
	return s
}

// MarshalStable 以稳定的键序编码 JSON（encoding/json 对 map 按键排序），末尾带换行。
func MarshalStable(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// MessageKeys 返回排序后的消息键（测试与差异比较用）。
func MessageKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String 用于调试输出。
func (f *Field) String() string { return fmt.Sprintf("%s(%s)", f.ID, f.Type) }
