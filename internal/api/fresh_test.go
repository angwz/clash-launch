package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/service"
)

// 请求订阅时按需拉取（反馈 #119）：输入新鲜时不打上游；过期时请求拿到新结果；并发请求只拉一次；
// 上游失败时照旧提供上一次成功的结果；上游慢时按时返回旧结果，下一次请求拿到新的。
func TestRefreshOnRequest(t *testing.T) {
	var hits atomic.Int32
	var fail atomic.Bool
	var slow atomic.Int64 // 毫秒
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if d := slow.Load(); d > 0 {
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = fmt.Fprintf(w, "proxies:\n  - {name: 节点 %d, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example}\n", n)
	}))
	defer up.Close()

	h := newHarness(t, nil)
	h.svc.Fresh = service.FreshConfig{After: time.Hour, Wait: 3 * time.Second}
	c, _ := h.setup()
	anon := h.client()
	s := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{"name": "机场", "kind": "subscription", "url": up.URL + "/sub"}))
	c.want(200, "POST", "/api/v1/sources/"+s.ID+"/refresh", nil)
	doc := "clash-launch/profile: v1\nname: 按需\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.34\"}\nsources:\n  - {ref: " + s.ID + ", mode: inline}\n"
	p := decode[profileDTO](t, c.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+p.ID+"/links", map[string]string{}))
	path := strings.TrimPrefix(l.URL, h.http.URL)
	get := func() string {
		r, b := anon.do("GET", path, nil, "User-Agent", "clash.meta")
		if r.StatusCode != 200 {
			t.Fatalf("订阅：%d %s", r.StatusCode, b)
		}
		return string(b)
	}
	waitFor := func(want string) string {
		var body string
		for i := 0; i < 50; i++ {
			if body = get(); strings.Contains(body, want) {
				return body
			}
			time.Sleep(100 * time.Millisecond)
		}
		return body
	}
	if b := waitFor("节点 1"); !strings.Contains(b, "节点 1") {
		t.Fatalf("初次生成：%s", b)
	}
	base := hits.Load()

	// 新鲜：不打上游
	get()
	if hits.Load() != base {
		t.Fatalf("输入新鲜时不应拉取：%d → %d", base, hits.Load())
	}

	// 过期：请求时拉取，这次请求就拿到新结果
	h.svc.Fresh.After = time.Millisecond
	time.Sleep(5 * time.Millisecond)
	if b := get(); !strings.Contains(b, fmt.Sprintf("节点 %d", base+1)) {
		t.Fatalf("过期后应返回新拉取的节点（第 %d 次）：%s", base+1, b)
	}

	// 并发：同一份配置同时有多个请求时只拉一次。间隔取 1 秒：开始时已过期，拉取完成后在并发请求期间保持新鲜
	h.svc.Fresh.After = time.Second
	time.Sleep(1100 * time.Millisecond)
	before := hits.Load()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); get() }()
	}
	wg.Wait()
	if n := hits.Load() - before; n != 1 {
		t.Errorf("8 个并发请求应只拉取 1 次，实际 %d 次", n)
	}

	// 上游失败：照旧提供上一次成功的结果
	h.svc.Fresh.After = time.Millisecond
	last := hits.Load()
	fail.Store(true)
	time.Sleep(5 * time.Millisecond)
	if b := get(); !strings.Contains(b, fmt.Sprintf("节点 %d", last)) {
		t.Errorf("上游失败时应保留上一次的节点：%s", b)
	}
	fail.Store(false)

	// 上游慢：按时返回旧结果，后台完成后下一次请求拿到新的
	h.svc.Fresh.Wait = 200 * time.Millisecond
	slow.Store(1500)
	time.Sleep(5 * time.Millisecond)
	start := time.Now()
	b := get()
	if d := time.Since(start); d > time.Second {
		t.Errorf("上游慢时不应久等：%v", d)
	}
	if strings.Contains(b, fmt.Sprintf("节点 %d", hits.Load())) && hits.Load() > last+1 {
		t.Errorf("等待超时的请求不应已拿到新结果：%s", b)
	}
	slow.Store(0)
	h.svc.Fresh.After = time.Hour // 后台那次拉取完成后不再触发新的拉取
	target := fmt.Sprintf("节点 %d", last+2)
	if b := waitFor(target); !strings.Contains(b, target) {
		t.Errorf("后台拉取完成后应提供新结果 %s：%s", target, b)
	}
}
