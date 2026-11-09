package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
)

var catalogFS = catalogdata.FS

// linkByToken 按令牌查找链接：先按 SHA-256 查找，再以常数时间比较原文（第 13.2 节）。
func (s *Server) linkByToken(r *http.Request, tok string) (*store.Link, error) {
	if len(tok) < 20 || len(tok) > 100 {
		return nil, store.ErrNotFound
	}
	l, err := s.Svc.Store.Links().ByTokenHash(r.Context(), auth.HashToken(tok))
	if err != nil {
		return nil, err
	}
	if !auth.Equal(l.Token, tok) {
		return nil, store.ErrNotFound
	}
	return l, nil
}

// rateLimited 对订阅端点按 IP 与令牌限流；超限时返回 429 与 Retry-After（第 13.3 节）。
func (s *Server) rateLimited(w http.ResponseWriter, r *http.Request, tok string) bool {
	key := s.clientIP(r).String() + "|" + auth.HashToken(tok)[:16]
	ok, wait := s.limit.allow(key, time.Now())
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return true
	}
	return false
}

// subscribe 提供方案最近一次成功的渲染结果。只读缓存，不触发拉取或渲染（第 13.3 节）。
func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	if s.rateLimited(w, r, tok) {
		return
	}
	l, err := s.linkByToken(r, tok)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = s.Svc.Store.Links().Accessed(r.Context(), l.ID, time.Now(), r.UserAgent())
	rd, err := s.Svc.Store.Renders().Get(r.Context(), l.OwnerID, l.ProfileID)
	if err != nil || len(rd.GoodYAML) == 0 {
		// 尚无成功结果：返回 503 与 YAML 注释，而不是空配置，以免客户端用空配置覆盖旧配置。
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("# Clash, Launch!: this profile has no successful render yet; the client should keep its current configuration.\n"))
		return
	}
	body := bytes.ReplaceAll(rd.GoodYAML, []byte(service.LinkTokenPlaceholder), []byte(tok))
	sum := sha256.Sum256(body)
	tag := `"` + hex.EncodeToString(sum[:16]) + `"`
	h := w.Header()
	for k, v := range rd.GoodHeaders {
		h.Set(k, v)
	}
	if s.Cfg.PublicURL != "" {
		h.Set("profile-web-page-url", strings.TrimSuffix(s.Cfg.PublicURL, "/")+"/profiles/"+url.PathEscape(l.ProfileID))
	}
	h.Set("Content-Type", "text/yaml; charset=utf-8")
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", tag)
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body) //nolint:gosec // 以 text/yaml 与 nosniff 返回，不会被当作 HTML 解释
}

// hosted 提供「交给客户端」模式下由本服务托管的节点文件（/p/<令牌>/<源>）。
func (s *Server) hosted(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	if s.rateLimited(w, r, tok) {
		return
	}
	l, err := s.linkByToken(r, tok)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, err := s.Svc.HostedProvider(r.Context(), l, r.PathValue("source"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, service.ErrNotHosted) {
			http.NotFound(w, r)
			return
		}
		s.Log.Error("hosted provider", "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	_, _ = w.Write(body) //nolint:gosec // 同上
}

func guardCheck(s *Server, u *url.URL) error {
	return s.Cfg.Guard.CheckURL(u)
}

func sourceReason(err error) string { return source.Reason(err) }

func parseAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(strings.Trim(s, "[]"))
	return a.Unmap(), err
}
