package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 订阅链接访问限制（需求 R42）：与 R23 的提醒互补——提醒是事后发现，限制是事前挡住。
//
//   - 客户端：按 User-Agent 中第一个「名称/版本」的名称比较（与订阅分析显示的客户端名相同），不区分大小写；
//   - 国家或地区：按本地地理库查询请求来源；查不到（没有地理库、内网地址）时视为不符合，宁可挡住；
//   - 每天次数：按所有者的时区计算「天」，只计成功的拉取。计数在内存中累加，每个进程每天第一次检查时
//     从数据库（与尚未写入的内存记录）读出当天已有的次数，所以重启不会清零。多实例部署时各实例分别计数，
//     最多可能多放行（实例数 - 1）倍，这是有意的取舍：不为一个软限制引入跨实例的同步。
//
// 被挡住的请求照常记入订阅分析（状态码 403 或 429），提醒（R23）不把它们当作新的来源。

// ErrLinkLimits 表示访问限制的取值无效。
var ErrLinkLimits = errors.New("service: 访问限制无效")

const (
	linkLimitMaxClients   = 20
	linkLimitMaxCountries = 60
	linkLimitMaxPerDay    = 100000
)

// NormalizeLinkLimits 校验并规范访问限制：去掉首尾空白与重复项，国家代码转为大写并排序。
func NormalizeLinkLimits(l store.LinkLimits) (store.LinkLimits, error) {
	var out store.LinkLimits
	seen := map[string]bool{}
	for _, c := range l.Clients {
		c = strings.TrimSpace(c)
		k := strings.ToLower(c)
		if c == "" || seen[k] {
			continue
		}
		if len(c) > 40 || strings.ContainsAny(c, "/ \t") {
			return out, fmt.Errorf("%w：客户端名称 %q 应为 User-Agent 中的名称（不含版本与空格）", ErrLinkLimits, c)
		}
		seen[k] = true
		out.Clients = append(out.Clients, c)
	}
	for _, c := range l.Countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c == "" || slices.Contains(out.Countries, c) {
			continue
		}
		if len(c) != 2 || c[0] < 'A' || c[0] > 'Z' || c[1] < 'A' || c[1] > 'Z' {
			return out, fmt.Errorf("%w：国家或地区 %q 应为两个字母的代码（ISO 3166-1）", ErrLinkLimits, c)
		}
		out.Countries = append(out.Countries, c)
	}
	slices.Sort(out.Countries)
	if len(out.Clients) > linkLimitMaxClients || len(out.Countries) > linkLimitMaxCountries {
		return out, fmt.Errorf("%w：客户端最多 %d 个，国家或地区最多 %d 个", ErrLinkLimits, linkLimitMaxClients, linkLimitMaxCountries)
	}
	if l.PerDay < 0 || l.PerDay > linkLimitMaxPerDay {
		return out, fmt.Errorf("%w：每天次数应在 0 到 %d 之间", ErrLinkLimits, linkLimitMaxPerDay)
	}
	out.PerDay = l.PerDay
	return out, nil
}

// LinkDenial 是被访问限制挡住的原因。Status 为 0 表示放行。
type LinkDenial struct {
	Status     int
	Reason     string // client | country | per_day
	RetryAfter int    // 秒；只用于每天次数超限
}

type dayCount struct {
	day int64 // 所有者时区当天零点的 Unix 秒（作为「哪一天」的键）
	n   int
}

type linkCounter struct {
	mu sync.Mutex
	m  map[string]*dayCount
}

// CheckLinkAccess 判断这次请求是否被链接的访问限制挡住。count 为 true 时放行的请求计入当天次数
// （配置本身的请求；托管节点文件随配置一起拉取，不重复计数）。
func (s *Service) CheckLinkAccess(ctx context.Context, l *store.Link, ua string, addr netip.Addr, count bool) LinkDenial {
	lim := l.Limits
	if lim.Empty() {
		return LinkDenial{}
	}
	if len(lim.Clients) > 0 {
		c := clientOf(ua)
		if !slices.ContainsFunc(lim.Clients, func(x string) bool { return strings.EqualFold(x, c) }) {
			return LinkDenial{Status: http.StatusForbidden, Reason: "client"}
		}
	}
	if len(lim.Countries) > 0 {
		cc := ""
		if s.Geo != nil && addr.IsValid() {
			cc = strings.ToUpper(s.Geo.Lookup(addr).Country)
		}
		if !slices.Contains(lim.Countries, cc) {
			return LinkDenial{Status: http.StatusForbidden, Reason: "country"}
		}
	}
	if lim.PerDay > 0 && count {
		return s.countDay(ctx, l, lim.PerDay)
	}
	return LinkDenial{}
}

// countDay 按所有者的时区累计当天的拉取次数；达到上限时返回 429 与到下一天零点的秒数。
func (s *Service) countDay(ctx context.Context, l *store.Link, limit int) LinkDenial {
	loc := time.UTC
	if u, err := s.Store.Users().ByID(ctx, l.OwnerID); err == nil && u.Timezone != "" {
		if z, err := time.LoadLocation(u.Timezone); err == nil {
			loc = z
		}
	}
	now := s.now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	day := start.Unix()
	c := &s.linkCounts
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]*dayCount{}
	}
	e := c.m[l.ID]
	if e == nil || e.day != day {
		// 本进程今天第一次检查这个链接：从数据库与尚未写入的内存记录读出当天已有的次数
		n, err := s.Store.Stats().CountOK(ctx, l.ID, start)
		if err != nil {
			if s.Log != nil {
				s.Log.Warn("读取订阅链接当天次数失败，按 0 计", "link", l.ID, "error", err)
			}
			n = 0
		}
		e = &dayCount{day: day, n: n + s.Stats().pendingOK(l.ID, start)}
		c.m[l.ID] = e
	}
	if e.n >= limit {
		next := start.AddDate(0, 0, 1)
		return LinkDenial{Status: http.StatusTooManyRequests, Reason: "per_day", RetryAfter: int(next.Sub(now).Seconds()) + 1}
	}
	e.n++
	return LinkDenial{}
}
