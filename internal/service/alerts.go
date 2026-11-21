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
	AlertPassword  Alert = "password"   // 密码被修改
	AlertQuestions Alert = "questions"  // 安全问题被修改
	AlertRecovered Alert = "recovered"  // 通过找回重设了密码
	AlertMFA       Alert = "mfa"        // 两步验证启用或关闭、恢复码重新生成
	AlertPasskey   Alert = "passkey"    // 通行密钥添加或删除
	AlertToken     Alert = "token"      // 生成了个人访问令牌（需求 R25）
	AlertDeletion  Alert = "deletion"   // 发起了注销（需求 R20）

	// 以下是运行提醒（需求 R23、R24）：同时进入站内提醒列表，邮件的结尾提示不同于安全提醒
	AlertSourceFailing Alert = "source_failing" // 订阅节点源连续拉取失败
	AlertCheckFailed   Alert = "check_failed"   // 配置在后台重新生成时检查不通过
	AlertLinkNew       Alert = "link_new"       // 订阅链接被新的国家或地区、新的客户端拉取
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
		"test":           {"Clash, Launch! 提醒邮件测试", "这是一封测试邮件。收到它说明提醒邮件已经可以送达，提醒现已启用。"},
		"new_device":     {"新设备登录了你的 Clash, Launch! 账号", "你的账号刚刚在一个新的设备或网络上登录。"},
		"password":       {"你的 Clash, Launch! 密码已修改", "你的账号密码刚刚被修改。"},
		"questions":      {"你的 Clash, Launch! 安全问题已修改", "你的账号安全问题刚刚被修改。"},
		"recovered":      {"你的 Clash, Launch! 密码已通过找回重设", "有人回答了安全问题并重设了你的账号密码，所有设备都已退出登录。"},
		"mfa":            {"你的 Clash, Launch! 两步验证设置已变更", "你的账号两步验证设置刚刚发生变化。"},
		"passkey":        {"你的 Clash, Launch! 通行密钥已变更", "你的账号通行密钥刚刚发生变化。"},
		"token":          {"你的 Clash, Launch! 账号生成了访问令牌", "你的账号刚刚生成了一个个人访问令牌，持有它的程序可以调用接口。"},
		"deletion":       {"你的 Clash, Launch! 账号将被注销", "你的账号刚刚发起了注销，所有设备已退出登录。冷静期内重新登录即可撤销；到期后全部数据会被删除，订阅链接随之失效。"},
		"source_failing": {"Clash, Launch! 节点源连续拉取失败", "一个订阅节点源连续两次拉取失败。订阅链接继续提供上一次成功的节点。"},
		"check_failed":   {"Clash, Launch! 配置检查不通过", "一份配置在后台重新生成时没有通过检查。订阅链接继续提供上一次通过的版本。"},
		"link_new":       {"Clash, Launch! 订阅链接出现新的来源", "你的一个订阅链接第一次被新的国家或地区、或新的客户端拉取。如果不是你自己或你分享的人，链接可能已经泄露。"},
	},
	"en": {
		"test":           {"Clash, Launch! alert test", "This is a test message. Receiving it means alerts can be delivered; alerts are now enabled."},
		"new_device":     {"New sign-in to your Clash, Launch! account", "Your account was just signed in from a new device or network."},
		"password":       {"Your Clash, Launch! password was changed", "The password of your account was just changed."},
		"questions":      {"Your Clash, Launch! security questions were changed", "The security questions of your account were just changed."},
		"recovered":      {"Your Clash, Launch! password was reset", "Someone answered the security questions and reset your password; all devices were signed out."},
		"mfa":            {"Your Clash, Launch! two-step verification changed", "The two-step verification settings of your account just changed."},
		"passkey":        {"Your Clash, Launch! passkeys changed", "The passkeys of your account just changed."},
		"token":          {"A Clash, Launch! access token was created", "A personal access token was just created for your account; any program holding it can call the API."},
		"deletion":       {"Your Clash, Launch! account is scheduled for deletion", "Deletion of your account was just requested and all devices were signed out. Sign in again during the waiting period to cancel; after it ends, all data is deleted and subscription links stop working."},
		"source_failing": {"Clash, Launch! node source keeps failing", "A subscription node source failed to refresh twice in a row. Subscription links keep serving the last successful nodes."},
		"check_failed":   {"Clash, Launch! config check failed", "A config failed its check while being regenerated in the background. Subscription links keep serving the last version that passed."},
		"link_new":       {"Clash, Launch! subscription link has a new source", "One of your subscription links was fetched for the first time from a new country or region, or by a new client. If that was not you or someone you shared it with, the link may have leaked."},
	},
}

var alertFooter = map[string]string{
	"zh": "如果这不是你本人的操作，请立即修改密码，并在「账号」页面检查登录设备、两步验证、通行密钥与访问令牌。",
	"en": "If this wasn't you, change your password now and review signed-in devices, two-step verification, passkeys and access tokens on the Account page.",
}

// alertOpsFooter 是运行提醒的结尾提示。
var alertOpsFooter = map[string]string{
	"zh": "详情与处理办法见网站上的提醒列表。",
	"en": "See the notifications list on the site for details and what to do.",
}

// opsAlert 报告是否为运行提醒（结尾不提示修改密码）。
func opsAlert(kind string) bool {
	return kind == string(AlertSourceFailing) || kind == string(AlertCheckFailed) || kind == string(AlertLinkNew)
}

var alertLabels = map[string]map[string]string{
	"zh": {"time": "时间", "user": "账号", "ip": "网络", "device": "设备", "change": "变更", "name": "名称", "scope": "权限", "error": "原因", "country": "国家或地区", "client": "客户端", "link": "链接", "when": "注销时间"},
	"en": {"time": "Time", "user": "Account", "ip": "Network", "device": "Device", "change": "Change", "name": "Name", "scope": "Scope", "error": "Reason", "country": "Country or region", "client": "Client", "link": "Link", "when": "Deletion time"},
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
	switch {
	case opsAlert(kind):
		b.WriteString("\n" + alertOpsFooter[lang] + "\n")
	case kind != "test":
		b.WriteString("\n" + alertFooter[lang] + "\n")
	}
	return t[0], b.String()
}
