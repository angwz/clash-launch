package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
)

// 个人 SMTP 提醒（第 14.1 节）：用户填写自己的 SMTP 服务器与收件地址，测试邮件发送成功后才启用。
// 修改设置会回到「未验证」状态。口令加密存储，只用于该用户本人的提醒。发送失败只记日志。

// ErrSMTPUnavailable 表示服务没有配置邮件发送（Mailer 为 nil）。
var ErrSMTPUnavailable = errors.New("service: 邮件提醒不可用")

// SMTPInput 是用户提交的设置；Password 为空表示保留原口令。
type SMTPInput struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// SMTPView 是返回给用户的设置（不含口令）。
type SMTPView struct {
	SMTPInput
	HasPassword bool       `json:"hasPassword"`
	Enabled     bool       `json:"enabled"`
	VerifiedAt  *time.Time `json:"verifiedAt,omitempty"`
}

// GetSMTP 返回用户的设置；没有设置时返回 nil。
func (s *Service) GetSMTP(ctx context.Context, userID string) (*SMTPView, error) {
	x, err := s.Store.SMTP().Get(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := &SMTPView{SMTPInput: SMTPInput{Host: x.Host, Port: x.Port, Security: x.Security, Username: x.Username, From: x.From, To: x.To}, HasPassword: x.Password != "", Enabled: x.Enabled}
	if !x.VerifiedAt.IsZero() {
		t := x.VerifiedAt
		v.VerifiedAt = &t
	}
	return v, nil
}

// PutSMTP 保存设置（未验证、未启用）；需要再发送测试邮件。
func (s *Service) PutSMTP(ctx context.Context, userID string, in SMTPInput) error {
	if s.Mailer == nil {
		return ErrSMTPUnavailable
	}
	in.Host, in.Username, in.From, in.To = strings.TrimSpace(in.Host), strings.TrimSpace(in.Username), strings.TrimSpace(in.From), strings.TrimSpace(in.To)
	if in.Password == "" {
		if old, err := s.Store.SMTP().Get(ctx, userID); err == nil {
			in.Password = old.Password
		}
	}
	if err := s.Mailer.CheckMail(source.Mail{Host: in.Host, Port: in.Port, Security: in.Security, Username: in.Username, Password: in.Password, From: in.From, To: in.To}); err != nil {
		return err
	}
	if err := s.Store.SMTP().Put(ctx, &store.SMTP{UserID: userID, Host: in.Host, Port: in.Port, Security: in.Security, Username: in.Username, Password: in.Password, From: in.From, To: in.To}); err != nil {
		return err
	}
	s.audit(ctx, userID, "smtp.update", map[string]string{"host": in.Host})
	return nil
}

// DeleteSMTP 删除设置并停止提醒。
func (s *Service) DeleteSMTP(ctx context.Context, userID string) error {
	if err := s.Store.SMTP().Delete(ctx, userID); err != nil {
		return err
	}
	s.audit(ctx, userID, "smtp.delete", nil)
	return nil
}

// TestSMTP 发送测试邮件；成功后启用提醒。每个用户每小时最多 5 封测试邮件。
func (s *Service) TestSMTP(ctx context.Context, userID string) error {
	if s.Mailer == nil {
		return ErrSMTPUnavailable
	}
	g, now := s.g(), s.now()
	if w := g.smtpTest.Wait(userID, now); w > 0 {
		return &WaitError{Wait: w}
	}
	g.smtpTest.Fail(userID, now) // 每次尝试都计数
	x, err := s.Store.SMTP().Get(ctx, userID)
	if err != nil {
		return err
	}
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return err
	}
	subject, body := alertText(langOf(u), "test", nil)
	if err := s.Mailer.Send(ctx, mailFor(x, subject, body)); err != nil {
		return err
	}
	x.Enabled, x.VerifiedAt = true, now
	if err := s.Store.SMTP().Put(ctx, x); err != nil {
		return err
	}
	s.audit(ctx, userID, "smtp.verified", nil)
	return nil
}

func mailFor(x *store.SMTP, subject, body string) source.Mail {
	return source.Mail{Host: x.Host, Port: x.Port, Security: x.Security, Username: x.Username, Password: x.Password, From: x.From, To: x.To, Subject: subject, Body: body}
}

func langOf(u *store.User) string {
	if strings.HasPrefix(u.Lang, "zh") {
		return "zh"
	}
	return "en"
}

// sendAlert 在后台发送一条提醒：只有已验证并启用的设置才会发送；失败只记日志。
func (s *Service) sendAlert(userID string, kind Alert, detail map[string]string) {
	if s.Mailer == nil {
		return
	}
	at := s.now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		x, err := s.Store.SMTP().Get(ctx, userID)
		if err != nil || !x.Enabled {
			return
		}
		u, err := s.Store.Users().ByID(ctx, userID)
		if err != nil {
			return
		}
		d := map[string]string{"time": at.UTC().Format("2006-01-02 15:04 MST"), "user": u.Username}
		for k, v := range detail {
			d[k] = v
		}
		subject, body := alertText(langOf(u), string(kind), d)
		if err := s.Mailer.Send(ctx, mailFor(x, subject, body)); err != nil && s.Log != nil {
			s.Log.Warn("提醒邮件发送失败", "user", userID, "kind", kind, "error", err)
		}
	}()
}
