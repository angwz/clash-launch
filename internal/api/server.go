// Package api 是 HTTP 层：管理接口（/api/v1/）、订阅端点（/s/、/p/）、健康检查与静态前端。
// 契约以 api/openapi.yaml 为准（第 12 节），TestContract 检查路由与契约一致。
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
)

// Config 是 HTTP 层的配置。
type Config struct {
	PublicURLs     []string       // CLASH_LAUNCH_PUBLIC_URL（可多个，第一个为主地址）；决定生成的链接与 Cookie 的 Secure，见 publicurl.go
	TrustedProxies []netip.Prefix // 只从这些网段读取 X-Forwarded-For/-Proto
	SessionIdle    time.Duration  // 默认 7 天
	SessionMax     time.Duration  // 默认 30 天
	MaxBody        int64          // 默认 1 MiB（上传另行放宽）
	SubRate        int            // 每个 IP+令牌每分钟的订阅请求上限，默认 30
	Static         fs.FS          // 前端构建产物（web/dist）；nil 时只提供 API
	Guard          source.Guard   // 添加订阅时的 URL 检查（与拉取层同一策略）
	MetricsToken   string         // CLASH_LAUNCH_METRICS_TOKEN：设置后 /metrics 凭令牌访问（需求 R26）
}

// Server 是 HTTP 服务。
type Server struct {
	Svc     *service.Service
	Cfg     Config
	Log     *slog.Logger
	setupMu sync.Mutex // 串行化首次注册：两个并发请求不能都成为管理员
	limit   *limiter
	// 按用户限制预览渲染与内核检查（反馈 #71）：渲染与 mihomo -t 都占用服务器 CPU，开放注册后需要上限
	renderLimit  *limiter
	checkLimit   *limiter
	traceLimit   *limiter     // 分流追踪与体检：每个用户每分钟 60 次（体检计 1 次，routing 第 7 节）
	missLimit    *limiter     // /u/ 上猜错短码的次数（按 IP），超过后限速一段时间（反馈 #74）
	static       http.Handler // 前端构建产物；短链接查不到时交给它（反馈 #75）
	regLimit     *limiter     // 注册：每个 IP 每小时 10 次
	recoverLimit *limiter     // 找回（取问题与提交）：每个 IP 每小时 30 次
	passkeyLimit *limiter     // 通行密钥登录的开始：每个 IP 每小时 120 次
	mux          *http.ServeMux
}

// New 构造服务并注册路由。
func New(svc *service.Service, cfg Config, log *slog.Logger) *Server {
	if cfg.SessionIdle <= 0 {
		cfg.SessionIdle = 7 * 24 * time.Hour
	}
	if cfg.SessionMax <= 0 {
		cfg.SessionMax = 30 * 24 * time.Hour
	}
	if cfg.MaxBody <= 0 {
		cfg.MaxBody = 1 << 20
	}
	if cfg.SubRate <= 0 {
		cfg.SubRate = 30
	}
	s := &Server{Svc: svc, Cfg: cfg, Log: log, limit: newLimiter(cfg.SubRate, time.Minute), regLimit: newLimiter(10, time.Hour), recoverLimit: newLimiter(30, time.Hour), passkeyLimit: newLimiter(120, time.Hour), renderLimit: newLimiter(renderPerMinute, time.Minute), checkLimit: newLimiter(checksPerMinute, time.Minute), traceLimit: newLimiter(tracesPerMinute, time.Minute), missLimit: newLimiter(30, time.Hour), mux: http.NewServeMux()}
	for _, r := range s.routes() {
		s.mux.HandleFunc(r.Method+" "+r.Pattern, r.Handler)
	}
	if cfg.Static != nil {
		s.static = staticHandler(cfg.Static)
		s.mux.Handle("GET /", s.static)
	}
	// 上线、下线通知联系人（R22）
	s.Svc.Events.OnPresence(svc.AnnouncePresence)
	return s
}

// Handler 返回带全部中间件的处理器。
func (s *Server) Handler() http.Handler {
	return s.recoverer(s.recordMetrics(s.secureHeaders(s.limitBody(s.mux))))
}

