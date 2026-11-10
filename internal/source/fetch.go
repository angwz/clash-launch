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
)

// 错误分类：调用方据此决定诊断与界面提示。
var (
	ErrTooLarge = errors.New("source: 响应体超过上限")
	ErrStatus   = errors.New("source: 非 2xx 响应")
)

// DefaultUA 促使机场返回 Clash 格式（第 5.4 节）；不附带本项目的名称、版本与对外地址。
const DefaultUA = "clash.meta"

// Config 是拉取器的配置。
type Config struct {
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
}

// Response 是成功的拉取结果。
type Response struct {
	Body      []byte
	Hash      string            // 内容的 SHA-256（快照按内容哈希）
	Headers   map[string]string // 只保存订阅输出会用到的响应头
	FetchedAt time.Time
}

// keptHeaders 是保存下来、供订阅输出选用的响应头（第 5.4 节）。
var keptHeaders = []string{"subscription-userinfo", "profile-update-interval", "content-disposition", "profile-web-page-url"}

// Fetcher 执行受控的 HTTP 拉取。可并发使用。
//
// 服务器直接连接目标（没有出站代理，见 docs/decisions/0003）；在 net.Dialer.Control 中检查
// 实际连接的地址，因此即使域名在检查之后被改为解析到内网（DNS rebinding），连接时仍会被拒绝。
type Fetcher struct {
	cfg    Config
	client *http.Client
	now    func() time.Time
}

// New 构造拉取器。
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
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: cfg.Guard.control}
	tr := &http.Transport{DialContext: dialer.DialContext, Proxy: nil, ForceAttemptHTTP2: true, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: cfg.Timeout, MaxResponseHeaderBytes: 64 << 10}
	f.client = &http.Client{
		Transport: tr,
		Timeout:   cfg.Timeout,
		// 每次重定向都重新检查目标；超过上限即停止。
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= cfg.MaxRedirects {
				return fmt.Errorf("source: 重定向超过 %d 次", cfg.MaxRedirects)
			}
			req.Header.Del("Referer")
			return cfg.Guard.CheckURL(req.URL)
		},
	}
	return f, nil
}

// Fetch 拉取一个地址。
func (f *Fetcher) Fetch(ctx context.Context, r Request) (*Response, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, fmt.Errorf("%w：地址无效", ErrBlocked)
	}
	if err := f.cfg.Guard.CheckURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	// 只发送必要的请求头：不带 Referer、Cookie、X-Forwarded-*。
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
	resp, err := f.client.Do(req)
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
	out := &Response{Body: body, Hash: hex.EncodeToString(h[:]), Headers: map[string]string{}, FetchedAt: f.now().UTC()}
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
	if strings.Contains(msg, "://") {
		return "connection error"
	}
	return msg
}
