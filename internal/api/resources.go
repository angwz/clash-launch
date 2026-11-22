package api

import (
	"cmp"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/engine"
	"github.com/angwz/clash-launch/internal/kernel"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/ruleset"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/version"
)

func etag(v int64) string { return `"v` + strconv.FormatInt(v, 10) + `"` }

// ifMatch 读取 If-Match（第 12.1 节：修改必须带 If-Match）。返回期望的版本。
func ifMatch(r *http.Request) (int64, error) {
	h := strings.TrimPrefix(strings.Trim(r.Header.Get("If-Match"), " "), "W/")
	if h == "" {
		return 0, badRequest("If-Match is required")
	}
	v, err := strconv.ParseInt(strings.TrimPrefix(strings.Trim(h, `"`), "v"), 10, 64)
	if err != nil {
		return 0, badRequest("invalid If-Match")
	}
	return v, nil
}

func page(r *http.Request) store.Page {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return store.Page{After: r.URL.Query().Get("cursor"), Limit: n}
}

type list[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next,omitempty"`
}

// —— 节点源 ——

type snapshotDTO struct {
	FetchedAt    *time.Time `json:"fetchedAt,omitempty"`
	LastAttempt  *time.Time `json:"lastAttempt,omitempty"`
	LastError    string     `json:"lastError,omitempty"`
	FailingSince *time.Time `json:"failingSince,omitempty"`
	Hash         string     `json:"hash,omitempty"`
}

type sourceDTO struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	URL        string            `json:"url,omitempty"`
	Content    string            `json:"content,omitempty"`
	UA         string            `json:"ua,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Interval   int               `json:"interval"`
	Processing json.RawMessage   `json:"processing"`
	Version    int64             `json:"version"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
	Snapshot   *snapshotDTO      `json:"snapshot,omitempty"`
	// Summary 是节点概况：可用节点数、协议分布、订阅的流量与到期时间（反馈 #76）
	Summary *sourceSummary `json:"summary,omitempty"`
	// Grant 非空表示这是别人授权给我的只读节点（反馈 #75）：不返回地址与内容，不能修改
	Grant *sourceGrantDTO `json:"grant,omitempty"`
}

type sourceGrantDTO struct {
	ID   string `json:"id"`
	From string `json:"from"`
}

type sourceReq struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	URL        string            `json:"url"`
	Content    string            `json:"content"`
	UA         string            `json:"ua"`
	Headers    map[string]string `json:"headers"`
	Interval   int               `json:"interval"`
	Processing json.RawMessage   `json:"processing"`
}

func tp(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Server) toSource(r *http.Request, u *store.User, src *store.Source) sourceDTO {
	d := sourceDTO{ID: src.ID, Name: src.Name, Kind: src.Kind, URL: src.URL, Content: string(src.Content), UA: src.UA, Headers: src.Headers, Interval: src.Interval, Processing: src.Processing, Version: src.Version, CreatedAt: src.CreatedAt, UpdatedAt: src.UpdatedAt}
	if len(d.Processing) == 0 {
		d.Processing = json.RawMessage("{}")
	}
	if snap, err := s.Svc.Store.Snapshots().Get(r.Context(), u.ID, service.SourceKey(src.ID)); err == nil {
		d.Snapshot = &snapshotDTO{FetchedAt: tp(snap.FetchedAt), LastAttempt: tp(snap.LastAttempt), LastError: snap.LastError, FailingSince: tp(snap.FailingSince), Hash: snap.Hash}
		if src.Kind == "subscription" && len(snap.Content) > 0 {
			d.Summary = s.summarize(src, snap.Content, snap.Headers)
		}
	}
	if src.Kind != "subscription" {
		d.Summary = s.summarize(src, src.Content, nil)
	}
	return d
}

var sourceKinds = []string{"single", "batch", "subscription", "file"}

