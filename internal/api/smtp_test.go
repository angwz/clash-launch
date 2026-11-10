package api

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/source"
)

type captured struct {
	mu     sync.Mutex
	events []string
}

func (c *captured) Notify(_ context.Context, _ string, kind service.Alert, _ map[string]string) {
	c.mu.Lock()
	c.events = append(c.events, string(kind))
	c.mu.Unlock()
}

func (c *captured) has(kind service.Alert) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e == string(kind) {
			return true
		}
	}
	return false
}

// 提醒邮件设置：只允许 25/465/587、拒绝换行注入、不返回口令、保存后回到未验证；
// 安全相关的变更产生对应的提醒事件。
func TestSMTPSettingsAndAlerts(t *testing.T) {
	h := newHarness(t, nil)
	ev := &captured{}
	h.svc.Notifier = ev
	h.svc.Mailer = &source.Mailer{Guard: source.Guard{}}
	admin, _ := h.setup()
	const pw = "correct horse battery"

	if r := string(admin.want(200, "GET", "/api/v1/account/smtp", nil)); !strings.Contains(r, `"settings":null`) || !strings.Contains(r, `"available":true`) {
		t.Errorf("未设置：%s", r)
	}
	base := map[string]any{"host": "smtp.example.com", "port": 465, "security": "tls", "username": "a@example.com", "password": "fake-smtp-password", "from": "a@example.com", "to": "a@example.com"}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range base {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	admin.want(422, "PUT", "/api/v1/account/smtp", with("port", 2525))
	admin.want(422, "PUT", "/api/v1/account/smtp", with("to", "a@example.com\r\nBcc: x@example.com"))
	admin.want(422, "PUT", "/api/v1/account/smtp", with("host", "localhost"))
	admin.want(204, "PUT", "/api/v1/account/smtp", base)
	got := string(admin.want(200, "GET", "/api/v1/account/smtp", nil))
	if strings.Contains(got, "fake-smtp-password") || !strings.Contains(got, `"hasPassword":true`) || !strings.Contains(got, `"enabled":false`) {
		t.Errorf("设置：%s", got)
	}
	// 口令留空：保留原口令
	admin.want(204, "PUT", "/api/v1/account/smtp", with("password", ""))
	if s, _ := h.db.SMTP().Get(context.Background(), firstAdmin(t, h)); s.Password != "fake-smtp-password" {
		t.Error("留空的口令应保留")
	}

	// 安全事件
	admin.want(204, "PUT", "/api/v1/account/password", map[string]any{"current": pw, "password": "another strong password"})
	admin.want(204, "PUT", "/api/v1/account/questions", map[string]any{"password": "another strong password", "questions": []map[string]any{{"builtin": 1}, {"builtin": 2}, {"builtin": 3}}, "answers": []string{"aa", "bb", "cc"}})
	for _, k := range []service.Alert{service.AlertPassword, service.AlertQuestions} {
		if !ev.has(k) {
			t.Errorf("缺少提醒事件 %s：%v", k, ev.events)
		}
	}
	admin.want(204, "DELETE", "/api/v1/account/smtp", nil)
}
