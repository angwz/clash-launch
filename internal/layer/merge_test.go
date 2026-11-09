package layer

import (
	"bytes"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

func parse(t testing.TB, s string) *yaml.Node {
	t.Helper()
	var l diag.List
	n, err := yamlx.Parse([]byte(s), yamlx.DefaultLimits, "test", &l)
	if err != nil {
		t.Fatalf("解析失败：%v %v", err, l.Items())
	}
	return n
}

// canon 输出与呈现无关的规范形式：去掉块式/流式等风格，只比较结构、标签与值。
func canon(t testing.TB, n *yaml.Node) string {
	t.Helper()
	n = plain(yamlx.Clone(n))
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(n); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(b.String())
}

// 合并语义的表驱动测试：每一行对应第 9.2 节表格中的一条规则或一个边界。
func TestMerge(t *testing.T) {
	cases := []struct {
		name, base, raw, want string
		codes                 []diag.Code
	}{
		{"映射递归合并", "dns: {enable: true, ipv6: false}", "dns: {ipv6: true, listen: ':53'}", "dns: {enable: true, ipv6: true, listen: ':53'}", nil},
		{"标量替换", "mode: rule", "mode: global", "mode: global", nil},
		{"列表默认替换", "rules: [a, b]", "rules: [c]", "rules: [c]", nil},
		{"null 设为 null 而非删除", "secret: x", "secret: null", "secret: null", nil},
		{"新键追加在末尾", "a: 1", "b: 2", "a: 1\nb: 2", nil},
		{"!replace 映射不递归", "dns: {enable: true, ipv6: false}", "dns: !replace {listen: ':53'}", "dns: {listen: ':53'}", nil},
		{"!replace 列表", "rules: [a]", "rules: !replace [b]", "rules: [b]", nil},
		{"!append", "rules: [a, b]", "rules: !append [c]", "rules: [a, b, c]", nil},
		{"!prepend", "rules: [a, b]", "rules: !prepend [z]", "rules: [z, a, b]", nil},
		{"!append 到不存在的列表", "a: 1", "rules: !append [c]", "a: 1\nrules: [c]", nil},
		{"!delete", "a: 1\nb: 2", "a: !delete ~", "b: 2", nil},
		{"!delete 不存在的键只给信息", "a: 1", "b: !delete ~", "a: 1", []diag.Code{diag.LayerDeleteMiss}},
		{"嵌套映射中的标签", "dns: {nameserver: [a, b]}", "dns: {nameserver: !replace [c]}", "dns: {nameserver: [c]}", nil},
		{"新插入映射中的标签同样生效", "a: 1", "dns: {nameserver: !append [c]}", "a: 1\ndns: {nameserver: [c]}", nil},
		{
			"!patch-by:name 合并、追加与删除",
			"proxy-groups: [{name: A, type: select, proxies: [x]}, {name: B, type: select, proxies: [y]}]",
			"proxy-groups: !patch-by:name\n  - {name: A, type: url-test}\n  - {name: C, type: select, proxies: [z]}\n  - !delete {name: B}",
			"proxy-groups: [{name: A, type: url-test, proxies: [x]}, {name: C, type: select, proxies: [z]}]",
			nil,
		},
		{"!patch-by 删除不存在的元素", "g: [{name: A}]", "g: !patch-by:name\n  - !delete {name: Z}", "g: [{name: A}]", []diag.Code{diag.LayerDeleteMiss}},
		{"!patch-by 元素缺少键", "g: [{name: A}]", "g: !patch-by:name\n  - {type: x}", "g: [{name: A}]", []diag.Code{diag.LayerPatchKey}},
		{"!patch-by 目标不是映射列表", "g: [a, b]", "g: !patch-by:name\n  - {name: A}", "g: [a, b]", []diag.Code{diag.LayerPatchTarget}},
		{"未知标签是错误", "a: 1", "a: !merge 2", "a: 1", []diag.Code{diag.LayerUnknownTag}},
		{"列表元素上的标签位置错误", "a: [1]", "a: [!delete 1]", "a: []", []diag.Code{diag.LayerTagPlacement}},
		{"键上的标签位置错误", "a: 1", "!replace a: 2", "a: 1", []diag.Code{diag.LayerTagPlacement}},
		{"!replace 用于标量是错误", "a: 1", "a: !replace 2", "a: 1", []diag.Code{diag.LayerTagKind}},
		{"!append 用于映射是错误", "a: [1]", "a: !append {b: 1}", "a: [1]", []diag.Code{diag.LayerTagKind}},
		{"!append 到非列表是错误", "a: x", "a: !append [1]", "a: x", []diag.Code{diag.LayerTagKind}},
		{"映射替换为标量", "dns: {enable: true}", "dns: off", "dns: off", nil},
		{"标准标签保留", "a: 1", "a: !!str 1", "a: !!str 1", nil},
		{"空原始编辑", "a: 1", "", "a: 1", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := parse(t, c.base)
			var raw *yaml.Node
			if c.raw != "" {
				raw = parse(t, c.raw)
			}
			before := canon(t, base)
			var l diag.List
			got := Merge(base, raw, nil, &l)
			if canon(t, base) != before {
				t.Fatal("Merge 修改了输入")
			}
			if g, w := canon(t, got.Node), canon(t, parse(t, c.want)); g != w {
				t.Errorf("结果不符\n得到：\n%s\n期望：\n%s", g, w)
			}
			for _, code := range c.codes {
				if !l.Has(code) {
					t.Errorf("缺少诊断 %s；实际 %v", code, l.Items())
				}
			}
			if len(c.codes) == 0 {
				for _, d := range l.Items() {
					if d.Severity == diag.Error {
						t.Errorf("意外的错误诊断 %s", d)
					}
				}
			}
		})
	}
}

