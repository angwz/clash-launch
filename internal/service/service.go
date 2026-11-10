// Package service 编排外壳：从存储读取方案与快照，调用纯函数引擎渲染，执行内核检查，
// 保存「最近一次成功」的结果；拉取订阅与规则源并更新快照；后台调度。API 与命令行共用它。
package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/target"
)

// LinkTokenPlaceholder 出现在「交给客户端」模式的托管地址中；提供订阅时替换为请求者的链接令牌。
// 它只由 base64url 安全字符组成，替换为令牌后 YAML 的结构与引号不变。
const LinkTokenPlaceholder = "LINKTOKEN-PLACEHOLDER-0000000000000000000000"

// Policy 是拉取策略。服务器直接拉取订阅与规则源（没有出站代理，见 docs/decisions/0003）。
type Policy struct {
	StaleAfter      time.Duration // 连续失败多久后提示，默认 7 天
	DefaultInterval time.Duration // 订阅与规则源的默认拉取间隔，默认 6 小时
}

// Service 是外壳的业务编排。
type Service struct {
	Store         store.Store
	Catalog       *catalog.Catalog
	CatalogDigest string
	Presets       []*target.Preset
	Tools         *node.Tools
	Kernel        *kernel.Checker // nil 表示没有内核可用
	Fetcher       *source.Fetcher
	Log           *slog.Logger
	PublicURL     string // 例如 https://cl.example.com（无结尾斜杠）
	Policy        Policy
	Now           func() time.Time

	locks sync.Map // 方案 ID → *sync.Mutex：同一方案的渲染串行
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) lock(id string) func() {
	m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// SourceKey 与 RuleKey 是快照的键。
func SourceKey(id string) string             { return "src/" + id }
func RuleKey(id, part string) string         { return "rs/" + id + "/" + part }
func (s *Service) staleAfter() time.Duration { return or(s.Policy.StaleAfter, 7*24*time.Hour) }

func or(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// RefreshSource 立即拉取一个订阅，更新快照，并重新渲染引用它的方案。
func (s *Service) RefreshSource(ctx context.Context, owner, id string) (*store.Snapshot, error) {
	src, err := s.Store.Sources().Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if src.Kind != string(engine.KindSubscription) || src.URL == "" {
		return nil, fmt.Errorf("service: 只有订阅需要拉取")
	}
	prev, _ := s.Store.Snapshots().Get(ctx, owner, SourceKey(id))
	resp, err := s.Fetcher.Fetch(ctx, source.Request{URL: src.URL, UA: src.UA, Headers: src.Headers})
	accept, reason := false, ""
	if err == nil {
		var l diag.List
		nodes, f := node.Parse(resp.Body, "check", s.Catalog, &l)
		switch {
		case f == node.FormatUnknown:
			reason = "content not recognised"
		case len(nodes) == 0:
			reason = "0 proxies after parsing"
		default:
			accept = true
		}
	}
	snap := source.Update(toSourceSnap(prev), resp, err, accept, reason, s.now())
	stored := fromSourceSnap(SourceKey(id), owner, snap)
	if err := s.Store.Snapshots().Put(ctx, owner, stored); err != nil {
		return nil, err
	}
	s.Log.Info("refresh source", "owner", owner, "source", id, "ok", snap.LastError == "", "error", snap.LastError)
	s.rerenderUsing(ctx, owner, func(p *profile.Profile) bool {
		for _, r := range p.Sources {
			if r.Ref == id {
				return true
			}
		}
		return false
	})
	return stored, nil
}

// RefreshRuleSource 拉取规则源的 URL（toml 一对或 ini），更新快照，并重新渲染引用它的方案。
func (s *Service) RefreshRuleSource(ctx context.Context, owner, id string) error {
	rs, err := s.Store.RuleSources().Get(ctx, owner, id)
	if err != nil {
		return err
	}
	parts := map[string]string{"groups": rs.GroupsURL, "rulesets": rs.RulesetsURL, "ini": rs.INIURL}
	var firstErr error
	for _, part := range []string{"groups", "rulesets", "ini"} {
		u := parts[part]
		if u == "" {
			continue
		}
		prev, _ := s.Store.Snapshots().Get(ctx, owner, RuleKey(id, part))
		resp, err := s.Fetcher.Fetch(ctx, source.Request{URL: u})
		if err != nil && firstErr == nil {
			firstErr = err
		}
		snap := source.Update(toSourceSnap(prev), resp, err, err == nil && len(bytes.TrimSpace(resp.Body)) > 0, "empty", s.now())
		if err := s.Store.Snapshots().Put(ctx, owner, fromSourceSnap(RuleKey(id, part), owner, snap)); err != nil {
			return err
		}
	}
	s.rerenderUsing(ctx, owner, func(p *profile.Profile) bool { return p.Rules != nil && p.Rules.Ref == id })
	return firstErr
}

func (s *Service) rerenderUsing(ctx context.Context, owner string, uses func(*profile.Profile) bool) {
	ps, err := s.Store.Profiles().List(ctx, owner, store.Page{Limit: 200})
	if err != nil {
		return
	}
	for _, p := range ps {
		var l diag.List
		doc, err := profile.Parse(p.Doc, &l)
		if err != nil || !uses(doc) {
			continue
		}
		if _, _, err := s.RenderProfile(ctx, owner, p.ID); err != nil {
			s.Log.Warn("render after refresh", "profile", p.ID, "error", err)
		}
	}
}

func toSourceSnap(s *store.Snapshot) *source.Snapshot {
	if s == nil {
		return nil
	}
	return &source.Snapshot{Content: s.Content, Hash: s.Hash, Headers: s.Headers, FetchedAt: s.FetchedAt, LastError: s.LastError, LastAttempt: s.LastAttempt, FailingSince: s.FailingSince}
}

func fromSourceSnap(key, owner string, s *source.Snapshot) *store.Snapshot {
	return &store.Snapshot{Key: key, OwnerID: owner, Content: s.Content, Hash: s.Hash, Headers: s.Headers, FetchedAt: s.FetchedAt, LastError: s.LastError, LastAttempt: s.LastAttempt, FailingSince: s.FailingSince}
}

// Result 是一次渲染（或预览）的结果。
type Result struct {
	YAML        []byte            `json:"yaml"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	Touched     []string          `json:"touched"`
	Kernel      kernel.Result     `json:"kernel"`
	Key         string            `json:"key"`
	Headers     map[string]string `json:"headers"`
	Fatal       string            `json:"fatal,omitempty"`
	Nodes       []*node.Node      `json:"-"`
	Numbering   []engine.Numbered `json:"numbering"`
}

// OK 报告结果可以发布：没有致命错误，内核检查没有失败。
func (r *Result) OK() bool {
	return r.Fatal == "" && r.Kernel.Status != kernel.Failed && r.Kernel.Status != kernel.TimedOut
}

// assemble 从存储读取方案引用的源与规则源，得到引擎输入与缓存键。
func (s *Service) assemble(ctx context.Context, owner string, p *profile.Profile, profileUpdated time.Time) (engine.BuildInput, []string, time.Time, map[string]map[string]string, diag.List, error) {
	var ds diag.List
	in := engine.BuildInput{Catalog: s.Catalog, Tools: s.Tools}
	var hashes []string
	latest := profileUpdated
	headers := map[string]map[string]string{}
	for i, ref := range p.Refs() {
		src, err := s.Store.Sources().Get(ctx, owner, ref.Ref)
		if errors.Is(err, store.ErrNotFound) {
			ds.Add(diag.ProfileSourceRef, "sources["+strconv.Itoa(i)+"]", "ref", ref.Ref)
			continue
		}
		if err != nil {
			return in, nil, latest, nil, ds, err
		}
		var proc node.Processing
		if len(src.Processing) > 0 {
			if err := json.Unmarshal(src.Processing, &proc); err != nil {
				return in, nil, latest, nil, ds, fmt.Errorf("源 %s 的整理设置无效: %w", src.ID, err)
			}
		}
		si := engine.SourceInput{Ref: src.ID, Name: src.Name, Kind: engine.SourceKind(src.Kind), Mode: ref.Mode, Processing: proc, Interval: src.Interval}
		switch si.Kind {
		case engine.KindSubscription:
			si.URL = src.URL
			if snap, err := s.Store.Snapshots().Get(ctx, owner, SourceKey(src.ID)); err == nil {
				si.Content, si.Updated = snap.Content, snap.FetchedAt
				headers[src.ID] = snap.Headers
				hashes = append(hashes, "src:"+snap.Hash)
				if snap.FailingSince.Before(s.now().Add(-s.staleAfter())) && !snap.FailingSince.IsZero() {
					ds.Add(diag.FetchStaleLong, src.ID, "source", src.Name, "days", strconv.Itoa(int(s.staleAfter().Hours()/24)))
				}
				if snap.LastError != "" {
					ds.Add(diag.ProfileSnapshotOld, src.ID, "source", src.Name, "since", snap.FailingSince.Format(time.RFC3339))
				}
			} else if ref.Mode != profile.ModeProvider {
				ds.Add(diag.FetchFailed, src.ID, "source", src.Name, "reason", "not fetched yet")
			}
		default:
			si.Content, si.Updated = src.Content, src.UpdatedAt
			si.HostedURL = s.PublicURL + "/p/" + LinkTokenPlaceholder + "/" + src.ID
		}
		h := sha256.Sum256(append(append([]byte(src.Name+"|"+src.Kind+"|"+string(ref.Mode)+"|"+src.URL+"|"), src.Processing...), src.Content...))
		hashes = append(hashes, "cfg:"+hex.EncodeToString(h[:]))
		if si.Updated.After(latest) {
			latest = si.Updated
		}
		in.Sources = append(in.Sources, si)
	}
	if p.Rules != nil {
		rs, err := s.Store.RuleSources().Get(ctx, owner, p.Rules.Ref)
		if errors.Is(err, store.ErrNotFound) {
			ds.Add(diag.ProfileRuleRef, "rules", "ref", p.Rules.Ref)
		} else if err != nil {
			return in, nil, latest, nil, ds, err
		} else {
			ri := &engine.RulesInput{Name: rs.Name, Groups: []byte(rs.GroupsText), Rulesets: []byte(rs.RulesetsText), INI: []byte(rs.INIText), Updated: rs.UpdatedAt}
			for part, dst := range map[string]*[]byte{"groups": &ri.Groups, "rulesets": &ri.Rulesets, "ini": &ri.INI} {
				if snap, err := s.Store.Snapshots().Get(ctx, owner, RuleKey(rs.ID, part)); err == nil && snap.Hash != "" {
					*dst = snap.Content
					if snap.FetchedAt.After(ri.Updated) {
						ri.Updated = snap.FetchedAt
					}
				}
			}
			h := sha256.Sum256(append(append(append([]byte{}, ri.Groups...), ri.Rulesets...), ri.INI...))
			hashes = append(hashes, "rules:"+hex.EncodeToString(h[:]))
			if ri.Updated.After(latest) {
				latest = ri.Updated
			}
			in.Rules = ri
		}
	}
	sort.Strings(hashes)
	return in, hashes, latest, headers, ds, nil
}

// Render 渲染方案文档（不写存储）。doc 是方案文档原文；updated 是方案版本的时间。
func (s *Service) Render(ctx context.Context, owner string, doc []byte, updated time.Time, check bool) (*Result, error) {
	var ds diag.List
	p, err := profile.Parse(doc, &ds)
	if err != nil {
		return &Result{Diagnostics: ds.Items(), Fatal: err.Error()}, nil
	}
	in, hashes, latest, headers, ads, err := s.assemble(ctx, owner, p, updated)
	if err != nil {
		return nil, err
	}
	ds.Append(ads.Items()...)
	built := engine.Build(in)
	if built.Updated.After(latest) {
		latest = built.Updated
	}
	out, rerr := engine.Render(engine.Input{Profile: p, Catalog: s.Catalog, Presets: s.Presets, Generated: built.Generated, Diagnostics: append(ds.Items(), built.Diagnostics...), Updated: latest})
	res := &Result{YAML: out.YAML, Diagnostics: out.Diagnostics, Touched: out.Touched, Key: engine.CacheKey(doc, hashes, s.CatalogDigest), Nodes: built.Nodes, Numbering: built.Numbering}
	if rerr != nil {
		res.Fatal = rerr.Error()
		return res, nil
	}
	res.Headers = s.responseHeaders(p, headers, built.Numbering)
	if check {
		var kd diag.List
		switch {
		case out.Preset != nil && !out.Preset.Kernel:
			res.Kernel = kernel.Result{Status: kernel.Unavailable, Message: "no kernel for preset " + out.Preset.ID}
		case s.Kernel == nil:
			res.Kernel = kernel.Result{Status: kernel.Unavailable, Message: "kernel check is not configured"}
			kd.Add(diag.KernelUnavailable, "", "reason", res.Kernel.Message)
		default:
			res.Kernel = s.Kernel.Check(ctx, out.YAML, &kd)
		}
		res.Diagnostics = append(res.Diagnostics, kd.Items()...)
	}
	return res, nil
}

// RenderProfile 渲染并保存方案：输入未变时直接返回已有结果；成功时更新「最近一次成功」，
// 失败时只记录失败原因，订阅端点继续提供上一次成功的结果（第 7.1 节）。
func (s *Service) RenderProfile(ctx context.Context, owner, id string) (*store.Render, *Result, error) {
	defer s.lock(id)()
	p, err := s.Store.Profiles().Get(ctx, owner, id)
	if err != nil {
		return nil, nil, err
	}
	prev, err := s.Store.Renders().Get(ctx, owner, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, nil, err
	}
	res, err := s.Render(ctx, owner, p.Doc, p.UpdatedAt, true)
	if err != nil {
		return nil, nil, err
	}
	r := &store.Render{ProfileID: id, OwnerID: owner}
	if prev != nil {
		*r = *prev
	}
	if prev != nil && prev.GoodKey == res.Key && prev.LastStatus == "ok" {
		return prev, res, nil
	}
	diags, _ := json.Marshal(res.Diagnostics)
	r.LastKey, r.LastAt, r.LastDiags = res.Key, s.now(), diags
	if res.OK() {
		sum := sha256.Sum256(res.YAML)
		r.GoodKey, r.GoodYAML, r.GoodETag, r.GoodAt, r.GoodHeaders = res.Key, res.YAML, `"`+hex.EncodeToString(sum[:16])+`"`, s.now(), res.Headers
		r.LastStatus, r.LastError = "ok", ""
	} else {
		r.LastStatus = "failed"
		r.LastError = res.Fatal
		if r.LastError == "" {
			r.LastError = res.Kernel.Message
		}
	}
	if err := s.Store.Renders().Put(ctx, owner, r); err != nil {
		return nil, nil, err
	}
	return r, res, nil
}

// responseHeaders 计算订阅响应头（第 13.1 节）。
func (s *Service) responseHeaders(p *profile.Profile, snaps map[string]map[string]string, numbering []engine.Numbered) map[string]string {
	h := map[string]string{}
	interval := p.Output.UpdateInterval
	if interval <= 0 {
		interval = 12
	}
	h["profile-update-interval"] = strconv.Itoa(interval)
	name := p.Name
	if name == "" {
		name = "clash-launch"
	}
	h["content-disposition"] = "attachment; filename*=UTF-8''" + url.PathEscape(name+".yaml")
	switch {
	case p.Output.UserInfo == "sum":
		var ids []string
		for _, n := range numbering {
			ids = append(ids, n.Ref)
		}
		if v := sumUserinfo(ids, snaps); v != "" {
			h["subscription-userinfo"] = v
		}
	case strings.HasPrefix(p.Output.UserInfo, "source:"):
		if v := snaps[strings.TrimPrefix(p.Output.UserInfo, "source:")]["subscription-userinfo"]; v != "" {
			h["subscription-userinfo"] = v
		}
	}
	return h
}

// sumUserinfo 汇总各订阅的 subscription-userinfo：流量相加，过期时间取最早（第 13.1 节）。
func sumUserinfo(ids []string, snaps map[string]map[string]string) string {
	var up, down, total, expire int64
	found := false
	for _, id := range ids {
		v := snaps[id]["subscription-userinfo"]
		if v == "" {
			continue
		}
		found = true
		for _, part := range strings.Split(v, ";") {
			k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil {
				continue
			}
			switch strings.ToLower(k) {
			case "upload":
				up += n
			case "download":
				down += n
			case "total":
				total += n
			case "expire":
				if n > 0 && (expire == 0 || n < expire) {
					expire = n
				}
			}
		}
	}
	if !found {
		return ""
	}
	s := fmt.Sprintf("upload=%d; download=%d; total=%d", up, down, total)
	if expire > 0 {
		s += "; expire=" + strconv.FormatInt(expire, 10)
	}
	return s
}
