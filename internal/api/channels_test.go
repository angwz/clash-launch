package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/source"
)

// 提醒渠道（R31）：校验与 SSRF 防护、不返回密钥、加密存储、测试送达后启用、修改回到未验证、
// 提醒发给已启用的渠道并记录结果、数量上限、只能操作自己的渠道。
func TestChannels(t *testing.T) {
	h := newHarness(t, nil)
	var mu sync.Mutex
	var bodies []map[string]any
	fail := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		mu.Lock()
		defer mu.Unlock()
		bodies = append(bodies, m)
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	got := func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), bodies...)
	}
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	g, _ := source.ParseAllowlist("127.0.0.1")
	h.svc.Pusher = &source.Pusher{Guard: g, TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	admin, _ := h.setup()
	const pw = "correct horse battery"

	if r := string(admin.want(200, "GET", "/api/v1/account/channels", nil)); !strings.Contains(r, `"items":[]`) || !strings.Contains(r, `"available":true`) {
		t.Errorf("空列表：%s", r)
	}
	// 校验：http、内网、未知种类、缺字段
	admin.want(422, "POST", "/api/v1/account/channels", map[string]any{"kind": "webhook", "url": "http://example.com/hook"})
	admin.want(422, "POST", "/api/v1/account/channels", map[string]any{"kind": "webhook", "url": "https://10.0.0.1/hook"})
	admin.want(422, "POST", "/api/v1/account/channels", map[string]any{"kind": "slack", "url": "https://example.com/hook"})
	admin.want(422, "POST", "/api/v1/account/channels", map[string]any{"kind": "telegram", "token": "x"})
	admin.want(422, "POST", "/api/v1/account/channels", map[string]any{"kind": "ntfy", "url": "https://example.com", "topic": "a b"})

	hook := srv.URL + "/hook/fake-webhook-path-secret"
	c := decode[map[string]any](t, admin.want(201, "POST", "/api/v1/account/channels", map[string]any{"name": "我的 Webhook", "kind": "webhook", "url": hook, "secret": "fake-sign-secret"}))
	id, _ := c["id"].(string)
	list := string(admin.want(200, "GET", "/api/v1/account/channels", nil))
	if strings.Contains(list, "fake-webhook-path-secret") || strings.Contains(list, "fake-sign-secret") || !strings.Contains(list, `"hasSecret":true`) || !strings.Contains(list, `"enabled":false`) || !strings.Contains(list, `"host":"127.0.0.1`) {
		t.Errorf("列表不应含密钥：%s", list)
	}
	// 加密存储：库里的密文不含明文
	var blob []byte
	// 此时只有一行；不用占位符，两种数据库通用
	if err := h.db.Raw().QueryRowContext(context.Background(), "SELECT config_enc FROM notify_channels").Scan(&blob); err != nil || strings.Contains(string(blob), "fake-sign-secret") || len(blob) == 0 {
		t.Errorf("config_enc 应为密文：%v %q", err, blob)
	}

	// 未启用时提醒不发送
	admin.want(204, "PUT", "/api/v1/account/password", map[string]any{"current": pw, "password": "another strong password"})
	time.Sleep(300 * time.Millisecond)
	if n := len(got()); n != 0 {
		t.Fatalf("未启用的渠道不应收到提醒：%d", n)
	}
	// 测试送达后启用
	v := decode[map[string]any](t, admin.want(200, "POST", "/api/v1/account/channels/"+id+"/test", nil))
	if v["enabled"] != true || v["verifiedAt"] == nil || len(got()) != 1 || got()[0]["kind"] != "test" {
		t.Fatalf("测试后：%v %v", v, got())
	}
	// 提醒发给已启用的渠道
	admin.want(204, "PUT", "/api/v1/account/password", map[string]any{"current": "another strong password", "password": pw})
	waitFor(t, func() bool { return len(got()) == 2 })
	if b := got()[1]; b["kind"] != "password" || !strings.Contains(b["text"].(string), "admin") {
		t.Errorf("提醒内容：%v", b)
	}
	// 发送失败记在渠道上，渠道保持启用
	mu.Lock()
	fail = true
	mu.Unlock()
	admin.want(204, "PUT", "/api/v1/account/password", map[string]any{"current": pw, "password": "another strong password"})
	waitFor(t, func() bool {
		return strings.Contains(string(admin.want(200, "GET", "/api/v1/account/channels", nil)), "HTTP 503")
	})
	if r := string(admin.want(200, "GET", "/api/v1/account/channels", nil)); !strings.Contains(r, `"enabled":true`) {
		t.Errorf("失败后应仍启用：%s", r)
	}
	// 测试失败返回 422，原因可见
	admin.want(422, "POST", "/api/v1/account/channels/"+id+"/test", nil)
	mu.Lock()
	fail = false
	mu.Unlock()

	// 修改：密钥与地址留空时保留，回到未验证
	u := decode[map[string]any](t, admin.want(200, "PUT", "/api/v1/account/channels/"+id, map[string]any{"name": "改名", "kind": "telegram"}))
	if u["enabled"] != false || u["name"] != "改名" || u["kind"] != "webhook" || u["hasSecret"] != true {
		t.Errorf("修改后：%v", u)
	}
	admin.want(422, "PUT", "/api/v1/account/channels/"+id, map[string]any{"name": strings.Repeat("长", 41)})

	// 别人的渠道：404
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	other := h.client()
	other.login("bob", "a long enough password")
	other.want(404, "POST", "/api/v1/account/channels/"+id+"/test", nil)
	other.want(404, "DELETE", "/api/v1/account/channels/"+id, nil)

	// 数量上限
	for i := 0; i < 4; i++ {
		admin.want(201, "POST", "/api/v1/account/channels", map[string]any{"kind": "ntfy", "url": "https://ntfy.example.com", "topic": "t" + string(rune('a'+i))})
	}
	admin.want(409, "POST", "/api/v1/account/channels", map[string]any{"kind": "bark", "url": "https://bark.example.com", "key": "k"})
	admin.want(204, "DELETE", "/api/v1/account/channels/"+id, nil)
	admin.want(404, "DELETE", "/api/v1/account/channels/"+id, nil)
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("等待超时")
}
