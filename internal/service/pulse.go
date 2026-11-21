package service

import (
	"context"
	"errors"

	"github.com/angwz/clash-launch/internal/store"
)

// 在线指示灯（需求 R11）的服务端数据。
//
// 在线人数按实时通道（R22）的连接去重计数：同一用户的多个标签页、多台设备只算一人，断开 15 秒内仍算在线。
// 是否告诉普通用户由管理员决定，默认不告诉；管理员总能看到。
const settingPulseOnline = "pulse.show_online"

// ShowOnlineToUsers 报告是否向普通用户显示在线人数；未设定时为否。
func (s *Service) ShowOnlineToUsers(ctx context.Context) (bool, error) {
	v, err := s.Store.Settings().Get(ctx, settingPulseOnline)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v == "1", nil
}

// SetShowOnlineToUsers 设定是否向普通用户显示在线人数（管理员），并记入审计。
func (s *Service) SetShowOnlineToUsers(ctx context.Context, admin string, show bool) error {
	v, detail := "0", `{"show":false}`
	if show {
		v, detail = "1", `{"show":true}`
	}
	if err := s.Store.Settings().Set(ctx, settingPulseOnline, v); err != nil {
		return err
	}
	return s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: admin, Action: "admin.pulse_online", Detail: []byte(detail)})
}

// OnlineFor 返回这个用户能看到的在线人数；无权查看或实时通道未启用时返回 0（JSON 中省略）。
// 读取设置失败时记录日志并按「不显示」处理：在线人数只是提示信息，不应让心跳或指示灯接口因此失败。
func (s *Service) OnlineFor(ctx context.Context, u *store.User) int {
	if s.Events == nil {
		return 0
	}
	if u.Role != "admin" {
		show, err := s.ShowOnlineToUsers(ctx)
		if err != nil && s.Log != nil {
			s.Log.Warn("读取在线人数显示设置失败", "error", err)
		}
		if !show {
			return 0
		}
	}
	return s.Events.Online()
}
