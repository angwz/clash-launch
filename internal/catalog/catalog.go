// Package catalog 加载声明式字段目录（catalog/<pack>/）并回答三个问题：
// 某个位置允许哪些字段、字段是什么类型与含义、字段按什么顺序渲染。
//
// 引擎不认识任何 Clash 字段，全部知识都来自目录数据。目录中字段的书写顺序
// 就是渲染顺序；顶层顺序由 pack.yaml 的 order 列表给出，与 mihomo 官方示例
// 配置 docs/config.yaml 一致（含示例中被注释掉的键）。
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// 字段类型。list/map 的元素类型在 Items/Values 中；oneOf 按 Discriminator 的值选择 Variants；
// anyOf 为无判别键的联合（如「字符串或字符串列表」）。
const (
	TString   = "string"
	TInt      = "int"
	TFloat    = "float"
	TBool     = "bool"
	TEnum     = "enum"
	TDuration = "duration" // 带单位的字符串或整数，具体以字段说明为准
	TList     = "list"
	TMap      = "map"
	TObject   = "object"
	TOneOf    = "oneOf"
	TAnyOf    = "anyOf"
	TAny      = "any"
)

// 重要程度与稳定性的取值（第 8.2 节）。
var (
	Importances = []string{"core", "recommended", "optional", "expert"}
	Stabilities = []string{"stable", "experimental", "deprecated"}
)

// Text 是两种语言的「标题、一句话摘要」。
type Text struct {
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"`
}

// EnumValue 是枚举的一个取值。
type EnumValue struct {
	Value string `yaml:"value" json:"value"`
	EN    string `yaml:"en" json:"-"`
	ZH    string `yaml:"zh" json:"-"`
}

// Example 是可一键填入的示例。
type Example struct {
	Value yaml.Node `yaml:"value" json:"-"`
	EN    string    `yaml:"en" json:"-"`
	ZH    string    `yaml:"zh" json:"-"`
}

// Field 是目录中的一个字段定义。
type Field struct {
	Key           string            `yaml:"key"`
	Include       string            `yaml:"include"` // 仅在 fields 列表中出现：在此位置展开片段
	Keys          []string          `yaml:"keys"`    // 与 include 搭配：只按此顺序取片段中的这些字段
	Type          string            `yaml:"type"`
	Items         *Field            `yaml:"items"`
	Values        *Field            `yaml:"values"` // map 的值类型
	Of            []*Field          `yaml:"of"`     // anyOf 的候选类型
	Enum          []EnumValue       `yaml:"enum"`
	Default       yaml.Node         `yaml:"default"` // Kind 为 0 表示没有默认值
	Importance    string            `yaml:"importance"`
	Stability     string            `yaml:"stability"`
	Since         string            `yaml:"since"`
	Removed       string            `yaml:"removed"`
	Unverified    bool              `yaml:"unverified"`
	Replacement   string            `yaml:"replacement"`
	Examples      []Example         `yaml:"examples"`
	Related       []string          `yaml:"related"`
	Doc           string            `yaml:"doc"`
	Sensitive     bool              `yaml:"sensitive"`
	UI            map[string]string `yaml:"ui"`
	Targets       map[string]string `yaml:"targets"`
	Reason        string            `yaml:"reason"` // type: any 时必须说明原因
	Fields        []*Field          `yaml:"fields"`
	Discriminator string            `yaml:"discriminator"`
	Variants      []*Variant        `yaml:"variants"`
	ZHText        []string          `yaml:"zh"` // [标题, 摘要]
	ENText        []string          `yaml:"en"` // [Title, Summary]
	DetailText    map[string]string `yaml:"detail"`

	// 加载后计算的字段
	ID     string `yaml:"-"` // 稳定的点分标识，也是 i18n 键的后缀
	DocURL string `yaml:"-"`
	index  map[string]*Field
	vindex map[string]*Variant
}

// Variant 是 oneOf 字段的一个分支（如 proxies 中的一种协议）。
type Variant struct {
	Value   string            `yaml:"value"` // 判别键的取值
	Doc     string            `yaml:"doc"`
	Fields  []*Field          `yaml:"fields"`
	Targets map[string]string `yaml:"targets"`
	ZHText  []string          `yaml:"zh"`
	ENText  []string          `yaml:"en"`
	Since   string            `yaml:"since"`

	ID     string `yaml:"-"`
	DocURL string `yaml:"-"`
	index  map[string]*Field
}

