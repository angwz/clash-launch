package service

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/angwz/clash-launch/internal/match"
	"github.com/angwz/clash-launch/internal/source"
)

// TraceConfig 是分流追踪的部署配置（routing 第 6 节）。零值表示开启、缓存 256 MiB、没有地理数据。
type TraceConfig struct {
	Disabled   bool       // CLASH_LAUNCH_TRACE=off：不下载规则内容，追踪与体检返回 feature_disabled
	CacheBytes int64      // CLASH_LAUNCH_TRACE_CACHE_MB；0 表示默认 256 MiB
	Geo        *match.Geo // 与内核检查共用 CLASH_LAUNCH_GEODATA；nil 时地理规则无法判断
}

const (
	defaultTraceCache = 256 << 20
	maxRuleTTL        = 6 * time.Hour   // 规则内容最多缓存这么久就重新拉取
	failTTL           = 5 * time.Minute // 拉取失败后这么久内不再重试，避免地图轮询反复请求上游
	prefetchWorkers   = 4               // 后台预取的并发数（全站共用）
)

// ruleSpec 描述最终配置中的一个 rule-provider。
type ruleSpec struct {
	Name, Type, Behavior, Format, URL string
	Interval                          time.Duration
	Payload                           []string // type: inline
}

// key 是缓存键：同一地址按不同的行为与格式解析得到的是不同的规则集；classical 里的 GEOIP 还取决于 geodata-mode。
func (r ruleSpec) key(geodataMode bool) string {
	k := r.URL + "\x00" + r.Behavior + "\x00" + r.Format
	if r.Behavior == "classical" {
		k += "\x00" + strconv.FormatBool(geodataMode)
	}
	return k
}

type cacheEntry struct {
	key       string
	set       *match.Set
	err       error // 最近一次拉取或编译的错误；set 不为 nil 时表示内容可能过期
	hash      string
	fetchedAt time.Time // 最近一次尝试的时间
	goodAt    time.Time // 最近一次成功的时间
	size      int64
}

// ruleCache 是规则内容的内存缓存：按内容字节做 LRU，同一地址的并发拉取合并为一次。不落盘（决定 0007 #5）。
type ruleCache struct {
	mu    sync.Mutex
	max   int64
	size  int64
	items map[string]*list.Element // 键 → *cacheEntry
	lru   *list.List               // 最近使用的在前
	sf    singleflight.Group
	sem   chan struct{}   // 后台预取的并发上限
	busy  map[string]bool // 正在后台预取的键：地图轮询时不重复启动
}

func newRuleCache(max int64) *ruleCache {
	if max <= 0 {
		max = defaultTraceCache
	}
	return &ruleCache{max: max, items: map[string]*list.Element{}, lru: list.New(), sem: make(chan struct{}, prefetchWorkers), busy: map[string]bool{}}
}

func (c *ruleCache) peek(key string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.lru.MoveToFront(el)
		e := *el.Value.(*cacheEntry)
		return &e
	}
	return nil
}

func (c *ruleCache) put(e *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[e.key]; ok {
		c.size -= el.Value.(*cacheEntry).size
		el.Value = e
		c.lru.MoveToFront(el)
	} else {
		c.items[e.key] = c.lru.PushFront(e)
	}
	c.size += e.size
	for c.size > c.max && c.lru.Len() > 1 {
		last := c.lru.Back()
		old := last.Value.(*cacheEntry)
		c.lru.Remove(last)
		delete(c.items, old.key)
		c.size -= old.size
	}
}

// traceCache 返回进程内唯一的规则内容缓存。
func (s *Service) traceCache() *ruleCache {
	s.traceOnce.Do(func() { s.rules = newRuleCache(s.Trace.CacheBytes) })
	return s.rules
}

// ruleTTL 是规则内容的有效期：min(rule-provider 的 interval, 6 小时)。
func ruleTTL(interval time.Duration) time.Duration {
	if interval > 0 && interval < maxRuleTTL {
		return interval
	}
	return maxRuleTTL
}

// cachedSet 只读缓存，不拉取：用于地图（内容已缓存时才给出条数与样例）。stale 表示内容已过有效期或最近一次拉取失败。
func (s *Service) cachedSet(spec ruleSpec, env match.Env) (set *match.Set, stale bool) {
	if spec.Type == "inline" {
		set, err := match.CompileInline(spec.Name, spec.Behavior, spec.Payload, env)
		if err != nil {
			return nil, false
		}
		return set, false
	}
	e := s.traceCache().peek(spec.key(env.GeodataMode))
	if e == nil || e.set == nil {
		return nil, false
	}
	return e.set, e.err != nil || s.now().Sub(e.goodAt) > ruleTTL(spec.Interval)
}

