package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 统计记录器（反馈 #66–#68）：计数与订阅请求先在内存中累加，由后台每分钟写入一次数据库（Run），
// 退出前再写一次（Flush），订阅响应不必等待数据库写入。数据库中只有「某天、某项、多少次」与订阅请求记录；
// 去重用到的「谁在什么时候做过」只在内存中，重启即清空。
//
// 计数规则：
//   - visit：每次打开网站算一次（刷新不算，由前端判断），同一账号 30 分钟内再次打开不重复计；
//   - pv：切换到某类页面（key = 页面类型），同一账号同一页面 1 分钟内不重复计；
//   - uv：每天访问的不同账号数；
//   - login：每次登录成功；
//   - field_change / field_expand：改了某个字段的值 / 展开它的说明，同一账号同一字段同一动作 10 分钟内只计一次；
//   - sub 系列：每次订阅请求。
type Stats struct {
	mu     sync.Mutex
	counts map[store.CountDelta]int64 // N 字段不用，作为键时置 0
	reqs   []store.SubRequest
	seen   map[string]time.Time // 去重：键 → 上次计数的时间
	uvDay  int
	uv     map[string]bool
}

// 去重的时间窗。
const (
	visitGap = 30 * time.Minute
	pvGap    = time.Minute
	fieldGap = 10 * time.Minute
)

