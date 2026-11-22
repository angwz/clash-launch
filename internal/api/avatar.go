package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 头像与默认昵称（需求 R19）。

// setAvatar 换成预置头像；preset 为空时随机换一个。
func (s *Server) setAvatar(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Preset string `json:"preset"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	key, err := s.Svc.SetAvatarPreset(r.Context(), p.User, req.Preset)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"avatar": key})
}

// uploadAvatar 接收图片本身作为请求体（PNG、JPEG、GIF，至多 2 MiB）。
func (s *Server) uploadAvatar(w http.ResponseWriter, r *http.Request, p *principal) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || len(data) > 2<<20 {
		s.fail(w, service.ErrAvatarTooLarge)
		return
	}
	if err != nil {
		s.fail(w, badRequest("read body: %v", err))
		return
	}
	key, err := s.Svc.UploadAvatar(r.Context(), p.User, data)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"avatar": key})
}

// avatarImage 提供上传的头像图片。需要登录：头像与昵称一样，是站内可见的资料，不对站外公开。
// 地址带版本（?v=），图片变化时地址随之变化，因此可以长期缓存。
func (s *Server) avatarImage(w http.ResponseWriter, r *http.Request, _ *principal) {
	mime, data, err := s.Svc.Store.Users().AvatarImage(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, store.ErrNotFound)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	_, _ = w.Write(data) //nolint:gosec // G705：内容是服务器重新编码过的 PNG 或 JPEG，类型由服务器决定
}

// suggestNickname 从预置词表随机给出一个昵称（账号页的「换一个」）；不保存。
func (s *Server) suggestNickname(w http.ResponseWriter, r *http.Request, p *principal) {
	writeJSON(w, http.StatusOK, map[string]string{"nickname": service.RandomNickname(langOf(r, p.User.Lang))})
}

// langOf 是选择预置昵称词表的语言：请求参数 lang、账户设置，最后是浏览器的 Accept-Language。
func langOf(r *http.Request, userLang string) string {
	if l := r.URL.Query().Get("lang"); l != "" {
		return l
	}
	if userLang != "" {
		return userLang
	}
	return strings.TrimSpace(strings.SplitN(r.Header.Get("Accept-Language"), ",", 2)[0])
}