// loadSet 取得规则集内容：有效期内直接用缓存，否则拉取（同一地址并发合并）。拉取失败时如有旧内容则继续使用并标记 stale。
// hosts 是请求者的站点白名单（普通用户只允许公共代码托管站，反馈 #72）；不允许的地址即使已在缓存中也不返回，
// 避免借管理员拉取过的内容绕过白名单。
func (s *Service) loadSet(ctx context.Context, spec ruleSpec, env match.Env, hosts source.Hosts) (set *match.Set, stale bool, err error) {
	switch {
	case spec.Type == "inline":
		set, err := match.CompileInline(spec.Name, spec.Behavior, spec.Payload, env)
		return set, false, err
	case spec.URL == "":
		return nil, false, fmt.Errorf("规则集 %s 没有地址（只有 path）", spec.Name)
	}
	if hosts != nil {
		u, perr := url.Parse(spec.URL)
		if perr != nil || !hosts.Allows(u) {
			return nil, false, fmt.Errorf("%w：%s", source.ErrHostNotAllowed, spec.URL)
		}
	}
	c := s.traceCache()
	key := spec.key(env.GeodataMode)
	now := s.now()
	if e := c.peek(key); e != nil {
		fresh := e.set != nil && e.err == nil && now.Sub(e.goodAt) <= ruleTTL(spec.Interval)
		recentFail := e.err != nil && now.Sub(e.fetchedAt) < failTTL
		if fresh || recentFail {
			if e.set == nil {
				return nil, false, e.err
			}
			return e.set, e.err != nil, nil
		}
	}
	v, _, _ := c.sf.Do(key, func() (any, error) {
		prev := c.peek(key)
		// 拉取不随单个请求取消：多个请求可能在等同一次拉取，内容也会留在缓存中供之后使用
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		resp, ferr := s.Fetcher.Fetch(fctx, source.Request{URL: spec.URL, Hosts: hosts})
		e := &cacheEntry{key: key, fetchedAt: s.now()}
		if ferr == nil {
			sum := sha256.Sum256(resp.Body)
			e.hash = hex.EncodeToString(sum[:])
			if prev != nil && prev.set != nil && prev.hash == e.hash {
				e.set = prev.set // 内容没变，不重新编译
			} else {
				e.set, ferr = match.Compile(spec.Name, spec.Behavior, spec.Format, resp.Body, env)
			}
			e.size = int64(len(resp.Body))
			e.goodAt = e.fetchedAt
		}
		if ferr != nil {
			e.err = ferr
			if prev != nil && prev.set != nil { // 继续用旧内容
				e.set, e.hash, e.size, e.goodAt = prev.set, prev.hash, prev.size, prev.goodAt
			} else {
				e.set, e.size = nil, 0
			}
		}
		c.put(e)
		return e, nil
	})
	e := v.(*cacheEntry)
	if e.set == nil {
		return nil, false, e.err
	}
	return e.set, e.err != nil, nil
}

// prefetch 在后台拉取尚未缓存的规则集，供地图随后显示条数与样例。返回仍在等待的个数。
func (s *Service) prefetch(specs []ruleSpec, env match.Env, hosts source.Hosts) int {
	c := s.traceCache()
	pending := 0
	for _, spec := range specs {
		if spec.Type == "inline" || spec.URL == "" {
			continue
		}
		key := spec.key(env.GeodataMode)
		if e := c.peek(key); e != nil && (e.set != nil || s.now().Sub(e.fetchedAt) < failTTL) {
			continue
		}
		pending++
		c.mu.Lock()
		running := c.busy[key]
		c.busy[key] = true
		c.mu.Unlock()
		if running {
			continue
		}
		go func(spec ruleSpec) {
			defer func() {
				c.mu.Lock()
				delete(c.busy, key)
				c.mu.Unlock()
			}()
			c.sem <- struct{}{}
			defer func() { <-c.sem }()
			if _, _, err := s.loadSet(context.Background(), spec, env, hosts); err != nil && s.Log != nil {
				s.Log.Debug("预取规则集失败", "url", spec.URL, "error", err)
			}
		}(spec)
	}
	return pending
}
