package node

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
)

func tools(t testing.TB) *Tools {
	t.Helper()
	tl, err := LoadTools(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func cat(t testing.TB) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// 刁钻名称表（testdata/regions/tricky.yaml）：地区识别的回归测试。
func TestTrickyRegions(t *testing.T) {
	b, err := os.ReadFile("../../testdata/regions/tricky.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name     string `yaml:"name"`
			Code     string `yaml:"code"`
			Kind     string `yaml:"kind"`
			Inferred bool   `yaml:"inferred"`
			Info     bool   `yaml:"info"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	tl := tools(t)
	for _, c := range f.Cases {
		if c.Info {
			var l diag.List
			n := &Node{Name: c.Name, Origin: "t"}
			if kept := filter([]*Node{n}, Processing{}, tl, "t", &l); len(kept) != 0 {
				t.Errorf("%q 应被识别为信息节点", c.Name)
			}
			continue
		}
		r := tl.Regions.Detect(c.Name)
		if r.Code != c.Code || string(r.Kind) != c.Kind || r.Inferred != c.Inferred {
			t.Errorf("%q：得到 %s/%s 推断=%v（%s），期望 %s/%s 推断=%v", c.Name, r.Code, r.Kind, r.Inferred, r.Explain(), c.Code, c.Kind, c.Inferred)
		}
	}
	if len(f.Cases) < 50 {
		t.Fatalf("刁钻名称表过短：%d", len(f.Cases))
	}
}

func TestFlags(t *testing.T) {
	if Flag("SG") != string([]rune{0x1F1F8, 0x1F1EC}) {
		t.Fatal("Flag(SG) 错误")
	}
	if c, ok := FlagCode("x " + Flag("JP") + " y"); !ok || c != "JP" {
		t.Fatal("FlagCode 错误")
	}
	if StripFlags(Flag("HK")+" 香港 01") != "香港 01" {
		t.Fatal("StripFlags 错误")
	}
}

func TestDecorStable(t *testing.T) {
	d := tools(t).Decor
	a := d.Pick("fp1", "")
	if d.Pick("fp1", "") != a {
		t.Fatal("同一指纹应得到同一 emoji")
	}
	if d.Pick("fp1", a) == a {
		t.Fatal("与前一个相同时应顺延")
	}
	for _, e := range d.List() {
		if _, ok := FlagCode(e); ok {
			t.Fatal("装饰 emoji 中不能有国旗")
		}
	}
}

func TestParseRate(t *testing.T) {
	cases := map[string]float64{"HK 01": 1, "HK 01 x2": 2, "日本 2倍": 2, "SG 0.5×": 0.5, "US ×1.5": 1.5, "倍率：3 TW": 3, "Hong Kong 04 x2": 2, "IEPL 10G": 1}
	for name, want := range cases {
		if got := ParseRate(name); got != want {
			t.Errorf("%q：倍率 %v，期望 %v", name, got, want)
		}
	}
}

const links = `vmess://eyJ2IjoiMiIsInBzIjoi8J+HuPCfh6wg5paw5Yqg5Z2hIDAxIiwiYWRkIjoiMTkyLjAuMi4xIiwicG9ydCI6IjQ0MyIsImlkIjoiMTExMTExMTEtMjIyMi0zMzMzLTQ0NDQtNTU1NTU1NTU1NTU1IiwiYWlkIjoiMCIsIm5ldCI6IndzIiwidGxzIjoidGxzIn0=
ss://YWVzLTEyOC1nY206ZXhhbXBsZQ@192.0.2.2:8388#%E5%B9%BF%E6%B8%AF%20IEPL%20%E6%96%B0%E5%8A%A0%E5%9D%A1
trojan://example-pw@example.com:443?sni=example.com#HK%2001
wireguard://abc@192.0.2.3:51820#wg
garbage line without scheme
vmess://not-base64!!!
`

func TestParseLinks(t *testing.T) {
	var l diag.List
	nodes, f := Parse([]byte(links), "src_test", cat(t), &l)
	if f != FormatLinks || len(nodes) != 3 {
		t.Fatalf("格式 %s，节点 %d；诊断 %v", f, len(nodes), l.Items())
	}
	if nodes[0].Port != 443 || nodes[1].Server != "192.0.2.2" {
		t.Errorf("字段不符：%+v %+v", nodes[0], nodes[1])
	}
	if p := nodes[0].Mapping; p != nil {
		if v := p.Content; len(v) == 0 {
			t.Fatal("空映射")
		}
	}
	var skipped int
	for _, d := range l.Items() {
		if d.Code == diag.NodeLineSkipped {
			skipped++
		}
		for _, v := range d.Args {
			if strings.Contains(v, "example-pw") || strings.Contains(v, "abc@") {
				t.Errorf("诊断中出现了凭据：%s", d)
			}
		}
	}
	if skipped != 3 {
		t.Errorf("应跳过 3 行（wireguard、无 scheme、坏 vmess），实际 %d：%v", skipped, l.Items())
	}
	// base64 订阅：整体编码后应得到同样的节点
	var l2 diag.List
	enc := []byte("dm1lc3M6Ly9leUoySWpvaU1pSXNJbkJ6SWpvaVlTSXNJbUZrWkNJNklqRTVNaTR3TGpJdU1TSXNJbkJ2Y25RaU9pSTBORE1pTENKcFpDSTZJakV4TVRFeE1URXhMVEl5TWpJdE16TXpNeTAwTkRRMExUVTFOVFUxTlRVMU5UVTFOU0lzSW1GcFpDSTZJakFpZlE9PQ")
	nodes2, f2 := Parse(enc, "src_b64", cat(t), &l2)
	if f2 != FormatBase64 || len(nodes2) != 1 {
		t.Fatalf("base64：格式 %s，节点 %d；%v", f2, len(nodes2), l2.Items())
	}
}

func TestParseClashYAML(t *testing.T) {
	src := `proxies:
  - {name: a, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example}
  - {name: bad, type: vmess, server: 192.0.2.2, port: 443}
  - not-a-proxy
`
	var l diag.List
	nodes, f := Parse([]byte(src), "y", cat(t), &l)
	if f != FormatClash || len(nodes) != 1 || !l.Has(diag.NodeInvalid) || !l.Has(diag.NodeNotProxy) {
		t.Fatalf("格式 %s，节点 %d，诊断 %v", f, len(nodes), l.Items())
	}
}

func TestParseSingbox(t *testing.T) {
	src := `{"outbounds":[
 {"type":"vless","tag":"🇯🇵 东京","server":"192.0.2.9","server_port":443,"uuid":"11111111-2222-3333-4444-555555555555","flow":"xtls-rprx-vision",
  "tls":{"enabled":true,"server_name":"example.com","utls":{"enabled":true,"fingerprint":"chrome"},"reality":{"enabled":true,"public_key":"jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0","short_id":"ab"}}},
 {"type":"hysteria2","tag":"hy2","server":"example.com","server_port":443,"server_ports":["2080:3000"],"password":"pw","up_mbps":50,"tls":{"enabled":true,"server_name":"example.com"},"multiplex":{"enabled":true}},
 {"type":"direct","tag":"direct"},
 {"type":"wireguard","tag":"wg"}
]}`
	var l diag.List
	nodes, f := Parse([]byte(src), "sb", cat(t), &l)
	if f != FormatSingbox || len(nodes) != 2 {
		t.Fatalf("格式 %s，节点 %d；%v", f, len(nodes), l.Items())
	}
	var v map[string]any
	_ = nodes[1].Mapping.Decode(&v)
	if v["ports"] != "2080-3000" || v["up"] != "50 Mbps" || v["sni"] != "example.com" {
		t.Errorf("hysteria2 映射不符：%v", v)
	}
	if !l.Has(diag.NodeSingboxField) || !l.Has(diag.NodeSingboxSkip) {
		t.Errorf("未转换的字段与出站应报告：%v", l.Items())
	}
}

func TestUnrecognized(t *testing.T) {
	var l diag.List
	nodes, f := Parse([]byte("just some text with a token AAAAAAAAAAAAAAAAAAAAAAAA"), "x", cat(t), &l)
	if f != FormatUnknown || len(nodes) != 0 || !l.Has(diag.NodeUnrecognized) {
		t.Fatalf("应报告无法识别：%v", l.Items())
	}
	for _, d := range l.Items() {
		if strings.Contains(d.Args["preview"], "AAAAAAAAAAAAAAAA") {
			t.Error("预览中的长串应脱敏")
		}
	}
}

func TestProcess(t *testing.T) {
	src := `proxies:
  - {name: "剩余流量：10 GB", type: ss, server: 192.0.2.1, port: 1, cipher: aes-128-gcm, password: p}
  - {name: 广港 IEPL 新加坡 01, type: ss, server: 192.0.2.2, port: 1, cipher: aes-128-gcm, password: p}
  - {name: 广港 IEPL 新加坡 01 副本, type: ss, server: 192.0.2.2, port: 1, cipher: aes-128-gcm, password: p}
  - {name: 自建 01, type: ss, server: 192.0.2.3, port: 1, cipher: aes-128-gcm, password: p}
  - {name: 游戏 02, type: ss, server: 192.0.2.4, port: 1, cipher: aes-128-gcm, password: p}
`
	var l diag.List
	nodes, _ := Parse([]byte(src), "s", cat(t), &l)
	nodes = Process(nodes, Processing{Exclude: "游戏", Rename: []RenameRule{{Pattern: " IEPL", Replace: ""}}}, tools(t), "s", &l)
	if len(nodes) != 2 {
		t.Fatalf("应保留 2 个节点，实际 %d：%v", len(nodes), l.Items())
	}
	if nodes[0].Name != Flag("SG")+" 广港 新加坡 01" || nodes[0].Region.Code != "SG" {
		t.Errorf("国旗或地区不符：%q %+v", nodes[0].Name, nodes[0].Region)
	}
	if nodes[1].Region.Code != "" || nodes[1].Emoji == "" || !strings.HasPrefix(nodes[1].Name, nodes[1].Emoji+" ") {
		t.Errorf("未识别地区应带装饰 emoji：%q", nodes[1].Name)
	}
	for _, c := range []diag.Code{diag.NodeInfoRemoved, diag.NodeDuplicate, diag.NodeFiltered} {
		if !l.Has(c) {
			t.Errorf("缺少诊断 %s", c)
		}
	}
	// 手动指定按指纹生效
	var l2 diag.List
	again, _ := Parse([]byte(src), "s", cat(t), &l2)
	fp := again[3].Fingerprint
	again = Process(again, Processing{Overrides: map[string]Override{fp: {Region: "JP"}}}, tools(t), "s", &l2)
	for _, n := range again {
		if n.Fingerprint == fp && (n.Region.Code != "JP" || n.Region.Kind != RegionManual) {
			t.Errorf("手动指定未生效：%+v", n.Region)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(links))
	f.Add([]byte("proxies: [{name: a, type: ss}]"))
	f.Add([]byte(`{"outbounds":[{"type":"vmess"}]}`))
	f.Add([]byte("c3M6Ly9ZV1Z6TFRFeU9DMW5ZMjA2WlhoaGJYQnNaUUAxOTIuMC4yLjI6ODM4OA"))
	c, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<14 {
			return
		}
		var l diag.List
		_, _ = Parse(b, "fuzz", c, &l)
		for _, d := range l.Items() {
			if d.Code == diag.NodeInvalid && strings.Contains(d.Args["reason"], "SECRETVALUE") {
				t.Fatalf("凭据进入了错误信息：%s", d)
			}
		}
	})
}