// HTTPServer 返回设置了超时与请求头上限的 http.Server（第 14.2 节：服务自身防慢速连接）。
func (s *Server) HTTPServer(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if s.secure(r) {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := s.Cfg.MaxBody
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/account/restore"):
			limit = 64 << 20 // 还原要带上整个备份文件（需求 R21）
		case strings.HasPrefix(r.URL.Path, "/api/v1/sources"), strings.HasSuffix(r.URL.Path, "/import"), r.URL.Path == "/api/v1/import/preview":
			limit = 10 << 20 // 节点原文与导入可以较大（第 14.5 节：上传有体积上限）
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.Log.Error("panic", "path", redactPath(r.URL.Path), "error", fmt.Sprint(v))
				problem(w, http.StatusInternalServerError, "internal", "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// redactPath 去掉路径中的令牌：日志只记前 6 个字符（第 13.2 节）。
func redactPath(p string) string {
	for _, pre := range []string{"/s/", "/p/", "/r/"} {
		if strings.HasPrefix(p, pre) {
			rest := p[len(pre):]
			tok, tail, _ := strings.Cut(rest, "/")
			if tail != "" {
				tail = "/" + tail
			}
			return pre + auth.LogToken(tok) + tail
		}
	}
	return p
}

// clientIP 返回客户端地址：只有直连地址属于受信任代理时才读取 X-Forwarded-For（第 14.2 节）。
func (s *Server) clientIP(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	direct, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	direct = direct.Unmap()
	if !s.trusted(direct) {
		return direct
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- { // 从右往左取第一个不受信任的地址
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			break
		}
		a = a.Unmap()
		if !s.trusted(a) {
			return a
		}
	}
	return direct
}

func (s *Server) trusted(a netip.Addr) bool {
	for _, p := range s.Cfg.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// ipPrefix 返回地址所在的 /24 或 /48，用于会话列表展示（不保存完整地址）。
func ipPrefix(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	bits := 24
	if a.Is6() {
		bits = 48
	}
	p, _ := a.Prefix(bits)
	return p.String()
}

// —— RFC 9457 问题详情 ——

type problemBody struct {
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Status      int               `json:"status"`
	Detail      string            `json:"detail,omitempty"`
	Diagnostics []diag.Diagnostic `json:"diagnostics,omitempty"`
}

func problem(w http.ResponseWriter, status int, code, detail string, ds ...diag.Diagnostic) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problemBody{Type: code, Title: http.StatusText(status), Status: status, Detail: detail, Diagnostics: ds})
}

// fail 把错误映射为问题详情：不存在与不属于当前用户一律 404（第 14.3 节）。
func (s *Server) fail(w http.ResponseWriter, err error) {
	var wait *service.WaitError
	var renameWait *service.RenameWaitError
	var denied *service.InviteDeniedError
	switch {
	case errors.Is(err, store.ErrNotFound):
		problem(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, store.ErrConflict):
		problem(w, http.StatusPreconditionFailed, "conflict", "the resource was modified elsewhere")
	case errors.Is(err, store.ErrExists):
		problem(w, http.StatusConflict, "exists", "a resource with the same unique name already exists")
	case errors.Is(err, auth.ErrWeakPassword):
		problem(w, http.StatusUnprocessableEntity, "auth.weak_password", err.Error())
	case errors.Is(err, errBadRequest):
		problem(w, http.StatusBadRequest, "bad_request", strings.TrimPrefix(err.Error(), errBadRequest.Error()+": "))
	case errors.Is(err, service.ErrShortCode):
		problem(w, http.StatusUnprocessableEntity, "short.code", err.Error())
	case errors.Is(err, service.ErrRecipient):
		problem(w, http.StatusNotFound, "message.recipient", "the recipient does not exist or does not accept messages")
	case errors.Is(err, service.ErrMessage):
		problem(w, http.StatusUnprocessableEntity, "message.invalid", err.Error())
	case errors.Is(err, service.ErrTimezone):
		problem(w, http.StatusUnprocessableEntity, "account.timezone", err.Error())
	case errors.Is(err, service.ErrTraceDisabled):
		problem(w, http.StatusNotFound, "feature_disabled", "routing trace is disabled on this server")
	case errors.Is(err, service.ErrNoRules):
		problem(w, http.StatusBadRequest, "routing.no_rules", "this config has no routing rules to start from")
	case errors.Is(err, service.ErrBadDraft):
		problem(w, http.StatusBadRequest, "routing.draft", err.Error())
	case errors.Is(err, service.ErrInvalidHost):
		problem(w, http.StatusBadRequest, "invalid_host", err.Error())
	case errors.Is(err, source.ErrHostNotAllowed):
		problem(w, http.StatusUnprocessableEntity, "rule.host_not_allowed", err.Error())
	case errors.Is(err, service.ErrClientFetch):
		problem(w, http.StatusConflict, "source.client_fetch", "subscriptions on this account are fetched by the client, not the server")
	case errors.Is(err, service.ErrNodeMode):
		problem(w, http.StatusBadRequest, "node_mode.invalid", err.Error())
	case errors.Is(err, service.ErrLinkLimits):
		problem(w, http.StatusUnprocessableEntity, "link.limits", err.Error())
	case errors.Is(err, service.ErrMFAPolicy):
		problem(w, http.StatusBadRequest, "mfa.policy", err.Error())
	case errors.Is(err, service.ErrMFAPolicySelf):
		problem(w, http.StatusConflict, "mfa.policy_self", "enable two-step verification for yourself before requiring it of others")
	case errors.Is(err, service.ErrMFARequired):
		problem(w, http.StatusConflict, "mfa.required_by_policy", "two-step verification is required on this site and cannot be turned off")
	case errors.Is(err, service.ErrTokenInvalid):
		problem(w, http.StatusUnprocessableEntity, "token.invalid", err.Error())
	case errors.Is(err, service.ErrTokenLimit):
		problem(w, http.StatusConflict, "token.limit", "too many access tokens; revoke one first")
	case errors.Is(err, service.ErrTermsOutdated):
		problem(w, http.StatusConflict, "terms.outdated", "the terms of use have changed; read and accept the latest version")
	case errors.Is(err, service.ErrTermsText):
		problem(w, http.StatusUnprocessableEntity, "terms.invalid", err.Error())
	case errors.Is(err, service.ErrRegistrationClosed):
		problem(w, http.StatusForbidden, "register.closed", "registration is closed; a valid invite is required")
	case errors.Is(err, service.ErrUsername):
		problem(w, http.StatusUnprocessableEntity, "register.username", err.Error())
	case errors.Is(err, auth.ErrPoW):
		problem(w, http.StatusBadRequest, "pow.invalid", "proof of work is invalid or expired")
	case errors.Is(err, auth.ErrQuestions):
		problem(w, http.StatusUnprocessableEntity, "account.questions", err.Error())
	case errors.Is(err, service.ErrRecoveryFailed):
		problem(w, http.StatusForbidden, "recovery.failed", "recovery failed")
	case errors.Is(err, service.ErrWrongPassword):
		problem(w, http.StatusForbidden, "account.wrong_password", "current password is incorrect")
	case errors.Is(err, service.ErrMFA):
		problem(w, http.StatusUnauthorized, "mfa.invalid", "verification code is invalid or expired")
	case errors.Is(err, service.ErrMFAState):
		problem(w, http.StatusConflict, "mfa.state", "two-step verification is not in a state that allows this")
	case errors.Is(err, service.ErrPasskey):
		problem(w, http.StatusUnauthorized, "passkey.invalid", "passkey verification failed")
	case errors.Is(err, service.ErrPasskeyUnavailable):
		problem(w, http.StatusConflict, "passkey.unavailable", "passkeys need CLASH_LAUNCH_PUBLIC_URL to be set")
	case errors.Is(err, service.ErrQuota):
		problem(w, http.StatusForbidden, "quota.exceeded", err.Error())
	case errors.Is(err, source.ErrSMTP):
		problem(w, http.StatusUnprocessableEntity, "smtp.failed", err.Error())
	case errors.Is(err, service.ErrRetention):
		problem(w, http.StatusUnprocessableEntity, "admin.retention", err.Error())
	case errors.Is(err, service.ErrLastAdmin):
		problem(w, http.StatusConflict, "account.last_admin", "the last administrator cannot be deleted")
	case errors.Is(err, service.ErrDeleted):
		problem(w, http.StatusConflict, "account.deleted", "this account has already been deleted")
	case errors.Is(err, service.ErrBackupPassphrase):
		problem(w, http.StatusUnprocessableEntity, "backup.passphrase", "the backup passphrase is wrong or the file is damaged")
	case errors.Is(err, service.ErrBackupFormat):
		problem(w, http.StatusUnprocessableEntity, "backup.format", err.Error())
	case errors.Is(err, service.ErrBackupWeak):
		problem(w, http.StatusUnprocessableEntity, "backup.weak", "the backup passphrase must be at least 10 characters")
	case errors.Is(err, service.ErrExportNeedsLink):
		problem(w, http.StatusConflict, "export.needs_link", "this config has sources handed to the client; create a subscription link before exporting with proxies")
	case errors.Is(err, service.ErrProtected):
		problem(w, http.StatusConflict, "profile.protected", "this config is protected from deletion; turn off protection first")
	case errors.Is(err, source.ErrPush):
		problem(w, http.StatusUnprocessableEntity, "push.failed", err.Error())
	case errors.Is(err, service.ErrChannelsUnavailable):
		problem(w, http.StatusConflict, "channel.unavailable", "notification channels are not available on this server")
	case errors.Is(err, service.ErrChannelLimit):
		problem(w, http.StatusConflict, "channel.limit", err.Error())
	case errors.Is(err, service.ErrChannelName):
		problem(w, http.StatusUnprocessableEntity, "channel.name", err.Error())
	case errors.Is(err, service.ErrSMTPUnavailable):
		problem(w, http.StatusConflict, "smtp.unavailable", "email alerts are not available on this server")
	case errors.Is(err, service.ErrSelf):
		problem(w, http.StatusConflict, "admin.self", "administrators cannot do this to their own account")
	case errors.Is(err, service.ErrInvalidLogin):
		problem(w, http.StatusUnauthorized, "auth.invalid", "invalid username or password")
	case errors.As(err, &renameWait):
		problem(w, http.StatusConflict, "account.rename_wait", "the username can be changed again at "+renameWait.Until.UTC().Format(time.RFC3339))
	case errors.Is(err, service.ErrNameReserved):
		problem(w, http.StatusConflict, "account.name_reserved", err.Error())
	case errors.Is(err, service.ErrNickname):
		problem(w, http.StatusUnprocessableEntity, "account.nickname", err.Error())
	case errors.As(err, &denied):
		problem(w, http.StatusForbidden, "invite."+denied.Reason, err.Error())
	case errors.Is(err, service.ErrInviteInput):
		problem(w, http.StatusUnprocessableEntity, "invite.input", err.Error())
	case errors.As(err, &wait):
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Wait.Seconds())+1))
		problem(w, http.StatusTooManyRequests, "auth.locked", "too many failed attempts; try again later")
	default:
		s.Log.Error("request failed", "error", err)
		problem(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

var errBadRequest = errors.New("bad request")

func badRequest(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errBadRequest, fmt.Sprintf(format, a...))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return badRequest("request body too large")
		}
		return badRequest("invalid JSON: %v", err)
	}
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		return badRequest("request body too large")
	}
	return nil
}

