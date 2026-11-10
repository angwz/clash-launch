package service

import (
	"context"
	"sort"
	"strings"
)

// 安全提醒（第 14.1 节）：出现异常登录或安全设置变更时，通过用户本人配置的 SMTP 发邮件。
// 服务层只产生事件；默认由 sendAlert 发送邮件（smtp.go），设置了 Notifier 时交给它（测试用）。
// 发送失败只记日志，不影响触发它的操作。

// Alert 是提醒的种类。
type Alert string

const (
	AlertNewDevice Alert = "new_device" // 新设备或新网络首次登录
	AlertPassword  Alert = "password"   // 口令被修改
	AlertQuestions Alert = "questions"  // 安全问题被修改
	AlertRecovered Alert = "recovered"  // 通过找回重设了口令
	AlertMFA       Alert = "mfa"        // 两步验证启用或关闭、恢复码重新生成
	AlertPasskey   Alert = "passkey"    // 通行密钥添加或删除
)

// Notifier 接收一条提醒事件。
type Notifier interface {
	Notify(ctx context.Context, userID string, kind Alert, detail map[string]string)
}

func (s *Service) notify(ctx context.Context, userID string, kind Alert, detail map[string]string) {
	if s.Notifier != nil {
		s.Notifier.Notify(ctx, userID, kind, detail)
		return
	}
	s.sendAlert(userID, kind, detail)
}

// alertTexts 是提醒邮件的标题与正文开头（中文、英文）。邮件按用户的界面语言发送。
var alertTexts = map[string]map[string][2]string{
	"zh": {
		"test":       {"Clash, Launch! 提醒邮件测试", "这是一封测试邮件。收到它说明提醒邮件已经可以送达，提醒现已启用。"},
		"new_device": {"新设备登录了你的 Clash, Launch! 账号", "你的账号刚刚在一个新的设备或网络上登录。"},
		"password":   {"你的 Clash, Launch! 口令已修改", "你的账号口令刚刚被修改。"},
		"questions":  {"你的 Clash, Launch! 安全问题已修改", "你的账号安全问题刚刚被修改。"},
		"recovered":  {"你的 Clash, Launch! 口令已通过找回重设", "有人回答了安全问题并重设了你的账号口令，所有设备都已退出登录。"},
		"mfa":        {"你的 Clash, Launch! 两步验证设置已变更", "你的账号两步验证设置刚刚发生变化。"},
		"passkey":    {"你的 Clash, Launch! 通行密钥已变更", "你的账号通行密钥刚刚发生变化。"},
	},
	"en": {
		"test":       {"Clash, Launch! alert test", "This is a test message. Receiving it means alerts can be delivered; alerts are now enabled."},
		"new_device": {"New sign-in to your Clash, Launch! account", "Your account was just signed in from a new device or network."},
		"password":   {"Your Clash, Launch! password was changed", "The password of your account was just changed."},
		"questions":  {"Your Clash, Launch! security questions were changed", "The security questions of your account were just changed."},
		"recovered":  {"Your Clash, Launch! password was reset", "Someone answered the security questions and reset your password; all devices were signed out."},
		"mfa":        {"Your Clash, Launch! two-step verification changed", "The two-step verification settings of your account just changed."},
		"passkey":    {"Your Clash, Launch! passkeys changed", "The passkeys of your account just changed."},
	},
}

var alertFooter = map[string]string{
	"zh": "如果这不是你本人的操作，请立即修改口令，并在「账号」页面检查登录设备、两步验证与通行密钥。",
	"en": "If this wasn't you, change your password now and review signed-in devices, two-step verification and passkeys on the Account page.",
}

var alertLabels = map[string]map[string]string{
	"zh": {"time": "时间", "user": "账号", "ip": "网络", "device": "设备", "change": "变更", "name": "名称"},
	"en": {"time": "Time", "user": "Account", "ip": "Network", "device": "Device", "change": "Change", "name": "Name"},
}

// alertText 生成提醒邮件的标题与正文（纯文本）。细节按键名排序，便于测试与阅读。
func alertText(lang, kind string, detail map[string]string) (string, string) {
	t := alertTexts[lang][kind]
	var b strings.Builder
	b.WriteString(t[1] + "\n\n")
	keys := make([]string, 0, len(detail))
	for k := range detail {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		label := alertLabels[lang][k]
		if label == "" {
			label = k
		}
		b.WriteString(label + ": " + detail[k] + "\n")
	}
	if kind != "test" {
		b.WriteString("\n" + alertFooter[lang] + "\n")
	}
	return t[0], b.String()
}
