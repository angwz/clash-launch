package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"encoding/json"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/metrics"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/render"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Run 是后台调度：定期拉取到期的订阅与规则源（第 5.4 节：后台按间隔拉取，订阅请求从不触发拉取），
// 并确保每个方案都有渲染结果。单实例假设：多实例部署时改用数据库租约锁（第 17.1 节）。
// 每个任务的运行时间、耗时与失败记入 metrics.Default，供后台任务面板（需求 R36）与 /metrics（需求 R26）。
func (s *Service) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	s.runJob(ctx, "fetch", s.tick)
	s.runJob(ctx, "footprints", s.backfillJob) // 足迹上线后补记一次（需求 R10），之后立即返回
	s.daily(ctx)
	lastPrune := s.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runJob(ctx, "fetch", s.tick)
			s.runJob(ctx, "deletions", s.RunDueDeletions) // 冷静期已满的注销（需求 R20）
			s.runJob(ctx, "stats", s.FlushStats)
			if s.now().Sub(lastPrune) >= 24*time.Hour { // 清理每天一次
				s.daily(ctx)
				lastPrune = s.now()
			}
		}
	}
}

// daily 是每天一次的清理任务。
func (s *Service) daily(ctx context.Context) {
	s.runJob(ctx, "prune_audit", s.PruneAudit)
	s.runJob(ctx, "prune_requests", s.PruneRequests)
	s.runJob(ctx, "purge_threads", s.PurgeThreads) // 回收站中超过一年的对话（反馈 #75）
	s.runJob(ctx, "purge_backups", s.PurgeBackups) // 过期的还原留底（需求 R21）
}

type jobKey struct{}

// jobState 收集一次任务运行中的失败；任务函数沿用「失败只记日志」，同时经 jobFailed 记一笔。
type jobState struct{ err string }

// runJob 运行一个后台任务并记录运行情况。
func (s *Service) runJob(ctx context.Context, name string, fn func(context.Context)) {
	st := &jobState{}
	done := metrics.Default.JobStart(name, s.now())
	fn(context.WithValue(ctx, jobKey{}, st))
	done(s.now(), st.err)
}

// jobFailed 记下任务运行中的第一个失败；不在任务中调用时什么也不做。
func jobFailed(ctx context.Context, err error) {
	if st, ok := ctx.Value(jobKey{}).(*jobState); ok && st.err == "" && err != nil {
		st.err = err.Error()
	}
}

func (s *Service) tick(ctx context.Context) {
	now := s.now()
	interval := or(s.Policy.DefaultInterval, 6*time.Hour)
	subs, err := s.Store.Sources().Due(ctx, now, 10000)
	if err != nil {
		s.Log.Error("scheduler: list sources", "error", err)
		jobFailed(ctx, err)
		return
	}
	// 订阅只由客户端拉取的账号（反馈 #71）：跳过，不记日志。策略按所有者在本轮内缓存
	clientFetch := map[string]bool{}
	for _, src := range subs {
		cf, ok := clientFetch[src.OwnerID]
		if !ok {
			pol, err := s.nodeModeOf(ctx, src.OwnerID)
			if err != nil {
				s.Log.Warn("scheduler: node mode", "owner", src.OwnerID, "error", err)
				continue
			}
			cf = pol.ClientFetch
			clientFetch[src.OwnerID] = cf
		}
		if cf {
			continue
		}
		every := interval
		if src.Interval > 0 {
			every = time.Duration(src.Interval) * time.Second
		}
		snap, err := s.Store.Snapshots().Get(ctx, src.OwnerID, SourceKey(src.ID))
		if err == nil && now.Sub(snap.LastAttempt) < every {
			continue
		}
		if _, err := s.RefreshSource(ctx, src.OwnerID, src.ID); err != nil {
			s.Log.Warn("scheduler: refresh", "source", src.ID, "error", err)
			jobFailed(ctx, err)
		}
	}
	rules, err := s.Store.RuleSources().All(ctx, 10000)
	if err == nil {
		for _, rs := range rules {
			if rs.GroupsURL == "" && rs.RulesetsURL == "" && rs.INIURL == "" {
				continue
			}
			every := interval
			if rs.Interval > 0 {
				every = time.Duration(rs.Interval) * time.Second
			}
			part := "groups"
			if rs.INIURL != "" {
				part = "ini"
			}
			snap, err := s.Store.Snapshots().Get(ctx, rs.OwnerID, RuleKey(rs.ID, part))
			if err == nil && now.Sub(snap.LastAttempt) < every {
				continue
			}
			if err := s.RefreshRuleSource(ctx, rs.OwnerID, rs.ID); err != nil {
				s.Log.Warn("scheduler: refresh rules", "rules", rs.ID, "error", err)
			}
		}
	}
	profiles, err := s.Store.Profiles().All(ctx, 10000)
	if err != nil {
		return
	}
	for _, p := range profiles {
		if _, err := s.Store.Renders().Get(ctx, p.OwnerID, p.ID); errors.Is(err, store.ErrNotFound) {
			if _, _, err := s.RenderProfile(ctx, p.OwnerID, p.ID); err != nil {
				s.Log.Warn("scheduler: render", "profile", p.ID, "error", err)
				jobFailed(ctx, err)
			}
		}
	}
}