func (s *Server) applySource(p *principal, req sourceReq, src *store.Source) error {
	if strings.TrimSpace(req.Name) == "" || !slices.Contains(sourceKinds, req.Kind) {
		return badRequest("name and a valid kind (single, batch, subscription, file) are required")
	}
	if req.Kind == "subscription" {
		u, err := url.Parse(req.URL)
		if err != nil {
			return badRequest("invalid url")
		}
		if err := guardCheck(s, u); err != nil {
			return badRequest("%v", err)
		}
	} else if strings.TrimSpace(req.Content) == "" {
		return badRequest("content is required for %s sources", req.Kind)
	}
	if len(req.Processing) > 0 {
		var proc node.Processing
		if err := json.Unmarshal(req.Processing, &proc); err != nil {
			return badRequest("invalid processing: %v", err)
		}
	}
	src.Name, src.Kind, src.URL, src.UA, src.Headers, src.Interval, src.Processing = strings.TrimSpace(req.Name), req.Kind, req.URL, req.UA, req.Headers, req.Interval, req.Processing
	src.Content = nil
	if req.Kind != "subscription" {
		src.Content, src.URL = []byte(req.Content), ""
	}
	return nil
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request, p *principal) {
	items, err := s.Svc.Store.Sources().List(r.Context(), p.User.ID, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[sourceDTO]{Items: []sourceDTO{}}
	for i := range items {
		out.Items = append(out.Items, s.toSource(r, p.User, &items[i]))
	}
	if len(items) > 0 && len(items) == pageSize(r) {
		out.Next = items[len(items)-1].ID
	}
	// 别人授权给我的节点附在第一页之后（只读，反馈 #75）；分页只针对自己的节点
	if page(r).After == "" {
		gs, err := s.Svc.GrantedSources(r.Context(), p.User)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, g := range gs {
			d := sourceDTO{ID: g.Source.ID, Name: g.Source.Name, Kind: g.Source.Kind, Interval: g.Source.Interval, Processing: json.RawMessage("{}"), CreatedAt: g.Grant.CreatedAt, UpdatedAt: g.Source.UpdatedAt, Grant: &sourceGrantDTO{ID: g.Grant.ID, From: g.From}}
			// 概况（节点数、协议）可以看，内容与地址不给（反馈 #76）
			if g.Source.Kind == "subscription" {
				if snap, err := s.Svc.Store.Snapshots().Get(r.Context(), g.Grant.OwnerID, service.SourceKey(g.Source.ID)); err == nil && len(snap.Content) > 0 {
					d.Summary = s.summarize(g.Source, snap.Content, snap.Headers)
				}
			} else {
				d.Summary = s.summarize(g.Source, g.Source.Content, nil)
			}
			out.Items = append(out.Items, d)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func pageSize(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 200 {
		return 50
	}
	return n
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request, p *principal) {
	var req sourceReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	src := &store.Source{}
	if err := s.applySource(p, req, src); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.quota(r, p, service.QuotaSource, int64(len(src.Content)), src.Interval); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Store.Sources().Create(r.Context(), p.User.ID, src); err != nil {
		s.fail(w, err)
		return
	}
	s.Svc.Footprint(r.Context(), p.User.ID, service.FPFirstSource, "", map[string]string{"name": src.Name})
	w.Header().Set("ETag", etag(src.Version))
	writeJSON(w, http.StatusCreated, s.toSource(r, p.User, src))
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request, p *principal) {
	src, err := s.Svc.Store.Sources().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(src.Version))
	writeJSON(w, http.StatusOK, s.toSource(r, p.User, src))
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := ifMatch(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	var req sourceReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	src, err := s.Svc.Store.Sources().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	before, prevURL := int64(len(src.Content)), src.URL
	if err := s.applySource(p, req, src); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.quota(r, p, "", int64(len(src.Content))-before, src.Interval); err != nil {
		s.fail(w, err)
		return
	}
	src.Version = v
	if err := s.Svc.Store.Sources().Update(r.Context(), p.User.ID, src); err != nil {
		s.fail(w, err)
		return
	}
	// 保存已成功：重新生成用到它的配置，订阅链接随之更新（R47）。这一步出错时只记日志，
	// 不把已经成功的保存报告为失败；订阅继续提供上一次成功的结果。
	if err := s.Svc.SourceSaved(r.Context(), p.User, prevURL, src); err != nil {
		s.Svc.Log.Warn("rerender after source save", "source", src.ID, "error", err)
	}
	w.Header().Set("ETag", etag(src.Version))
	writeJSON(w, http.StatusOK, s.toSource(r, p.User, src))
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteSource(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshSource(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.CheckRefresh(p.User); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := s.Svc.Store.Sources().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	snap, err := s.Svc.RefreshSource(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshotDTO{FetchedAt: tp(snap.FetchedAt), LastAttempt: tp(snap.LastAttempt), LastError: snap.LastError, FailingSince: tp(snap.FailingSince), Hash: snap.Hash})
}

type nodeDTO struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Server      string      `json:"server"` // 已打码
	Port        int         `json:"port"`
	Fingerprint string      `json:"fingerprint"`
	Region      node.Region `json:"region"`
	Emoji       string      `json:"emoji,omitempty"`
	Tags        []string    `json:"tags"`
	Rate        float64     `json:"rate"`
	Origin      string      `json:"origin"`
}

// maskHost 打码服务器地址：保留首尾少量字符，中间以 * 代替（节点表格展示用）。
func maskHost(h string) string {
	if len(h) <= 6 {
		return "***"
	}
	return h[:3] + "***" + h[len(h)-3:]
}

func (s *Server) sourceNodes(w http.ResponseWriter, r *http.Request, p *principal) {
	src, err := s.Svc.Store.Sources().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	var proc node.Processing
	_ = json.Unmarshal(src.Processing, &proc)
	s.writeNodes(w, r, p, src, src.Content, proc)
}

// nodesPreviewReq 是节点源编辑中、尚未保存的内容与整理设置（R49）。
// content 只对手动节点有效；订阅始终用拉取快照，整理设置可以预览。
type nodesPreviewReq struct {
	Content    *string         `json:"content,omitempty"`
	Processing json.RawMessage `json:"processing,omitempty"`
}

// previewSourceNodes 按编辑中的内容解析节点，不写存储：失焦即可看到结果，与保存后的解析完全一致（同一函数）。
func (s *Server) previewSourceNodes(w http.ResponseWriter, r *http.Request, p *principal) {
	src, err := s.Svc.Store.Sources().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	var req nodesPreviewReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	var proc node.Processing
	if len(req.Processing) > 0 {
		if err := json.Unmarshal(req.Processing, &proc); err != nil {
			s.fail(w, badRequest("invalid processing: %v", err))
			return
		}
	}
	content := src.Content
	if req.Content != nil && src.Kind != "subscription" {
		content = []byte(*req.Content)
	}
	s.writeNodes(w, r, p, src, content, proc)
}

// writeNodes 解析并整理节点，写出节点表格的数据；订阅忽略 content，使用拉取快照。
func (s *Server) writeNodes(w http.ResponseWriter, r *http.Request, p *principal, src *store.Source, content []byte, proc node.Processing) {
	if src.Kind == "subscription" {
		snap, err := s.Svc.Store.Snapshots().Get(r.Context(), p.User.ID, service.SourceKey(src.ID))
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"nodes": []nodeDTO{}, "diagnostics": []diag.Diagnostic{}})
			return
		}
		content = snap.Content
	}
	var ds diag.List
	nodes, format := node.Parse(content, src.ID, s.Svc.Catalog, &ds)
	nodes = node.Process(nodes, proc, s.Svc.Tools, src.ID, &ds)
	out := []nodeDTO{}
	for _, n := range nodes {
		out = append(out, nodeDTO{Name: n.Name, Type: n.Type, Server: maskHost(n.Server), Port: n.Port, Fingerprint: n.Fingerprint, Region: n.Region, Emoji: n.Emoji, Tags: n.Tags, Rate: n.Rate, Origin: n.Origin})
	}
	writeJSON(w, http.StatusOK, map[string]any{"format": format, "nodes": out, "diagnostics": diag.Sorted(ds.Items())})
}

// —— 规则源 ——

type ruleSourceDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Preset       string    `json:"preset,omitempty"`
	GroupsURL    string    `json:"groupsUrl,omitempty"`
	RulesetsURL  string    `json:"rulesetsUrl,omitempty"`
	INIURL       string    `json:"iniUrl,omitempty"`
	GroupsText   string    `json:"groupsText,omitempty"`
	RulesetsText string    `json:"rulesetsText,omitempty"`
	INIText      string    `json:"iniText,omitempty"`
	Interval     int       `json:"interval"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type ruleSourceReq struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Preset       string `json:"preset"`
	GroupsURL    string `json:"groupsUrl"`
	RulesetsURL  string `json:"rulesetsUrl"`
	INIURL       string `json:"iniUrl"`
	GroupsText   string `json:"groupsText"`
	RulesetsText string `json:"rulesetsText"`
	INIText      string `json:"iniText"`
	Interval     int    `json:"interval"`
}

func toRule(x *store.RuleSource) ruleSourceDTO {
	return ruleSourceDTO{ID: x.ID, Name: x.Name, Kind: x.Kind, Preset: x.Preset, GroupsURL: x.GroupsURL, RulesetsURL: x.RulesetsURL, INIURL: x.INIURL, GroupsText: x.GroupsText, RulesetsText: x.RulesetsText, INIText: x.INIText, Interval: x.Interval, Version: x.Version, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt}
}

func (s *Server) applyRule(req ruleSourceReq, x *store.RuleSource) error {
	if strings.TrimSpace(req.Name) == "" || (req.Kind != "toml" && req.Kind != "ini") {
		return badRequest("name and kind (toml or ini) are required")
	}
	for _, raw := range []string{req.GroupsURL, req.RulesetsURL, req.INIURL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			return badRequest("invalid url")
		}
		if err := guardCheck(s, u); err != nil {
			return badRequest("%v", err)
		}
	}
	*x = store.RuleSource{ID: x.ID, OwnerID: x.OwnerID, Version: x.Version, CreatedAt: x.CreatedAt,
		Name: strings.TrimSpace(req.Name), Kind: req.Kind, Preset: req.Preset, GroupsURL: req.GroupsURL, RulesetsURL: req.RulesetsURL, INIURL: req.INIURL,
		GroupsText: req.GroupsText, RulesetsText: req.RulesetsText, INIText: req.INIText, Interval: req.Interval}
	return nil
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request, p *principal) {
	items, err := s.Svc.Store.RuleSources().List(r.Context(), p.User.ID, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[ruleSourceDTO]{Items: []ruleSourceDTO{}}
	for i := range items {
		out.Items = append(out.Items, toRule(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request, p *principal) {
	var req ruleSourceReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	x := &store.RuleSource{}
	if err := s.applyRule(req, x); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.CheckRuleURLs(p.User, x); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.quota(r, p, "", int64(len(x.GroupsText)+len(x.RulesetsText)+len(x.INIText)), x.Interval); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Store.RuleSources().Create(r.Context(), p.User.ID, x); err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusCreated, toRule(x))
}

func (s *Server) getRule(w http.ResponseWriter, r *http.Request, p *principal) {
	x, err := s.Svc.Store.RuleSources().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusOK, toRule(x))
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := ifMatch(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	var req ruleSourceReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	x, err := s.Svc.Store.RuleSources().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	prev := *x
	if err := s.applyRule(req, x); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.CheckRuleURLs(p.User, x); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.quota(r, p, "", 0, x.Interval); err != nil {
		s.fail(w, err)
		return
	}
	x.Version = v
	if err := s.Svc.Store.RuleSources().Update(r.Context(), p.User.ID, x); err != nil {
		s.fail(w, err)
		return
	}
	// 同节点源：保存成功后重新生成引用它的配置（R47），出错只记日志
	if err := s.Svc.RuleSourceSaved(r.Context(), p.User, &prev, x); err != nil {
		s.Svc.Log.Warn("rerender after rule source save", "rule_source", x.ID, "error", err)
	}
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusOK, toRule(x))
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.RuleSources().Delete(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshRule(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.CheckRefresh(p.User); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := s.Svc.Store.RuleSources().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.RefreshRuleSource(r.Context(), p.User.ID, r.PathValue("id")); err != nil && !errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]string{"error": sourceReason(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// —— 方案 ——

type renderDTO struct {
	Status      string          `json:"status"` // ok | failed | pending
	Error       string          `json:"error,omitempty"`
	GoodAt      *time.Time      `json:"goodAt,omitempty"`
	LastAt      *time.Time      `json:"lastAt,omitempty"`
	Diagnostics json.RawMessage `json:"diagnostics"`
}

type profileDTO struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Doc       string     `json:"doc"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Protected bool       `json:"protected"`
	Render    *renderDTO `json:"render,omitempty"`
}

type profileReq struct {
	Doc  string `json:"doc"`
	Note string `json:"note"`
	// Autosave：编辑器的自动保存。10 分钟内连续的自动保存在历史中合并为一个版本（反馈 #35）。
	Autosave bool `json:"autosave"`
}

func (s *Server) toProfile(r *http.Request, owner string, p *store.Profile) profileDTO {
	d := profileDTO{ID: p.ID, Name: p.Name, Doc: string(p.Doc), Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Protected: p.Protected}
	if x, err := s.Svc.Store.Renders().Get(r.Context(), owner, p.ID); err == nil {
		d.Render = &renderDTO{Status: x.LastStatus, Error: x.LastError, GoodAt: tp(x.GoodAt), LastAt: tp(x.LastAt), Diagnostics: diagsJSON(x.LastDiags)}
	} else {
		d.Render = &renderDTO{Status: "pending", Diagnostics: json.RawMessage("[]")}
	}
	return d
}

// parseDoc 校验方案文档并返回名称。结构错误返回 422 与诊断。
func parseDoc(doc string) (*profile.Profile, []diag.Diagnostic, error) {
	var ds diag.List
	p, err := profile.Parse([]byte(doc), &ds)
	return p, ds.Items(), err
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request, p *principal) {
	items, err := s.Svc.Store.Profiles().List(r.Context(), p.User.ID, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[profileDTO]{Items: []profileDTO{}}
	for i := range items {
		out.Items = append(out.Items, s.toProfile(r, p.User.ID, &items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request, p *principal) {
	var req profileReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	doc, ds, err := parseDoc(req.Doc)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "profile.invalid", err.Error(), ds...)
		return
	}
	x := &store.Profile{Name: doc.Name, Doc: []byte(req.Doc)}
	if err := s.quota(r, p, service.QuotaProfile, int64(len(req.Doc)), 0); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Store.Profiles().Create(r.Context(), p.User.ID, x, req.Note); err != nil {
		s.fail(w, err)
		return
	}
	if _, _, err := s.Svc.RenderProfile(r.Context(), p.User.ID, x.ID); err != nil {
		s.Log.Warn("render after create", "error", err)
	}
	s.audit(r.Context(), p.User.ID, "profile.create", map[string]string{"profile": x.ID})
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusCreated, s.toProfile(r, p.User.ID, x))
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request, p *principal) {
	x, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusOK, s.toProfile(r, p.User.ID, x))
}

// autosaveWindow 是自动保存合并为同一版本的时间窗（从上一次自动保存算起）。
const autosaveWindow = 10 * time.Minute

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := ifMatch(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	var req profileReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	doc, ds, err := parseDoc(req.Doc)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "profile.invalid", err.Error(), ds...)
		return
	}
	x := &store.Profile{ID: r.PathValue("id"), Name: doc.Name, Doc: []byte(req.Doc), Version: v}
	// 每次保存都会增加一个版本：按新文档的大小计入存储
	if err := s.quota(r, p, "", int64(len(req.Doc)), 0); err != nil {
		s.fail(w, err)
		return
	}
	save := func() error { return s.Svc.Store.Profiles().Update(r.Context(), p.User.ID, x, req.Note) }
	if req.Autosave {
		save = func() error { return s.Svc.Store.Profiles().Autosave(r.Context(), p.User.ID, x, autosaveWindow) }
	}
	if err := save(); err != nil {
		s.fail(w, err)
		return
	}
	if _, _, err := s.Svc.RenderProfile(r.Context(), p.User.ID, x.ID); err != nil {
		s.Log.Warn("render after update", "error", err)
	}
	s.audit(r.Context(), p.User.ID, "profile.update", map[string]string{"profile": x.ID})
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusOK, s.toProfile(r, p.User.ID, x))
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteProfile(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "profile.delete", map[string]string{"profile": r.PathValue("id")})
	w.WriteHeader(http.StatusNoContent)
}

