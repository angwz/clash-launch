// Package profile 定义方案文档（第 4.1 节）：方案的权威形式是一份 YAML 文档，
// 数据库只是存储。文档可以导出、导入、放进 git；凭据不出现在文档中，只以源的引用代替。
//
//	clash-launch/profile: v1
//	name: 手机 · Stash
//	target: { pack: mihomo, preset: stash, core: "1.19.33" }
//	sources:                 # 有序：顺序即节点顺序；订阅按出现顺序编号（GROUPID）
//	  - ref: src_airport_a
//	    mode: inline          # inline 写入配置 | provider 交给客户端
//	rules: { ref: rs_domainrouter }
//	form: { mixed-port: 7890 }  # 表单层：只记录用户改过的字段
//	raw: |                    # 原始编辑（后备隐藏能源）：原文保存
//	  rules: !prepend [...]
package profile

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// DocType 是文档类型标识键；Version 是当前支持的版本。
const (
	DocType = "clash-launch/profile"
	Version = "v1"
)

// Mode 是源的写法（第 5.6 节）。
type Mode string

const (
	ModeInline   Mode = "inline"   // 写入配置：服务器解析节点并写进 proxies
	ModeProvider Mode = "provider" // 交给客户端：只写 proxy-providers
)

// Target 是目标：格式包 + 预设 + 内核版本。
type Target struct {
	Pack   string `yaml:"pack"`
	Preset string `yaml:"preset"`
	Core   string `yaml:"core"`
}

// SourceRef 是方案中对一个源的有序引用。
type SourceRef struct {
	Ref      string `yaml:"ref"`
	Mode     Mode   `yaml:"mode,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty"` // 停用：不参与编号与输出
}

// Chain 是一条链式代理规则：被选中的节点经由 Via 拨号（mihomo 的 dialer-proxy）。
// 流量路径为 本机 → Via → 节点 → 目标。mihomo 1.19 已移除 relay 组，dialer-proxy 是唯一的链式写法。
type Chain struct {
	Via string `yaml:"via"` // 前置：节点名或策略组名（最终配置中的名字）
	// Select 使用与 groups.toml 现代选择器相同的键（region、source、protocol、tags、name、exclude）；
	// 省略表示全部节点。保持原始映射，由引擎按选择器规则解析并报告错误。
	Select   map[string]any `yaml:"select,omitempty"`
	Disabled bool           `yaml:"disabled,omitempty"`
}

// RuleRef 引用规则源。
type RuleRef struct {
	Ref string `yaml:"ref"`
}

// Output 是与渲染有关的方案设置。
type Output struct {
	Timezone string `yaml:"timezone,omitempty"` // 文件头时间的时区，默认 UTC
	// ProxiesFlow 为 true 时 proxies 每个节点写成一行（流式映射）；默认与官方示例一致用块格式。
	ProxiesFlow bool `yaml:"proxies-flow,omitempty"`
	// ProxiesBlock 是 1.0 的旧开关：块格式现在是默认值，保留字段只为旧方案能照常解析。
	ProxiesBlock bool `yaml:"proxies-block,omitempty"`
	// UpdateInterval 是订阅响应头 profile-update-interval（小时），默认 12。
	UpdateInterval int `yaml:"update-interval,omitempty"`
	// UserInfo 选择 subscription-userinfo：none（默认）| sum | source:<ref>。
	UserInfo string `yaml:"userinfo,omitempty"`
	// EmptyGroups 是没有匹配到节点的策略组的处理方式：prune（默认，移除）| fallback（指向 MATCH 目标）| keep（mihomo 默认）。
	EmptyGroups string `yaml:"empty-groups,omitempty"`
}

// Profile 是解析后的方案文档。Form 与 Raw 保持原样：Form 是映射节点，Raw 是原文。
type Profile struct {
	Name    string      `yaml:"name"`
	Target  Target      `yaml:"target"`
	Sources []SourceRef `yaml:"sources,omitempty"`
	Rules   *RuleRef    `yaml:"rules,omitempty"`
	Chains  []Chain     `yaml:"chains,omitempty"` // 链式代理：按顺序套用，后面的覆盖前面的
	Output  Output      `yaml:"output,omitempty"`
	Form    *yaml.Node  `yaml:"-"`
	Raw     string      `yaml:"-"`
}

var knownKeys = []string{DocType, "name", "target", "sources", "rules", "chains", "output", "form", "raw"}

