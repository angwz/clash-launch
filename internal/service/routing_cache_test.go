package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/match"
	"github.com/angwz/clash-launch/internal/source"
)

func TestRuleCacheLRU(t *testing.T) {
	c := newRuleCache(10)
	c.put(&cacheEntry{key: "a", size: 6})
	c.put(&cacheEntry{key: "b", size: 3})
	c.peek("a") // a 最近用过
	c.put(&cacheEntry{key: "c", size: 4})
	if c.peek("b") != nil || c.peek("a") == nil || c.peek("c") == nil || c.size != 10 {
		t.Errorf("应淘汰最久未用的 b：size=%d", c.size)
	}
	c.put(&cacheEntry{key: "a", size: 1}) // 替换时按新大小计
	if c.size != 5 {
		t.Errorf("替换后 size=%d", c.size)
	}
}

func TestLoadSetStaleAndSingleFlight(t *testing.T) {
	var fail atomic.Bool
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(20 * time.Millisecond) // 让并发请求重叠
		if fail.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("+.example.com\n"))
	}))
	defer srv.Close()
	guard, _ := source.ParseAllowlist("127.0.0.1")
	f, _ := source.New(source.Config{Guard: guard})
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	s := &Service{Fetcher: f, Now: func() time.Time { return now }}
	spec := ruleSpec{Name: "x", Type: "http", Behavior: "domain", Format: "text", URL: srv.URL + "/x.list", Interval: time.Hour}

	// 并发的 5 个请求只拉取一次
	done := make(chan *match.Set, 5)
	for i := 0; i < 5; i++ {
		go func() {
			set, _, err := s.loadSet(context.Background(), spec, match.Env{}, nil)
			if err != nil {
				t.Error(err)
			}
			done <- set
		}()
	}
	for i := 0; i < 5; i++ {
		if set := <-done; set == nil || set.Count != 1 {
			t.Fatalf("内容：%+v", set)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("并发拉取应合并为一次，实际 %d 次", hits.Load())
	}
	// 有效期内不再拉取
	if _, stale, _ := s.loadSet(context.Background(), spec, match.Env{}, nil); stale || hits.Load() != 1 {
		t.Errorf("有效期内：stale=%v hits=%d", stale, hits.Load())
	}
	// 过期后上游失败：继续用旧内容并标记过期；失败后 5 分钟内不重试
	now = now.Add(2 * time.Hour)
	fail.Store(true)
	set, stale, err := s.loadSet(context.Background(), spec, match.Env{}, nil)
	if err != nil || set == nil || !stale || hits.Load() != 2 {
		t.Fatalf("上游失败：set=%v stale=%v err=%v hits=%d", set != nil, stale, err, hits.Load())
	}
	if _, stale, _ := s.loadSet(context.Background(), spec, match.Env{}, nil); !stale || hits.Load() != 2 {
		t.Errorf("失败后应暂缓重试：hits=%d", hits.Load())
	}
	if _, stale := s.cachedSet(spec, match.Env{}); !stale {
		t.Error("地图读取缓存时也应看到过期标记")
	}
	// 白名单外的地址：即使已缓存也不返回
	if _, _, err := s.loadSet(context.Background(), spec, match.Env{}, source.Hosts{"raw.githubusercontent.com"}); err == nil {
		t.Error("白名单外的地址应被拒绝")
	}
}

func TestRawOverrides(t *testing.T) {
	cases := map[string]bool{
		"":                                    false,
		"dns: {enable: true}\n":               false,
		"rules: !prepend\n  - MATCH,DIRECT\n": false,
		"proxy-groups: !patch-by:name\n  - {name: a, type: select}\n": false,
		"rule-providers:\n  x: {type: inline}\n":                      false,
		"rules:\n  - MATCH,DIRECT\n":                                  true,
		"proxy-groups: !replace []\n":                                 true,
		"rule-providers: !delete ~\n":                                 true,
	}
	for raw, want := range cases {
		if got := rawOverrides(raw); got != want {
			t.Errorf("%q：%v，期望 %v", raw, got, want)
		}
	}
}

func TestHealthSites(t *testing.T) {
	sites, err := LoadHealthSites((&Service{}).catalogFS())
	if err != nil || len(sites) < 25 {
		t.Fatalf("%d %v", len(sites), err)
	}
	seen := map[string]bool{}
	for _, s := range sites {
		if seen[s.Host] {
			t.Errorf("重复：%s", s.Host)
		}
		seen[s.Host] = true
	}
}
