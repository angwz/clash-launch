package source

import (
	"errors"
	"time"
)

// Snapshot 是一个源最近一次成功拉取的内容（不可变，按内容哈希标识），加上最近的失败状态。
// 失败（网络错误、非 2xx、内容无法识别、解析后为 0 个节点）时继续使用上次成功的内容（第 5.4 节）。
type Snapshot struct {
	Content   []byte            `json:"-"`
	Hash      string            `json:"hash"`
	Headers   map[string]string `json:"headers,omitempty"`
	FetchedAt time.Time         `json:"fetchedAt"` // 成功拉取的时间（渲染时作为文件头时间的来源）
	Via       string            `json:"via,omitempty"`

	LastError    string    `json:"lastError,omitempty"` // 最近一次失败的原因（已脱敏）
	LastAttempt  time.Time `json:"lastAttempt"`
	FailingSince time.Time `json:"failingSince,omitempty"` // 连续失败的起点；成功后清零
}

// Valid 报告快照是否有可用内容。
func (s *Snapshot) Valid() bool { return s != nil && s.Hash != "" }

// Update 根据一次拉取的结果得到新的快照状态。accept 为 false 表示内容虽已下载、但不可用
// （无法识别或 0 个节点），按失败处理并保留旧内容。prev 不会被修改。
func Update(prev *Snapshot, resp *Response, err error, accept bool, reason string, now time.Time) *Snapshot {
	next := &Snapshot{}
	if prev != nil {
		*next = *prev
	}
	next.LastAttempt = now.UTC()
	if err == nil && resp != nil && accept {
		next.Content, next.Hash, next.Headers, next.FetchedAt, next.Via = resp.Body, resp.Hash, resp.Headers, resp.FetchedAt, resp.Via
		next.LastError, next.FailingSince = "", time.Time{}
		return next
	}
	switch {
	case err != nil:
		next.LastError = Reason(err)
	default:
		next.LastError = reason
	}
	if next.FailingSince.IsZero() {
		next.FailingSince = now.UTC()
	}
	return next
}

// StaleFor 报告连续失败是否超过 d（默认 7 天，可配置），用于方案页顶部的提示。
func (s *Snapshot) StaleFor(d time.Duration, now time.Time) bool {
	return s != nil && !s.FailingSince.IsZero() && now.Sub(s.FailingSince) > d
}

// Reason 把错误转换为可以展示与存储的原因（不含 URL 与凭据）。
func Reason(err error) string {
	switch {
	case errors.Is(err, ErrBlocked):
		return "blocked by the fetch policy"
	case errors.Is(err, ErrNoEgress):
		return "no egress proxy is configured"
	case errors.Is(err, ErrEgressDown):
		return "egress proxy unavailable"
	case errors.Is(err, ErrTooLarge):
		return "response too large"
	case errors.Is(err, ErrStatus):
		return err.Error()
	}
	return err.Error()
}