// Pack 是格式包元数据（pack.yaml）。
type Pack struct {
	ID        string       `yaml:"id" json:"id"`
	Label     string       `yaml:"label" json:"label"`
	DocRoot   string       `yaml:"doc-root" json:"docRoot"`
	CoreMin   string       `yaml:"core-min" json:"coreMin"`
	CoreMax   string       `yaml:"core-max" json:"coreMax"`
	Reference string       `yaml:"reference" json:"reference"` // 示例配置的来源与版本
	Order     []string     `yaml:"order" json:"order"`
	Files     []string     `yaml:"files" json:"-"`
	Nav       []NavSection `yaml:"nav" json:"nav"`
}

// NavSection 是界面的一个分节（第 16.1 节「分节导航（来自目录）」）：把顶层键分组。
// 每个顶层键恰好属于一个分节；分节内的显示顺序即 keys 的顺序。
// Generated 表示该分节的内容由节点源或规则源生成，表单中只给出入口，不提供逐字段编辑。
type NavSection struct {
	ID        string   `yaml:"id" json:"id"`
	ZH        string   `yaml:"zh" json:"zh"`
	EN        string   `yaml:"en" json:"en"`
	Generated string   `yaml:"generated,omitempty" json:"generated,omitempty"` // sources | rules
	Keys      []string `yaml:"keys" json:"keys"`
}

type file struct {
	Fragments map[string][]*Field `yaml:"fragments"`
	Fields    []*Field            `yaml:"fields"`
}

// Catalog 是加载完成的目录。只读，可并发使用。
type Catalog struct {
	Pack Pack
	Root *Field // 伪字段：顶层映射，Fields 按 Pack.Order 排列
	all  []*Field
}

// Load 从文件系统（通常是嵌入的 catalog/<pack>/）加载目录并做结构校验。
func Load(fsys fs.FS, dir string) (*Catalog, error) {
	var pack Pack
	if err := decodeFile(fsys, path.Join(dir, "pack.yaml"), &pack); err != nil {
		return nil, err
	}
	fragments := map[string][]*Field{}
	top := map[string]*Field{}
	for _, name := range pack.Files {
		var f file
		if err := decodeFile(fsys, path.Join(dir, name), &f); err != nil {
			return nil, err
		}
		for k, v := range f.Fragments {
			if _, dup := fragments[k]; dup {
				return nil, fmt.Errorf("catalog: 片段 %s 重复定义（%s）", k, name)
			}
			fragments[k] = v
		}
		for _, fd := range f.Fields {
			if _, dup := top[fd.Key]; dup {
				return nil, fmt.Errorf("catalog: 顶层字段 %s 重复定义（%s）", fd.Key, name)
			}
			top[fd.Key] = fd
		}
	}
	root := &Field{Key: "", Type: TObject, ID: ""}
	seen := map[string]bool{}
	for _, k := range pack.Order {
		fd, ok := top[k]
		if !ok {
			return nil, fmt.Errorf("catalog: pack.yaml 的 order 中有 %s，但没有任何文件定义它", k)
		}
		if seen[k] {
			return nil, fmt.Errorf("catalog: order 中 %s 重复", k)
		}
		seen[k] = true
		root.Fields = append(root.Fields, fd)
	}
	for k := range top {
		if !seen[k] {
			return nil, fmt.Errorf("catalog: 顶层字段 %s 未出现在 pack.yaml 的 order 中", k)
		}
	}
	if err := checkNav(pack.Nav, top); err != nil {
		return nil, err
	}
	c := &Catalog{Pack: pack, Root: root}
	l := &linker{c: c, fragments: fragments, expanding: map[string]bool{}, fragOf: map[*Field]string{}}
	if err := l.object(root, ""); err != nil {
		return nil, err
	}
	return c, nil
}

