package engine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/layer"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 导入（需求 R17）：把网上复制的完整 Clash/mihomo 配置反向解析为「要新建的节点源 + 一份配置文档」。
// 纯函数：不联网、不写存储；创建由调用方经已有的接口完成（配额、SSRF 防护、校验都沿用）。
//
// 映射（决定 0004 #31）：
//   - proxies → 一个手动节点源，整理全部关闭（不加国旗、不改名、不去重、保留信息节点），节点名与原文一致；
//   - proxy-providers 中 type: http 的 → 订阅节点源，「交给客户端」模式、名称不变，策略组的 use 因此照常生效；
//     其余类型（file、inline）原样留在直接编辑层；
//   - proxy-groups、rules、rule-providers → 原样写入直接编辑层（用户写下的组名、正则、规则不改写）；
//   - 其余键按字段目录归入表单层；面板与外部控制等运行期设置不导入；目录里没有的键丢弃，或按选择原样留在直接编辑层。
// 每一项去掉、改写或无法导入的内容都进入诊断。

// ImportSource 是导入时要新建的一个节点源。配置文档中以 "@" + Key 作为它的 ref 占位，创建后替换为真实 ID。
type ImportSource struct {
	Key        string            `json:"key"`
	Kind       string            `json:"kind"` // batch | subscription
	Name       string            `json:"name"`
	URL        string            `json:"url,omitempty"`
	Content    string            `json:"content,omitempty"`
	UA         string            `json:"ua,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Interval   int               `json:"interval,omitempty"`
	Processing node.Processing   `json:"processing"`
	Mode       string            `json:"mode"` // inline | provider
}

// ImportCounts 汇总将要创建的内容。
type ImportCounts struct {
	Proxies       int `json:"proxies"`
	Providers     int `json:"providers"`
	Groups        int `json:"groups"`
	Rules         int `json:"rules"`
	RuleProviders int `json:"ruleProviders"`
	Form          int `json:"form"` // 归入表单层的顶层字段数
	Raw           int `json:"raw"`  // 原样留在直接编辑层的未知键数
}

// ImportPlan 是导入的结果。
type ImportPlan struct {
	Sources     []ImportSource    `json:"sources"`
	Doc         string            `json:"doc"`
	Counts      ImportCounts      `json:"counts"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// ImportInput 是导入的输入。
type ImportInput struct {
	Text        []byte
	Name        string // 配置名称；为空时用 "Imported"
	Catalog     *catalog.Catalog
	KeepUnknown bool // 目录里没有的顶层键原样留在直接编辑层，而不是丢弃
}

// runtimeKeys 是面板与外部控制等运行期设置：导入别人的配置时不应沿用（会把控制接口与密钥带进来）。
var runtimeKeys = []string{"external-controller", "external-controller-tls", "external-controller-unix", "external-controller-pipe", "external-controller-cors", "external-ui", "external-ui-name", "external-ui-url", "secret", "external-doh-server"}

// rawKeys 是原样写入直接编辑层的分节（输出时按此顺序）。
var rawKeys = []string{"proxy-providers", "proxy-groups", "rule-providers", "rules"}

// importProcessing 让节点名与原文一致：策略组按名字引用节点，任何改写都会让引用失效。
var importProcessing = node.Processing{Flags: node.FlagOff, NoDecor: true, KeepInfo: true, KeepDup: true}

// PlanImport 解析一份完整配置。输入不是 YAML 映射时返回错误（详情在诊断中）。
func PlanImport(in ImportInput) (ImportPlan, error) {
	var out diag.List
	plan := ImportPlan{Sources: []ImportSource{}}
	root, err := yamlx.Parse(in.Text, yamlx.DefaultLimits, "import", &out)
	if err != nil || root == nil || root.Kind != yaml.MappingNode {
		if err == nil {
			out.Add(diag.ImportInvalid, "import", "reason", "the text is not a YAML mapping (a Clash or mihomo config)")
		}
		plan.Diagnostics = out.Items()
		return plan, fmt.Errorf("engine: 导入的内容不是 YAML 映射")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Imported"
	}

	// 节点 → 一个手动节点源
	var refs []map[string]string
	if seq := yamlx.Get(root, "proxies"); seq != nil && seq.Kind == yaml.SequenceNode && len(seq.Content) > 0 {
		b, err := yaml.Marshal(yamlx.Map("proxies", seq))
		if err == nil {
			var parsed diag.List
			nodes, _ := node.Parse(b, "import.proxies", in.Catalog, &parsed)
			out.Append(parsed.Items()...)
			plan.Counts.Proxies = len(nodes)
			plan.Sources = append(plan.Sources, ImportSource{Key: "s0", Kind: "batch", Name: name, Content: string(b), Processing: importProcessing, Mode: "inline"})
			refs = append(refs, map[string]string{"ref": "@s0"})
		}
	}

	// 节点订阅：http 的转为订阅节点源，其余原样保留
	raw := yamlx.Map()
	if m := yamlx.Get(root, "proxy-providers"); m != nil && m.Kind == yaml.MappingNode {
		kept := yamlx.Map()
		for i := 0; i+1 < len(m.Content); i += 2 {
			pname, p := m.Content[i].Value, m.Content[i+1]
			src, ok := providerSource(pname, p, &out)
			if !ok {
				yamlx.Set(kept, pname, p)
				continue
			}
			src.Key = "s" + strconv.Itoa(len(plan.Sources))
			plan.Sources = append(plan.Sources, src)
			refs = append(refs, map[string]string{"ref": "@" + src.Key, "mode": "provider"})
			plan.Counts.Providers++
		}
		if len(kept.Content) > 0 {
			yamlx.Set(raw, "proxy-providers", kept)
		}
	}

	// 分流：原样写入直接编辑层
	for _, k := range rawKeys[1:] {
		v := yamlx.Get(root, k)
		if v == nil {
			continue
		}
		yamlx.Set(raw, k, v)
		switch k {
		case "proxy-groups":
			plan.Counts.Groups = len(v.Content)
		case "rule-providers":
			plan.Counts.RuleProviders = len(v.Content) / 2
		case "rules":
			plan.Counts.Rules = len(v.Content)
		}
	}

	// 其余键：表单层、运行期设置、未知键
	candidate := yamlx.Map()
	var unknown []string
	for i := 0; i+1 < len(root.Content); i += 2 {
		k, v := root.Content[i].Value, root.Content[i+1]
		switch {
		case k == "proxies" || slices.Contains(rawKeys, k):
		case slices.Contains(runtimeKeys, k):
			out.Add(diag.ImportRuntimeDropped, k, "key", k)
		case in.Catalog.Root.Child(k, root) == nil:
			unknown = append(unknown, k)
			if in.KeepUnknown {
				yamlx.Set(raw, k, v)
				out.Add(diag.ImportRawKept, k, "key", k)
			} else {
				out.Add(diag.ImportUnknownDropped, k, "key", k)
			}
		default:
			yamlx.Set(candidate, k, v)
		}
	}
	plan.Counts.Raw = 0
	if in.KeepUnknown {
		plan.Counts.Raw = len(unknown)
	}
	// 与编辑器相同的校验：类型不符或目录里没有的子字段被去掉并报告
	form := layer.ValidateForm(candidate, in.Catalog, &out)
	plan.Counts.Form = len(form.Content) / 2

	doc := yamlx.Map(
		"clash-launch/profile", "v1",
		"name", name,
		"target", yamlx.Map("pack", in.Catalog.Pack.ID, "preset", "mihomo", "core", in.Catalog.Pack.CoreMax),
	)
	if len(refs) > 0 {
		seq := yamlx.Seq()
		for _, r := range refs {
			m := yamlx.Map("ref", r["ref"])
			if r["mode"] != "" {
				yamlx.Set(m, "mode", yamlx.Str(r["mode"]))
			}
			seq.Content = append(seq.Content, m)
		}
		yamlx.Set(doc, "sources", seq)
	}
	if len(form.Content) > 0 {
		yamlx.Set(doc, "form", form)
	}
	if len(raw.Content) > 0 {
		ordered := yamlx.Map()
		for _, k := range rawKeys {
			if v := yamlx.Get(raw, k); v != nil {
				yamlx.Set(ordered, k, v)
			}
		}
		for _, k := range yamlx.Keys(raw) {
			if !slices.Contains(rawKeys, k) {
				yamlx.Set(ordered, k, yamlx.Get(raw, k))
			}
		}
		b, err := yaml.Marshal(ordered)
		if err != nil {
			return plan, fmt.Errorf("engine: 生成直接编辑层：%w", err)
		}
		yamlx.Set(doc, "raw", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.LiteralStyle, Value: string(b)})
	}
	b, err := yaml.Marshal(doc)
	if err != nil {
		return plan, fmt.Errorf("engine: 生成配置文档：%w", err)
	}
	plan.Doc = string(b)
	plan.Diagnostics = out.Items()
	return plan, nil
}

