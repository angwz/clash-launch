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
