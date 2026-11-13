// Package engine 编排生成管线的纯函数部分（第 7 节）：
//
//	生成部分（节点、策略组、规则）─▶ 表单层 ─▶ 原始编辑层 ─▶ 目标适配 ─▶ 渲染 ─▶ 格式自检
//
// 不变量：相同的输入得到逐字节相同的输出；不联网、不读写数据库、不读取当前时间与随机数
// （文件头时间来自输入快照）。内核检查需要启动进程，由调用方在 Render 之后进行。
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/layer"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/render"
	"github.com/angwz/clash-launch/internal/target"
	"github.com/angwz/clash-launch/internal/version"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Input 是一次渲染的全部输入。它们都是值，不含任何连接或句柄。
type Input struct {
	Profile *profile.Profile
	Catalog *catalog.Catalog
	Presets []*target.Preset
	// Generated 是节点与规则生成器产出的分节（proxies、proxy-groups、proxy-providers、
	// rule-providers、rules），由 Build 从源与规则源的快照得到；可以为 nil。
	Generated *yaml.Node
	// Diagnostics 是生成阶段已经产生的诊断（跳过的节点、展开策略组时的警告……）。
	Diagnostics []diag.Diagnostic
	// Updated 是输入快照中最新的时间戳，写入文件头；零值时文件头不含 UPDATED。
	Updated time.Time
}

// Output 是渲染结果。
type Output struct {
	YAML        []byte            `json:"-"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	Touched     []string          `json:"touched"` // 来自原始编辑的路径（⚡）
	Preset      *target.Preset    `json:"-"`
}

// ErrFatal 表示出现了致命错误（原始编辑的 YAML 语法错误、格式自检失败）：没有可发布的结果。
var ErrFatal = errors.New("engine: 致命错误")

// Render 执行管线。致命错误时返回 ErrFatal，Output 中仍包含全部诊断。
func Render(in Input) (Output, error) {
	var out diag.List
	out.Append(in.Diagnostics...)
	res := Output{}
	p := in.Profile
	preset := target.Find(in.Presets, p.Target.Preset)
	if preset == nil {
		out.Add(diag.TargetUnknown, "target.preset", "preset", p.Target.Preset)
		res.Diagnostics = out.Items()
		return res, fmt.Errorf("%w：未知的目标预设 %s", ErrFatal, p.Target.Preset)
	}
	res.Preset = preset

	// 表单层 = 生成部分 + 用户设置过的目录字段。
	base := yamlx.Map()
	if in.Generated != nil {
		for i := 0; i+1 < len(in.Generated.Content); i += 2 {
			yamlx.Set(base, in.Generated.Content[i].Value, yamlx.Clone(in.Generated.Content[i+1]))
		}
	}
	form := layer.ValidateForm(p.Form, in.Catalog, &out)
	for i := 0; i+1 < len(form.Content); i += 2 {
		yamlx.Set(base, form.Content[i].Value, form.Content[i+1])
	}

	// 原始编辑层（后备隐藏能源）。
	var raw *yaml.Node
	if strings.TrimSpace(p.Raw) != "" {
		var err error
		raw, err = yamlx.Parse([]byte(p.Raw), yamlx.DefaultLimits, "raw", &out)
		if err != nil {
			res.Diagnostics = out.Items()
			return res, fmt.Errorf("%w：原始编辑无法解析", ErrFatal)
		}
	}
	merged := layer.Merge(base, raw, in.Catalog, &out)
	res.Touched = merged.Touched

	target.Adapt(merged.Node, merged.Touched, in.Catalog, preset, p.Target.Core, &out)
	if !preset.Kernel {
		out.Add(diag.TargetCoreNoCheck, "", "preset", preset.ID)
	}

	text, err := render.Render(merged.Node, render.Options{
		Catalog:      in.Catalog,
		Header:       header(p, preset, in.Updated),
		ProxiesFlow:  p.Output.ProxiesFlow,
	}, &out)
	res.Diagnostics = out.Items()
	if err != nil {
		return res, fmt.Errorf("%w：%w", ErrFatal, err)
	}
	res.YAML = text
	return res, nil
}

// header 生成文件头。时区取方案设置（默认 UTC）；时间来自输入快照，保证确定性。
func header(p *profile.Profile, preset *target.Preset, updated time.Time) []render.HeaderField {
	core := p.Target.Core
	if core == "" {
		core = "latest"
	}
	h := []render.HeaderField{
		{Key: "NAME", Value: p.Name},
		{Key: "TARGET", Value: preset.ID + " · " + p.Target.Pack + " " + core},
		{Key: "GENERATOR", Value: version.Name + " " + version.Version + " · " + version.Home},
	}
	if p.Name == "" {
		h = h[1:]
	}
	if !updated.IsZero() {
		loc := time.UTC
		if p.Output.Timezone != "" {
			if l, err := time.LoadLocation(p.Output.Timezone); err == nil {
				loc = l
			}
		}
		t := updated.In(loc)
		h = append(h, render.HeaderField{Key: "UPDATED", Value: t.Format("2006-01-02 15:04:05") + " (" + utcOffset(t) + ")"})
	}
	return h
}

func utcOffset(t time.Time) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	h, m := off/3600, off%3600/60
	if m == 0 {
		return fmt.Sprintf("UTC%s%d", sign, h)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, h, m)
}

// CacheKey 是渲染结果的缓存键：方案版本、输入快照、目录与引擎版本任一变化都会改变它（第 7.3 节）。
func CacheKey(profileDoc []byte, snapshots []string, catalogDigest string) string {
	h := sha256.New()
	write := func(s string) {
		_, _ = fmt.Fprintf(h, "%d:%s\n", len(s), s) // hash.Hash 的写入不会失败
	}
	write(version.Version)
	write(catalogDigest)
	write(string(profileDoc))
	for _, s := range snapshots {
		write(s)
	}
	return hex.EncodeToString(h.Sum(nil))
}
