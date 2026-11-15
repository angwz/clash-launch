package engine

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/yamlx"
)

func noUnapplied(t *testing.T, ds []diag.Diagnostic) {
	t.Helper()
	for _, d := range ds {
		if d.Code == diag.LayerUnapplied {
			t.Errorf("不应有未生效的修改：%v", d)
		}
	}
}

// 不做任何修改时，换算出的原始编辑与原来等价：重新渲染逐字节相同。
func TestAbsorbRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/profiles/*.yaml")
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			in := input(t, f)
			a, err := Render(in)
			if err != nil {
				t.Fatal(err)
			}
			raw, ds, err := Absorb(in, a.Preview)
			if err != nil {
				t.Fatalf("%v %v", err, ds)
			}
			noUnapplied(t, ds)
			in.Profile.Raw = raw
			b, err := Render(in)
			if err != nil {
				t.Fatalf("%v\n%s", err, raw)
			}
			if !bytes.Equal(a.YAML, b.YAML) {
				t.Errorf("往返后输出不同。原始编辑：\n%s", raw)
			}
		})
	}
}

const absorbGenerated = `proxies:
  - {name: HK 01, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pass}
  - {name: JP 01, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pass}
proxy-groups:
  - {name: Proxy, type: select, proxies: [HK 01, JP 01]}
rules:
  - DOMAIN-SUFFIX,example.com,Proxy
  - MATCH,DIRECT
`

func absorbInput(t *testing.T, form, generated string) Input {
	t.Helper()
	in := input(t, "../../testdata/profiles/m1-layers.yaml")
	var l diag.List
	p, err := profile.Parse([]byte("clash-launch/profile: v1\ntarget: {preset: mihomo}\nform:\n"+form), &l)
	if err != nil {
		t.Fatal(err, l.Items())
	}
	in.Profile = p
	g, err := yamlx.Parse([]byte(generated), yamlx.DefaultLimits, "gen", &l)
	if err != nil {
		t.Fatal(err, l.Items())
	}
	in.Generated = g
	return in
}

func edit(t *testing.T, text string, pairs ...string) []byte {
	t.Helper()
	for i := 0; i+1 < len(pairs); i += 2 {
		if !strings.Contains(text, pairs[i]) {
			t.Fatalf("预览中找不到 %q：\n%s", pairs[i], text)
		}
		text = strings.Replace(text, pairs[i], pairs[i+1], 1)
	}
	return []byte(text)
}

// 在完整 YAML 上修改：只保存差异，生成部分更新后修改继续生效。
func TestAbsorbKeepsIntentAcrossUpdates(t *testing.T) {
	in := absorbInput(t, "  mixed-port: 7890\n  log-level: info\n", absorbGenerated)
	a, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	full := edit(t, string(a.Preview),
		"log-level: info\n", "",
		"server: 192.0.2.2", "server: 192.0.2.20",
		"rules:\n", "rules:\n  - DOMAIN,intranet.example.com,DIRECT\n",
		"mixed-port: 7890\n", "mixed-port: 7891\nipv6: true\n",
	)
	raw, ds, err := Absorb(in, full)
	if err != nil {
		t.Fatalf("%v %v", err, ds)
	}
	noUnapplied(t, ds)
	for _, want := range []string{"!prepend", "!patch-by:name", "log-level: !delete", "mixed-port: 7891", "ipv6: true", "192.0.2.20"} {
		if !strings.Contains(raw, want) {
			t.Errorf("原始编辑缺少 %q：\n%s", want, raw)
		}
	}
	if strings.Contains(raw, "HK 01") || strings.Contains(raw, "MATCH") || strings.Contains(raw, "# ") {
		t.Errorf("没改的部分与注释不应写入：\n%s", raw)
	}

	// 订阅更新：多了一个节点、规则多了一条；用户的修改仍然生效
	in.Profile.Raw = raw
	in.Generated, _ = yamlx.Parse([]byte(strings.Replace(absorbGenerated, "proxy-groups:", "  - {name: SG 01, type: ss, server: 192.0.2.3, port: 8388, cipher: aes-128-gcm, password: example-pass}\nproxy-groups:", 1)), yamlx.DefaultLimits, "gen", nil)
	b, err := Render(in)
	if err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	y := string(b.YAML)
	for _, want := range []string{"SG 01", "192.0.2.20", "intranet.example.com", "mixed-port: 7891"} {
		if !strings.Contains(y, want) {
			t.Errorf("更新后缺少 %q：\n%s", want, y)
		}
	}
	if strings.Contains(y, "log-level") {
		t.Errorf("删掉的键不应回来：\n%s", y)
	}
}

// 预览中以注释显示的未启用分节：用户没写出它时不算删除；原有的原始编辑保留。
func TestAbsorbHiddenSections(t *testing.T) {
	in := absorbInput(t, "  mixed-port: 7890\n  dns: {enable: false, ipv6: true}\n", absorbGenerated)
	in.Profile.Raw = "dns:\n  listen: 0.0.0.0:1053\n"
	a, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a.Preview, []byte("# dns:")) {
		t.Fatalf("dns 应显示为注释：\n%s", a.Preview)
	}
	raw, ds, err := Absorb(in, a.Preview)
	if err != nil {
		t.Fatal(err, ds)
	}
	noUnapplied(t, ds)
	if strings.Contains(raw, "!delete") || !strings.Contains(raw, "listen: 0.0.0.0:1053") {
		t.Errorf("未启用分节应原样保留原有的原始编辑：\n%s", raw)
	}
}

// 无效的完整 YAML 不改动任何东西，并给出语法诊断。
func TestAbsorbInvalid(t *testing.T) {
	in := absorbInput(t, "  mixed-port: 7890\n", absorbGenerated)
	for _, text := range []string{"a: [1, 2\n", "- 1\n- 2\n"} {
		if _, ds, err := Absorb(in, []byte(text)); err == nil || len(ds) == 0 {
			t.Errorf("%q 应报错并给出诊断：%v %v", text, err, ds)
		}
	}
}
