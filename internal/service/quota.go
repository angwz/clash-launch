package service

import (
	"context"
	"fmt"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 配额（第 14.7 节）：每个用户的节点源数、方案数、订阅链接数、最短拉取间隔与存储体积。
// 全局默认值来自配置；管理员可以为单个用户调整（-1 表示使用默认值）。管理员本人不受配额限制。
// 存储体积按已保存的加密内容计算，在写入前检查「当前用量 + 本次内容」。

// QuotaDefaults 是全局默认配额；零值字段使用内置默认。
type QuotaDefaults struct {
	Sources     int
	Profiles    int
	Links       int
	MinInterval time.Duration
	Storage     int64 // 字节
}

// 内置默认：个人使用绰绰有余，同时限制单个账号能占用的资源。
var builtinQuota = QuotaDefaults{Sources: 50, Profiles: 20, Links: 50, MinInterval: 30 * time.Minute, Storage: 100 << 20}

// Limits 是解析后的有效配额；-1 表示不限制（管理员）。
type Limits struct {
	Sources     int   `json:"sources"`
	Profiles    int   `json:"profiles"`
	Links       int   `json:"links"`
	MinInterval int   `json:"minInterval"` // 秒
	Storage     int64 `json:"storage"`     // 字节
}

func (s *Service) defaults() QuotaDefaults {
	d := s.QuotaDefaults
	if d.Sources == 0 {
		d.Sources = builtinQuota.Sources
	}
	if d.Profiles == 0 {
		d.Profiles = builtinQuota.Profiles
	}
	if d.Links == 0 {
		d.Links = builtinQuota.Links
	}
	if d.MinInterval == 0 {
		d.MinInterval = builtinQuota.MinInterval
	}
	if d.Storage == 0 {
		d.Storage = builtinQuota.Storage
	}
	return d
}

// Limits 返回用户的有效配额。
func (s *Service) Limits(ctx context.Context, u *store.User) (Limits, error) {
	if u.Role == "admin" {
		return Limits{Sources: -1, Profiles: -1, Links: -1, MinInterval: 0, Storage: -1}, nil
	}
	q, err := s.Store.Quotas().Get(ctx, u.ID)
	if err != nil {
		return Limits{}, err
	}
	d := s.defaults()
	pick := func(v, def int) int {
		if v < 0 {
			return def
		}
		return v
	}
	l := Limits{Sources: pick(q.Sources, d.Sources), Profiles: pick(q.Profiles, d.Profiles), Links: pick(q.Links, d.Links), MinInterval: pick(q.MinInterval, int(d.MinInterval.Seconds())), Storage: q.Storage}
	if q.Storage < 0 {
		l.Storage = d.Storage
	}
	return l, nil
}

// Quota 的种类。
const (
	QuotaSource  = "sources"
	QuotaProfile = "profiles"
	QuotaLink    = "links"
)

// CheckCreate 在新建节点源、方案或订阅链接前检查数量与存储配额。extra 为本次写入的内容字节数。
func (s *Service) CheckCreate(ctx context.Context, u *store.User, kind string, extra int64) error {
	l, err := s.Limits(ctx, u)
	if err != nil {
		return err
	}
	usage, err := s.Store.Quotas().Usage(ctx, u.ID)
	if err != nil {
		return err
	}
	count, limit := 0, -1
	switch kind {
	case QuotaSource:
		count, limit = usage.Sources, l.Sources
	case QuotaProfile:
		count, limit = usage.Profiles, l.Profiles
	case QuotaLink:
		count, limit = usage.Links, l.Links
	}
	if limit >= 0 && count >= limit {
		return fmt.Errorf("%w：%s %d/%d", ErrQuota, kind, count, limit)
	}
	return s.checkStorage(l, usage, extra)
}

// CheckStorage 在更新内容前检查存储配额。
func (s *Service) CheckStorage(ctx context.Context, u *store.User, extra int64) error {
	l, err := s.Limits(ctx, u)
	if err != nil {
		return err
	}
	usage, err := s.Store.Quotas().Usage(ctx, u.ID)
	if err != nil {
		return err
	}
	return s.checkStorage(l, usage, extra)
}

func (s *Service) checkStorage(l Limits, u *store.Usage, extra int64) error {
	if l.Storage >= 0 && u.Bytes+extra > l.Storage {
		return fmt.Errorf("%w：storage %d/%d bytes", ErrQuota, u.Bytes+extra, l.Storage)
	}
	return nil
}

// CheckInterval 检查拉取间隔（秒）不短于配额；0 表示使用默认间隔，总是允许。
func (s *Service) CheckInterval(ctx context.Context, u *store.User, seconds int) error {
	if seconds == 0 {
		return nil
	}
	l, err := s.Limits(ctx, u)
	if err != nil {
		return err
	}
	if seconds < l.MinInterval {
		return fmt.Errorf("%w：interval %ds < %ds", ErrQuota, seconds, l.MinInterval)
	}
	return nil
}

// CheckRefresh 限制手动拉取：每个用户每小时最多 30 次（管理员不限）。
func (s *Service) CheckRefresh(u *store.User) error {
	if u.Role == "admin" {
		return nil
	}
	g, now := s.g(), s.now()
	if w := g.refresh.Wait(u.ID, now); w > 0 {
		return &WaitError{Wait: w}
	}
	g.refresh.Fail(u.ID, now)
	return nil
}
