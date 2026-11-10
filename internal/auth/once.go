package auth

import (
	"sync"
	"time"
)

// Once 记住已经用过的一次性值（挑战、凭证）直到它们过期，用来拒绝重放。
// 通行密钥需要它：同步型通行密钥（如 iCloud 钥匙串）的签名计数始终为 0，不能靠计数发现重放。
type Once struct {
	mu   sync.Mutex
	used map[string]time.Time
}

// NewOnce 创建一个空集合。
func NewOnce() *Once { return &Once{used: map[string]time.Time{}} }

// Use 第一次使用 key 时返回 true，并记住它直到 exp；之后（未过期前）返回 false。
func (o *Once) Use(key string, exp, now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for k, e := range o.used {
		if now.After(e) {
			delete(o.used, k)
		}
	}
	if _, dup := o.used[key]; dup {
		return false
	}
	o.used[key] = exp
	return true
}
