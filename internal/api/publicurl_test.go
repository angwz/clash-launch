package api

import (
	"net/http/httptest"
	"testing"
)

func TestParsePublicURLs(t *testing.T) {
	got, err := ParsePublicURLs(" https://a.example.com/ , http://b.example.com:8080,")
	if err != nil || len(got) != 2 || got[0] != "https://a.example.com" || got[1] != "http://b.example.com:8080" {
		t.Fatalf("解析：%v %v", got, err)
	}
	if got, err := ParsePublicURLs(""); err != nil || got != nil {
		t.Errorf("空值：%v %v", got, err)
	}
	for _, bad := range []string{"a.example.com", "ftp://a.example.com", "https://a.example.com/sub", "https://u:p@a.example.com", "https://a.example.com,https://A.example.com"} {
		if _, err := ParsePublicURLs(bad); err == nil {
			t.Errorf("应拒绝 %q", bad)
		}
	}
}

// 需求 R9：每个请求按 Host 选用对外地址；不在列表中的域名按主地址处理，写请求的 Origin 校验不通过。
func TestPublicURLPerHost(t *testing.T) {
	s := &Server{Cfg: Config{PublicURLs: []string{"https://a.example.com", "http://b.example.com:8080"}}}
	cases := []struct {
		host, origin string
		base         string
		secure, same bool
	}{
		{"a.example.com", "https://a.example.com", "https://a.example.com", true, true},
		{"B.example.com:8080", "http://b.example.com:8080", "http://b.example.com:8080", false, true},
		{"b.example.com:8080", "https://a.example.com", "http://b.example.com:8080", false, false}, // 跨域写请求
		{"evil.example.com", "https://evil.example.com", "https://a.example.com", true, false},     // 伪造 Host
		{"evil.example.com", "https://a.example.com", "https://a.example.com", true, false},       // Host 与 Origin 不一致
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/api/v1/x", nil)
		r.Host = c.host
		r.Header.Set("Origin", c.origin)
		if got := s.baseURL(r); got != c.base {
			t.Errorf("%s：链接基址 %s，应为 %s", c.host, got, c.base)
		}
		if got := s.secure(r); got != c.secure {
			t.Errorf("%s：Secure %v", c.host, got)
		}
		if got := s.sameOrigin(r); got != c.same {
			t.Errorf("%s + %s：Origin 校验 %v，应为 %v", c.host, c.origin, got, c.same)
		}
	}
}
