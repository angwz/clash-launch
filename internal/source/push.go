package source

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// 提醒推送（需求 R31，决定 0008）：把提醒发到用户本人配置的 Webhook、Telegram、Bark、ntfy。
// 与拉取订阅、发送邮件一样经过 SSRF 防护（URL 检查 + Dialer.Control 检查实际地址）。
// 只允许 https、不跟随重定向；响应只看状态码，正文最多读 64 KiB 后丢弃。
// 错误经过分类：Telegram 的令牌在 URL 路径中，Webhook 地址也常含密钥，因此错误信息从不包含 URL。

// ErrPush 表示推送设置无效或推送失败。
var ErrPush = errors.New("source: 推送失败")

// 渠道种类。
const (
	PushWebhook  = "webhook"
	PushTelegram = "telegram"
	PushBark     = "bark"
	PushNtfy     = "ntfy"
)

// PushKinds 是支持的渠道种类。
var PushKinds = []string{PushWebhook, PushTelegram, PushBark, PushNtfy}

// Push 是一个渠道的投递设置；各种类只用其中几项。
type Push struct {
	Kind   string `json:"kind"`
	URL    string `json:"url,omitempty"`    // webhook：完整地址；bark、ntfy：服务器地址
	Token  string `json:"token,omitempty"`  // telegram：Bot 令牌；ntfy：访问令牌（可选）
	Chat   string `json:"chat,omitempty"`   // telegram：chat ID 或 @频道名
	Key    string `json:"key,omitempty"`    // bark：设备 key
	Topic  string `json:"topic,omitempty"`  // ntfy：主题
	Secret string `json:"secret,omitempty"` // webhook：签名密钥（可选）
}

// PushMessage 是一条提醒的内容。
type PushMessage struct {
	Kind   string            `json:"kind"`
	Title  string            `json:"title"`
	Text   string            `json:"text"`
	Detail map[string]string `json:"detail,omitempty"`
	Time   time.Time         `json:"time"`
}

// Pusher 发送推送。可并发使用。
type Pusher struct {
	Guard   Guard
	Timeout time.Duration // 单次请求超时，默认 10 s
	TLS     *tls.Config   // 测试时可替换（信任自签证书）
	// TelegramBase 默认 https://api.telegram.org；测试时指向本地服务器
	TelegramBase string
}

