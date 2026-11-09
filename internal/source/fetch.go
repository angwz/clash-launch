package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// 错误分类：调用方据此决定诊断与界面提示。
var (
	ErrNoEgress   = errors.New("source: 没有配置出站代理，且该源不允许直连")
	ErrEgressDown = errors.New("source: 出站代理不可用")
	ErrTooLarge   = errors.New("source: 响应体超过上限")
	ErrStatus     = errors.New("source: 非 2xx 响应")
)

// DefaultUA 促使机场返回 Clash 格式（第 5.4 节）；不附带本项目的名称、版本与对外地址。
const DefaultUA = "clash.meta"

// Config 是拉取器的配置。
type Config struct {
	Egress       *url.URL      // 出站代理：socks5h://、socks5:// 或 http://；nil 表示没有出口
	Guard        Guard         // SSRF 策略
	MaxBytes     int64         // 响应体上限，默认 10 MiB
	Timeout      time.Duration // 单次请求超时，默认 30 s
	MaxRedirects int           // 默认 5
}

// Request 是一次拉取。
type Request struct {
	URL     string
	UA      string            // 为空时使用 DefaultUA
	Headers map[string]string // 用户为该源设置的请求头
	Direct  bool              // 允许不经出口直连（只有管理员能为源打开）
}

// Response 是成功的拉取结果。
type Response struct {
	Body      []byte
	Hash      string            // 内容的 SHA-256（快照按内容哈希）
	Headers   map[string]string // 只保存订阅输出会用到的响应头
	FetchedAt time.Time
	Via       string // egress | direct
}

// keptHeaders 是保存下来、供订阅输出选用的响应头（第 5.4 节）。
var keptHeaders = []string{"subscription-userinfo", "profile-update-interval", "content-disposition", "profile-web-page-url"}

// Fetcher 执行受控的 HTTP 拉取。可并发使用。
type Fetcher struct {
	cfg    Config
	viaEg  *http.Client
	direct *http.Client
	now    func() time.Time
}

// New 构造拉取器。出站代理的连接失败归类为 ErrEgressDown；出口不可用时不回退直连。
func New(cfg Config) (*Fetcher, error) {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 10 << 20
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRedirects <= 0 {
		cfg.MaxRedirects = 5
	}
	f := &Fetcher{cfg: cfg, now: time.Now}
	// 直连：在 Dialer.Control 中检查实际连接的地址（防 DNS rebinding）。
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: cfg.Guard.control}
	f.direct = f.client(&http.Transport{DialContext: dialer.DialContext, Proxy: nil, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: cfg.Timeout, MaxResponseHeaderBytes: 64 << 10})
	if cfg.Egress != nil {
		tr, err := egressTransport(cfg.Egress, cfg.Timeout)
		if err != nil {
			return nil, err
		}
		f.viaEg = f.client(tr)
	}
	return f, nil
}

func egressTransport(u *url.URL, timeout time.Duration) (*http.Transport, error) {
	base := &http.Transport{ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout, MaxResponseHeaderBytes: 64 << 10}
	switch u.Scheme {
	case "socks5", "socks5h":
		// 两种写法都由代理解析域名（x/net/proxy 把主机名原样交给代理），服务器不做 DNS 查询，
		// 因而不会因解析而泄露真实 IP。
		var auth *proxy.Auth
		if u.User != nil {
			pw, _ := u.User.Password()
			auth = &proxy.Auth{User: u.User.Username(), Password: pw}
		}
		d, err := proxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: 10 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("source: 出站代理配置无效: %w", err)
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("source: SOCKS5 拨号器不支持 context")
		}
		base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := cd.DialContext(ctx, network, addr)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrEgressDown, err)
			}
			return c, nil
		}
	case "http":
		base.Proxy = http.ProxyURL(u)
		d := &net.Dialer{Timeout: 10 * time.Second}
		base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			c, err := d.DialContext(ctx, network, addr) // 只会连接代理本身
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrEgressDown, err)
			}
			return c, nil
		}
	default:
		return nil, fmt.Errorf("source: 出站代理只支持 socks5h://、socks5:// 与 http://（得到 %q）", u.Scheme)
	}
	return base, nil
}

func (f *Fetcher) client(tr *http.Transport) *http.Client {
	return &http.Client{
		Transport: tr,
		Timeout:   f.cfg.Timeout,
		// 每次重定向都重新检查目标；超过上限即停止。
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= f.cfg.MaxRedirects {
				return fmt.Errorf("source: 重定向超过 %d 次", f.cfg.MaxRedirects)
			}
			req.Header.Del("Referer")
			return f.cfg.Guard.CheckURL(req.URL)
		},
	}
}

// EgressConfigured 报告是否配置了出站代理。
func (f *Fetcher) EgressConfigured() bool { return f.viaEg != nil }

// Fetch 拉取一个地址。出口不可用时返回 ErrEgressDown，绝不回退直连。
func (f *Fetcher) Fetch(ctx context.Context, r Request) (*Response, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, fmt.Errorf("%w：地址无效", ErrBlocked)
	}
	if err := f.cfg.Guard.CheckURL(u); err != nil {
		return nil, err
	}
	c, via := f.viaEg, "egress"
	if r.Direct {
		c, via = f.direct, "direct"
	}
	if c == nil {
		return nil, ErrNoEgress
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	// 只发送必要的请求头：不带 Referer、Cookie、X-Forwarded-*（第 14.6 节第 3 条）。
	for k, v := range r.Headers {
		switch strings.ToLower(k) {
		case "host", "cookie", "referer", "x-forwarded-for", "x-forwarded-proto", "x-forwarded-host", "forwarded", "x-real-ip":
			continue
		}
		req.Header.Set(k, v)
	}
	ua := r.UA
	if ua == "" {
		ua = DefaultUA
	}
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, classify(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%w：HTTP %d", ErrStatus, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.cfg.MaxBytes+1))
	if err != nil {
		return nil, classify(err)
	}
	if int64(len(body)) > f.cfg.MaxBytes {
		return nil, fmt.Errorf("%w（%d 字节）", ErrTooLarge, f.cfg.MaxBytes)
	}
	h := sha256.Sum256(body)
	out := &Response{Body: body, Hash: hex.EncodeToString(h[:]), Headers: map[string]string{}, FetchedAt: f.now().UTC(), Via: via}
	for _, k := range keptHeaders {
		if v := resp.Header.Get(k); v != "" {
			out.Headers[k] = v
		}
	}
	return out, nil
}

// classify 保留错误类别，同时去掉可能含有 URL（及其中凭据）的细节。
func classify(err error) error {
	switch {
	case errors.Is(err, ErrBlocked):
		return fmt.Errorf("%w", ErrBlocked)
	case errors.Is(err, ErrEgressDown):
		return ErrEgressDown
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("source: 超时")
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("source: 请求失败：%s", redactURLError(ue))
	}
	return fmt.Errorf("source: 请求失败")
}

func redactURLError(ue *url.Error) string {
	inner := ue.Err
	if errors.Is(inner, ErrBlocked) {
		return "blocked"
	}
	var ne net.Error
	if errors.As(inner, &ne) && ne.Timeout() {
		return "timeout"
	}
	msg := inner.Error()
	if i := strings.Index(msg, "://"); i >= 0 {
		return "connection error"
	}
	return msg
}
