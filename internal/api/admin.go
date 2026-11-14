package api

import (
	"encoding/json"
	"net/http"
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
	list, err := s.Svc.Store.Audit().All(r.Context(), page(r))
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
	writeJSON(w, http.StatusOK, auditList(out))
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
		"publicURL": s.Cfg.PublicURL,
		"features":  map[string]bool{"passkeys": pkErr == nil, "smtp": s.Svc.Mailer != nil},
		"auditDays": int(s.Svc.AuditRetention.Hours() / 24),
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
