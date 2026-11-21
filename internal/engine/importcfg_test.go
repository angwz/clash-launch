package engine

import (
	"strings"
	"testing"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/target"
)

// 网上常见的一份完整配置：节点、http 与 file 两种节点订阅、策略组（含 use、filter、include-all）、
// 规则集与规则，以及面板设置和一个未知键。
const importSample = `
mixed-port: 7890
allow-lan: false
mode: rule
log-level: info
external-controller: 127.0.0.1:9090
secret: example-secret
made-up-key: 1
dns:
  enable: true
  nameserver: [https://dns.example.com/dns-query]
proxies:
  - {name: 香港 01, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pw}
  - {name: "剩余流量：100 GB", type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pw}
proxy-providers:
  机场:
    type: http
    url: https://example.com/sub?token=example-token
    interval: 3600
    path: ./providers/a.yaml
    filter: 港|日
    health-check: {enable: true, url: https://www.gstatic.com/generate_204, interval: 300}
    header: {User-Agent: [clash.meta]}
  本地:
    type: file
    path: ./local.yaml
proxy-groups:
  - {name: 节点选择, type: select, proxies: [自动选择, 香港 01, DIRECT], use: [机场]}
  - {name: 自动选择, type: url-test, use: [机场], filter: 港, url: https://www.gstatic.com/generate_204, interval: 300}
  - {name: 全部, type: select, include-all: true}
rule-providers:
  ads: {type: http, behavior: domain, url: https://example.com/ads.yaml, path: ./ads.yaml, interval: 86400}
rules:
  - RULE-SET,ads,REJECT
  - DOMAIN-SUFFIX,example.com,香港 01
  - GEOIP,CN,DIRECT,no-resolve
  - MATCH,节点选择
`

func importCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestPlanImport(t *testing.T) {
	cat := importCatalog(t)
	plan, err := PlanImport(ImportInput{Text: []byte(importSample), Name: "导入", Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	c := plan.Counts
	if c.Proxies != 2 || c.Providers != 1 || c.Groups != 3 || c.Rules != 4 || c.RuleProviders != 1 || c.Raw != 0 {
		t.Errorf("计数：%+v", c)
	}
	if len(plan.Sources) != 2 || plan.Sources[0].Kind != "batch" || plan.Sources[1].Kind != "subscription" || plan.Sources[1].Name != "机场" || plan.Sources[1].Mode != "provider" {
		t.Fatalf("节点源：%+v", plan.Sources)
	}
	sub := plan.Sources[1]
	if sub.URL != "https://example.com/sub?token=example-token" || sub.Interval != 3600 || sub.Processing.Include != "港|日" || sub.UA != "clash.meta" {
		t.Errorf("订阅映射：%+v", sub)
	}
	codes := map[diag.Code]int{}
	for _, d := range plan.Diagnostics {
		codes[d.Code]++
	}
	if codes[diag.ImportRuntimeDropped] != 2 || codes[diag.ImportUnknownDropped] != 1 || codes[diag.ImportProviderKept] != 1 || codes[diag.ImportProviderKey] != 2 {
		t.Errorf("诊断：%v", codes)
	}
	for _, gone := range []string{"external-controller", "example-secret", "made-up-key"} {
		if strings.Contains(plan.Doc, gone) {
			t.Errorf("不应导入 %s：\n%s", gone, plan.Doc)
		}
	}

	// 选择保留未知键：进入直接编辑层
	kept, err := PlanImport(ImportInput{Text: []byte(importSample), Catalog: cat, KeepUnknown: true})
	if err != nil || kept.Counts.Raw != 1 || !strings.Contains(kept.Doc, "made-up-key") {
		t.Errorf("保留未知键：%v %+v", err, kept.Counts)
	}

	// 走一遍真实管线：节点名、策略组、规则与规则集原样出现在生成的配置中
	var ds diag.List
	doc := strings.NewReplacer("@s0", "src-a", "@s1", "src-b").Replace(plan.Doc)
	p, err := profile.Parse([]byte(doc), &ds)
	if err != nil {
		t.Fatalf("配置文档：%v\n%s", err, doc)
	}
	tools, err := node.LoadTools(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	var srcs []SourceInput
	for i, s := range plan.Sources {
		ref := []string{"src-a", "src-b"}[i]
		srcs = append(srcs, SourceInput{Ref: ref, Name: s.Name, Kind: SourceKind(s.Kind), Mode: profile.Mode(s.Mode), Content: []byte(s.Content), URL: s.URL, Interval: s.Interval, Processing: s.Processing})
	}
	built := Build(BuildInput{Sources: srcs, Catalog: cat, Tools: tools})
	presets, err := target.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Render(Input{Profile: p, Catalog: cat, Presets: presets, Generated: built.Generated, Diagnostics: built.Diagnostics})
	if err != nil {
		t.Fatalf("渲染：%v %v", err, res.Diagnostics)
	}
	y := string(res.YAML)
	for _, want := range []string{"name: 香港 01", "剩余流量：100 GB", "name: 节点选择", "- 香港 01", "include-all: true", "filter: 港", "RULE-SET,ads,REJECT", "DOMAIN-SUFFIX,example.com,香港 01", "GEOIP,CN,DIRECT,no-resolve", "MATCH,节点选择", "机场:", "url: https://example.com/sub?token=example-token", "本地:", "ads:", "mixed-port: 7890", "dns:"} {
		if !strings.Contains(y, want) {
			t.Errorf("生成的配置缺少 %q：\n%s", want, y)
		}
	}
	if strings.Contains(y, "🇭🇰") || strings.Contains(y, "external-controller") {
		t.Errorf("节点名不应被改写，也不应带入面板设置：\n%s", y)
	}
}

func TestPlanImportInvalid(t *testing.T) {
	cat := importCatalog(t)
	for _, text := range []string{"- just\n- a list\n", "not: [valid", "plain text"} {
		if plan, err := PlanImport(ImportInput{Text: []byte(text), Catalog: cat}); err == nil {
			t.Errorf("%q 应被拒绝：%+v", text, plan)
		} else if len(plan.Diagnostics) == 0 {
			t.Errorf("%q 应给出诊断", text)
		}
	}
}
