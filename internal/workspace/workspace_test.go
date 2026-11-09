package workspace_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/target"
	"github.com/angwz/clash-launch/internal/workspace"
)

var update = flag.Bool("update", false, "更新 golden 文件（更新后必须人工审阅差异）")

func render(t testing.TB, file string) engine.Output {
	t.Helper()
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var l diag.List
	p, err := profile.Parse(text, &l)
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := catalog.Load(catalogdata.FS, "mihomo")
	presets, _ := target.Load(catalogdata.FS, "mihomo")
	gen, ds, updated, err := workspace.Build("../../testdata/workspace/workspace.yaml", p, cat)
	if err != nil {
		t.Fatal(err)
	}
	if updated.IsZero() {
		updated = time.Unix(0, 0)
	}
	out, err := engine.Render(engine.Input{Profile: p, Catalog: cat, Presets: presets, Generated: gen, Diagnostics: append(l.Items(), ds...), Updated: updated})
	if err != nil {
		t.Fatalf("%v\n%v", err, out.Diagnostics)
	}
	return out
}

// 端到端 golden：有序源 + 真实规则源（domainrouter_v2 release）或外部配置 ini → 完整配置。
// 文件头时间来自输入快照中最新的时间戳；两次渲染逐字节一致。
func TestWorkspaceGolden(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/workspace/*.profile.yaml")
	if len(files) < 2 {
		t.Fatal("缺少方案")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			a, b := render(t, f), render(t, f)
			if !bytes.Equal(a.YAML, b.YAML) {
				t.Fatal("两次渲染结果不同")
			}
			for _, d := range a.Diagnostics {
				if d.Severity == diag.Error {
					t.Errorf("意外的错误：%s", d)
				}
				for _, v := range d.Args {
					if strings.Contains(v, "example-token") || strings.Contains(v, "example-trojan") {
						t.Errorf("诊断中出现了凭据：%s", d)
					}
				}
			}
			golden := strings.TrimSuffix(f, ".profile.yaml") + ".golden"
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
