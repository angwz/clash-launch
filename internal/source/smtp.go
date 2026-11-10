package source

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"
)

// 发送提醒邮件（第 14.1、14.5 节）：连接用户填写的 SMTP 服务器。
// 与拉取订阅一样经过 SSRF 防护：主机名检查 + 在 Dialer.Control 中检查实际连接的地址；只允许 25、465、587 端口。
// 口令只在加密连接中发送：465 为隐式 TLS；25 与 587 必须支持 STARTTLS，否则拒绝发送。

// ErrSMTP 表示邮件发送失败；错误信息经过分类，不包含口令。
var ErrSMTP = errors.New("source: 邮件发送失败")

// Mail 是一封邮件及其投递设置。
type Mail struct {
	Host     string
	Port     int
	Security string // tls（隐式 TLS，465）| starttls（25、587）
	Username string
	Password string
	From     string
	To       string
	Subject  string
	Body     string // 纯文本
}

// Mailer 发送邮件。
type Mailer struct {
	Guard   Guard
	Ports   []int         // 允许的端口，默认 25、465、587
	Timeout time.Duration // 整个会话的超时，默认 20 s
	TLS     *tls.Config   // 测试时可替换（如信任自签证书）；默认按主机名校验证书
}

// SMTPPorts 是默认允许的端口。
var SMTPPorts = []int{25, 465, 587}

// CheckMail 校验投递设置与地址，不联网。供保存设置前使用。
func (m *Mailer) CheckMail(x Mail) error {
	ports := m.Ports
	if ports == nil {
		ports = SMTPPorts
	}
	if !slices.Contains(ports, x.Port) {
		return fmt.Errorf("%w：端口只能是 %v", ErrSMTP, ports)
	}
	if x.Security != "tls" && x.Security != "starttls" {
		return fmt.Errorf("%w：安全方式只能是 tls 或 starttls", ErrSMTP)
	}
	for _, f := range []string{x.Host, x.Username, x.Password, x.From, x.To, x.Subject} {
		if strings.ContainsAny(f, "\r\n") {
			return fmt.Errorf("%w：字段中不能有换行", ErrSMTP)
		}
	}
	if err := m.Guard.CheckHost(x.Host); err != nil {
		return fmt.Errorf("%w：%w", ErrSMTP, err)
	}
	for _, a := range []string{x.From, x.To} {
		if p, err := mail.ParseAddress(a); err != nil || p.Name != "" {
			return fmt.Errorf("%w：地址 %q 无效（只填邮箱地址）", ErrSMTP, a)
		}
	}
	return nil
}

// Send 投递一封邮件。
func (m *Mailer) Send(ctx context.Context, x Mail) error {
	if err := m.CheckMail(x); err != nil {
		return err
	}
	timeout := m.Timeout
	if timeout == 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := &net.Dialer{Timeout: 10 * time.Second, Control: m.Guard.control}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(x.Host, strconv.Itoa(x.Port)))
	if err != nil {
		return classifySMTP("连接", err)
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	tlsCfg := &tls.Config{ServerName: x.Host, MinVersion: tls.VersionTLS12}
	if m.TLS != nil {
		tlsCfg = m.TLS.Clone()
		if tlsCfg.ServerName == "" {
			tlsCfg.ServerName = x.Host
		}
	}
	if x.Security == "tls" {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return classifySMTP("TLS", err)
		}
		conn = tc
	}
	c, err := smtp.NewClient(conn, x.Host)
	if err != nil {
		return classifySMTP("问候", err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Hello("localhost"); err != nil {
		return classifySMTP("EHLO", err)
	}
	if x.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%w：服务器不支持 STARTTLS，拒绝以明文发送口令", ErrSMTP)
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return classifySMTP("STARTTLS", err)
		}
	}
	if x.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", x.Username, x.Password, x.Host)); err != nil {
			return classifySMTP("认证", err)
		}
	}
	if err := c.Mail(x.From); err != nil {
		return classifySMTP("发件人", err)
	}
	if err := c.Rcpt(x.To); err != nil {
		return classifySMTP("收件人", err)
	}
	w, err := c.Data()
	if err != nil {
		return classifySMTP("DATA", err)
	}
	if _, err := w.Write(buildMessage(x, time.Now())); err != nil {
		return classifySMTP("正文", err)
	}
	if err := w.Close(); err != nil {
		return classifySMTP("正文", err)
	}
	return c.Quit()
}

// classifySMTP 只保留阶段与错误的大类：服务器的响应文本可能回显用户名，不原样返回。
func classifySMTP(stage string, err error) error {
	var tp interface{ Temporary() bool }
	switch {
	case errors.Is(err, ErrBlocked):
		return fmt.Errorf("%w：%s：%w", ErrSMTP, stage, err)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w：%s：超时", ErrSMTP, stage)
	case errors.As(err, &tp):
		return fmt.Errorf("%w：%s：网络错误", ErrSMTP, stage)
	}
	var te *textproto.Error
	if errors.As(err, &te) {
		return fmt.Errorf("%w：%s：服务器返回 %d", ErrSMTP, stage, te.Code)
	}
	return fmt.Errorf("%w：%s失败", ErrSMTP, stage)
}

// buildMessage 生成 RFC 5322 邮件：主题按 RFC 2047 编码，正文为 base64 的 UTF-8 纯文本。
func buildMessage(x Mail, now time.Time) []byte {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := x.From[strings.LastIndexByte(x.From, '@')+1:]
	var b strings.Builder
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", x.From)
	h("To", x.To)
	h("Subject", mime.QEncoding.Encode("utf-8", x.Subject))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Content-Type", `text/plain; charset="utf-8"`)
	h("Content-Transfer-Encoding", "base64")
	h("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(x.Body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return []byte(b.String())
}
