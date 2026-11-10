package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
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
	if err := s.Svc.Store.Sources().Create(r.Context(), p.User.ID, src); err != nil {
		s.fail(w, err)
		return
	}
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
	if err := s.applySource(p, req, src); err != nil {
		s.fail(w, err)
		return
	}
	src.Version = v
	if err := s.Svc.Store.Sources().Update(r.Context(), p.User.ID, src); err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("ETag", etag(src.Version))
	writeJSON(w, http.StatusOK, s.toSource(r, p.User, src))
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Sources().Delete(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshSource(w http.ResponseWriter, r *http.Request, p *principal) {
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
	content := src.Content
	if src.Kind == "subscription" {
		snap, err := s.Svc.Store.Snapshots().Get(r.Context(), p.User.ID, service.SourceKey(src.ID))
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"nodes": []nodeDTO{}, "diagnostics": []diag.Diagnostic{}})
			return
		}
		content = snap.Content
	}
	var proc node.Processing
	_ = json.Unmarshal(src.Processing, &proc)
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
	if err := s.applyRule(req, x); err != nil {
		s.fail(w, err)
		return
	}
	x.Version = v
	if err := s.Svc.Store.RuleSources().Update(r.Context(), p.User.ID, x); err != nil {
		s.fail(w, err)
		return
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
	Render    *renderDTO `json:"render,omitempty"`
}

type profileReq struct {
	Doc  string `json:"doc"`
	Note string `json:"note"`
}

func (s *Server) toProfile(r *http.Request, owner string, p *store.Profile) profileDTO {
	d := profileDTO{ID: p.ID, Name: p.Name, Doc: string(p.Doc), Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
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
	if err := s.Svc.Store.Profiles().Update(r.Context(), p.User.ID, x, req.Note); err != nil {
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
	if err := s.Svc.Store.Profiles().Delete(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
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
	writeJSON(w, http.StatusOK, previewResp{YAML: string(res.YAML), Diagnostics: diag.Sorted(res.Diagnostics), Touched: nonNil(res.Touched), Kernel: res.Kernel, Fatal: res.Fatal, Diff: Diff(string(current), string(res.YAML)), Numbering: nonNil(res.Numbering), Nodes: len(res.Nodes)})
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
}

// baseURL 返回对外地址：配置了 CLASH_LAUNCH_PUBLIC_URL 时以它为准，否则根据请求推断（第 14.2 节）。
func (s *Server) baseURL(r *http.Request) string {
	if s.Cfg.PublicURL != "" {
		return strings.TrimSuffix(s.Cfg.PublicURL, "/")
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
	return linkDTO{ID: l.ID, URL: s.baseURL(r) + "/s/" + l.Token, Note: l.Note, CreatedAt: l.CreatedAt, LastAccess: tp(l.LastAccess), LastUA: l.LastUA, Version: l.Version}
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
	var req struct {
		Note string `json:"note"`
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
	l.Note, l.Version = truncate(req.Note, 200), v
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
	if err := s.Svc.Store.Links().Delete(r.Context(), p.User.ID, l.ID); err != nil {
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

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
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
