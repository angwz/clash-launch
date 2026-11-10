package auth

import (
	"sync"
	"time"
)

// Backoff 是按键（账号、IP）记录连续失败的退避器（第 14.1 节：登录失败按账号与 IP 指数退避；
// 找回连续失败后锁定 15 分钟）。前 Free 次失败不等待；之后等待 Base·2^(n-Free-1)，不超过 Max。
// 成功后清零。状态在进程内（单实例假设，见 docs/decisions/0002 #6）。
type Backoff struct {
	Free int
	Base time.Duration
	Max  time.Duration

	mu sync.Mutex
	m  map[string]*backoffEntry
}

type backoffEntry struct {
	fails int
	until time.Time
	last  time.Time
}

// NewBackoff 创建退避器。
func NewBackoff(free int, base, maxWait time.Duration) *Backoff {
	return &Backoff{Free: free, Base: base, Max: maxWait, m: map[string]*backoffEntry{}}
}

// Wait 返回 key 还需要等待的时间；0 表示可以尝试。
func (b *Backoff) Wait(key string, now time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.m[key]
	if e == nil || !now.Before(e.until) {
		return 0
	}
	return e.until.Sub(now)
}

// Fail 记录一次失败，返回之后需要等待的时间。
func (b *Backoff) Fail(key string, now time.Time) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.m[key]
	if e == nil {
		e = &backoffEntry{}
		b.m[key] = e
	}
	// 长时间没有失败时重新计数
	if !e.last.IsZero() && now.Sub(e.last) > 24*time.Hour {
		e.fails = 0
	}
	e.fails++
	e.last = now
	if e.fails <= b.Free {
		return 0
	}
	shift := min(e.fails-b.Free-1, 20)
	d := min(b.Base<<shift, b.Max)
	e.until = now.Add(d)
	if len(b.m) > 100000 {
		for k, v := range b.m {
			if now.Sub(v.last) > 24*time.Hour {
				delete(b.m, k)
			}
		}
	}
	return d
}

// Reset 在成功后清零。
func (b *Backoff) Reset(key string) {
	b.mu.Lock()
	delete(b.m, key)
	b.mu.Unlock()
}