type revisionDTO struct {
	N         int       `json:"n"`
	Note      string    `json:"note"`
	Doc       string    `json:"doc"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) listRevisions(w http.ResponseWriter, r *http.Request, p *principal) {
	if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	revs, err := s.Svc.Store.Profiles().Revisions(r.Context(), p.User.ID, r.PathValue("id"), page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[revisionDTO]{Items: []revisionDTO{}}
	for _, v := range revs {
		out.Items = append(out.Items, revisionDTO{N: v.N, Note: v.Note, Doc: string(v.Doc), CreatedAt: v.CreatedAt})
	}
	if len(revs) > 0 {
		out.Next = strconv.Itoa(revs[len(revs)-1].N)
	}
	writeJSON(w, http.StatusOK, out)
}

// rollback 把方案恢复到某个版本：恢复本身也生成一个新版本（第 16.1 节）。
func (s *Server) rollback(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := ifMatch(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	var req struct {
		N int `json:"n"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	rev, err := s.Svc.Store.Profiles().Revision(r.Context(), p.User.ID, r.PathValue("id"), req.N)
	if err != nil {
		s.fail(w, err)
		return
	}
	doc, _, err := parseDoc(string(rev.Doc))
	if err != nil {
		s.fail(w, badRequest("revision %d is not a valid document", req.N))
		return
	}
	x := &store.Profile{ID: r.PathValue("id"), Name: doc.Name, Doc: rev.Doc, Version: v}
	if err := s.Svc.Store.Profiles().Update(r.Context(), p.User.ID, x, "rollback to "+strconv.Itoa(req.N)); err != nil {
		s.fail(w, err)
		return
	}
	if _, _, err := s.Svc.RenderProfile(r.Context(), p.User.ID, x.ID); err != nil {
		s.Log.Warn("render after rollback", "error", err)
	}
	w.Header().Set("ETag", etag(x.Version))
	writeJSON(w, http.StatusOK, s.toProfile(r, p.User.ID, x))
}