// checkNav 校验分节：ID 唯一、标题齐全、generated 取值合法，每个顶层键恰好出现一次。
func checkNav(nav []NavSection, top map[string]*Field) error {
	ids := map[string]bool{}
	where := map[string]string{}
	for _, s := range nav {
		if s.ID == "" || s.ZH == "" || s.EN == "" || len(s.Keys) == 0 {
			return fmt.Errorf("catalog: 分节 %q 缺少 id、标题或 keys", s.ID)
		}
		if ids[s.ID] {
			return fmt.Errorf("catalog: 分节 %s 重复", s.ID)
		}
		ids[s.ID] = true
		if s.Generated != "" && s.Generated != "sources" && s.Generated != "rules" {
			return fmt.Errorf("catalog: 分节 %s 的 generated 只能是 sources 或 rules", s.ID)
		}
		for _, k := range s.Keys {
			if _, ok := top[k]; !ok {
				return fmt.Errorf("catalog: 分节 %s 引用了不存在的顶层字段 %s", s.ID, k)
			}
			if prev, dup := where[k]; dup {
				return fmt.Errorf("catalog: 顶层字段 %s 同时出现在分节 %s 与 %s", k, prev, s.ID)
			}
			where[k] = s.ID
		}
	}
	for k := range top {
		if _, ok := where[k]; !ok {
			return fmt.Errorf("catalog: 顶层字段 %s 不属于任何分节（pack.yaml 的 nav）", k)
		}
	}
	return nil
}

func decodeFile(fsys fs.FS, name string, out any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("catalog: 读取 %s: %w", name, err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("catalog: 解析 %s: %w", name, err)
	}
	return nil
}

// linker 展开片段、计算 ID 与文档链接，并校验每个字段的必填信息。
type linker struct {
	c         *Catalog
	fragments map[string][]*Field
	expanding map[string]bool
	fragOf    map[*Field]string // 片段中的顶层字段 → 片段名；这些字段的 ID 以片段为根，所有引用处共用
}

func (l *linker) expand(fields []*Field, where string) ([]*Field, error) {
	var out []*Field
	for _, f := range fields {
		if f.Include == "" {
			out = append(out, f)
			continue
		}
		if f.Key != "" {
			return nil, fmt.Errorf("catalog: %s 中的 include 项不能同时有 key", where)
		}
		frag, ok := l.fragments[f.Include]
		if !ok {
			return nil, fmt.Errorf("catalog: %s 引用了不存在的片段 %s", where, f.Include)
		}
		if l.expanding[f.Include] {
			return nil, fmt.Errorf("catalog: 片段 %s 循环引用", f.Include)
		}
		l.expanding[f.Include] = true
		inner, err := l.expand(frag, "fragment "+f.Include)
		delete(l.expanding, f.Include)
		if err != nil {
			return nil, err
		}
		if len(f.Keys) > 0 {
			picked := make([]*Field, 0, len(f.Keys))
			for _, k := range f.Keys {
				var hit *Field
				for _, x := range inner {
					if x.Key == k {
						hit = x
					}
				}
				if hit == nil {
					return nil, fmt.Errorf("catalog: %s 从片段 %s 中取的字段 %s 不存在", where, f.Include, k)
				}
				picked = append(picked, hit)
			}
			inner = picked
		}
		for _, x := range inner {
			if _, ok := l.fragOf[x]; !ok {
				l.fragOf[x] = f.Include
			}
		}
		out = append(out, inner...)
	}
	return out, nil
}