// 每个用户每分钟的预览渲染与内核检查次数上限（管理员不限）。编辑器在输入停顿约 0.5 秒后预览、
// 自动保存后检查，正常编辑远低于这两个值。
const (
	renderPerMinute = 240
	checksPerMinute = 30
	tracesPerMinute = 60
)

// renderAllowed 检查用户的预览与内核检查配额；超出时写出 429 并返回 false。
func (s *Server) renderAllowed(w http.ResponseWriter, p *principal, check bool) bool {
	if p.User.Role == "admin" {
		return true
	}
	now := time.Now()
	ok, wait := s.renderLimit.allow(p.User.ID, now)
	code := "rate_limited"
	if ok && check {
		ok, wait = s.checkLimit.allow(p.User.ID, now)
		code = "rate_limited.check"
	}
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		problem(w, http.StatusTooManyRequests, code, "too many previews or kernel checks; try again shortly")
	}
	return ok
}

// —— 订阅端点限流（第 13.3 节）——

type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	hits   map[string][]time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, hits: map[string][]time.Time{}}
}

// over 报告 key 在当前窗口内是否已经用完次数（只查看，不计数）。
func (l *limiter) over(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	n := 0
	for _, t := range l.hits[key] {
		if !t.Before(cut) {
			n++
		}
	}
	return n >= l.n
}