type previewResp struct {
	YAML        string            `json:"yaml"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	Touched     []string          `json:"touched"`
	Kernel      kernel.Result     `json:"kernel"`
	Fatal       string            `json:"fatal,omitempty"`
	Diff        []DiffLine        `json:"diff"`
	// Numbering 是订阅的编号（GROUPID），由引擎计算；界面在源列表中显示，不自行推算。
	Numbering []engine.Numbered `json:"numbering"`
	// Chains 是每条链式代理规则实际套用到的节点与 provider；Outbounds 是可作前置的节点与策略组。
	Chains    []engine.ChainResult `json:"chains"`
	Outbounds []string             `json:"outbounds"`
	// Nodes 是写入配置的节点数。
	Nodes int `json:"nodes"`
}

// preview 用请求体中的草稿渲染并校验，不保存（第 12.2 节）。check=true 时执行内核检查。
func (s *Server) preview(w http.ResponseWriter, r *http.Request, p *principal) {
	var req profileReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if !s.renderAllowed(w, p, r.URL.Query().Get("check") == "true") {
		return
	}
	var current []byte
	if x, err := s.Svc.Store.Renders().Get(r.Context(), p.User.ID, r.PathValue("id")); err == nil {
		current = x.GoodYAML
	} else if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	res, err := s.Svc.Render(r.Context(), p.User.ID, []byte(req.Doc), time.Now(), r.URL.Query().Get("check") == "true")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, previewResp{YAML: cmp.Or(string(res.Preview), string(res.YAML)), Diagnostics: diag.Sorted(res.Diagnostics), Touched: nonNil(res.Touched), Kernel: res.Kernel, Fatal: res.Fatal, Diff: Diff(string(current), string(res.YAML)), Numbering: nonNil(res.Numbering), Chains: nonNil(res.Chains), Outbounds: nonNil(res.Outbounds), Nodes: len(res.Nodes)})
}

// ruleHosts 返回当前用户的分流规则地址可以使用的站点（反馈 #72）；restricted=false 表示不限制（管理员）。
func (s *Server) ruleHosts(w http.ResponseWriter, _ *http.Request, p *principal) {
	h := s.Svc.RuleHostsFor(p.User)
	writeJSON(w, http.StatusOK, map[string]any{"restricted": h != nil, "hosts": nonNil([]string(h))})
}

// absorb 把在完整 YAML 上的修改换算成原始编辑层（反馈 #70）。不保存：前端把结果写回草稿，
// 照常走预览与自动保存。完整 YAML 无效时 ok=false，诊断说明原因，原始编辑保持不变。
func (s *Server) absorb(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Doc  string `json:"doc"`
		YAML string `json:"yaml"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if !s.renderAllowed(w, p, false) {
		return
	}
	if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	raw, ds, err := s.Svc.Absorb(r.Context(), p.User.ID, []byte(req.Doc), []byte(req.YAML))
	if err != nil && !errors.Is(err, engine.ErrAbsorb) && !errors.Is(err, engine.ErrFatal) {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": err == nil, "raw": raw, "diagnostics": diag.Sorted(ds)})
}

// renderStatus 返回最近一次成功的结果与最近一次尝试的诊断。
func (s *Server) renderStatus(w http.ResponseWriter, r *http.Request, p *principal) {
	x, err := s.Svc.Store.Renders().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "yaml": "", "diagnostics": []any{}})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": x.LastStatus, "error": x.LastError, "yaml": string(x.GoodYAML), "goodAt": tp(x.GoodAt), "lastAt": tp(x.LastAt), "diagnostics": diagsJSON(x.LastDiags)})
}

