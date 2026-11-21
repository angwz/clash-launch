package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/store"
	"github.com/angwz/clash-launch/internal/version"
)

// 管理与审计接口（第 14.1、14.7 节）。管理员只能看到用户的元数据与用量，看不到他人的内容。

type auditDTO struct {
	ID     string          `json:"id"`
	At     time.Time       `json:"at"`
	Action string          `json:"action"`
	Detail json.RawMessage `json:"detail"`
	User   string          `json:"user,omitempty"` // 管理员视图：账号名
}

func toAudit(e store.AuditEntry) auditDTO {
	d := json.RawMessage(e.Detail)
	if len(d) == 0 || !json.Valid(d) {
		d = json.RawMessage("{}")
	}
	return auditDTO{ID: e.ID, At: e.At, Action: e.Action, Detail: d}
}

// myAudit 返回与本人有关的审计记录（新的在前）。
func (s *Server) myAudit(w http.ResponseWriter, r *http.Request, p *principal) {
	if n, size, ok := pageNo(r); ok {
		n = capPage(n, size, auditCountCap)
		list, total, err := s.Svc.Store.Audit().Paged(r.Context(), p.User.ID, (n-1)*size, size, auditCountCap)
		if err != nil {
			s.fail(w, err)
			return
		}
		out := make([]auditDTO, 0, len(list))
		for _, e := range list {
			out = append(out, toAudit(e))
		}
		writeJSON(w, http.StatusOK, pagedDTO[auditDTO]{Items: out, Total: min(total, auditCountCap), More: total > auditCountCap, Page: n, Size: size})
		return
	}
	list, err := s.Svc.Store.Audit().List(r.Context(), p.User.ID, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]auditDTO, 0, len(list))
	for _, e := range list {
		out = append(out, toAudit(e))
	}
	writeJSON(w, http.StatusOK, auditList(out))
}

// myQuota 返回本人的配额与用量。
func (s *Server) myQuota(w http.ResponseWriter, r *http.Request, p *principal) {
	l, err := s.Svc.Limits(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.Svc.Store.Quotas().Usage(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"limits": l, "usage": u})
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request, _ *principal) {
	if n, size, ok := pageNo(r); ok {
		list, total, err := s.Svc.AdminUsersPaged(r.Context(), r.URL.Query().Get("q"), (n-1)*size, size)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": total, "page": n, "size": size, "defaults": s.Svc.QuotaDefaultsView()})
		return
	}
	list, err := s.Svc.AdminUsers(r.Context(), page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "defaults": s.Svc.QuotaDefaultsView()})
}