// Parse 解析方案文档。未知键给出警告（可能来自更新的版本）；结构错误给出 error 并返回错误。
func Parse(text []byte, out *diag.List) (*Profile, error) {
	root, err := yamlx.Parse(text, yamlx.DefaultLimits, "profile", out)
	if err != nil {
		return nil, err
	}
	if root == nil || root.Kind != yaml.MappingNode {
		out.Add(diag.ProfileInvalid, "profile", "reason", "document must be a mapping")
		return nil, fmt.Errorf("profile: 文档必须是映射")
	}
	ver := yamlx.Get(root, DocType)
	if ver == nil || ver.Value != Version {
		v := "(missing)"
		if ver != nil {
			v = ver.Value
		}
		out.Add(diag.ProfileVersion, DocType, "version", v)
		return nil, fmt.Errorf("profile: 不支持的版本 %s", v)
	}
	for _, k := range yamlx.Keys(root) {
		if !slices.Contains(knownKeys, k) {
			out.Add(diag.ProfileUnknownKey, k, "key", k)
		}
	}
	p := &Profile{}
	for _, k := range []string{"name", "target", "sources", "rules", "chains", "output"} {
		v := yamlx.Get(root, k)
		if v == nil {
			continue
		}
		var err error
		switch k {
		case "name":
			err = v.Decode(&p.Name)
		case "target":
			err = v.Decode(&p.Target)
		case "sources":
			err = v.Decode(&p.Sources)
		case "rules":
			p.Rules = &RuleRef{}
			err = v.Decode(p.Rules)
		case "chains":
			err = v.Decode(&p.Chains)
		case "output":
			err = v.Decode(&p.Output)
		}
		if err != nil {
			out.Add(diag.ProfileInvalid, k, "reason", err.Error())
			return nil, fmt.Errorf("profile: %s: %w", k, err)
		}
	}
	if f := yamlx.Get(root, "form"); f != nil && (f.Kind != yaml.ScalarNode || f.ShortTag() != "!!null") {
		p.Form = f
	}
	if r := yamlx.Get(root, "raw"); r != nil {
		if r.Kind != yaml.ScalarNode {
			out.Add(diag.ProfileInvalid, "raw", "reason", "raw must be a string (use a | block)")
			return nil, fmt.Errorf("profile: raw 必须是字符串")
		}
		p.Raw = r.Value
	}
	if err := p.normalize(out); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Profile) normalize(out *diag.List) error {
	if p.Target.Pack == "" {
		p.Target.Pack = "mihomo"
	}
	if p.Target.Preset == "" {
		p.Target.Preset = "mihomo"
	}
	seen := map[string]bool{}
	for i := range p.Sources {
		s := &p.Sources[i]
		if s.Ref == "" {
			out.Add(diag.ProfileInvalid, yamlx.Index("sources", i), "reason", "missing ref")
			return fmt.Errorf("profile: sources[%d] 缺少 ref", i)
		}
		if seen[s.Ref] {
			out.Add(diag.ProfileInvalid, yamlx.Index("sources", i), "reason", "duplicate ref "+s.Ref)
			return fmt.Errorf("profile: 源 %s 重复引用", s.Ref)
		}
		seen[s.Ref] = true
		switch s.Mode {
		case "":
			s.Mode = ModeInline
		case ModeInline, ModeProvider:
		default:
			out.Add(diag.ProfileInvalid, yamlx.Index("sources", i)+".mode", "reason", "mode must be inline or provider")
			return fmt.Errorf("profile: sources[%d].mode 无效：%s", i, s.Mode)
		}
	}
	switch p.Output.EmptyGroups {
	case "", "prune", "fallback", "keep":
	default:
		out.Add(diag.ProfileInvalid, "output.empty-groups", "reason", "empty-groups must be prune, fallback or keep")
		return fmt.Errorf("profile: output.empty-groups 无效：%s", p.Output.EmptyGroups)
	}
	return nil
}

// Marshal 输出规范形式的方案文档（键序固定），用于导出与版本记录。
// raw 以字面块原样保存，注释与空行逐字节保留。
func (p *Profile) Marshal() ([]byte, error) {
	root := yamlx.Map(DocType, Version)
	if p.Name != "" {
		yamlx.Set(root, "name", yamlx.Str(p.Name))
	}
	var t yaml.Node
	if err := t.Encode(p.Target); err != nil {
		return nil, err
	}
	t.Style = yaml.FlowStyle
	yamlx.Set(root, "target", &t)
	if len(p.Sources) > 0 {
		var s yaml.Node
		if err := s.Encode(p.Sources); err != nil {
			return nil, err
		}
		yamlx.Set(root, "sources", &s)
	}
	if p.Rules != nil {
		yamlx.Set(root, "rules", yamlx.Map("ref", p.Rules.Ref))
	}
	if p.Output != (Output{}) {
		var o yaml.Node
		if err := o.Encode(p.Output); err != nil {
			return nil, err
		}
		yamlx.Set(root, "output", &o)
	}
	if p.Form != nil && len(p.Form.Content) > 0 {
		yamlx.Set(root, "form", yamlx.Clone(p.Form))
	}
	if p.Raw != "" {
		raw := yamlx.Str(p.Raw)
		raw.Style = yaml.LiteralStyle
		yamlx.Set(root, "raw", raw)
	}
	return yaml.Marshal(root)
}

// Refs 返回启用的源引用（保持顺序）。
func (p *Profile) Refs() []SourceRef {
	var out []SourceRef
	for _, s := range p.Sources {
		if !s.Disabled {
			out = append(out, s)
		}
	}
	return out
}

// String 用于日志：只含名称与目标，不含任何内容。
func (p *Profile) String() string {
	return strings.TrimSpace(fmt.Sprintf("%s (%s/%s %s)", p.Name, p.Target.Pack, p.Target.Preset, p.Target.Core))
}