// exportProfile 导出方案文档。文档中本来就没有凭据（源以引用代替）。
func (s *Server) exportProfile(w http.ResponseWriter, r *http.Request, p *principal) {
	x, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(x.Name+".profile.yaml"))
	_, _ = w.Write(x.Doc) //nolint:gosec // 以 text/yaml 与 nosniff 返回，不会被当作 HTML 解释
}

// —— 订阅链接 ——

type linkDTO struct {
	ID         string     `json:"id"`
	URL        string     `json:"url"`
	Note       string     `json:"note"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastAccess *time.Time `json:"lastAccess,omitempty"`
	LastUA     string     `json:"lastUa,omitempty"`
	Version    int64      `json:"version"`
	// Limits 是访问限制（需求 R42）；没有限制时为空对象
	Limits store.LinkLimits `json:"limits"`
}

// baseURL 返回对外地址：配置了 CLASH_LAUNCH_PUBLIC_URL 时取与请求域名相同的那个（见 publicurl.go），
// 否则根据请求推断（第 14.2 节）。
func (s *Server) baseURL(r *http.Request) string {
	if p := s.publicFor(r); p != "" {
		return p
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// 只有直连地址是受信任代理时才采信 X-Forwarded-Proto
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if a, err := parseAddr(host); err == nil && s.trusted(a) && r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
	}
	return scheme + "://" + r.Host
}

func (s *Server) toLink(r *http.Request, l *store.Link) linkDTO {
	return linkDTO{ID: l.ID, URL: s.baseURL(r) + "/s/" + l.Token, Note: l.Note, CreatedAt: l.CreatedAt, LastAccess: tp(l.LastAccess), LastUA: l.LastUA, Version: l.Version, Limits: l.Limits}
}

func (s *Server) listLinks(w http.ResponseWriter, r *http.Request, p *principal) {
	if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	ls, err := s.Svc.Store.Links().List(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[linkDTO]{Items: []linkDTO{}}
	for i := range ls {
		out.Items = append(out.Items, s.toLink(r, &ls[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Note string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.quota(r, p, service.QuotaLink, 0, 0); err != nil {
		s.fail(w, err)
		return
	}
	tok := auth.NewToken(32) // 256 位，base64url 编码为 43 个字符（第 13.2 节）
	l := &store.Link{ProfileID: r.PathValue("id"), Token: tok, TokenHash: auth.HashToken(tok), Note: truncate(req.Note, 200)}
	if err := s.Svc.Store.Links().Create(r.Context(), p.User.ID, l); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "link.create", map[string]string{"link": l.ID})
	writeJSON(w, http.StatusCreated, s.toLink(r, l))
}

func (s *Server) ownedLink(r *http.Request, p *principal) (*store.Link, error) {
	l, err := s.Svc.Store.Links().Get(r.Context(), p.User.ID, r.PathValue("lid"))
	if err != nil {
		return nil, err
	}
	if l.ProfileID != r.PathValue("id") {
		return nil, store.ErrNotFound
	}
	return l, nil
}

// resetLink 生成新令牌，旧链接立即失效（第 13.2 节）。
func (s *Server) resetLink(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := ifMatch(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	l, err := s.ownedLink(r, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	tok := auth.NewToken(32)
	l.Token, l.TokenHash, l.Version = tok, auth.HashToken(tok), v
	if err := s.Svc.Store.Links().Update(r.Context(), p.User.ID, l); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "link.reset", map[string]string{"link": l.ID})
	writeJSON(w, http.StatusOK, s.toLink(r, l))
}

func (s *Server) patchLink(w http.ResponseWriter, r *http.Request, p *principal) {
	v, err := ifMatch(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	// 只改出现的字段：备注、访问限制（需求 R42）
	var req struct {
		Note   *string           `json:"note"`
		Limits *store.LinkLimits `json:"limits"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	l, err := s.ownedLink(r, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	if req.Note != nil {
		l.Note = truncate(*req.Note, 200)
	}
	if req.Limits != nil {
		lim, err := service.NormalizeLinkLimits(*req.Limits)
		if err != nil {
			s.fail(w, err)
			return
		}
		if !reflect.DeepEqual(lim, l.Limits) {
			s.audit(r.Context(), p.User.ID, "link.limits", map[string]string{"link": l.ID})
		}
		l.Limits = lim
	}
	l.Version = v
	if err := s.Svc.Store.Links().Update(r.Context(), p.User.ID, l); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toLink(r, l))
}

