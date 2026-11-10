package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

// 安全测试清单（第 14.5 节）：SSRF 的各类地址、重定向、DNS rebinding、体积上限。

func TestCheckAddr(t *testing.T) {
	var g Guard
	blocked := []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0",
		"224.0.0.1", "240.0.0.1", "255.255.255.255", "198.18.0.1", "192.0.2.10", "::1", "fe80::1", "fc00::1", "ff02::1",
		"::ffff:127.0.0.1", "64:ff9b::a00:1", "2002:7f00:1::", "::", "2001:db8::1"}
	for _, s := range blocked {
		if err := g.CheckAddr(netip.MustParseAddr(s)); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s 应被拒绝", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "64:ff9b::808:808"} {
		if err := g.CheckAddr(netip.MustParseAddr(s)); err != nil {
			t.Errorf("%s 不应被拒绝：%v", s, err)
		}
	}
}

func TestCheckHost(t *testing.T) {
	var g Guard
	for _, h := range []string{"localhost", "a.localhost", "router", "nas.local", "x.internal", "box.lan", "0x7f.1", "2130706433", "0177.0.0.1", "[::1]", ""} {
		if err := g.CheckHost(h); !errors.Is(err, ErrBlocked) {
			t.Errorf("主机 %q 应被拒绝", h)
		}
	}
	for _, h := range []string{"example.com", "bad.cafe", "sub.example.org", "1.1.1.1"} {
		if err := g.CheckHost(h); err != nil {
			t.Errorf("主机 %q 不应被拒绝：%v", h, err)
		}
	}
	u, _ := url.Parse("file:///etc/passwd")
	if err := g.CheckURL(u); !errors.Is(err, ErrBlocked) {
		t.Error("file:// 应被拒绝")
	}
	allow, err := ParseAllowlist("127.0.0.1, 10.0.0.0/8, intranet")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"127.0.0.1", "10.2.3.4", "intranet"} {
		if err := allow.CheckHost(h); err != nil {
			t.Errorf("白名单中的 %q 应被允许：%v", h, err)
		}
	}
}

// Dialer.Control 检查实际连接的地址：域名在检查之后被解析到内网（DNS rebinding）时仍被拒绝。
func TestControlBlocksRebinding(t *testing.T) {
	var g Guard
	if err := g.control("tcp", "127.0.0.1:443", nil); !errors.Is(err, ErrBlocked) {
		t.Error("连接回环地址应被拒绝")
	}
	if err := g.control("tcp", "[fd00::1]:443", nil); !errors.Is(err, ErrBlocked) {
		t.Error("连接 ULA 地址应被拒绝")
	}
	if err := g.control("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("公网地址不应被拒绝：%v", err)
	}
}

func localGuard() Guard {
	g, _ := ParseAllowlist("127.0.0.1")
	return g
}

func TestDirectFetchAndLimits(t *testing.T) {
	var gotUA, gotRef, gotXFF string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotRef, gotXFF = r.Header.Get("User-Agent"), r.Header.Get("Referer"), r.Header.Get("X-Forwarded-For")
		switch r.URL.Path {
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		case "/redirect-internal":
			http.Redirect(w, r, "http://10.0.0.1/secret", http.StatusFound)
		case "/status":
			w.WriteHeader(http.StatusForbidden)
		default:
			w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=3; expire=4")
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	f, err := New(Config{Guard: localGuard(), MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	resp, err := f.Fetch(ctx, Request{URL: srv.URL + "/ok", Headers: map[string]string{"Referer": "x", "X-Forwarded-For": "1.2.3.4", "Authorization": "Bearer t"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "ok" || resp.Headers["subscription-userinfo"] == "" {
		t.Errorf("响应不符：%+v", resp)
	}
	if gotUA != DefaultUA || gotRef != "" || gotXFF != "" {
		t.Errorf("请求头不应泄露身份信息：UA=%q Referer=%q XFF=%q", gotUA, gotRef, gotXFF)
	}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/big"}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("超过上限应报错：%v", err)
	}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/redirect-internal"}); !errors.Is(err, ErrBlocked) {
		t.Errorf("重定向到内网应被拒绝：%v", err)
	}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/status"}); !errors.Is(err, ErrStatus) {
		t.Errorf("非 2xx 应报错：%v", err)
	}
	// 未加入白名单时，回环地址的测试服务器本身就应被拒绝
	f2, _ := New(Config{})
	if _, err := f2.Fetch(ctx, Request{URL: srv.URL}); !errors.Is(err, ErrBlocked) {
		t.Errorf("回环地址应被拒绝：%v", err)
	}
}

func TestSnapshotUpdate(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ok := &Response{Body: []byte("a"), Hash: "h1", FetchedAt: t0}
	s := Update(nil, ok, nil, true, "", t0)
	s2 := Update(s, nil, ErrStatus, false, "", t0.Add(time.Hour))
	if s2.Hash != "h1" || s2.LastError == "" || s2.FailingSince != t0.Add(time.Hour) {
		t.Fatalf("失败时应沿用旧内容并记录原因：%+v", s2)
	}
	s3 := Update(s2, &Response{Body: []byte(""), Hash: "h2"}, nil, false, "0 proxies", t0.Add(8*24*time.Hour))
	if s3.Hash != "h1" || s3.FailingSince != t0.Add(time.Hour) || !s3.StaleFor(7*24*time.Hour, t0.Add(8*24*time.Hour)) {
		t.Fatalf("不可用的内容按失败处理，失败起点不变：%+v", s3)
	}
	s4 := Update(s3, &Response{Body: []byte("b"), Hash: "h3", FetchedAt: t0.Add(9 * 24 * time.Hour)}, nil, true, "", t0.Add(9*24*time.Hour))
	if s4.Hash != "h3" || !s4.FailingSince.IsZero() || s4.LastError != "" {
		t.Fatalf("成功后应清除失败状态：%+v", s4)
	}
}
