package service

import "context"

// 安全提醒（第 14.1 节）：出现异常登录或安全设置变更时，通过用户本人配置的 SMTP 发邮件。
// 服务层只产生事件；发送由 Notifier 完成（未配置时不发送），发送失败只记日志，不影响触发它的操作。

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

// Notifier 向用户发送一条提醒。实现需要自行判断该用户是否启用了提醒。
type Notifier interface {
	Notify(ctx context.Context, userID string, kind Alert, detail map[string]string)
}

func (s *Service) notify(ctx context.Context, userID string, kind Alert, detail map[string]string) {
	if s.Notifier != nil {
		s.Notifier.Notify(ctx, userID, kind, detail)
	}
}