func (s *Server) deleteLink(w http.ResponseWriter, r *http.Request, p *principal) {
	l, err := s.ownedLink(r, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.DeleteLink(r.Context(), p.User, l.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "link.delete", map[string]string{"link": l.ID})
	w.WriteHeader(http.StatusNoContent)
}

// —— 目录 ——

func (s *Server) catalogMeta(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("pack") != s.Svc.Catalog.Pack.ID {
		problem(w, http.StatusNotFound, "not_found", "unknown pack")
		return
	}
	tag := `"` + s.Svc.CatalogDigest[:16] + `"`
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	presets, _ := ruleset.LoadPresets(catalogFS)
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, map[string]any{
		"pack": s.Svc.Catalog.Pack, "digest": s.Svc.CatalogDigest, "version": version.Version,
		"sections": s.Svc.Catalog.Sections(), "presets": s.Svc.Presets, "ruleSources": presets,
		"decor": s.Svc.Tools.Decor.List(),
	})
}

func (s *Server) catalogSchema(w http.ResponseWriter, r *http.Request) {
	s.catalogDoc(w, r, func(c *catalog.Catalog, lang string) any { return c.JSONSchema(lang) })
}

func (s *Server) catalogMessages(w http.ResponseWriter, r *http.Request) {
	s.catalogDoc(w, r, func(c *catalog.Catalog, lang string) any { return c.Messages(lang) })
}

