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
	"time"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/auth"
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
	svc := &service.Service{Store: db, Catalog: cat, CatalogDigest: digest, Presets: presets, Tools: tools, Kernel: checker, Fetcher: fetcher, Log: log, PoWBits: 4}
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
	// 契约：数组字段即使为空也编码为 []，不是 null（没有原始编辑时 touched 为空）
	raw := string(c.want(200, "POST", "/api/v1/profiles/"+pid+"/preview", map[string]string{"doc": p.Doc}))
	for _, k := range []string{`"touched":[]`, `"numbering":[]`} {
		if !strings.Contains(raw, k) {
			t.Errorf("预览响应缺少 %s：%s", k, raw)
		}
	}
	if strings.Contains(raw, `":null`) {
		t.Errorf("预览响应中有 null 数组：%s", raw)
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
	snap := decode[snapshotDTO](t, c.want(200, "POST", "/api/v1/sources/"+s.ID+"/refresh", nil))
	if snap.LastError != "" || snap.Hash == "" {
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

// 注册：默认关闭；关闭时只能凭有效的一次性邀请注册，邀请用过即作废、过期失效；
// 管理员可以打开注册；普通用户访问管理接口得到 404。
func TestRegistrationAndInvites(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	anon := h.client()
	if st := decode[registrationDTO](t, anon.want(200, "GET", "/api/v1/register", nil)); st.Open {
		t.Fatal("注册应默认关闭")
	}
	reg := func(user, invite string) (*http.Response, []byte) {
		return anon.do("POST", "/api/v1/register", registerBody(t, anon, user, invite))
	}
	if r, _ := reg("alice", ""); r.StatusCode != 403 {
		t.Fatalf("关闭时无邀请应拒绝：%d", r.StatusCode)
	}
	inv := decode[inviteDTO](t, admin.want(201, "POST", "/api/v1/admin/invites", map[string]any{"hours": 24, "note": "给朋友"}))
	tok := inv.URL[strings.Index(inv.URL, "#")+1:]
	if !strings.Contains(inv.URL, "/register#") || inv.ExpiresAt == nil {
		t.Fatalf("邀请：%+v", inv)
	}
	if r, b := reg("alice", tok); r.StatusCode != 201 {
		t.Fatalf("凭邀请注册：%d %s", r.StatusCode, b)
	}
	if r, _ := reg("carol", tok); r.StatusCode != 403 {
		t.Errorf("邀请只能用一次：%d", r.StatusCode)
	}
	// 过期的邀请
	tok2, inv2, _ := h.svc.CreateInvite(context.Background(), firstAdmin(t, h), time.Nanosecond, "")
	time.Sleep(5 * time.Millisecond)
	if r, _ := reg("dave", tok2); r.StatusCode != 403 {
		t.Errorf("过期的邀请应失效：%d", r.StatusCode)
	}
	list := decode[list[inviteDTO]](t, admin.want(200, "GET", "/api/v1/admin/invites", nil))
	statuses := map[string]string{}
	for _, x := range list.Items {
		statuses[x.ID] = x.Status
		if x.URL != "" {
			t.Error("列表中不应再返回邀请链接")
		}
	}
	if statuses[inv.ID] != "used" || statuses[inv2.ID] != "expired" {
		t.Errorf("邀请状态：%v", statuses)
	}
	admin.want(204, "DELETE", "/api/v1/admin/invites/"+inv2.ID, nil)
	// 普通用户看不到管理接口
	alice := h.client()
	alice.login("alice", "a long enough password")
	alice.want(404, "GET", "/api/v1/admin/invites", nil)
	alice.want(404, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	// 打开注册后无需邀请
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	if r, b := reg("erin", ""); r.StatusCode != 201 {
		t.Fatalf("开放注册：%d %s", r.StatusCode, b)
	}
	if r, _ := reg("frank", "stale-invite"); r.StatusCode != 201 {
		t.Errorf("开放注册时应忽略邀请参数：%d", r.StatusCode)
	}
	if r, _ := reg(" bob", ""); r.StatusCode != 422 {
		t.Errorf("首尾空白的用户名应报错而不是被改写：%d", r.StatusCode)
	}
	if r, _ := reg("ab", ""); r.StatusCode != 422 {
		t.Errorf("过短的用户名应拒绝：%d", r.StatusCode)
	}
	if r, b := reg("ALICE", ""); r.StatusCode != 409 || !strings.Contains(string(b), `"exists"`) {
		t.Errorf("用户名应不区分大小写地唯一，且返回 409 exists：%d %s", r.StatusCode, b)
	}
}

func firstAdmin(t *testing.T, h *harness) string {
	u, err := h.db.Users().ByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// registerBody 构造注册请求：三个安全问题与已求解的工作量证明（测试中难度为 4 位）。
func registerBody(t *testing.T, c *client, user, invite string) map[string]any {
	t.Helper()
	return map[string]any{
		"username":  user,
		"password":  "a long enough password",
		"invite":    invite,
		"questions": []map[string]any{{"builtin": 0}, {"builtin": 1}, {"builtin": -1, "text": "我的第一辆自行车"}},
		"answers":   []string{"Beijing", "Mr. Whiskers", "红色"},
		"pow":       solvePoW(t, c, "register"),
	}
}

func solvePoW(t *testing.T, c *client, purpose string) map[string]string {
	t.Helper()
	ch := decode[struct {
		Challenge string `json:"challenge"`
		Bits      int    `json:"bits"`
	}](t, c.want(200, "GET", "/api/v1/pow?purpose="+purpose, nil))
	return map[string]string{"challenge": ch.Challenge, "nonce": auth.Solve(ch.Challenge, ch.Bits)}
}