func (l *linker) object(f *Field, id string) error {
	fields, err := l.expand(f.Fields, id)
	if err != nil {
		return err
	}
	f.Fields = fields
	f.index = make(map[string]*Field, len(fields))
	for _, ch := range fields {
		if _, dup := f.index[ch.Key]; dup {
			return fmt.Errorf("catalog: %s 下字段 %s 重复", id, ch.Key)
		}
		f.index[ch.Key] = ch
		if err := l.field(ch, l.childID(ch, id), f.DocURL); err != nil {
			return err
		}
	}
	return nil
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

func (l *linker) docURL(doc, inherited string) string {
	switch {
	case doc == "":
		return inherited
	case strings.HasPrefix(doc, "https://"):
		return doc
	case strings.HasPrefix(doc, "#"):
		base, _, _ := strings.Cut(inherited, "#")
		return base + doc
	default:
		page, anchor, _ := strings.Cut(doc, "#")
		u := l.c.Pack.DocRoot + strings.Trim(page, "/") + "/"
		if anchor != "" {
			u += "#" + anchor
		}
		return u
	}
}

func (l *linker) field(f *Field, id, inheritedDoc string) error {
	if f.ID != "" {
		// 同一片段被多处展开时，字段对象是共享的；ID 取首次展开处，
		// 文本只写一份，所有引用处共用（i18n 键不重复）。
		return nil
	}
	f.ID = id
	f.DocURL = l.docURL(f.Doc, inheritedDoc)
	l.c.all = append(l.c.all, f)
	if f.Key == "" {
		return fmt.Errorf("catalog: %s 缺少 key", id)
	}
	if err := checkMeta(f, id); err != nil {
		return err
	}
	switch f.Type {
	case TString, TInt, TFloat, TBool, TDuration:
	case TEnum:
		if len(f.Enum) == 0 {
			return fmt.Errorf("catalog: 枚举 %s 没有取值", id)
		}
	case TAny:
		if f.Reason == "" {
			return fmt.Errorf("catalog: %s 的类型为 any，必须写明原因（reason）", id)
		}
	case TList:
		if f.Items == nil {
			return fmt.Errorf("catalog: 列表 %s 缺少 items", id)
		}
		return l.elem(f.Items, id+"[]", f.DocURL)
	case TMap:
		if f.Values == nil {
			return fmt.Errorf("catalog: 映射 %s 缺少 values", id)
		}
		return l.elem(f.Values, id+".*", f.DocURL)
	case TAnyOf:
		if len(f.Of) < 2 {
			return fmt.Errorf("catalog: anyOf %s 至少要有两个候选", id)
		}
		for i, o := range f.Of {
			if err := l.elem(o, fmt.Sprintf("%s|%d", id, i), f.DocURL); err != nil {
				return err
			}
		}
	case TObject:
		return l.object(f, id)
	case TOneOf:
		return l.oneOf(f, id)
	default:
		return fmt.Errorf("catalog: %s 的类型 %q 无效", id, f.Type)
	}
	return nil
}

// elem 处理 list/map/anyOf 的元素类型：它们没有自己的说明文字，
// 但可以是对象或 oneOf，需要递归链接。
func (l *linker) elem(f *Field, id, doc string) error {
	f.ID = id
	f.DocURL = doc
	switch f.Type {
	case TObject:
		return l.object(f, id)
	case TOneOf:
		return l.oneOf(f, id)
	case TList:
		if f.Items == nil {
			return fmt.Errorf("catalog: 列表 %s 缺少 items", id)
		}
		return l.elem(f.Items, id+"[]", doc)
	case TMap:
		if f.Values == nil {
			return fmt.Errorf("catalog: 映射 %s 缺少 values", id)
		}
		return l.elem(f.Values, id+".*", doc)
	case TEnum:
		if len(f.Enum) == 0 {
			return fmt.Errorf("catalog: 枚举 %s 没有取值", id)
		}
	case TString, TInt, TFloat, TBool, TDuration, TAny:
	case TAnyOf:
		for i, o := range f.Of {
			if err := l.elem(o, fmt.Sprintf("%s|%d", id, i), doc); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("catalog: %s 的元素类型 %q 无效", id, f.Type)
	}
	return nil
}

func (l *linker) oneOf(f *Field, id string) error {
	if f.Discriminator == "" || len(f.Variants) == 0 {
		return fmt.Errorf("catalog: oneOf %s 需要 discriminator 与 variants", id)
	}
	f.vindex = map[string]*Variant{}
	for _, v := range f.Variants {
		if _, dup := f.vindex[v.Value]; dup {
			return fmt.Errorf("catalog: %s 的分支 %s 重复", id, v.Value)
		}
		f.vindex[v.Value] = v
		v.ID = id + "." + v.Value
		v.DocURL = l.docURL(v.Doc, f.DocURL)
		if len(v.ZHText) < 1 || len(v.ENText) < 1 {
			return fmt.Errorf("catalog: 分支 %s 缺少中英文名称", v.ID)
		}
		fields, err := l.expand(v.Fields, v.ID)
		if err != nil {
			return err
		}
		v.Fields = fields
		v.index = make(map[string]*Field, len(fields))
		for _, ch := range fields {
			if _, dup := v.index[ch.Key]; dup {
				return fmt.Errorf("catalog: %s 下字段 %s 重复", v.ID, ch.Key)
			}
			v.index[ch.Key] = ch
			if err := l.field(ch, l.childID(ch, v.ID), v.DocURL); err != nil {
				return err
			}
		}
		if _, ok := v.index[f.Discriminator]; !ok {
			return fmt.Errorf("catalog: 分支 %s 缺少判别字段 %s", v.ID, f.Discriminator)
		}
	}
	return nil
}

func checkMeta(f *Field, id string) error {
	if !slices.Contains(Importances, f.Importance) {
		return fmt.Errorf("catalog: %s 的 importance %q 无效", id, f.Importance)
	}
	if f.Stability == "" {
		f.Stability = "stable"
	}
	if !slices.Contains(Stabilities, f.Stability) {
		return fmt.Errorf("catalog: %s 的 stability %q 无效", id, f.Stability)
	}
	if len(f.ZHText) != 2 || len(f.ENText) != 2 || f.ZHText[1] == "" || f.ENText[1] == "" {
		return fmt.Errorf("catalog: %s 必须有中英文的 [标题, 摘要]", id)
	}
	if f.DocURL == "" {
		return fmt.Errorf("catalog: %s 没有文档链接", id)
	}
	if f.Stability == "deprecated" && f.Replacement == "" {
		return fmt.Errorf("catalog: 已弃用字段 %s 必须写明替代字段（replacement）", id)
	}
	return nil
}

// Fields 返回全部带说明的字段（不含 list/map 的元素类型），按加载顺序。
func (c *Catalog) Fields() []*Field { return slices.Clone(c.all) }

// Child 返回对象中 key 对应的字段定义。对 oneOf，需要传入所在映射节点以读取判别键；
// 判别值未知时返回 nil。
func (f *Field) Child(key string, mapping *yaml.Node) *Field {
	if f == nil {
		return nil
	}
	switch f.Type {
	case TObject:
		return f.index[key]
	case TOneOf:
		v := f.VariantOf(mapping)
		if v == nil {
			return nil
		}
		return v.index[key]
	case TAnyOf:
		for _, o := range f.Of {
			if o.Type == TObject || o.Type == TOneOf {
				if ch := o.Child(key, mapping); ch != nil {
					return ch
				}
			}
		}
	}
	return nil
}

// VariantOf 读取映射中的判别键，返回对应分支。
func (f *Field) VariantOf(mapping *yaml.Node) *Variant {
	if f == nil || f.Type != TOneOf || mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == f.Discriminator {
			return f.vindex[mapping.Content[i+1].Value]
		}
	}
	return nil
}

// Variant 按判别值返回分支。
func (f *Field) Variant(value string) *Variant {
	if f == nil {
		return nil
	}
	return f.vindex[value]
}

// Elem 返回 list 的元素类型或 map 的值类型。
func (f *Field) Elem() *Field {
	if f == nil {
		return nil
	}
	switch f.Type {
	case TList:
		return f.Items
	case TMap:
		return f.Values
	case TAnyOf:
		for _, o := range f.Of {
			if e := o.Elem(); e != nil {
				return e
			}
		}
	}
	return nil
}

// KeyOrder 返回该映射位置的键顺序；未知位置（目录外映射、map 类型）返回 nil，
// 调用方据此保留原有顺序。
func (f *Field) KeyOrder(mapping *yaml.Node) []string {
	if f == nil {
		return nil
	}
	var fields []*Field
	switch f.Type {
	case TObject:
		fields = f.Fields
	case TOneOf:
		if v := f.VariantOf(mapping); v != nil {
			fields = v.Fields
		}
	case TAnyOf:
		for _, o := range f.Of {
			if o.Type == TObject || o.Type == TOneOf {
				return o.KeyOrder(mapping)
			}
		}
	}
	if fields == nil {
		return nil
	}
	out := make([]string, len(fields))
	for i, ch := range fields {
		out[i] = ch.Key
	}
	return out
}

// IsMapping 报告该字段的值是否为映射（object、oneOf 或 map）。
func (f *Field) IsMapping() bool {
	return f != nil && (f.Type == TObject || f.Type == TOneOf || f.Type == TMap)
}

// childID 返回字段的稳定 ID：来自片段的字段以片段为根（fragments.<名>.<键>），
// 其余字段沿所在位置拼接。
func (l *linker) childID(ch *Field, parent string) string {
	if frag, ok := l.fragOf[ch]; ok {
		return "fragments." + frag + "." + ch.Key
	}
	return join(parent, ch.Key)
}

// Digest 返回目录数据的内容摘要（文件路径与内容的 SHA-256），用作缓存键的一部分。
func Digest(fsys fs.FS, dir string) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\n%d\n", p, len(b)) // hash.Hash 的写入不会失败
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