// ErrNotHosted 表示请求的源不是由本服务托管的 provider。
var ErrNotHosted = errors.New("service: 该源不由本服务托管")

// HostedProvider 返回「交给客户端」模式下由本服务托管的节点文件（第 5.6 节）：
// 链接所属方案中以 provider 模式引用的单条、批量或文件源，整理后输出为 proxies 列表。
func (s *Service) HostedProvider(ctx context.Context, link *store.Link, sourceID string) ([]byte, error) {
	p, err := s.Store.Profiles().Get(ctx, link.OwnerID, link.ProfileID)
	if err != nil {
		return nil, err
	}
	var ds diag.List
	doc, err := profile.Parse(p.Doc, &ds)
	if err != nil {
		return nil, err
	}
	policy, err := s.nodeModeOf(ctx, link.OwnerID)
	if err != nil {
		return nil, err
	}
	var ref *profile.SourceRef
	for _, r := range doc.Refs() {
		// 按实际生效的写法判断：锁定账号的源可能由默认值改为交给客户端（反馈 #46）
		if r.Ref == sourceID && policy.effective(r.Mode) == profile.ModeProvider {
			ref = &r
		}
	}
	if ref == nil {
		return nil, ErrNotHosted
	}
	src, err := s.Store.Sources().Get(ctx, link.OwnerID, sourceID)
	if err != nil {
		return nil, err
	}
	if src.Kind == string(engine.KindSubscription) {
		return nil, ErrNotHosted
	}
	var proc node.Processing
	_ = json.Unmarshal(src.Processing, &proc) // 整理设置以 JSON 存储（json 标签）
	nodes, _ := node.Parse(src.Content, src.ID, s.Catalog, &ds)
	nodes = node.Process(nodes, proc, s.Tools, src.ID, &ds)
	seq := yamlx.Seq()
	for _, n := range nodes {
		seq.Content = append(seq.Content, n.Mapping)
	}
	return render.Render(yamlx.Map("proxies", seq), render.Options{Catalog: s.Catalog}, &ds)
}

// NeedsSetup 报告是否还没有任何账号。没有账号时，第一个访问者可以直接创建管理员（反馈 #17：
// 个人项目，不再要求日志中的一次性令牌）。
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.Store.Users().Count(ctx)
	if err != nil {
		return false, fmt.Errorf("service: 统计账号: %w", err)
	}
	return n == 0, nil
}

// CreateAdmin 创建管理员账号（admin create 命令与首次注册共用）。
func (s *Service) CreateAdmin(ctx context.Context, username, password string) (*store.User, error) {
	if err := CheckUsername(username); err != nil {
		return nil, err
	}
	if err := auth.CheckPassword(password); err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := &store.User{Username: username, PasswordHash: hash, Role: "admin"}
	if err := s.Store.Users().Create(ctx, u); err != nil {
		return nil, err
	}
	_ = s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: u.ID, Action: "admin.create"})
	s.Footprint(ctx, u.ID, FPJoined, "", nil)
	s.Footprint(ctx, "", FPSiteStart, "", map[string]string{"user": u.Username}) // 第一位管理员创建之日即网站上线之日
	return u, nil
}

// backfillJob 是补记足迹的后台任务形式。
func (s *Service) backfillJob(ctx context.Context) {
	if err := s.BackfillFootprints(ctx); err != nil {
		s.Log.Error("补记足迹失败", "error", err)
		jobFailed(ctx, err)
	}
}
