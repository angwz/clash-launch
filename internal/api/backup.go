package api

import (
	"net/http"
	"net/url"
	"time"
)

// 个人备份与还原（需求 R21）：下载加密的备份文件、还原前比较、整体覆盖、撤销最近一次还原。
// 规则在 internal/service/backup.go。都需要当前密码：备份里有全部节点凭据，还原会覆盖全部个人数据。

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Password   string `json:"password"`
		Passphrase string `json:"passphrase"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	file, err := s.Svc.ExportBackup(r.Context(), p.User, req.Password, req.Passphrase)
	if err != nil {
		s.fail(w, err)
		return
	}
	name := "clash-launch-" + p.User.Username + "-" + time.Now().UTC().Format("20060102") + ".backup"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(file) //nolint:gosec // 以 application/octet-stream 附件下载并带 nosniff，不会被当作 HTML 解释
}

type restoreReq struct {
	File       string `json:"file"` // 备份文件的原文
	Passphrase string `json:"passphrase"`
	Password   string `json:"password"`
}

func (s *Server) previewRestore(w http.ResponseWriter, r *http.Request, p *principal) {
	var req restoreReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	cur, in, err := s.Svc.RestorePreview(r.Context(), p.User, []byte(req.File), req.Passphrase)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": cur, "backup": in})
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request, p *principal) {
	var req restoreReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	rep, err := s.Svc.Restore(r.Context(), p.User, []byte(req.File), req.Passphrase, req.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) undoStatus(w http.ResponseWriter, r *http.Request, p *principal) {
	x, err := s.Svc.UndoStatus(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	if x == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "createdAt": x.CreatedAt, "expiresAt": x.ExpiresAt})
}

func (s *Server) undoRestore(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	rep, err := s.Svc.UndoRestore(r.Context(), p.User, req.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// —— 注销账户（需求 R20）——

// requestDeletion 发起注销：需要当前密码（启用两步验证时还要验证码）。所有会话立即失效，本次请求的 Cookie 一并清除。
func (s *Server) requestDeletion(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	at, err := s.Svc.RequestDeletion(r.Context(), p.User, req.Password, req.Code)
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(r), Value: "", Path: "/", HttpOnly: true, Secure: s.secure(r), SameSite: http.SameSiteLaxMode, MaxAge: -1}) //nolint:gosec // 清除 Cookie；Secure 与设置时一致
	writeJSON(w, http.StatusOK, map[string]any{"at": at})
}

// adminDeleteUser 由管理员立即注销一个账号（没有冷静期），记入审计。
func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.AdminDeleteUser(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
