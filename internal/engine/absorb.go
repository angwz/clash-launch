package engine

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/layer"
	"github.com/angwz/clash-launch/internal/target"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// ErrAbsorb 表示用户编辑的完整 YAML 无法使用（语法错误、顶层不是映射）；详情在诊断中。
var ErrAbsorb = errors.New("engine: 完整 YAML 无效")

// Absorb 把用户在完整 YAML（与预览一致）上的修改换算成原始编辑层（反馈 #70）。
//
// 比较的基准是「没有原始编辑时」的生成结果（同样经过目标适配），差异由 layer.Diff 给出；
// 因此只保存用户改动的部分，订阅与规则更新后修改继续生效。
// 预览中以注释显示的未启用分节不在可编辑的正文里：用户没有写出它们时，沿用原有的原始编辑，
// 不当作删除。最后用新的原始编辑重新合并一遍，没能出现在最终配置中的修改逐条报告，不静默丢弃。
func Absorb(in Input, full []byte) (string, []diag.Diagnostic, error) {
	var out, scratch diag.List
	base, preset, err := prepare(in, &scratch)
	if err != nil {
		return "", scratch.Items(), err
	}
	edited, err := yamlx.Parse(full, yamlx.DefaultLimits, "yaml", &out)
	if err != nil {
		return "", out.Items(), ErrAbsorb
	}
	if edited == nil {
		edited = yamlx.Map()
	}
	if edited.Kind != yaml.MappingNode {
		out.Add(diag.YAMLNotMapping, "yaml")
		return "", out.Items(), ErrAbsorb
	}
	cur, err := parseRaw(in.Profile.Raw, &scratch)
	if err != nil {
		cur = nil // 原有的原始编辑本身无效：以用户现在看到的完整 YAML 为准
	}
	core := in.Profile.Target.Core

	// 当前预览中被写成注释的分节
	now := layer.Merge(base, cur, in.Catalog, &scratch)
	target.Adapt(now.Node, now.Touched, in.Catalog, preset, core, &scratch)
	hidden := keysOf(pruneDisabled(now.Node, &scratch))

	ref := yamlx.Clone(base)
	target.Adapt(ref, nil, in.Catalog, preset, core, &scratch)

	view := yamlx.Clone(edited)
	for _, k := range hidden {
		if yamlx.Get(edited, k) == nil {
			yamlx.Delete(view, k)
			if v := yamlx.Get(ref, k); v != nil {
				yamlx.Set(view, k, yamlx.Clone(v)) // 视为未改动
			}
		}
	}
	d := layer.Diff(ref, view)
	if cur != nil {
		for _, k := range hidden {
			if yamlx.Get(edited, k) == nil {
				if v := yamlx.Get(cur, k); v != nil {
					yamlx.Set(d, k, yamlx.Clone(v))
				}
			}
		}
	}

	// 验证：用新的原始编辑走一遍同样的合并与适配，和用户写下的内容比较
	next := layer.Merge(base, d, in.Catalog, &scratch)
	target.Adapt(next.Node, next.Touched, in.Catalog, preset, core, &scratch)
	pruneDisabled(next.Node, &scratch)
	want := yamlx.Clone(edited)
	for _, k := range hidden {
		if yamlx.Get(edited, k) == nil {
			yamlx.Delete(next.Node, k)
		}
	}
	for _, p := range mismatches(next.Node, want, "") {
		out.Add(diag.LayerUnapplied, p, "path", p)
	}

	if len(d.Content) == 0 {
		return "", out.Items(), nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(d); err != nil {
		return "", out.Items(), fmt.Errorf("engine: 写出原始编辑：%w", err)
	}
	if err := enc.Close(); err != nil {
		return "", out.Items(), fmt.Errorf("engine: 写出原始编辑：%w", err)
	}
	return buf.String(), out.Items(), nil
}

func keysOf(m map[string]*yaml.Node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mismatches 列出 got 与 want 不一致的路径：映射逐键深入，其余在第一个不一致处停止。
func mismatches(got, want *yaml.Node, path string) []string {
	if layer.Equal(got, want) {
		return nil
	}
	if got == nil || want == nil || got.Kind != yaml.MappingNode || want.Kind != yaml.MappingNode {
		if path == "" {
			path = "."
		}
		return []string{path}
	}
	var out []string
	seen := map[string]bool{}
	for i := 0; i+1 < len(want.Content); i += 2 {
		k := want.Content[i].Value
		seen[k] = true
		out = append(out, mismatches(yamlx.Get(got, k), want.Content[i+1], yamlx.Join(path, k))...)
	}
	for i := 0; i+1 < len(got.Content); i += 2 {
		if k := got.Content[i].Value; !seen[k] {
			out = append(out, yamlx.Join(path, k))
		}
	}
	return out
}
