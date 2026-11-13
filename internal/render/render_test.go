package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

var update = flag.Bool("update", false, "更新 golden 文件（更新后必须人工审阅差异）")

var tricky = []string{
	"🇸🇬 广港 IEPL 新加坡 01", "🪐 Node", "yes", "No", "on", "OFF", "y", "~", "null", "True",
	"0123", "0x1F", "1_000", "1:20", "1e3", ".5", ".inf", "-1", "+1", "2026-11-09",
	"a: b", "a #b", "a:", "*.lan", "+.google.com", "-", "- x", "?x", ":x", "!x", "&x", "%x", "@x", "`x",
	"[x]", "{x}", "a,b", "'q'", `"d"`, `back\slash`, "it's", " lead", "trail ", "", "<<", "=",
	"::1", "2001:db8::1", "192.0.2.1", "198.18.0.1/16", "tls://223.5.5.5:853", "https://example.com/a?b=c#d",
	"DOMAIN-SUFFIX,example.com,DIRECT", "geosite:cn,private", "line1\nline2", "line1\nline2\n", "x\n\n",
	" indented\nsecond", "\nleading newline", "tab\there", "nul\x00", "bell\a", "\u2028sep", "\uFEFFbom",
	"中文：冒号", "ja 日本語", "fa فارسی", "a\u200db", "#hash",
}

func mustRender(t testing.TB, root *yaml.Node, opt Options) string {
	t.Helper()
	var l diag.List
	out, err := Render(root, opt, &l)
	if err != nil {
		t.Fatalf("渲染失败：%v\n%v", err, l.Items())
	}
	return string(out)
}

func TestTrickyScalars(t *testing.T) {
	for _, s := range tricky {
		root := yamlx.Map(
			"a", s,
			"list", yamlx.StrSeq(s, "x"),
			"hosts", yamlx.Map(s, s),
			"proxies", yamlx.Seq(yamlx.Map("name", s, "type", "ss", "server", "example.com", "port", 443, "alpn", yamlx.StrSeq(s))),
		)
		mustRender(t, root, Options{})
	}
}

func TestStyle(t *testing.T) {
	root := yamlx.Map(
		"name", "🇸🇬 新加坡",
		"flag", "yes",
		"num", "0123",
		"wild", "*.lan",
		"empty", yamlx.Map(),
		"proxies", yamlx.Seq(yamlx.Map("name", "🇭🇰 HK", "type", "ss", "port", 443), yamlx.Map("name", "JP", "type", "ss", "port", 443)),
		"proxy-groups", yamlx.Seq(yamlx.Map("name", "G", "proxies", yamlx.StrSeq("🇭🇰 HK", "JP")), yamlx.Map("name", "H", "proxies", yamlx.StrSeq("G"))),
		"mode", "rule",
	)
	// 默认：简单键挨着写；节点与策略组为块格式，条目之间空一行，嵌套列表不空行。
	got := mustRender(t, root, Options{Header: []HeaderField{{"name", "测试"}}})
	want := "# NAME: 测试\n\nname: 🇸🇬 新加坡\nflag: 'yes'\nnum: '0123'\nwild: '*.lan'\nempty: {}\n\n" +
		"proxies:\n  - name: 🇭🇰 HK\n    type: ss\n    port: 443\n\n  - name: JP\n    type: ss\n    port: 443\n\n" +
		"proxy-groups:\n  - name: G\n    proxies:\n      - 🇭🇰 HK\n      - JP\n\n  - name: H\n    proxies:\n      - G\n\nmode: rule\n"
	if got != want {
		t.Errorf("得到：\n%s\n期望：\n%s", got, want)
	}
	// 单行节点：每个节点一行，不空行。
	got = mustRender(t, yamlx.Map("proxies", yamlx.Seq(yamlx.Map("name", "A", "type", "ss"), yamlx.Map("name", "B", "type", "ss"))), Options{ProxiesFlow: true})
	if want := "proxies:\n  - {name: A, type: ss}\n  - {name: B, type: ss}\n"; got != want {
		t.Errorf("单行：\n%s", got)
	}
}

func TestDeterministic(t *testing.T) {
	root := yamlx.Map("b", 1, "a", yamlx.Map("y", 1, "x", 2), "proxies", yamlx.Seq(yamlx.Map("type", "ss", "name", "n")))
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	a := mustRender(t, root, Options{Catalog: cat})
	b := mustRender(t, root, Options{Catalog: cat})
	if a != b {
		t.Fatal("两次渲染结果不同")
	}
}

// FuzzRender：任意 UTF-8 字符串出现在值、键、流式节点与列表中，渲染必须通过格式自检。
func FuzzRender(f *testing.F) {
	for _, s := range tricky {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return // YAML 文本只能承载合法 UTF-8；输入在解析阶段就已保证
		}
		root := yamlx.Map(
			"a", s,
			"list", yamlx.StrSeq(s),
			"hosts", yamlx.Map(s, s),
			"proxies", yamlx.Seq(yamlx.Map("name", s, "type", s, "tags", yamlx.StrSeq(s, s))),
		)
		var l diag.List
		if _, err := Render(root, Options{}, &l); err != nil {
			t.Fatalf("自检失败：%q：%v", s, err)
		}
	})
}

// golden：testdata/render/*.in.yaml 经目录排序与渲染后，必须与 *.golden.yaml 逐字节一致。
func TestGolden(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob("../../testdata/render/*.in.yaml")
	if len(files) == 0 {
		t.Fatal("没有 golden 输入")
	}
	for _, in := range files {
		t.Run(filepath.Base(in), func(t *testing.T) {
			src, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			var l diag.List
			root, err := yamlx.Parse(src, yamlx.DefaultLimits, in, &l)
			if err != nil {
				t.Fatal(l.Items())
			}
			got := mustRender(t, root, Options{Catalog: cat, Header: []HeaderField{{"NAME", "golden"}, {"TARGET", "mihomo · mihomo 1.19.33"}}})
			golden := strings.TrimSuffix(in, ".in.yaml") + ".golden.yaml"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("缺少 golden 文件（用 -update 生成并审阅）：%v", err)
			}
			if got != string(want) {
				t.Errorf("与 %s 不一致；确认改动是有意的后用 -update 更新并审阅差异", golden)
			}
		})
	}
}
