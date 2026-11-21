package source

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type pushHit struct {
	path, auth, ts, sig, ua string
	body                    map[string]any
	raw                     []byte
}

// startPushServer 启动一个 https 服务器，记录收到的请求；status 是返回的状态码。
func startPushServer(t *testing.T, status int) (*httptest.Server, *Pusher, func() []pushHit) {
	t.Helper()
	var mu sync.Mutex
	var hits []pushHit
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		h := pushHit{path: r.URL.Path, auth: r.Header.Get("Authorization"), ts: r.Header.Get("X-Timestamp"), sig: r.Header.Get("X-Signature-256"), ua: r.Header.Get("User-Agent"), raw: raw}
		_ = json.Unmarshal(raw, &h.body)
		mu.Lock()
		hits = append(hits, h)
		mu.Unlock()
		if status == http.StatusFound {
			http.Redirect(w, r, "https://example.com/", status)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	g, err := ParseAllowlist("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	p := &Pusher{Guard: g, TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, TelegramBase: srv.URL}
	return srv, p, func() []pushHit {
		mu.Lock()
		defer mu.Unlock()
		return append([]pushHit(nil), hits...)
	}
}

func TestPush(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	msg := PushMessage{Kind: "new_device", Title: "标题", Text: "正文", Detail: map[string]string{"ip": "192.0.2.1"}, Time: at}
	srv, p, hits := startPushServer(t, 200)
	tgToken := "123456:" + strings.Repeat("A", 30)

	cases := []struct {
		x     Push
		path  string
		check func(t *testing.T, h pushHit)
	}{
		{Push{Kind: PushWebhook, URL: srv.URL + "/hook?k=1", Secret: "s3cret"}, "/hook", func(t *testing.T, h pushHit) {
			if h.body["title"] != "标题" || h.body["kind"] != "new_device" || h.body["time"] != "2026-11-21T08:00:00Z" {
				t.Errorf("webhook 正文：%s", h.raw)
			}
			mac := hmac.New(sha256.New, []byte("s3cret"))
			mac.Write([]byte(h.ts + "."))
			mac.Write(h.raw)
			if h.ts != "1795248000" || h.sig != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
				t.Errorf("签名：ts=%s sig=%s", h.ts, h.sig)
			}
		}},
		{Push{Kind: PushTelegram, Token: tgToken, Chat: "-1001"}, "/bot" + tgToken + "/sendMessage", func(t *testing.T, h pushHit) {
			if h.body["chat_id"] != "-1001" || h.body["text"] != "标题\n\n正文" {
				t.Errorf("telegram 正文：%s", h.raw)
			}
		}},
		{Push{Kind: PushBark, URL: srv.URL + "/", Key: "devkey"}, "/push", func(t *testing.T, h pushHit) {
			if h.body["device_key"] != "devkey" || h.body["title"] != "标题" || h.body["body"] != "正文" {
				t.Errorf("bark 正文：%s", h.raw)
			}
		}},
		{Push{Kind: PushNtfy, URL: srv.URL, Topic: "alerts", Token: "tk_example"}, "", func(t *testing.T, h pushHit) {
			if h.body["topic"] != "alerts" || h.body["message"] != "正文" || h.auth != "Bearer tk_example" {
				t.Errorf("ntfy：auth=%q 正文 %s", h.auth, h.raw)
			}
		}},
	}
	for i, c := range cases {
		if err := p.Send(ctx, c.x, msg); err != nil {
			t.Fatalf("%s：%v", c.x.Kind, err)
		}
		h := hits()[i]
		if h.path != c.path && (c.path != "" || h.path != "/") {
			t.Errorf("%s 路径 %q", c.x.Kind, h.path)
		}
		if h.ua != "clash-launch" {
			t.Errorf("User-Agent %q", h.ua)
		}
		c.check(t, h)
	}
	// 没有密钥时不签名
	if err := p.Send(ctx, Push{Kind: PushWebhook, URL: srv.URL}, msg); err != nil {
		t.Fatal(err)
	}
	if h := hits()[len(hits())-1]; h.sig != "" || h.ts != "" {
		t.Errorf("无密钥时不应签名：%+v", h)
	}

	// 截断：Telegram 按字符、ntfy 按字节，不切开字符
	if s := truncRunes(strings.Repeat("长", 5000), 4096); len([]rune(s)) != 4096 || !strings.HasSuffix(s, "…") {
		t.Errorf("truncRunes %d", len([]rune(s)))
	}
	if s := truncBytes(strings.Repeat("长", 2000), 4096); len(s) > 4096 || !strings.HasSuffix(s, "…") || !strings.HasPrefix(s, "长") {
		t.Errorf("truncBytes %d", len(s))
	}

	// 设置校验
	bad := []Push{
		{Kind: "slack", URL: srv.URL},
		{Kind: PushWebhook, URL: strings.Replace(srv.URL, "https", "http", 1)},
		{Kind: PushWebhook, URL: "https://user:pw@example.com/hook"},
		{Kind: PushWebhook, URL: "https://example.com/hook#x"},
		{Kind: PushWebhook, URL: "https://example.com/hook", Secret: "有 空格"},
		{Kind: PushTelegram, Token: "abc", Chat: "1"},
		{Kind: PushTelegram, Token: tgToken, Chat: "../x"},
		{Kind: PushBark, URL: "https://example.com", Key: "a/b"},
		{Kind: PushNtfy, URL: "https://example.com", Topic: "a b"},
	}
	for _, x := range bad {
		if err := p.CheckPush(x); !errors.Is(err, ErrPush) {
			t.Errorf("%+v 应被拒绝：%v", x, err)
		}
	}
	// 没有白名单时，内网与本地主机名被拒绝
	strict := &Pusher{}
	for _, u := range []string{srv.URL, "https://localhost/x", "https://10.0.0.1/x", "https://ntfy.lan"} {
		if err := strict.CheckPush(Push{Kind: PushWebhook, URL: u}); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s 应被 SSRF 防护拒绝：%v", u, err)
		}
	}

	// 非 2xx 与重定向都是失败；错误信息不含 URL 与令牌
	_, p500, _ := startPushServer(t, 500)
	err := p500.Send(ctx, Push{Kind: PushTelegram, Token: tgToken, Chat: "1"}, msg)
	if !errors.Is(err, ErrPush) || !strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), tgToken) {
		t.Errorf("500：%v", err)
	}
	srv302, p302, _ := startPushServer(t, http.StatusFound)
	if err := p302.Send(ctx, Push{Kind: PushWebhook, URL: srv302.URL}, msg); !errors.Is(err, ErrPush) || !strings.Contains(err.Error(), "302") {
		t.Errorf("重定向：%v", err)
	}
	// 证书不被信任
	untrusted := &Pusher{Guard: p.Guard, TelegramBase: srv.URL}
	if err := untrusted.Send(ctx, Push{Kind: PushTelegram, Token: tgToken, Chat: "1"}, msg); err == nil || strings.Contains(err.Error(), tgToken) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("证书错误：%v", err)
	}
}
