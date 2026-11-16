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
// 带 ?download=1 时作为文件下载（反馈 #72「导出」）：内容相同，附带文件名，总是返回完整内容，
// 请求记录中单独记为 export。
func (s *Server) subscribe(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w}
	var link *store.Link
	kind := kindOf(r, "config")
	defer func() { s.recordSub(r, link, sw, kind, "") }()
	tok := r.PathValue("token")
	if s.rateLimited(sw, r, tok) {
		return
	}
	l, err := s.linkByToken(r, tok)
	if err != nil {
		http.NotFound(sw, r)
		return
	}
	link = l
	s.serveConfig(sw, r, l, tok, kind == "export")
}

// short 提供旧式短链接 /u/<短码>（反馈 #74，继续有效）。
func (s *Server) short(w http.ResponseWriter, r *http.Request) {
	s.serveShort(w, r, r.PathValue("code"), true)
}

// rootShort 提供站点根路径下的短链接 /<短码>（反馈 #75）。保留的路径（本站页面、静态文件、常见软件路径）
// 与带点的文件名直接交给前端；查不到短码时也交给前端，由前端显示「页面不存在」。
func (s *Server) rootShort(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if strings.Contains(code, ".") || service.IsReservedCode(code) {
		s.fallback(w, r)
		return
	}
	s.serveShort(w, r, code, false)
}

// fallback 交给前端构建产物（单页应用）；没有前端时返回 404。
func (s *Server) fallback(w http.ResponseWriter, r *http.Request) {
	if s.static != nil {
		s.static.ServeHTTP(w, r)
		return
	}
	problem(w, http.StatusNotFound, "not_found", "resource not found")
}

// serveShort 直接返回短链接所指配置链接的内容（不重定向，不暴露原链接），请求同时计入原链接与短链接。
// 不存在、已停用、已过期或已收回授权的短码：/u/ 返回 404 并记入试探来源；根路径交给前端。
// 同一 IP 一小时内猜错 30 次后限速。
func (s *Server) serveShort(w http.ResponseWriter, r *http.Request, code string, legacy bool) {
	sw := &statusWriter{ResponseWriter: w}
	ip := s.clientIP(r).String()
	now := time.Now()
	if s.missLimit.over(ip, now) {
		sw.Header().Set("Retry-After", "600")
		http.Error(sw, "too many requests", http.StatusTooManyRequests)
		s.recordSub(r, nil, sw, "config", "")
		return
	}
	code = strings.ToLower(code)
	sl, err := s.Svc.Store.ShortLinks().ByCode(r.Context(), code)
	var l *store.Link
	if err == nil && sl.Active(now) {
		l, err = s.Svc.Store.Links().Get(r.Context(), sl.OwnerID, sl.LinkID)
	}
	if err != nil || l == nil {
		s.missLimit.allow(ip, now) // 只记录猜错的次数
		if legacy {
			http.NotFound(sw, r)
			s.recordSub(r, nil, sw, "config", "")
			return
		}
		s.fallback(w, r)
		return
	}
	kind := kindOf(r, "config")
	defer func() { s.recordSub(r, l, sw, kind, sl.ID) }()
	if s.rateLimited(sw, r, "short:"+code) {
		return
	}
	s.serveConfig(sw, r, l, l.Token, kind == "export")
}

// kindOf 返回请求记录中的类型：带 ?download=1 时为 export。
func kindOf(r *http.Request, def string) string {
	if r.URL.Query().Get("download") != "" {
		return "export"
	}
	return def
}

// serveConfig 写出一条配置链接的最近一次成功渲染结果。tok 是该链接的令牌，用于替换托管节点文件地址中的占位符。
func (s *Server) serveConfig(w http.ResponseWriter, r *http.Request, l *store.Link, tok string, download bool) {
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
	if p := s.publicFor(r); p != "" {
		h.Set("profile-web-page-url", p+"/profiles/"+url.PathEscape(l.ProfileID))
	}
	h.Set("Content-Type", "text/yaml; charset=utf-8")
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", tag)
	if download {
		h.Set("Content-Disposition", attachment(s.exportName(r, l)))
		_, _ = w.Write(body) //nolint:gosec // 同下
		return
	}
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body) //nolint:gosec // 以 text/yaml 与 nosniff 返回，不会被当作 HTML 解释
}

// hosted 提供「交给客户端」模式下由本服务托管的节点文件（/p/<令牌>/<源>）。
func (s *Server) hosted(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w}
	w = sw
	var link *store.Link
	defer func() { s.recordSub(r, link, sw, "provider", "") }()
	tok := r.PathValue("token")
	if s.rateLimited(w, r, tok) {
		return
	}
	l, err := s.linkByToken(r, tok)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	link = l
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

