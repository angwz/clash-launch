package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHostsAllows(t *testing.T) {
	h := Hosts{"raw.githubusercontent.com"}
	for raw, want := range map[string]bool{
		"https://raw.githubusercontent.com/a/b/main/x.ini": true,
		"https://RAW.githubusercontent.com/x":              true,
		"http://raw.githubusercontent.com/x":               false, // 只接受 https
		"https://evil.raw.githubusercontent.com/x":         false, // 不放行子域名
		"https://raw.githubusercontent.com.example.com/x":  false,
		"https://user@raw.githubusercontent.com/x":         false,
		"https://example.com/x":                            false,
	} {
		u, _ := url.Parse(raw)
		if got := h.Allows(u); got != want {
			t.Errorf("%s：%v，期望 %v", raw, got, want)
		}
	}
	if err := h.Check("https://example.com/a"); !errors.Is(err, ErrHostNotAllowed) || !errors.Is(err, ErrBlocked) || !strings.Contains(err.Error(), "example.com") {
		t.Errorf("Check 应返回带主机名的 ErrHostNotAllowed：%v", err)
	}
	if hs, err := ParseHosts(" GitHub.com , cdn.jsdelivr.net,"); err != nil || len(hs) != 2 || hs[0] != "github.com" {
		t.Errorf("ParseHosts：%v %v", hs, err)
	}
	for _, bad := range []string{"https://github.com", "*.github.com", "github.com/x"} {
		if _, err := ParseHosts(bad); err == nil {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
}

// 重定向的每一跳都检查站点：允许的站点重定向到其他站点时拒绝。
func TestFetchHostsRedirect(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, "https://localhost"+strings.TrimPrefix(r.Host, "127.0.0.1")+"/ok", http.StatusFound)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	f, err := New(Config{Guard: localGuard()})
	if err != nil {
		t.Fatal(err)
	}
	f.client.Transport.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	ctx := context.Background()
	h := Hosts{"127.0.0.1"}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/ok", Hosts: h}); err != nil {
		t.Fatalf("允许的站点：%v", err)
	}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/away", Hosts: h}); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("重定向到其他站点应被拒绝：%v", err)
	}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/ok", Hosts: Hosts{"example.com"}}); !errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("不在列表中的站点应被拒绝：%v", err)
	}
	if _, err := f.Fetch(ctx, Request{URL: srv.URL + "/away"}); err != nil && errors.Is(err, ErrHostNotAllowed) {
		t.Errorf("不限制站点时不应按站点拒绝：%v", err)
	}
}
