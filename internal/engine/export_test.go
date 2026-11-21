package engine

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
)

const exportSample = `
proxies:
  - {name: 香港 01, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pw}
  - {name: 日本 02, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pw}
proxy-providers:
  机场: {type: http, url: "https://example.com/sub", path: ./p.yaml}
proxy-groups:
  - {name: 节点选择, type: select, proxies: [自动选择, 香港 01, 日本 02, DIRECT], use: [机场]}
  - {name: 自动选择, type: url-test, proxies: [香港 01, 日本 02]}
  - {name: 全部, type: select, include-all: true}
  - {name: 香港, type: select, use: [机场], filter: 港}
rule-providers:
  ads: {type: http, behavior: domain, url: "https://example.com/ads.yaml", path: ./ads.yaml}
rules:
  - RULE-SET,ads,REJECT
  - AND,((RULE-SET,ads),(NETWORK,UDP)),REJECT
  - DOMAIN-SUFFIX,example.com,日本 02
  - DOMAIN,rule-set.example.com,节点选择
  - GEOIP,CN,DIRECT,no-resolve
  - MATCH,节点选择
`

func parseSample(t *testing.T) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(exportSample), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0]
}

func dump(t *testing.T, n *yaml.Node) string {
	t.Helper()
	b, err := yaml.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func codes(l *diag.List) map[diag.Code]int {
	m := map[diag.Code]int{}
	for _, d := range l.Items() {
		m[d.Code]++
	}
	return m
}

// 导出（R17）：不含节点时去掉节点与 provider，组内引用一并去掉，空组写 DIRECT 占位，
// 指向节点的规则改为 DIRECT；不含规则集时去掉 rule-providers 与使用它们的规则（含逻辑规则）。
func TestStripExport(t *testing.T) {
	root := parseSample(t)
	var out diag.List
	stripExport(root, Export{NoNodes: true, NoRules: true}, &out)
	got := dump(t, root)
	for _, gone := range []string{"proxies:\n    - {name: 香港", "proxy-providers", "rule-providers", "RULE-SET", "use:", "香港 01", "日本 02,"} {
		if strings.Contains(got, gone) {
			t.Errorf("应去掉 %q：\n%s", gone, got)
		}
	}
	for _, kept := range []string{"proxies: [自动选择, DIRECT]", "name: 自动选择, type: url-test, proxies: [DIRECT]", "include-all: true", "filter: 港", "DOMAIN-SUFFIX,example.com,DIRECT", "DOMAIN,rule-set.example.com,节点选择", "GEOIP,CN,DIRECT,no-resolve", "MATCH,节点选择"} {
		if !strings.Contains(got, kept) {
			t.Errorf("应包含 %q：\n%s", kept, got)
		}
	}
	c := codes(&out)
	if c[diag.ExportNodesRemoved] != 1 || c[diag.ExportGroupPlaceholder] != 2 || c[diag.ExportRuleTarget] != 1 || c[diag.ExportRulesRemoved] != 1 {
		t.Errorf("诊断：%v", c)
	}

	// 零值不改动
	root = parseSample(t)
	before := dump(t, root)
	var none diag.List
	stripExport(root, Export{}, &none)
	if dump(t, root) != before || len(none.Items()) != 0 {
		t.Error("完整导出不应改动")
	}
}
