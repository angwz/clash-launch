package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// 静态文件：页面路径回退到 index.html（不缓存）；资源长期缓存，缺失时 404；
// API 与订阅前缀下的未知路径、点文件都返回 404，不回退到页面。
func TestStaticHandler(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>app</title>")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"changelog/en.md":      {Data: []byte("# Changelog")},
		"assets/app-1.js":      {Data: []byte("console.log(1)")},
		".gitkeep":             {Data: nil},
		"assets/.hidden":       {Data: []byte("x")},
	}
	h := staticHandler(fsys)
	cases := []struct {
		path   string
		status int
		body   string
		cache  string
	}{
		{"/", 200, "<title>app</title>", "no-cache"},
		{"/profiles/abc", 200, "<title>app</title>", "no-cache"},
		{"/index.html", 301, "", ""}, // 标准库把 /index.html 重定向到 /
		{"/favicon.svg", 200, "<svg/>", ""},
		{"/manifest.webmanifest", 200, "{}", ""},
		{"/changelog/en.md", 200, "# Changelog", "no-cache"},
		{"/assets/app-1.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"/assets/missing.js", 404, "", ""},
		{"/api/v1/unknown", 404, `"not_found"`, ""},
		{"/s/unknown/extra", 404, "", ""},
		{"/.gitkeep", 404, "", ""},
		{"/assets/.hidden", 404, "", ""},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != c.status {
			t.Errorf("%s：状态 %d，期望 %d", c.path, w.Code, c.status)
			continue
		}
		if c.body != "" && !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("%s：响应 %q 不含 %q", c.path, w.Body.String(), c.body)
		}
		if got := w.Header().Get("Cache-Control"); c.cache != "" && got != c.cache {
			t.Errorf("%s：Cache-Control %q，期望 %q", c.path, got, c.cache)
		}
	}
}

// 安装为应用（R29）与更新日志（R27）的文件类型：系统类型表里不一定有，显式给出。
func TestStaticTypes(t *testing.T) {
	h := staticHandler(fstest.MapFS{
		"index.html":           {Data: []byte("x")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"changelog/zh-CN.md":   {Data: []byte("# 更新日志")},
	})
	for path, want := range map[string]string{"/manifest.webmanifest": "application/manifest+json", "/changelog/zh-CN.md": "text/markdown; charset=utf-8"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if got := w.Header().Get("Content-Type"); got != want {
			t.Errorf("%s：Content-Type %q，期望 %q", path, got, want)
		}
	}
}
