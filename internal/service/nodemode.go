package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/store"
)

// 节点写法的全局策略（反馈 #46）：
//   - 管理员设定默认写法（写入配置 / 交给客户端），新加入配置的源使用它；
//   - 管理员与经邀请注册的用户可在配置中逐个源自选；
//   - 开放注册的普通用户不能自选：生成时一律使用默认写法，与文档中记录的写法不同时报告诊断。
//
// 锁定只在生成时生效，不改写用户的配置文档：管理员放开限制或用户被改为受邀后，原先的选择恢复生效。
const settingNodeMode = "profile.node_mode"

// ErrNodeMode 表示节点写法的取值无效。
var ErrNodeMode = errors.New("service: 节点写法只能是 inline 或 provider")

// NodeModePolicy 是某个用户看到的节点写法策略。
type NodeModePolicy struct {
	Default profile.Mode `json:"default"`
	Locked  bool         `json:"locked"` // true：只能使用默认写法
}

// NodeModeDefault 返回管理员设定的默认写法；未设定时为写入配置。
func (s *Service) NodeModeDefault(ctx context.Context) (profile.Mode, error) {
	v, err := s.Store.Settings().Get(ctx, settingNodeMode)
	if errors.Is(err, store.ErrNotFound) {
		return profile.ModeInline, nil
	}
	if err != nil {
		return "", err
	}
	switch m := profile.Mode(v); m {
	case profile.ModeInline, profile.ModeProvider:
		return m, nil
	default:
		return "", fmt.Errorf("全局设置 %s 的值无效：%q", settingNodeMode, v)
	}
}

// SetNodeModeDefault 设定默认写法（管理员）。
func (s *Service) SetNodeModeDefault(ctx context.Context, admin string, m profile.Mode) error {
	if m != profile.ModeInline && m != profile.ModeProvider {
		return ErrNodeMode
	}
	if err := s.Store.Settings().Set(ctx, settingNodeMode, string(m)); err != nil {
		return err
	}
	return s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: admin, Action: "admin.node_mode", Detail: []byte(`{"default":"` + string(m) + `"}`)})
}

// NodeModeFor 返回用户的节点写法策略。
func (s *Service) NodeModeFor(ctx context.Context, u *store.User) (NodeModePolicy, error) {
	d, err := s.NodeModeDefault(ctx)
	if err != nil {
		return NodeModePolicy{}, err
	}
	return NodeModePolicy{Default: d, Locked: u.Role != "admin" && !u.Invited}, nil
}

// nodeModeOf 按所有者查策略；用于生成与托管 provider（只有所有者 ID 的场景）。
func (s *Service) nodeModeOf(ctx context.Context, owner string) (NodeModePolicy, error) {
	u, err := s.Store.Users().ByID(ctx, owner)
	if err != nil {
		return NodeModePolicy{}, fmt.Errorf("读取配置所有者 %s: %w", owner, err)
	}
	return s.NodeModeFor(ctx, u)
}

// effective 返回某个源实际使用的写法。
func (p NodeModePolicy) effective(m profile.Mode) profile.Mode {
	if p.Locked {
		return p.Default
	}
	if m == "" {
		return profile.ModeInline
	}
	return m
}
