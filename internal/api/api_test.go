package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/target"
)

// TestContract：路由表与 api/openapi.yaml 的路径和方法逐条一致（第 12.1 节）。
func TestContract(t *testing.T) {
	b, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var spec []string
	for p, ops := range doc.Paths {
		for m := range ops {
			spec = append(spec, strings.ToUpper(m)+" "+p)
		}
	}
	h := newHarness(t, nil)
	var impl []string
	for _, r := range h.srv.Routes() {
		impl = append(impl, r.Method+" "+r.Pattern)
	}
	slices.Sort(spec)
	slices.Sort(impl)
	for _, x := range impl {
		if !slices.Contains(spec, x) {
			t.Errorf("路由未写入契约：%s", x)
		}
	}
	for _, x := range spec {
		if !slices.Contains(impl, x) {
			t.Errorf("契约中的路由没有实现：%s", x)
		}
	}
}

type harness struct {
	t    *testing.T
	srv  *Server
	http *httptest.Server
	db   *store.DB
	svc  *service.Service
}

func newHarness(t *testing.T, checker *kernel.Checker) *harness {
	t.Helper()
	dir := t.TempDir()
	keys, _, err := store.LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), "sqlite:"+filepath.Join(dir, "t.db"), keys)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Migrate(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	cat, _ := catalog.Load(catalogdata.FS, "mihomo")
	digest, _ := catalog.Digest(catalogdata.FS, "mihomo")
	presets, _ := target.Load(catalogdata.FS, "mihomo")
	tools, _ := node.LoadTools(catalogdata.FS)
	guard, _ := source.ParseAllowlist("127.0.0.1")
	fetcher, _ := source.New(source.Config{Guard: guard})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &service.Service{Store: db, Catalog: cat, CatalogDigest: digest, Presets: presets, Tools: tools, Kernel: checker, Fetcher: fetcher, Log: log}
	h := &harness{t: t, db: db, svc: svc}
	h.http = httptest.NewServer(nil)
	t.Cleanup(h.http.Close)
	svc.PublicURL = h.http.URL
	h.srv = New(svc, Config{PublicURL: h.http.URL, Guard: guard}, log)
	h.http.Config.Handler = h.srv.Handler()
	return h
}

type client struct {
	h    *harness
	c    *http.Client
	csrf string
}

func (h *harness) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{h: h, c: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *client) do(method, path string, body any, hdr ...string) (*http.Response, []byte) {
	c.h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.h.http.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.h.http.URL)
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.c.Do(req)
	if err != nil {
		c.h.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, b
}

func (c *client) want(code int, method, path string, body any, hdr ...string) []byte {
	c.h.t.Helper()
	resp, b := c.do(method, path, body, hdr...)
	if resp.StatusCode != code {
		c.h.t.Fatalf("%s %s：%d，期望 %d：%s", method, path, resp.StatusCode, code, b)
	}
	return b
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%v：%s", err, b)
	}
	return v
}

func (c *client) login(user, pw string) {
	b := c.want(200, "POST", "/api/v1/session", map[string]string{"username": user, "password": pw})
	c.csrf = decode[sessionResp](c.h.t, b).CSRF
}

const groupsToml = `[[custom_groups]]
name = "PROXY"
type = "select"
rule = ["[]DIRECT", ".*"]
`
const rulesetsToml = `[[rulesets]]
group = "DIRECT"
ruleset = "[]GEOIP,CN"
[[rulesets]]
group = "PROXY"
ruleset = "[]MATCH"
`