func (s *Server) adminPatchUser(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Disabled *bool `json:"disabled"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if req.Disabled == nil {
		s.fail(w, badRequest("nothing to change"))
		return
	}
	if err := s.Svc.AdminSetDisabled(r.Context(), p.User.ID, r.PathValue("id"), *req.Disabled); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminResetPassword(w http.ResponseWriter, r *http.Request, p *principal) {
	var req passwordReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.AdminResetPassword(r.Context(), p.User.ID, r.PathValue("id"), req.Password); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSetQuota(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Sources     int   `json:"sources"`
		Profiles    int   `json:"profiles"`
		Links       int   `json:"links"`
		MinInterval int   `json:"minInterval"`
		Storage     int64 `json:"storage"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	for _, v := range []int64{int64(req.Sources), int64(req.Profiles), int64(req.Links), int64(req.MinInterval), req.Storage} {
		if v < -1 {
			s.fail(w, badRequest("quota values are -1 (default) or non-negative"))
			return
		}
	}
	q := store.Quota{UserID: r.PathValue("id"), Sources: req.Sources, Profiles: req.Profiles, Links: req.Links, MinInterval: req.MinInterval, Storage: req.Storage}
	if err := s.Svc.AdminSetQuota(r.Context(), p.User.ID, q); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminAudit 返回全部审计记录，并附上账号名。
func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request, _ *principal) {
	var (
		list  []store.AuditEntry
		total int
		err   error
	)
	n, size, paged := pageNo(r)
	if paged {
		n = capPage(n, size, adminAuditCountCap)
		list, total, err = s.Svc.Store.Audit().PagedAll(r.Context(), (n-1)*size, size, adminAuditCountCap)
	} else {
		list, err = s.Svc.Store.Audit().All(r.Context(), page(r))
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	names := map[string]string{}
	out := make([]auditDTO, 0, len(list))
	for _, e := range list {
		d := toAudit(e)
		if e.OwnerID != "" {
			if _, ok := names[e.OwnerID]; !ok {
				if u, err := s.Svc.Store.Users().ByID(r.Context(), e.OwnerID); err == nil {
					names[e.OwnerID] = u.Username
				}
			}
			d.User = names[e.OwnerID]
		}
		out = append(out, d)
	}
	if paged {
		writeJSON(w, http.StatusOK, pagedDTO[auditDTO]{Items: out, Total: min(total, adminAuditCountCap), More: total > adminAuditCountCap, Page: n, Size: size})
		return
	}
	writeJSON(w, http.StatusOK, auditList(out))
}

// pagedDTO 是按页码分页的列表（反馈 #75）：总数与页码，供界面显示「第 n / m 页」。
type pagedDTO[T any] struct {
	Items []T  `json:"items"`
	Total int  `json:"total"`
	More  bool `json:"more,omitempty"` // 总数只数到上限（R14d）：还有更多、但不能翻到
	Page  int  `json:"page"`
	Size  int  `json:"size"`
}

// 审计记录按页浏览时最多数到的条数（R14d）：个人 1000 页（每页 10 条），管理员 1000 页（每页 50 条）。
// 超过的部分显示为「10,000+」，不能翻到；限制计数与 OFFSET 的代价，千万行上不再需要数秒。
const (
	auditCountCap      = 10_000
	adminAuditCountCap = 50_000
)

// capPage 把页码限制在上限以内。
func capPage(n, size, maxCount int) int {
	if last := (maxCount + size - 1) / size; n > last {
		return last
	}
	return n
}

// pageNo 读取 ?pageNo=（从 1 开始）与 limit；没有 pageNo 时返回 ok=false，接口按游标分页（兼容旧客户端）。
func pageNo(r *http.Request) (n, size int, ok bool) {
	v := r.URL.Query().Get("pageNo")
	if v == "" {
		return 0, 0, false
	}
	n, _ = strconv.Atoi(v)
	if n < 1 || n > 100000 {
		n = 1
	}
	return n, pageSize(r), true
}

// adminStatus 是系统状态：版本、内核、目录、主密钥、可选功能是否可用。
func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request, _ *principal) {
	n, err := s.Svc.Store.Users().Count(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	keyID, keys := s.Svc.Store.KeyStatus()
	kernelVersion := ""
	if s.Svc.Kernel != nil {
		kernelVersion = s.Svc.Kernel.Version()
	}
	_, pkErr := s.Svc.RelyingParty()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   version.Version,
		"commit":    version.Commit,
		"built":     version.BuildDate,
		"kernel":    map[string]any{"available": s.Svc.Kernel != nil, "version": kernelVersion},
		"catalog":   map[string]any{"pack": s.Svc.Catalog.Pack.ID, "coreMax": s.Svc.Catalog.Pack.CoreMax, "digest": short(s.Svc.CatalogDigest)},
		"keys":      map[string]any{"current": keyID, "count": keys},
		"users":     n,
		"database":  s.Svc.Store.Dialect(),
		"publicURL": strings.Join(s.Cfg.PublicURLs, ", "),
		"features":  map[string]bool{"passkeys": pkErr == nil, "smtp": s.Svc.Mailer != nil},
		"auditDays": s.Svc.Retention(r.Context()).AuditDays,
	})
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// auditList 加上游标：下一页从本页最后一条之后开始。
func auditList(items []auditDTO) list[auditDTO] {
	out := list[auditDTO]{Items: items}
	if len(items) > 0 {
		out.Next = items[len(items)-1].ID
	}
	return out
}

// —— 数据保留（需求 R43）——

func (s *Server) getRetention(w http.ResponseWriter, r *http.Request, _ *principal) {
	writeJSON(w, http.StatusOK, s.Svc.Retention(r.Context()))
}

// putRetention 设置保留期；某一项为 null 时恢复为环境变量的值。
func (s *Server) putRetention(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		AuditDays   *int `json:"auditDays"`
		RequestDays *int `json:"requestDays"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	v, err := s.Svc.SetRetention(r.Context(), p.User, req.AuditDays, req.RequestDays)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// dataUsage 统计各表的数据量（按需读取：SQLite 上大表的计数需要时间）。
func (s *Server) dataUsage(w http.ResponseWriter, r *http.Request, _ *principal) {
	u, err := s.Svc.DataUsage(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}
