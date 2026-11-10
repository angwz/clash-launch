//go:build kernel

package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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
	loadAndListen(t, bin, geo, body, port)
}

// 验收：OpenClash 核心路径。登录 → 添加订阅（URL）→ 添加规则源（维护者真实的 domainrouter_v2 两份 toml，
// 经 URL 拉取）→ 新建目标为 openclash 的方案 → 创建订阅链接 → 匿名取回 → mihomo -t → 实际启动并监听。
// 订阅与规则文件由本机 httptest 提供（SSRF 白名单只放行 127.0.0.1），节点为虚构数据。
func TestKernelOpenClashFlow(t *testing.T) {
	bin, geo := mihomoPaths(t)
	h := newHarness(t, kernel.NewChecker(bin, geo, 1))
	files := map[string]string{
		"/sub":                  "../../testdata/workspace/snapshots/airport-a.txt",
		"/dr/groups.toml":       "../../testdata/rules/domainrouter/groups.toml",
		"/dr/rulesets-mrs.toml": "../../testdata/rules/domainrouter/rulesets-mrs.toml",
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, f)
	}))
	defer origin.Close()

	st, _ := h.svc.NewSetup(context.Background())
	h.srv.SetSetup(st)
	c := h.client()
	c.want(201, "POST", "/api/v1/setup", map[string]string{"token": st.Token, "username": "admin", "password": "correct horse battery"})
	c.login("admin", "correct horse battery")

	src := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{"name": "机场", "kind": "subscription", "url": origin.URL + "/sub"}))
	c.want(200, "POST", "/api/v1/sources/"+src.ID+"/refresh", nil)
	rs := decode[ruleSourceDTO](t, c.want(201, "POST", "/api/v1/rule-sources", map[string]any{
		"name": "domainrouter_v2", "kind": "toml", "preset": "domainrouter/mrs",
		"groupsUrl": origin.URL + "/dr/groups.toml", "rulesetsUrl": origin.URL + "/dr/rulesets-mrs.toml",
	}))
	c.want(200, "POST", "/api/v1/rule-sources/"+rs.ID+"/refresh", nil)

	port := freePort(t)
	doc := fmt.Sprintf("clash-launch/profile: v1\nname: OpenClash\ntarget: {pack: mihomo, preset: openclash, core: \"1.19.33\"}\n"+
		"sources:\n  - ref: %s\nrules: {ref: %s}\n"+
		"form:\n  mixed-port: %d\n  mode: rule\n  dns: {enable: true, enhanced-mode: fake-ip, nameserver: [https://doh.example.com/dns-query]}\n", src.ID, rs.ID, port)
	p := decode[profileDTO](t, c.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	diags := string(p.Render.Diagnostics)
	if p.Render.Status != "ok" || !strings.Contains(diags, "kernel.passed") {
		t.Fatalf("应通过内核检查：%+v %s", p.Render, diags)
	}
	if !strings.Contains(diags, "target.managed_field") {
		t.Errorf("openclash 预设应提示接管字段：%s", diags)
	}

	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+p.ID+"/links", map[string]string{}))
	resp, body := h.client().do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("订阅：%d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "# TARGET: openclash") || !strings.Contains(string(body), "rule-providers:") {
		t.Fatalf("订阅内容不符合预期：\n%s", body)
	}
	loadAndListen(t, bin, geo, body, port)
}

// loadAndListen 把订阅写入临时目录：先 mihomo -t，再实际启动，等待 mixed-port 开始监听。
func loadAndListen(t *testing.T, bin, geo string, body []byte, port int) {
	t.Helper()
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-d", dir, "-f", cfg)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond); err == nil {
			_ = conn.Close()
			t.Logf("mihomo 已加载订阅并在 %d 端口监听（%d 字节）", port, len(body))
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("mihomo 没有在 mixed-port 上开始监听")
}
