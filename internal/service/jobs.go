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
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/render"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// Run 是后台调度：定期拉取到期的订阅与规则源（第 5.4 节：后台按间隔拉取，订阅请求从不触发拉取），
// 并确保每个方案都有渲染结果。单实例假设：多实例部署时改用数据库租约锁（第 17.1 节）。
func (s *Service) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	s.tick(ctx)
	s.PruneAudit(ctx)
	lastPrune := s.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tick(ctx)
			if s.now().Sub(lastPrune) >= 24*time.Hour { // 审计日志每天清理一次
				s.PruneAudit(ctx)
				lastPrune = s.now()
			}
		}
	}
}

func (s *Service) tick(ctx context.Context) {
	now := s.now()
	interval := or(s.Policy.DefaultInterval, 6*time.Hour)
	subs, err := s.Store.Sources().Due(ctx, now, 10000)
	if err != nil {
		s.Log.Error("scheduler: list sources", "error", err)
		return
	}
	for _, src := range subs {
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
	var ref *profile.SourceRef
	for _, r := range doc.Refs() {
		if r.Ref == sourceID && r.Mode == profile.ModeProvider {
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
	return u, nil
}
