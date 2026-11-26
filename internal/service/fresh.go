package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/store"
)

// 请求订阅时按需拉取（反馈 #119，修订规范第 13.3 节「订阅请求只读缓存」）。
//
// 客户端请求一份配置时，它用到的节点订阅与规则源若距上次拉取已超过 FreshAfter（默认 60 秒），
// 先从上游重新拉取、重新生成这份配置，再返回最新结果；都还新鲜时直接返回缓存，与原来一样。
//
//   - 不让客户端久等：最多等 FreshWait（默认 3 秒）。来不及就先返回当前「最近一次成功」的结果，
//     拉取与生成在后台继续，下一次请求拿到新的。OpenClash 探测订阅信息的超时是 5 秒，超时会换 UA 重试。
//   - 失败不影响订阅：拉取失败或新结果没通过内核检查时，照旧提供上一次成功的结果（与定时拉取相同）。
//   - 并发合并：同一份配置同时有多个请求时只刷新一次，后来的请求等同一次的结果。
//   - 判断「上次拉取」用最近一次尝试的时间：上游持续失败时也至多每个间隔试一次，不会每个请求都打上游。
//   - 只拉取服务器要用的输入：交给客户端（provider）模式的订阅由客户端自己拉取，这里跳过。
//   - 开放注册（未经邀请）的用户：间隔取 FreshAfter 与其最短拉取间隔中较大的一个，
//     不能借反复请求自己的订阅链接，让服务器频繁访问上游（出站边界，决定 0003）。

// FreshConfig 是请求时按需拉取的设置。
type FreshConfig struct {
	After time.Duration // 超过这么久没拉取就在请求时拉取；0 表示关闭
	Wait  time.Duration // 请求最多等待的时间
}

// freshFlights 合并同一份配置的并发刷新。
type freshFlights struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

// EnsureFresh 在提供订阅前调用：输入过期时拉取并重新生成 owner 的配置 profileID，最多等 Wait。
// 返回是否已在等待时间内完成（调用方随后读取存储中最新的成功结果，不依赖这个返回值）。
func (s *Service) EnsureFresh(ctx context.Context, owner, profileID string) bool {
	if s.Fresh.After <= 0 {
		return true
	}
	key := owner + "/" + profileID
	s.freshFlights.mu.Lock()
	if s.freshFlights.m == nil {
		s.freshFlights.m = map[string]chan struct{}{}
	}
	done, running := s.freshFlights.m[key]
	if !running {
		done = make(chan struct{})
		s.freshFlights.m[key] = done
		go func() {
			defer func() {
				s.freshFlights.mu.Lock()
				delete(s.freshFlights.m, key)
				s.freshFlights.mu.Unlock()
				close(done)
			}()
			// 与请求脱钩：客户端断开或等待超时后，后台的拉取与生成继续完成
			bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			if err := s.refreshStale(bg, owner, profileID); err != nil {
				s.Log.Warn("refresh on request", "owner", owner, "profile", profileID, "error", err)
			}
		}()
	}
	s.freshFlights.mu.Unlock()
	wait := s.Fresh.Wait
	if wait <= 0 {
		wait = 3 * time.Second
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-done:
		return true
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// refreshStale 拉取 profileID 用到的过期输入；有拉取时同步重新生成这份配置。
func (s *Service) refreshStale(ctx context.Context, owner, profileID string) error {
	p, err := s.Store.Profiles().Get(ctx, owner, profileID)
	if err != nil {
		return err
	}
	var l diag.List
	doc, err := profile.Parse(p.Doc, &l)
	if err != nil {
		return nil // 文档无效：照常提供已有结果，原因在配置页的诊断中
	}
	after, err := s.freshAfterFor(ctx, owner)
	if err != nil {
		return err
	}
	now := s.now()
	stale := func(snap *store.Snapshot, err error) bool {
		if errors.Is(err, store.ErrNotFound) {
			return true
		}
		if err != nil {
			return false
		}
		last := snap.FetchedAt
		if snap.LastAttempt.After(last) {
			last = snap.LastAttempt
		}
		return now.Sub(last) >= after
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	fetched := false
	run := func(f func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); err != nil && !errors.Is(err, ErrClientFetch) {
				s.Log.Info("refresh on request: fetch failed", "owner", owner, "error", err)
			}
			mu.Lock()
			fetched = true
			mu.Unlock()
		}()
	}
	for _, ref := range doc.Refs() {
		src, srcOwner, granted, err := s.resolveSource(ctx, owner, ref.Ref)
		if err != nil || src.Kind != string(engine.KindSubscription) || src.URL == "" {
			continue
		}
		if !granted && ref.Mode == profile.ModeProvider {
			continue // 交给客户端：客户端自己拉取订阅地址
		}
		if !stale(s.Store.Snapshots().Get(ctx, srcOwner, SourceKey(src.ID))) {
			continue
		}
		id := src.ID
		run(func() error { _, err := s.RefreshSource(ctx, srcOwner, id); return err })
	}
	if doc.Routing == nil && doc.Rules != nil {
		if rs, err := s.Store.RuleSources().Get(ctx, owner, doc.Rules.Ref); err == nil {
			parts := map[string]string{"groups": rs.GroupsURL, "rulesets": rs.RulesetsURL, "ini": rs.INIURL}
			old := false
			for part, u := range parts {
				if u != "" && stale(s.Store.Snapshots().Get(ctx, owner, RuleKey(rs.ID, part))) {
					old = true
				}
			}
			if old {
				id := rs.ID
				run(func() error { return s.RefreshRuleSource(ctx, owner, id) })
			}
		}
	}
	wg.Wait()
	if !fetched {
		return nil
	}
	// 拉取会在后台重新生成用到这些输入的全部配置；这份配置在这里同步生成，请求拿到的是新结果。
	// 同一份配置的生成有锁，输入相同时直接返回已有结果，不会重复做内核检查。
	_, _, err = s.RenderProfile(ctx, owner, profileID)
	return err
}

// freshAfterFor 是 owner 的按需拉取间隔：管理员与受邀用户为 Fresh.After；开放注册的用户不短于其最短拉取间隔。
func (s *Service) freshAfterFor(ctx context.Context, owner string) (time.Duration, error) {
	u, err := s.Store.Users().ByID(ctx, owner)
	if err != nil {
		return 0, err
	}
	after := s.Fresh.After
	if u.Role != "admin" && !u.Invited {
		l, err := s.Limits(ctx, u)
		if err != nil {
			return 0, err
		}
		if d := time.Duration(l.MinInterval) * time.Second; d > after {
			after = d
		}
	}
	return after, nil
}