// statusWriter 记下响应的状态码与字节数，供订阅请求记录使用（反馈 #67）。
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// recordSub 记录一次订阅请求。l 为 nil 表示令牌无效或在查找令牌前被限流，只进入来源汇总。
// 不记录令牌本身。
func (s *Server) recordSub(r *http.Request, l *store.Link, w *statusWriter, kind, shortID string) {
	addr := s.clientIP(r)
	g := s.Svc.Geo.Lookup(addr) // 归属地（反馈 #74）；没有地理数据时为空
	q := store.SubRequest{At: time.Now(), IP: addr.String(), UA: r.UserAgent(), Status: w.status, Bytes: w.bytes, Kind: kind, Country: g.Country, ASN: g.ASN, Org: g.Org, ShortID: shortID}
	if q.Status == 0 {
		q.Status = http.StatusOK
	}
	if l != nil {
		q.LinkID, q.OwnerID = l.ID, l.OwnerID
	}
	s.Svc.Stats().Request(q)
}

// exportName 是导出文件的名称：配置名（去掉文件名中不能用的字符）加 .yaml；取不到名称时为 clash-launch.yaml。
func (s *Server) exportName(r *http.Request, l *store.Link) string {
	name := ""
	if p, err := s.Svc.Store.Profiles().Get(r.Context(), l.OwnerID, l.ProfileID); err == nil {
		name = p.Name
	}
	name = strings.Map(func(c rune) rune {
		if c < 0x20 || c == 0x7F || strings.ContainsRune(`\/:*?"<>|`, c) {
			return -1
		}
		return c
	}, strings.TrimSpace(name))
	if name == "" {
		name = "clash-launch"
	}
	return name + ".yaml"
}

// attachment 生成 Content-Disposition：ASCII 回退名加 RFC 5987 的 UTF-8 名称（中文配置名也能正确保存）。
func attachment(name string) string {
	ascii := strings.Map(func(c rune) rune {
		if c < 0x20 || c > 0x7E || c == '"' || c == 0x5C { // 0x5C 是反斜杠
			return '_'
		}
		return c
	}, name)
	return `attachment; filename="` + ascii + `"; filename*=UTF-8''` + url.PathEscape(name)
}

// importClients 是「导入到客户端」菜单中的客户端（与 web/src/lib/clients.ts 一致），用于记录名称。
var importClients = map[string]string{"verge": "Clash Verge Rev", "party": "Mihomo Party", "flclash": "FlClash", "cmfa": "Clash Meta for Android"}

// recordImport 记录一次「一键导入到客户端」（反馈 #76）：浏览器只是打开客户端的导入链接，本站收不到请求，
// 所以由前端在点击时上报，记为 import 类型的一次请求（客户端随后真正拉取配置时另记一次）。
// 可以是自己的配置链接（link）、自己的短链接（short），或别人授权给自己的短链接（code，计入授权者的统计）。
func (s *Server) recordImport(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Link   string `json:"link"`
		Short  string `json:"short"`
		Code   string `json:"code"`
		Client string `json:"client"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	name, ok := importClients[req.Client]
	if !ok {
		s.fail(w, badRequest("unknown client"))
		return
	}
	ctx := r.Context()
	var l *store.Link
	var shortID string
	var err error
	switch {
	case req.Link != "":
		l, err = s.Svc.Store.Links().Get(ctx, p.User.ID, req.Link)
	case req.Short != "":
		var sl *store.ShortLink
		if sl, err = s.Svc.Store.ShortLinks().Get(ctx, p.User.ID, req.Short); err == nil {
			shortID = sl.ID
			l, err = s.Svc.Store.Links().Get(ctx, sl.OwnerID, sl.LinkID)
		}
	case req.Code != "":
		var sl *store.ShortLink
		if sl, err = s.Svc.Store.ShortLinks().ByCode(ctx, req.Code); err == nil {
			if _, err = s.Svc.Store.Grants().ActiveFor(ctx, p.User.ID, "short", sl.ID); err == nil {
				shortID = sl.ID
				l, err = s.Svc.Store.Links().Get(ctx, sl.OwnerID, sl.LinkID)
			}
		}
	default:
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, store.ErrNotFound)
		return
	}
	addr := s.clientIP(r)
	g := s.Svc.Geo.Lookup(addr)
	s.Svc.Stats().Request(store.SubRequest{At: time.Now(), IP: addr.String(), UA: r.UserAgent(), Client: name, Status: http.StatusOK, Kind: "import",
		LinkID: l.ID, OwnerID: l.OwnerID, ShortID: shortID, Country: g.Country, ASN: g.ASN, Org: g.Org})
	w.WriteHeader(http.StatusNoContent)
}
