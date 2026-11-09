//go:build kernel

package kernel

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
)

// TestChecker 需要真实的 mihomo 与地理数据：go run ./tools/toolsync 之后运行
//
//	go test -tags kernel ./internal/kernel/
func NewTestChecker(t testing.TB) *Checker {
	t.Helper()
	bin := os.Getenv("CLASH_LAUNCH_MIHOMO")
	if bin == "" {
		bin = filepath.Join("..", "..", ".cache", "tools", "mihomo")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
	}
	geo := os.Getenv("CLASH_LAUNCH_GEODATA")
	if geo == "" {
		geo = filepath.Join("..", "..", ".cache", "tools", "geodata")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("没有 mihomo 可执行文件 %s：先运行 go run ./tools/toolsync", bin)
	}
	abs, _ := filepath.Abs(bin)
	absGeo, _ := filepath.Abs(geo)
	return NewChecker(abs, absGeo, 2)
}

func TestCheckerPassAndFail(t *testing.T) {
	c := NewTestChecker(t)
	good := []byte(`mixed-port: 7890
proxies:
  - {name: a, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example}
proxy-groups:
  - {name: PROXY, type: select, proxies: [a, DIRECT]}
rules:
  - GEOIP,CN,DIRECT
  - GEOSITE,cn,DIRECT
  - MATCH,PROXY
`)
	var l diag.List
	if r := c.Check(context.Background(), good, &l); r.Status != Passed {
		t.Fatalf("合法配置未通过：%+v %v", r, l.Items())
	}
	bad := []byte(`proxies:
  - {name: a, type: vmess, server: 192.0.2.1, port: 443}
rules:
  - MATCH,DIRECT
`)
	var l2 diag.List
	r := c.Check(context.Background(), bad, &l2)
	if r.Status != Failed || r.Path != "proxies[0]" {
		t.Fatalf("坏配置应失败并定位到 proxies[0]：%+v", r)
	}
	if !l2.Has(diag.KernelFailed) {
		t.Fatal("缺少 kernel.failed 诊断")
	}
}

func TestGoldenPassesKernel(t *testing.T) {
	c := NewTestChecker(t)
	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "render", "*.golden.yaml"))
	more, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "profiles", "*.golden"))
	files = append(files, more...)
	if len(files) < 2 {
		t.Fatal("golden 文件缺失")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var l diag.List
		if r := c.Check(context.Background(), b, &l); r.Status != Passed {
			t.Errorf("%s 未通过 mihomo -t：%+v", f, r)
		}
	}
}