func (h *harness) setup() (*client, string) {
	st, _ := h.svc.NewSetup(context.Background())
	h.srv.SetSetup(st)
	c := h.client()
	c.want(403, "POST", "/api/v1/setup", map[string]string{"token": "wrong", "username": "admin", "password": "correct horse battery"})
	c.want(422, "POST", "/api/v1/setup", map[string]string{"token": st.Token, "username": "admin", "password": "short"})
	c.want(201, "POST", "/api/v1/setup", map[string]string{"token": st.Token, "username": "admin", "password": "correct horse battery"})
	c.want(403, "POST", "/api/v1/setup", map[string]string{"token": st.Token, "username": "again", "password": "correct horse battery"})
	c.login("admin", "correct horse battery")
	src := decode[sourceDTO](h.t, c.want(201, "POST", "/api/v1/sources", map[string]any{
		"name": "我的 VPS", "kind": "batch",
		"content": "- {name: 香港 01, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pw}\n- {name: 日本 02, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pw}\n",
	}))
	rs := decode[ruleSourceDTO](h.t, c.want(201, "POST", "/api/v1/rule-sources", map[string]any{"name": "rules", "kind": "toml", "groupsText": groupsToml, "rulesetsText": rulesetsToml}))
	doc := "clash-launch/profile: v1\nname: 测试\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n  - ref: " + src.ID + "\nrules: {ref: " + rs.ID + "}\nform: {mixed-port: 7890}\n"
	p := decode[profileDTO](h.t, c.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	if p.Render == nil || p.Render.Status != "ok" {
		h.t.Fatalf("创建后应渲染成功：%+v %s", p.Render, p.Render.Diagnostics)
	}
	return c, p.ID
}

func TestEndToEnd(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	// 未登录的请求
	anon := h.client()
	anon.want(401, "GET", "/api/v1/profiles", nil)
	// CSRF：缺少令牌或跨源时拒绝写操作
	saved := c.csrf
	c.csrf = ""
	c.want(403, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{})
	c.csrf = saved
	c.want(403, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{}, "Origin", "https://evil.example.com")
	// 订阅链接
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	resp, body := anon.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "text/yaml") {
		t.Fatalf("订阅：%d %s", resp.StatusCode, body)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(body, &cfg); err != nil || cfg["mixed-port"] != 7890 || len(cfg["proxies"].([]any)) != 2 {
		t.Fatalf("订阅内容：%v\n%s", err, body)
	}
	if resp.Header.Get("profile-update-interval") != "12" || !strings.Contains(resp.Header.Get("Content-Disposition"), "UTF-8''") {
		t.Errorf("响应头：%v", resp.Header)
	}
	r304, _ := anon.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil, "If-None-Match", resp.Header.Get("ETag"))
	if r304.StatusCode != 304 {
		t.Errorf("If-None-Match 应返回 304：%d", r304.StatusCode)
	}
	// 重置：旧链接立即失效
	l2 := decode[linkDTO](t, c.want(200, "POST", "/api/v1/profiles/"+pid+"/links/"+l.ID+"/reset", nil, "If-Match", `"v1"`))
	if r, _ := anon.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil); r.StatusCode != 404 {
		t.Errorf("重置后旧链接应失效：%d", r.StatusCode)
	}
	if r, _ := anon.do("GET", strings.TrimPrefix(l2.URL, h.http.URL), nil); r.StatusCode != 200 {
		t.Errorf("新链接应可用：%d", r.StatusCode)
	}
	// If-Match：缺少为 400，过期为 412
	p := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	c.want(400, "PUT", "/api/v1/profiles/"+pid, map[string]string{"doc": p.Doc})
	c.want(200, "PUT", "/api/v1/profiles/"+pid, map[string]string{"doc": p.Doc + "raw: |\n  rules: !prepend\n    - DOMAIN,intranet.example.com,DIRECT\n"}, "If-Match", `"v1"`)
	c.want(412, "PUT", "/api/v1/profiles/"+pid, map[string]string{"doc": p.Doc}, "If-Match", `"v1"`)
	revs := decode[list[revisionDTO]](t, c.want(200, "GET", "/api/v1/profiles/"+pid+"/revisions", nil))
	if len(revs.Items) != 2 {
		t.Fatalf("版本数：%d", len(revs.Items))
	}
	c.want(200, "POST", "/api/v1/profiles/"+pid+"/rollback", map[string]int{"n": 1}, "If-Match", `"v2"`)
	// 预览不保存
	pv := decode[previewResp](t, c.want(200, "POST", "/api/v1/profiles/"+pid+"/preview", map[string]string{"doc": p.Doc + "raw: |\n  dns: {enable: true}\n"}))
	if !strings.Contains(pv.YAML, "dns:") || len(pv.Diff) == 0 || !slices.Contains(pv.Touched, "dns") {
		t.Errorf("预览：%+v", pv)
	}
	// 越权：另一个用户访问时得到 404（而不是 403，避免泄露资源存在，第 14.3 节）
	if err := h.db.Users().Create(context.Background(), &store.User{Username: "bob", PasswordHash: mustHash("another long password"), Role: "user"}); err != nil {
		t.Fatal(err)
	}
	bob := h.client()
	bob.login("bob", "another long password")
	for _, path := range []string{"/api/v1/profiles/" + pid, "/api/v1/profiles/" + pid + "/links", "/api/v1/profiles/" + pid + "/revisions", "/api/v1/profiles/" + pid + "/render", "/api/v1/profiles/" + pid + "/export"} {
		bob.want(404, "GET", path, nil)
	}
	bob.want(404, "DELETE", "/api/v1/profiles/"+pid, nil)
	bob.want(404, "POST", "/api/v1/profiles/"+pid+"/links/"+l.ID+"/reset", nil, "If-Match", `"v2"`)
	srcs := decode[list[sourceDTO]](t, c.want(200, "GET", "/api/v1/sources", nil))
	bob.want(404, "GET", "/api/v1/sources/"+srcs.Items[0].ID, nil)
	bob.want(404, "GET", "/api/v1/sources/"+srcs.Items[0].ID+"/nodes", nil)
	if b := decode[list[sourceDTO]](t, bob.want(200, "GET", "/api/v1/sources", nil)); len(b.Items) != 0 {
		t.Error("他人的源不应出现在列表中")
	}
	// 普通用户不能打开直连
	bob.want(400, "POST", "/api/v1/sources", map[string]any{"name": "x", "kind": "subscription", "url": "https://sub.example.com/s", "allowDirect": true})
	// SSRF：添加订阅时拒绝内部地址
	c.want(400, "POST", "/api/v1/sources", map[string]any{"name": "x", "kind": "subscription", "url": "http://169.254.169.254/latest"})
	// 节点表格：地址打码
	nodes := decode[map[string]json.RawMessage](t, c.want(200, "GET", "/api/v1/sources/"+srcs.Items[0].ID+"/nodes", nil))
	if strings.Contains(string(nodes["nodes"]), "192.0.2.1") {
		t.Error("节点表格中的地址应打码")
	}
	// 安全响应头
	resp, _ = anon.do("GET", "/healthz", nil)
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") || resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("安全响应头：%v", resp.Header)
	}
}