// 页面类型与字段路径的格式：只接受这些，防止任意字符串写入统计表。
var (
	pageRe  = regexp.MustCompile(`^[a-z][a-z-]{0,31}$`)
	fieldRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.\-]{0,127}$`)
)

func newStats() *Stats {
	return &Stats{counts: map[store.CountDelta]int64{}, seen: map[string]time.Time{}, uv: map[string]bool{}}
}

// Day 返回 UTC 日期 YYYYMMDD。
func Day(t time.Time) int {
	t = t.UTC()
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

func (s *Stats) add(scope, key string, now time.Time, n int64) {
	s.counts[store.CountDelta{Scope: scope, Key: key, Day: Day(now)}] += n
}

// once 报告 key 在 gap 之内是否第一次出现（并记下这次）。调用方持有锁。
func (s *Stats) once(key string, now time.Time, gap time.Duration) bool {
	if last, ok := s.seen[key]; ok && now.Sub(last) < gap {
		return false
	}
	s.seen[key] = now
	return true
}

// Event 是前端上报的一条使用事件。
type Event struct {
	Kind string `json:"kind"` // visit | pv | field_change | field_expand
	Key  string `json:"key"`  // pv：页面类型；field_*：字段路径；visit：空
}

// ErrEvent 表示上报的事件格式无效。
var ErrEvent = errors.New("service: 统计事件无效")

// Record 记录一个账号上报的事件（已登录）。
func (s *Stats) Record(user string, evs []Event, now time.Time) error {
	if len(evs) > 50 {
		return ErrEvent
	}
	for _, e := range evs {
		switch e.Kind {
		case "visit":
		case "pv":
			if !pageRe.MatchString(e.Key) {
				return ErrEvent
			}
		case "field_change", "field_expand":
			if !fieldRe.MatchString(e.Key) {
				return ErrEvent
			}
		default:
			return ErrEvent
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := Day(now); d != s.uvDay {
		s.uvDay, s.uv = d, map[string]bool{}
	}
	if !s.uv[user] {
		s.uv[user] = true
		s.add("uv", "", now, 1)
	}
	for _, e := range evs {
		gap := fieldGap
		switch e.Kind {
		case "visit":
			gap = visitGap
		case "pv":
			gap = pvGap
		}
		if s.once(user+"\x00"+e.Kind+"\x00"+e.Key, now, gap) {
			s.add(e.Kind, e.Key, now, 1)
		}
	}
	return nil
}

// Login 记录一次登录成功。
func (s *Stats) Login(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.add("login", "", now, 1)
}

// clientOf 从 User-Agent 取客户端名称：第一个「名称/版本」中的名称（如 clash-verge、mihomo、Stash）。
func clientOf(ua string) string {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return ""
	}
	name, _, _ := strings.Cut(strings.Fields(ua)[0], "/")
	if len(name) > 40 {
		name = name[:40]
	}
	return name
}

// Request 记录一次订阅请求。linkID 为空表示令牌无效。
func (s *Stats) Request(q store.SubRequest) {
	if len(q.UA) > 300 {
		q.UA = q.UA[:300]
	}
	if q.Client == "" { // 一键导入的记录由调用方给出客户端名称（反馈 #76）
		q.Client = clientOf(q.UA)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, q)
	if q.LinkID == "" {
		return // 令牌无效的请求只进入来源汇总，不计入订阅次数
	}
	s.add("sub_all", "", q.At, 1)
	s.add("sub", q.LinkID, q.At, 1)
	if q.ShortID != "" {
		s.add("short", q.ShortID, q.At, 1) // 短链接的请求次数（反馈 #74）
	}
	if q.Status >= 400 {
		s.add("sub_fail", "", q.At, 1)
	}
	if q.Client != "" {
		s.add("sub_client", q.Client, q.At, 1)
	}
}

// Flush 把内存中的计数与请求写入数据库。写入失败时数据放回内存，下次再试。
func (s *Stats) Flush(ctx context.Context, st store.StatsRepo, now time.Time) error {
	s.mu.Lock()
	counts, reqs := s.counts, s.reqs
	s.counts, s.reqs = map[store.CountDelta]int64{}, nil
	// 顺手清理过期的去重记录
	for k, t := range s.seen {
		if now.Sub(t) > visitGap {
			delete(s.seen, k)
		}
	}
	s.mu.Unlock()

	ds := make([]store.CountDelta, 0, len(counts))
	for k, n := range counts {
		k.N = n
		ds = append(ds, k)
	}
	sort.Slice(ds, func(i, j int) bool { // 固定顺序写入，避免并发事务以不同顺序加锁
		a, b := ds[i], ds[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Day < b.Day
	})
	err := st.AddCounts(ctx, ds)
	if err == nil {
		err = st.AddRequests(ctx, reqs)
		if err != nil {
			reqs = reqs[:len(reqs):len(reqs)]
			s.mu.Lock()
			s.reqs = append(reqs, s.reqs...)
			s.mu.Unlock()
		}
		return err
	}
	s.mu.Lock()
	for _, d := range ds {
		s.counts[store.CountDelta{Scope: d.Scope, Key: d.Key, Day: d.Day}] += d.N
	}
	s.reqs = append(reqs, s.reqs...)
	s.mu.Unlock()
	return err
}

// —— 设置项热度（反馈 #68）——

// FieldScores 是设置项热度的快照：每周重算一次，其间顺序不变。
type FieldScores struct {
	ComputedAt time.Time        `json:"computedAt"`
	Scores     map[string]int64 `json:"scores"` // 字段路径 → 热度（改值计 2，展开说明计 1）
}

const (
	settingFieldScores = "stats.field_scores"
	fieldScoresEvery   = 7 * 24 * time.Hour
)

// FieldScores 返回热度快照；超过一周时用全部历史计数重算并保存。
func (s *Service) FieldScores(ctx context.Context) (*FieldScores, error) {
	var fs FieldScores
	v, err := s.Store.Settings().Get(ctx, settingFieldScores)
	switch {
	case err == nil:
		if err := json.Unmarshal([]byte(v), &fs); err != nil {
			return nil, err
		}
		if s.now().Sub(fs.ComputedAt) < fieldScoresEvery {
			return &fs, nil
		}
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}
	change, err := s.Store.Stats().Totals(ctx, "field_change", nil)
	if err != nil {
		return nil, err
	}
	expand, err := s.Store.Stats().Totals(ctx, "field_expand", nil)
	if err != nil {
		return nil, err
	}
	fs = FieldScores{ComputedAt: s.now(), Scores: map[string]int64{}}
	for k, n := range change {
		fs.Scores[k] += 2 * n
	}
	for k, n := range expand {
		fs.Scores[k] += n
	}
	b, err := json.Marshal(fs)
	if err != nil {
		return nil, err
	}
	if err := s.Store.Settings().Set(ctx, settingFieldScores, string(b)); err != nil {
		return nil, err
	}
	return &fs, nil
}

// Stats 返回统计记录器（惰性创建，与 guards 相同的模式）。
func (s *Service) Stats() *Stats {
	s.statsOnce.Do(func() { s.statsVal = newStats() })
	return s.statsVal
}

// FlushStats 把内存中的统计写入数据库；失败只记录日志（数据留在内存，下次再写）。
func (s *Service) FlushStats(ctx context.Context) {
	if err := s.Stats().Flush(ctx, s.Store.Stats(), s.now()); err != nil {
		s.Log.Error("stats: flush", "error", err)
	}
}
