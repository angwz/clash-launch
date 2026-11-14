package engine

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/target"
)

var update = flag.Bool("update", false, "更新 golden 文件（更新后必须人工审阅差异）")

var fixedTime = time.Date(2026, 11, 8, 22, 8, 31, 0, time.UTC)

func input(t testing.TB, file string) Input {
	t.Helper()
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var l diag.List
	p, err := profile.Parse(text, &l)
	if err != nil {
		t.Fatalf("%v %v", err, l.Items())
	}
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	presets, err := target.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	return Input{Profile: p, Catalog: cat, Presets: presets, Diagnostics: l.Items(), Updated: fixedTime}
}

// 方案 golden：testdata/profiles/*.yaml 的渲染结果必须与 *.golden 逐字节一致，且两次渲染相同。
func TestProfilesGolden(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/profiles/*.yaml")
	if len(files) == 0 {
		t.Fatal("没有方案测试数据")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			a, err := Render(input(t, f))
			if err != nil {
				t.Fatalf("%v\n%v", err, a.Diagnostics)
			}
			b, _ := Render(input(t, f))
			if !bytes.Equal(a.YAML, b.YAML) {
				t.Fatal("两次渲染结果不同")
			}
			golden := strings.TrimSuffix(f, ".yaml") + ".golden"
			if *update {
				if err := os.WriteFile(golden, a.YAML, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("缺少 golden（用 -update 生成并审阅）：%v", err)
			}
			if !bytes.Equal(a.YAML, want) {
				t.Errorf("与 %s 不一致", golden)
			}
		})
	}
}

func TestFormValidation(t *testing.T) {
	in := input(t, "../../testdata/profiles/m1-layers.yaml")
	text := []byte(`clash-launch/profile: v1
target: {preset: mihomo}
form:
  mixed-port: "7890"
  mode: sideways
  not-a-field: 1
  rules: [MATCH,DIRECT]
  dns: {enable: true, bogus: 1}
`)
	var l diag.List
	p, err := profile.Parse(text, &l)
	if err != nil {
		t.Fatal(err)
	}
	in.Profile = p
	out, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, d := range out.Diagnostics {
		codes = append(codes, string(d.Code)+"@"+d.Path)
	}
	got := strings.Join(codes, " ")
	for _, want := range []string{"form.value_type@mixed-port", "form.enum_value@mode", "form.unknown_field@not-a-field", "form.generated_section@rules", "form.unknown_field@dns.bogus"} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少诊断 %s；实际 %s", want, got)
		}
	}
	if !bytes.Contains(out.YAML, []byte("dns:\n  enable: true\n")) {
		t.Errorf("合格的字段应保留：\n%s", out.YAML)
	}
}

func TestRawSyntaxErrorIsFatal(t *testing.T) {
	in := input(t, "../../testdata/profiles/m1-layers.yaml")
	in.Profile.Raw = "dns: [unclosed"
	out, err := Render(in)
	if err == nil || out.YAML != nil {
		t.Fatal("原始编辑语法错误应是致命错误，且不产出结果")
	}
}

// 依赖方向（第 19.1 节）：engine 不依赖 store、source、api。
func TestDependencyDirection(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/angwz/clash-launch/internal/engine").Output()
	if err != nil {
		t.Skipf("go list 不可用：%v", err)
	}
	// 只检查本项目的包：engine 经 kernel 使用 mihomo 库，后者自身导入 net/http；
	// 「engine 不联网」由纯函数约定与 internal/source 是唯一的网络包来保证。
	for _, forbidden := range []string{"/internal/store", "/internal/source", "/internal/api", "/internal/auth"} {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "clash-launch"+forbidden) {
				t.Errorf("engine 不得依赖 %s（经由 %s）", forbidden, line)
			}
		}
	}
}

// 目录中的推荐值（recommended）连同 enable: true 写入表单后，必须通过表单校验：类型或键名写错时，
// 界面会把无效值填进用户的方案。
func TestRecommendedValuesPassForm(t *testing.T) {
	in := input(t, "../../testdata/profiles/m1-layers.yaml")
	n := 0
	for _, f := range in.Catalog.Root.Fields {
		if f.Recommended.Kind == 0 {
			continue
		}
		n++
		body, err := yaml.Marshal(&f.Recommended)
		if err != nil {
			t.Fatal(err)
		}
		doc := "clash-launch/profile: v1\ntarget: {preset: mihomo}\nform:\n  " + f.Key + ":\n    enable: true\n" + indent(string(body), "    ")
		var l diag.List
		p, err := profile.Parse([]byte(doc), &l)
		if err != nil {
			t.Fatalf("%s: %v\n%s", f.Key, err, doc)
		}
		in.Profile = p
		out, err := Render(in)
		if err != nil {
			t.Fatalf("%s: %v %v", f.Key, err, out.Diagnostics)
		}
		for _, d := range out.Diagnostics {
			if strings.HasPrefix(string(d.Code), "form.") {
				t.Errorf("%s 的推荐值没有通过表单校验：%s %s", f.Key, d.Code, d.Path)
			}
		}
	}
	if n < 3 {
		t.Fatalf("推荐值过少：%d", n)
	}
}

func indent(s, pre string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = pre + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}

// 字段的建议值（guide.suggest）必须能通过表单校验：界面上「点击采用」写入的就是它（反馈 #27）。
// 嵌套字段写在已打开 enable 的父对象下，避免「总开关未开」之类与建议值无关的提示。
func TestGuideSuggestPassForm(t *testing.T) {
	in := input(t, "../../testdata/profiles/m1-layers.yaml")
	n := 0
	var walk func(fs []*catalog.Field, path []string)
	walk = func(fs []*catalog.Field, path []string) {
		for _, f := range fs {
			p := append(append([]string{}, path...), f.Key)
			if f.Type == catalog.TObject {
				walk(f.Fields, p)
			}
			if f.Guide == nil || f.Guide.Suggest.Kind == 0 {
				continue
			}
			n++
			body, err := yaml.Marshal(&f.Guide.Suggest)
			if err != nil {
				t.Fatal(err)
			}
			doc := "clash-launch/profile: v1\ntarget: {preset: mihomo}\nform:\n"
			for i, k := range p[:len(p)-1] {
				doc += strings.Repeat("  ", i+1) + k + ":\n"
			}
			doc += strings.Repeat("  ", len(p)) + f.Key + ": " + strings.TrimSpace(string(body)) + "\n"
			var l diag.List
			prof, err := profile.Parse([]byte(doc), &l)
			if err != nil {
				t.Fatalf("%s: %v\n%s", f.ID, err, doc)
			}
			in.Profile = prof
			out, err := Render(in)
			if err != nil {
				t.Fatalf("%s: %v %v", f.ID, err, out.Diagnostics)
			}
			for _, d := range out.Diagnostics {
				if d.Severity == diag.Error && strings.HasPrefix(string(d.Code), "form.") {
					t.Errorf("%s 的建议值没有通过表单校验：%s %s", f.ID, d.Code, d.Path)
				}
			}
		}
	}
	walk(in.Catalog.Root.Fields, nil)
	if n < 3 {
		t.Fatalf("建议值过少：%d", n)
	}
}