func TestSubscriptionRateLimitAnd503(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{}))
	path := strings.TrimPrefix(l.URL, h.http.URL)
	anon := h.client()
	var last int
	for i := 0; i < 31; i++ {
		r, _ := anon.do("GET", path, nil)
		last = r.StatusCode
	}
	if last != 429 {
		t.Errorf("第 31 次请求应被限流：%d", last)
	}
	// 尚无成功结果时返回 503 与 YAML 注释
	p2 := decode[profileDTO](t, c.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": "clash-launch/profile: v1\nname: 坏\nraw: \"dns: [unclosed\"\n"}))
	l2 := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+p2.ID+"/links", map[string]string{}))
	r, b := anon.do("GET", strings.TrimPrefix(l2.URL, h.http.URL), nil)
	if r.StatusCode != 503 || !strings.HasPrefix(string(b), "#") {
		t.Errorf("没有成功结果时应 503：%d %s", r.StatusCode, b)
	}
}

func TestSubscriptionFetch(t *testing.T) {
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=100; expire=1900000000")
		_, _ = w.Write([]byte("proxies:\n  - {name: 新加坡 01, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example}\n"))
	}))
	defer sub.Close()
	h := newHarness(t, nil)
	c, _ := h.setup()
	s := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{"name": "机场", "kind": "subscription", "url": sub.URL + "/sub?token=example"}))
	if s.Fetch != "direct" {
		t.Errorf("没有出口时管理员的订阅为直连：%s", s.Fetch)
	}
	snap := decode[snapshotDTO](t, c.want(200, "POST", "/api/v1/sources/"+s.ID+"/refresh", nil))
	if snap.LastError != "" || snap.Hash == "" || snap.Via != "direct" {
		t.Fatalf("拉取：%+v", snap)
	}
	n := decode[map[string]json.RawMessage](t, c.want(200, "GET", "/api/v1/sources/"+s.ID+"/nodes", nil))
	if !strings.Contains(string(n["nodes"]), `"code":"SG"`) {
		t.Errorf("节点：%s", n["nodes"])
	}
}

func mustHash(pw string) string {
	h, err := hashPassword(pw)
	if err != nil {
		panic(err)
	}
	return h
}