// providerSource 把一个 http 节点订阅转为订阅节点源；其他类型返回 false（原样保留）。
// 本项目生成 provider 时只能表达 url、interval、filter、exclude-filter 与附加前缀，其余键（含请求头）逐项报告。
func providerSource(name string, p *yaml.Node, out *diag.List) (ImportSource, bool) {
	where := "proxy-providers." + name
	if p.Kind != yaml.MappingNode {
		return ImportSource{}, false
	}
	if t := yamlx.Get(p, "type"); t == nil || t.Value != "http" {
		typ := "(missing)"
		if t != nil {
			typ = t.Value
		}
		out.Add(diag.ImportProviderKept, where, "name", name, "type", typ)
		return ImportSource{}, false
	}
	u := yamlx.Get(p, "url")
	if u == nil || u.Value == "" {
		out.Add(diag.ImportProviderKept, where, "name", name, "type", "http (no url)")
		return ImportSource{}, false
	}
	src := ImportSource{Kind: "subscription", Name: name, URL: u.Value, Processing: importProcessing, Mode: "provider"}
	for i := 0; i+1 < len(p.Content); i += 2 {
		k, v := p.Content[i].Value, p.Content[i+1]
		switch k {
		case "type", "url", "path":
		case "interval":
			if n, err := strconv.Atoi(v.Value); err == nil {
				src.Interval = n
			}
		case "filter":
			src.Processing.Include = v.Value
		case "exclude-filter":
			src.Processing.Exclude = v.Value
		case "override":
			if pre := yamlx.Get(v, "additional-prefix"); pre != nil {
				src.Processing.Prefix = pre.Value
			}
			for _, ok := range yamlx.Keys(v) {
				if ok != "additional-prefix" {
					out.Add(diag.ImportProviderKey, where, "name", name, "key", "override."+ok)
				}
			}
		case "header":
			// 服务器拉取时使用这些请求头；「交给客户端」时由客户端自己拉取，生成的 provider 不带请求头，因此照样报告
			out.Add(diag.ImportProviderKey, where, "name", name, "key", "header")
			for _, hk := range yamlx.Keys(v) {
				hv := yamlx.Get(v, hk)
				val := hv.Value
				if hv.Kind == yaml.SequenceNode && len(hv.Content) > 0 {
					val = hv.Content[0].Value
				}
				if strings.EqualFold(hk, "User-Agent") {
					src.UA = val
					continue
				}
				if src.Headers == nil {
					src.Headers = map[string]string{}
				}
				src.Headers[hk] = val
			}
		default:
			out.Add(diag.ImportProviderKey, where, "name", name, "key", k)
		}
	}
	return src, true
}