// allow 报告 key 在当前窗口内是否还能请求；否则返回需要等待的秒数。
func (l *limiter) allow(key string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-l.window)
	hs := l.hits[key]
	i := 0
	for i < len(hs) && hs[i].Before(cut) {
		i++
	}
	hs = hs[i:]
	if len(hs) >= l.n {
		l.hits[key] = hs
		return false, int(hs[0].Add(l.window).Sub(now).Seconds()) + 1
	}
	l.hits[key] = append(hs, now)
	if len(l.hits) > 100000 { // 粗略的内存上限：清理过期键
		for k, v := range l.hits {
			if len(v) == 0 || v[len(v)-1].Before(cut) {
				delete(l.hits, k)
			}
		}
	}
	return true, 0
}

// staticHandler 提供前端构建产物；未知的页面路径回退到 index.html（单页应用）。
//   - assets/ 下带内容哈希的文件长期缓存；缺失时返回 404，不回退（否则脚本请求会拿到 HTML）；
//   - index.html 每次重新验证，发布新版本后浏览器立即使用新资源；
//   - /api/、/s/、/p/ 下未注册的路径返回 404，不回退到页面；点文件（.gitkeep 等）不提供。
func staticHandler(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		for _, seg := range strings.Split(p, "/") {
			if strings.HasPrefix(seg, ".") {
				problem(w, http.StatusNotFound, "not_found", "resource not found")
				return
			}
		}
		if strings.HasPrefix(p, "api/") || strings.HasPrefix(p, "s/") || strings.HasPrefix(p, "p/") {
			problem(w, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		if strings.HasPrefix(p, "assets/") {
			if _, err := fs.Stat(fsys, p); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			files.ServeHTTP(w, r)
			return
		}
		if p != "" && p != "index.html" {
			if st, err := fs.Stat(fsys, p); err == nil && !st.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		// 页面路径：一律返回 index.html，由前端路由处理
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}