func (s *Server) catalogDoc(w http.ResponseWriter, r *http.Request, f func(*catalog.Catalog, string) any) {
	if r.PathValue("pack") != s.Svc.Catalog.Pack.ID {
		problem(w, http.StatusNotFound, "not_found", "unknown pack")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "zh-CN" {
		lang = "en"
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, f(s.Svc.Catalog, lang))
}

// diagnostics 返回全部诊断代码与两种语言的消息（前端渲染诊断用）。
func (s *Server) diagnostics(w http.ResponseWriter, _ *http.Request) {
	out := map[string]map[string]string{}
	for code, info := range diag.All() {
		out[string(code)] = map[string]string{"severity": string(info.Severity), "en": info.EN, "zh-CN": info.ZH}
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if _, err := s.Svc.Store.Users().Count(r.Context()); err != nil {
		problem(w, http.StatusServiceUnavailable, "not_ready", "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// nonNil 让空切片编码为 [] 而不是 null，与契约中的 required 数组一致。
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// diagsJSON 规整已保存的诊断列表：空值与早期写入的 null 都返回 []。
func diagsJSON(raw []byte) json.RawMessage {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(raw)
}

// quota 检查配额：kind 非空时检查数量（新建），extra 为新增的内容字节数，interval 为拉取间隔（秒，0 表示默认）。
func (s *Server) quota(r *http.Request, p *principal, kind string, extra int64, interval int) error {
	ctx := r.Context()
	if err := s.Svc.CheckInterval(ctx, p.User, interval); err != nil {
		return err
	}
	if kind != "" {
		return s.Svc.CheckCreate(ctx, p.User, kind, extra)
	}
	if extra > 0 {
		return s.Svc.CheckStorage(ctx, p.User, extra)
	}
	return nil
}

// protectProfile 开关删除保护（需求 R55）。不改变配置版本，返回更新后的配置。
func (s *Server) protectProfile(w http.ResponseWriter, r *http.Request, p *principal) {
	var in struct {
		Protected bool `json:"protected"`
	}
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	id := r.PathValue("id")
	if err := s.Svc.SetProfileProtected(r.Context(), p.User, id, in.Protected); err != nil {
		s.fail(w, err)
		return
	}
	pr, err := s.Svc.Store.Profiles().Get(r.Context(), p.User.ID, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"protected": pr.Protected})
}

// exportConfig 导出客户端可以直接使用的配置（需求 R17）：与订阅链接同源，可选去掉节点或规则集。
// 返回 JSON（YAML 正文、文件名、诊断与内核检查结果），由界面生成下载；同预览一样受渲染限流。
func (s *Server) exportConfig(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Nodes bool   `json:"nodes"`
		Rules bool   `json:"rules"`
		Link  string `json:"link"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if !s.renderAllowed(w, p, true) {
		return
	}
	res, err := s.Svc.ExportConfig(r.Context(), p.User.ID, r.PathValue("id"), engine.Export{NoNodes: !req.Nodes, NoRules: !req.Rules}, req.Link)
	if err != nil {
		s.fail(w, err)
		return
	}
	suffix := ""
	switch {
	case !req.Nodes && !req.Rules:
		suffix = ".skeleton"
	case !req.Nodes:
		suffix = ".no-proxies"
	case !req.Rules:
		suffix = ".no-rule-sets"
	}
	s.audit(r.Context(), p.User.ID, "profile.export", map[string]string{"profile": r.PathValue("id"), "nodes": strconv.FormatBool(req.Nodes), "rules": strconv.FormatBool(req.Rules)})
	writeJSON(w, http.StatusOK, map[string]any{"yaml": string(res.YAML), "filename": res.Name + suffix + ".yaml", "diagnostics": diag.Sorted(res.Diagnostics), "kernel": res.Kernel, "fatal": res.Fatal})
}

// previewImport 把一份完整的 Clash/mihomo 配置换算为导入计划（需求 R17）：要新建的节点源、配置文档与诊断。
// 不写存储；界面确认后经已有的接口创建节点源与配置（配额、SSRF 防护与校验照常生效）。
// 订阅的更新间隔低于本账号允许的最小值时提高到最小值并报告，避免创建时被配额拒绝。
func (s *Server) previewImport(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Text        string `json:"text"`
		Name        string `json:"name"`
		KeepUnknown bool   `json:"keepUnknown"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if !s.renderAllowed(w, p, false) {
		return
	}
	plan, err := engine.PlanImport(engine.ImportInput{Text: []byte(req.Text), Name: req.Name, Catalog: s.Svc.Catalog, KeepUnknown: req.KeepUnknown})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "diagnostics": diag.Sorted(plan.Diagnostics)})
		return
	}
	l, err := s.Svc.Limits(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	var extra diag.List
	for i := range plan.Sources {
		src := &plan.Sources[i]
		if src.Interval > 0 && src.Interval < l.MinInterval {
			extra.Add(diag.ImportIntervalRaised, "proxy-providers."+src.Name, "name", src.Name, "from", strconv.Itoa(src.Interval), "to", strconv.Itoa(l.MinInterval))
			src.Interval = l.MinInterval
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sources": plan.Sources, "doc": plan.Doc, "counts": plan.Counts, "diagnostics": diag.Sorted(append(plan.Diagnostics, extra.Items()...))})
}

// previewRuleSource 校验编辑中的规则源并预览策略组（O2）：不写存储，受预览限流约束。
func (s *Server) previewRuleSource(w http.ResponseWriter, r *http.Request, p *principal) {
	var in service.RulesPreviewInput
	if err := readJSON(r, &in); err != nil {
		s.fail(w, err)
		return
	}
	if !s.renderAllowed(w, p, false) {
		return
	}
	res, err := s.Svc.PreviewRules(r.Context(), p.User.ID, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
