//go:build kernel

package api

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/kernel"
)

func mihomoPaths(t *testing.T) (string, string) {
	bin := filepath.Join("..", "..", ".cache", "tools", "mihomo")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if v := os.Getenv("CLASH_LAUNCH_MIHOMO"); v != "" {
		bin = v
	}
	geo := filepath.Join("..", "..", ".cache", "tools", "geodata")
	if v := os.Getenv("CLASH_LAUNCH_GEODATA"); v != "" {
		geo = v
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("没有 mihomo：先运行 go run ./tools/toolsync")
	}
	b, _ := filepath.Abs(bin)
	g, _ := filepath.Abs(geo)
	return b, g
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// TestKernelLoadsSubscription：服务端渲染并通过内核检查后，mihomo 实际加载订阅链接返回的配置并开始监听。
func TestKernelLoadsSubscription(t *testing.T) {
	bin, geo := mihomoPaths(t)
	h := newHarness(t, kernel.NewChecker(bin, geo, 1))
	c, pid := h.setup()
	port := freePort(t)
	p := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	doc := strings.Replace(p.Doc, "mixed-port: 7890", fmt.Sprintf("mixed-port: %d", port), 1)
	up := decode[profileDTO](t, c.want(200, "PUT", "/api/v1/profiles/"+pid, map[string]string{"doc": doc}, "If-Match", `"v1"`))
	if up.Render.Status != "ok" || !strings.Contains(string(up.Render.Diagnostics), "kernel.passed") {
		t.Fatalf("应通过内核检查：%+v %s", up.Render, up.Render.Diagnostics)
	}
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{}))
	resp, body := h.client().do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("订阅：%d", resp.StatusCode)
	}
	dir := t.TempDir()
	for _, f := range []string{"geoip.metadb", "GeoSite.dat", "GeoIP.dat", "ASN.mmdb"} {
		b, err := os.ReadFile(filepath.Join(geo, f))
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, f), b, 0o600)
	}
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "-t", "-d", dir, "-f", cfg).CombinedOutput(); err != nil {
		t.Fatalf("mihomo -t：%v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-d", dir, "-f", cfg)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Logf("mihomo 已加载订阅并在 %d 端口监听", port)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("mihomo 没有在 mixed-port 上开始监听")
}