func TestMergeTouched(t *testing.T) {
	var l diag.List
	r := Merge(parse(t, "dns: {enable: true}"), parse(t, "dns: {ipv6: true}\nrules: !prepend [x]"), nil, &l)
	if strings.Join(r.Touched, ",") != "dns.ipv6,rules" {
		t.Errorf("Touched = %v", r.Touched)
	}
}

// FuzzMerge：任意输入不得 panic；对同一输入，结果必须确定；输入不得被修改。
func FuzzMerge(f *testing.F) {
	f.Add("a: [1]\nb: {c: 1}", "a: !append [2]\nb: !replace {d: 2}")
	f.Add("g: [{name: A}]", "g: !patch-by:name\n  - !delete {name: A}\n  - {name: B}")
	f.Add("x: 1", "x: !delete ~\ny: &a {z: 1}\nw: *a")
	f.Fuzz(func(t *testing.T, base, raw string) {
		var l diag.List
		b, err1 := yamlx.Parse([]byte(base), yamlx.Limits{MaxBytes: 1 << 16, MaxNodes: 10000}, "b", &l)
		r, err2 := yamlx.Parse([]byte(raw), yamlx.Limits{MaxBytes: 1 << 16, MaxNodes: 10000}, "r", &l)
		if err1 != nil || err2 != nil || b == nil || b.Kind != yaml.MappingNode {
			return
		}
		before := canonOrEmpty(b)
		var l1, l2 diag.List
		x := Merge(b, r, nil, &l1)
		y := Merge(b, r, nil, &l2)
		if canonOrEmpty(x.Node) != canonOrEmpty(y.Node) || len(l1.Items()) != len(l2.Items()) {
			t.Fatal("同一输入两次合并结果不同")
		}
		if canonOrEmpty(b) != before {
			t.Fatal("Merge 修改了输入")
		}
	})
}

func canonOrEmpty(n *yaml.Node) string {
	b, err := yaml.Marshal(n)
	if err != nil {
		return "<err:" + err.Error() + ">"
	}
	return string(b)
}

func plain(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	n.Style = 0
	for _, c := range n.Content {
		plain(c)
	}
	return n
}
