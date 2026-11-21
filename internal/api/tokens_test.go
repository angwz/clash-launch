package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
)

// enableTOTP 为客户端的账号启用两步验证。
func enableTOTP(t *testing.T, c *client) {
	t.Helper()
	st := decode[map[string]string](t, c.want(200, "POST", "/api/v1/account/totp/setup", nil))
	code, _ := auth.TOTPCode(st["secret"], auth.TOTPStep(time.Now()))
	c.want(200, "POST", "/api/v1/account/totp/enable", map[string]string{"code": code})
}

// bearer 用令牌发请求（不带 Cookie 与 CSRF）。
func bearer(h *harness, token, method, path string, body string) (int, string) {
	h.t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, h.http.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

// 个人访问令牌（R25）：明文只返回一次；read 只能读取，write 能修改；账号安全、消息与管理接口一律拒绝；
// 令牌不能管理令牌；吊销、过期、找回密码后失效；创建与吊销记审计。
func TestAccessTokens(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()

	admin.want(422, "POST", "/api/v1/account/tokens", map[string]any{"name": "", "scope": "read"})
	admin.want(422, "POST", "/api/v1/account/tokens", map[string]any{"name": "x", "scope": "admin"})
	ro := decode[tokenDTO](t, admin.want(201, "POST", "/api/v1/account/tokens", map[string]any{"name": "只读脚本", "scope": "read", "days": 30}))
	rw := decode[tokenDTO](t, admin.want(201, "POST", "/api/v1/account/tokens", map[string]any{"name": "写入脚本", "scope": "write"}))
	if !strings.HasPrefix(ro.Token, "clt_") || ro.ExpiresAt == nil || rw.ExpiresAt != nil {
		t.Fatalf("创建：%+v %+v", ro, rw)
	}
	list := decode[map[string][]tokenDTO](t, admin.want(200, "GET", "/api/v1/account/tokens", nil))["items"]
	if len(list) != 2 || list[0].Token != "" || list[1].Token != "" {
		t.Fatalf("列表不应含明文：%+v", list)
	}

	// read：能读，不能写
	if c, b := bearer(h, ro.Token, "GET", "/api/v1/profiles/"+pid, ""); c != 200 || !strings.Contains(b, pid) {
		t.Fatalf("read 读取配置：%d %s", c, b)
	}
	if c, _ := bearer(h, ro.Token, "GET", "/api/v1/me", ""); c != 200 {
		t.Fatalf("read 读取身份：%d", c)
	}
	if c, b := bearer(h, ro.Token, "POST", "/api/v1/sources", `{"name":"x","kind":"batch","content":""}`); c != 403 || !strings.Contains(b, "token.forbidden") {
		t.Fatalf("read 不能写：%d %s", c, b)
	}
	// write：能写（不需要 CSRF），但账号安全、消息、管理、令牌本身都不行
	if c, b := bearer(h, rw.Token, "POST", "/api/v1/sources", `{"name":"脚本加的","kind":"batch","content":"- {name: n1, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example-pw}\n"}`); c != 201 {
		t.Fatalf("write 新建节点源：%d %s", c, b)
	}
	for _, x := range [][2]string{
		{"GET", "/api/v1/account/sessions"}, {"PUT", "/api/v1/account/password"}, {"POST", "/api/v1/account/totp/setup"},
		{"GET", "/api/v1/account/tokens"}, {"POST", "/api/v1/account/tokens"}, {"GET", "/api/v1/admin/users"},
		{"GET", "/api/v1/messages"}, {"GET", "/api/v1/account/prefs"}, {"DELETE", "/api/v1/session"},
	} {
		if c, _ := bearer(h, rw.Token, x[0], x[1], `{}`); c != 403 {
			t.Errorf("%s %s 不应向令牌开放：%d", x[0], x[1], c)
		}
	}
	if c, _ := bearer(h, "clt_not-a-real-token", "GET", "/api/v1/profiles", ""); c != 401 {
		t.Fatalf("无效令牌：%d", c)
	}

	// 吊销后失效
	admin.want(204, "DELETE", "/api/v1/account/tokens/"+ro.ID, nil)
	admin.want(404, "DELETE", "/api/v1/account/tokens/"+ro.ID, nil)
	if c, _ := bearer(h, ro.Token, "GET", "/api/v1/profiles", ""); c != 401 {
		t.Fatalf("吊销后：%d", c)
	}
	// 过期后失效
	ctx := context.Background()
	exp := decode[tokenDTO](t, admin.want(201, "POST", "/api/v1/account/tokens", map[string]any{"name": "短期", "scope": "read", "days": 1}))
	h.svc.Now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	if c, _ := bearer(h, exp.Token, "GET", "/api/v1/profiles", ""); c != 401 {
		t.Fatalf("过期后：%d", c)
	}
	h.svc.Now = nil
	// 最近使用时间被记录
	tk, err := h.db.Tokens().ByHash(ctx, auth.HashToken(rw.Token))
	if err != nil || tk.LastUsedAt.IsZero() || tk.LastIP == "" {
		t.Fatalf("最近使用：%+v %v", tk, err)
	}
	// 管理员重设密码：令牌全部吊销
	u, _ := h.db.Users().ByUsername(ctx, "admin")
	if err := h.svc.AdminResetPassword(ctx, u.ID, u.ID, "another long password"); err != nil {
		t.Fatal(err)
	}
	if c, _ := bearer(h, rw.Token, "GET", "/api/v1/profiles", ""); c != 401 {
		t.Fatalf("重设密码后：%d", c)
	}
}

// 两步验证策略（R41）：管理员自己没启用时不能要求别人；要求后未启用的用户只能设置两步验证；
// 启用后放行；策略适用时不能关闭；令牌同样受限。
func TestMFAPolicy(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bobby", ""))
	bob := h.client()
	bob.login("bobby", "a long enough password")
	tok := decode[tokenDTO](t, bob.want(201, "POST", "/api/v1/account/tokens", map[string]any{"name": "脚本", "scope": "read"}))

	if p := decode[mfaPolicyDTO](t, admin.want(200, "GET", "/api/v1/admin/mfa-policy", nil)); p.Policy != "off" {
		t.Fatalf("默认：%+v", p)
	}
	admin.want(400, "PUT", "/api/v1/admin/mfa-policy", map[string]string{"policy": "everyone"})
	admin.want(409, "PUT", "/api/v1/admin/mfa-policy", map[string]string{"policy": "all"}) // 自己还没启用
	enableTOTP(t, admin)

	// 只要求管理员：bob 不受影响
	admin.want(200, "PUT", "/api/v1/admin/mfa-policy", map[string]string{"policy": "admins"})
	bob.want(200, "GET", "/api/v1/profiles", nil)
	admin.want(409, "POST", "/api/v1/account/totp/disable", map[string]string{"password": "correct horse battery"})

	// 要求全部用户：bob 被拦截，只能设置两步验证
	admin.want(200, "PUT", "/api/v1/admin/mfa-policy", map[string]string{"policy": "all"})
	resp, b := bob.do("GET", "/api/v1/profiles", nil)
	if resp.StatusCode != 403 || !strings.Contains(string(b), "mfa.setup_required") {
		t.Fatalf("未启用时应被拦截：%d %s", resp.StatusCode, b)
	}
	if me := decode[sessionResp](t, bob.want(200, "GET", "/api/v1/me", nil)); !me.MFASetup {
		t.Fatal("me 应报告需要设置两步验证")
	}
	if c, _ := bearer(h, tok.Token, "GET", "/api/v1/profiles", ""); c != 403 {
		t.Fatalf("令牌同样受限：%d", c)
	}
	bob.want(200, "GET", "/api/v1/account/totp", nil)
	enableTOTP(t, bob)
	bob.want(200, "GET", "/api/v1/profiles", nil)
	if me := decode[sessionResp](t, bob.want(200, "GET", "/api/v1/me", nil)); me.MFASetup {
		t.Fatal("启用后不再需要设置")
	}
	bob.want(409, "POST", "/api/v1/account/totp/disable", map[string]string{"password": "a long enough password"})

	// 关闭策略后可以关闭两步验证
	admin.want(200, "PUT", "/api/v1/admin/mfa-policy", map[string]string{"policy": "off"})
	bob.want(204, "POST", "/api/v1/account/totp/disable", map[string]string{"password": "a long enough password"})
}