var (
	reTGToken   = regexp.MustCompile(`^[0-9]{1,20}:[A-Za-z0-9_-]{20,100}$`)
	reTGChat    = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z0-9_]{4,64})$`)
	reBarkKey   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	reNtfyTopic = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	rePrintable = regexp.MustCompile(`^[!-~]{1,256}$`) // 可打印 ASCII，不含空格：令牌与密钥进入请求头或 HMAC
)

// CheckPush 校验投递设置，不联网。供保存设置前使用。
func (p *Pusher) CheckPush(x Push) error {
	bad := func(what string) error { return fmt.Errorf("%w：%s无效", ErrPush, what) }
	switch x.Kind {
	case PushWebhook:
		if err := p.checkURL(x.URL); err != nil {
			return err
		}
		if x.Secret != "" && !rePrintable.MatchString(x.Secret) {
			return bad("签名密钥")
		}
	case PushTelegram:
		if !reTGToken.MatchString(x.Token) {
			return bad("Bot 令牌")
		}
		if !reTGChat.MatchString(x.Chat) {
			return bad("chat ID")
		}
	case PushBark:
		if err := p.checkURL(x.URL); err != nil {
			return err
		}
		if !reBarkKey.MatchString(x.Key) {
			return bad("设备 key")
		}
	case PushNtfy:
		if err := p.checkURL(x.URL); err != nil {
			return err
		}
		if !reNtfyTopic.MatchString(x.Topic) {
			return bad("主题")
		}
		if x.Token != "" && !rePrintable.MatchString(x.Token) {
			return bad("访问令牌")
		}
	default:
		return fmt.Errorf("%w：不支持的渠道 %q", ErrPush, x.Kind)
	}
	return nil
}

// checkURL 只允许不带用户信息与片段的 https 地址，并经过 SSRF 检查。
func (p *Pusher) checkURL(raw string) error {
	if len(raw) > 2048 {
		return fmt.Errorf("%w：地址过长", ErrPush)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w：地址无效", ErrPush)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w：地址必须以 https:// 开头", ErrPush)
	}
	if u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%w：地址中不能有用户名、密码或 # 片段", ErrPush)
	}
	if err := p.Guard.CheckURL(u); err != nil {
		return fmt.Errorf("%w：%w", ErrPush, err)
	}
	return nil
}

// Send 投递一条提醒。
func (p *Pusher) Send(ctx context.Context, x Push, m PushMessage) error {
	if err := p.CheckPush(x); err != nil {
		return err
	}
	target, body, headers, err := p.build(x, m)
	if err != nil {
		return err
	}
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w：地址无效", ErrPush)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "clash-launch")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return classifyPush(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w：服务器返回 HTTP %d", ErrPush, resp.StatusCode)
	}
	return nil
}

func (p *Pusher) client() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: p.Guard.control}
	tr := &http.Transport{DialContext: d.DialContext, Proxy: nil, TLSClientConfig: p.TLS, TLSHandshakeTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10, DisableKeepAlives: true}
	// 不跟随重定向：3xx 直接作为响应返回，由状态码判定为失败
	return &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// build 按渠道生成请求地址、正文与额外的请求头。
func (p *Pusher) build(x Push, m PushMessage) (string, []byte, map[string]string, error) {
	var v any
	headers := map[string]string{}
	target := x.URL
	switch x.Kind {
	case PushWebhook:
		v = m
	case PushTelegram:
		base := p.TelegramBase
		if base == "" {
			base = "https://api.telegram.org"
		}
		target = base + "/bot" + x.Token + "/sendMessage" // 令牌已按格式校验，不含路径字符
		v = map[string]any{"chat_id": x.Chat, "text": truncRunes(m.Title+"\n\n"+m.Text, 4096), "disable_web_page_preview": true}
	case PushBark:
		target = strings.TrimRight(x.URL, "/") + "/push"
		v = map[string]string{"device_key": x.Key, "title": m.Title, "body": m.Text, "group": "Clash, Launch!"}
	case PushNtfy:
		// 以 JSON 发布时发到服务器根地址，主题放在正文里
		v = map[string]any{"topic": x.Topic, "title": m.Title, "message": truncBytes(m.Text, 4096)}
		if x.Token != "" {
			headers["Authorization"] = "Bearer " + x.Token
		}
	}
	body, err := json.Marshal(v)
	if err != nil {
		return "", nil, nil, err
	}
	if x.Kind == PushWebhook && x.Secret != "" {
		ts := strconv.FormatInt(m.Time.Unix(), 10)
		mac := hmac.New(sha256.New, []byte(x.Secret))
		mac.Write([]byte(ts + "."))
		mac.Write(body)
		headers["X-Timestamp"] = ts
		headers["X-Signature-256"] = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}
	return target, body, headers, nil
}

// classifyPush 只保留错误的大类；底层错误（*url.Error）含有 URL，可能带着令牌。
func classifyPush(err error) error {
	var ne net.Error
	switch {
	case errors.Is(err, ErrBlocked):
		return fmt.Errorf("%w：%w", ErrPush, ErrBlocked)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("%w：超时", ErrPush)
	}
	var te *tls.CertificateVerificationError
	if errors.As(err, &te) {
		return fmt.Errorf("%w：证书无效", ErrPush)
	}
	return fmt.Errorf("%w：连接失败", ErrPush)
}

// truncRunes 把 s 截到最多 n 个字符，超出时以「…」结尾。
func truncRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// truncBytes 把 s 截到最多 n 字节（不切开字符），超出时以「…」结尾。
func truncBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
