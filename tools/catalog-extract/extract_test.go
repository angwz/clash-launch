package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExampleOrder(t *testing.T) {
	dir := t.TempDir()
	src := `# port: 7890
mixed-port: 7890
dns:
  enable: false
  # ipv6: false
  nameserver:
    - 1.1.1.1
  nameserver-policy:
    "geosite:cn": a
proxies:
  - name: a
    type: ss
    cipher: x
  - name: b
    type: vmess
    uuid: u
  - name: c
    type: ss
    password: p
rule-providers:
  r1:
    behavior: domain
    type: http
`
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := exampleOrder(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"":                 {"port", "mixed-port", "dns", "proxies", "rule-providers"},
		"dns":              {"enable", "ipv6", "nameserver", "nameserver-policy"},
		"proxies.ss":       {"name", "type", "cipher", "password"},
		"proxies.vmess":    {"name", "type", "uuid"},
		"rule-providers.*": {"behavior", "type"},
	}
	for k, w := range want {
		if !reflect.DeepEqual(got[k], w) {
			t.Errorf("%q: 得到 %v，期望 %v", k, got[k], w)
		}
	}
}

func TestDiff(t *testing.T) {
	a := Result{Version: "a", Fields: []Field{{Path: "x", Type: "int"}, {Path: "y", Type: "string"}}}
	b := Result{Version: "b", Fields: []Field{{Path: "x", Type: "string"}, {Path: "z", Type: "bool"}}}
	d := diff(a, b)
	if len(d.Added) != 1 || d.Added[0].Path != "z" || len(d.Removed) != 1 || d.Removed[0].Path != "y" || len(d.Changed) != 1 || d.Changed[0].Path != "x" {
		t.Fatalf("差异不符：%+v", d)
	}
}

// TestScanMihomo 在本机有参考源码时运行（.cache/ref/mihomo，见 docs/development/setup.md）。
func TestScanMihomo(t *testing.T) {
	src := filepath.Join("..", "..", ".cache", "ref", "mihomo")
	if _, err := os.Stat(filepath.Join(src, "go.mod")); err != nil {
		t.Skip("没有 mihomo 参考源码")
	}
	r, err := scan(src)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, f := range r.Fields {
		have[f.Path] = true
	}
	for _, p := range []string{"mixed-port", "dns.enhanced-mode", "tun.stack", "proxies.vmess.ws-opts.path", "proxy-groups.url-test.tolerance", "rule-providers.behavior"} {
		if !have[p] {
			t.Errorf("扫描结果缺少 %s", p)
		}
	}
}
