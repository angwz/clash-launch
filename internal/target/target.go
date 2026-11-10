// Package target 加载目标客户端预设（catalog/<pack>/presets/），并在渲染前按目标适配配置（第 10 节）。
package target

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Preset 是一个目标客户端预设。
type Preset struct {
	ID       string            `yaml:"id" json:"id"`
	Label    map[string]string `yaml:"label" json:"label"` // en、zh
	Platform string            `yaml:"platform" json:"platform"`
	Core     string            `yaml:"core" json:"core"` // mihomo | stash
	Docs     string            `yaml:"docs" json:"docs"`
	// Basis 列出 defaults 与说明中每一项的依据链接（官方文档或源码）。查不到依据的项不写。
	Basis    []string          `yaml:"basis" json:"basis,omitempty"`
	Defaults yaml.Node         `yaml:"defaults" json:"-"` // 新建方案时的表单初值
	Notes    map[string]string `yaml:"notes" json:"notes"`
	Kernel   bool              `yaml:"kernel" json:"kernel"` // 是否有可用的 mihomo 内核检查
}

// MarshalJSON 把 Defaults 转换为普通 JSON 值（yaml.Node 本身的字段对前端没有意义）。
func (p *Preset) MarshalJSON() ([]byte, error) {
	type plain Preset
	return json.Marshal(struct {
		*plain
		Defaults any `json:"defaults,omitempty"`
	}{(*plain)(p), catalog.NodeJSON(&p.Defaults)})
}

// Load 加载格式包下的全部预设，按 ID 排序。
func Load(fsys fs.FS, pack string) ([]*Preset, error) {
	dir := path.Join(pack, "presets")
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("target: 读取预设目录: %w", err)
	}
	var out []*Preset
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		var p Preset
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("target: 解析 %s: %w", e.Name(), err)
		}
		if p.ID == "" || p.ID+".yaml" != e.Name() {
			return nil, fmt.Errorf("target: %s 的 id 必须与文件名一致", e.Name())
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("target: 预设 %s 重复", p.ID)
		}
		if p.Core != "mihomo" && p.Core != "stash" {
			return nil, fmt.Errorf("target: %s 的 core 必须是 mihomo 或 stash", p.ID)
		}
		if p.Label["en"] == "" || p.Label["zh"] == "" {
			return nil, fmt.Errorf("target: %s 缺少中英文名称", p.ID)
		}
		if p.Defaults.Kind != 0 && len(p.Basis) == 0 {
			return nil, fmt.Errorf("target: %s 写了 defaults，但没有依据链接", p.ID)
		}
		seen[p.ID] = true
		pp := p
		out = append(out, &pp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Find 按 ID 查找预设。
func Find(presets []*Preset, id string) *Preset {
	for _, p := range presets {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Adapt 按目标适配合并后的配置（就地修改 root）：
//  1. 内核版本门槛：since 高于所选内核、或 removed 不高于所选内核时警告，不删除；
//  2. 目标不支持的字段：来自表单的删除并逐条报告；来自原始编辑的保留（用户明确要求输出）；
//  3. 已弃用字段：提示替代字段。
//
// 客户端运行时是否改写某个字段不在这里提示：用户写什么就输出什么（反馈 #17）。
//
// touched 是原始编辑写入过的路径（layer.Result.Touched）。
func Adapt(root *yaml.Node, touched []string, cat *catalog.Catalog, p *Preset, core string, out *diag.List) {
	a := &adapter{touched: touched, p: p, core: parseVersion(core), coreText: core, out: out}
	a.walk(root, cat.Root, "")
}

type adapter struct {
	touched  []string
	p        *Preset
	core     []int
	coreText string
	out      *diag.List
}

func (a *adapter) fromRaw(p string) bool {
	for _, t := range a.touched {
		if p == t || strings.HasPrefix(p, t+".") || strings.HasPrefix(p, t+"[") {
			return true
		}
	}
	return false
}

func (a *adapter) walk(n *yaml.Node, f *catalog.Field, p string) {
	if f == nil || n == nil {
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); {
			key := n.Content[i].Value
			var ch *catalog.Field
			if f.Type == catalog.TMap {
				ch = f.Values
			} else {
				ch = f.Child(key, n)
			}
			cp := yamlx.Join(p, key)
			if ch != nil && f.Type != catalog.TMap && a.check(ch, cp) {
				n.Content = append(n.Content[:i], n.Content[i+2:]...)
				continue
			}
			a.walk(n.Content[i+1], ch, cp)
			i += 2
		}
	case yaml.SequenceNode:
		for i, it := range n.Content {
			a.walk(it, f.Elem(), yamlx.Index(p, i))
		}
	}
}

// check 报告字段的版本与支持情况；返回 true 表示应删除该字段。
func (a *adapter) check(f *catalog.Field, p string) bool {
	if mark := f.Targets[a.p.ID]; mark == "unsupported" || f.Targets[a.p.Core] == "unsupported" {
		if !a.fromRaw(p) {
			a.out.Add(diag.TargetUnsupported, p, "path", p, "preset", a.p.ID)
			return true
		}
	}
	if f.Since != "" && len(a.core) > 0 && compare(parseVersion(f.Since), a.core) > 0 {
		a.out.Add(diag.TargetSince, p, "path", p, "since", f.Since, "core", a.coreText)
	}
	if f.Removed != "" && len(a.core) > 0 && compare(parseVersion(f.Removed), a.core) <= 0 {
		a.out.Add(diag.TargetRemoved, p, "path", p, "removed", f.Removed, "core", a.coreText)
	}
	if f.Stability == "deprecated" && f.Removed == "" {
		a.out.Add(diag.TargetDeprecated, p, "path", p, "replacement", f.Replacement)
	}
	return false
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// compare 比较两个版本号：a<b 返回负数。
func compare(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

// CompareVersions 比较两个版本字符串（导出给其他包使用）。无法解析时视为相等。
func CompareVersions(a, b string) int {
	va, vb := parseVersion(a), parseVersion(b)
	if va == nil || vb == nil {
		return 0
	}
	return compare(va, vb)
}

// IDs 返回预设 ID 列表。
func IDs(ps []*Preset) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return slices.Clip(out)
}
