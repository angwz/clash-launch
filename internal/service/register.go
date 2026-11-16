package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 注册（docs/decisions/0003）：管理员开关「允许注册」，默认关闭。关闭时只能凭一次性邀请注册：
// 管理员生成邀请链接（可选有效期），发给别人；邀请用过一次即作废。
// 安全问题与工作量证明在 M5 加入注册流程。

const settingRegistration = "registration.open"

// ErrRegistrationClosed 表示注册关闭且没有有效的邀请。
var ErrRegistrationClosed = errors.New("service: 注册已关闭，需要有效的邀请")

// ErrUsername 表示用户名不合规。
var ErrUsername = errors.New("service: 用户名需为 1–32 个字符，不能包含空格或控制字符")

// RegistrationOpen 报告是否允许无邀请注册。
func (s *Service) RegistrationOpen(ctx context.Context) (bool, error) {
	v, err := s.Store.Settings().Get(ctx, settingRegistration)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return v == "true", err
}

// SetRegistration 打开或关闭无邀请注册。
func (s *Service) SetRegistration(ctx context.Context, admin string, open bool) error {
	v := "false"
	if open {
		v = "true"
	}
	if err := s.Store.Settings().Set(ctx, settingRegistration, v); err != nil {
		return err
	}
	return s.Store.Audit().Add(ctx, &store.AuditEntry{OwnerID: admin, Action: "admin.registration", Detail: []byte(`{"open":` + v + `}`)})
}

// CheckUsername 校验用户名：1–32 个字符，首尾无空白，不含控制字符（反馈 #17：用户名可以简单）。
func CheckUsername(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 32 || strings.TrimSpace(name) != name {
		return ErrUsername
	}
	for _, r := range name {
		if r <= ' ' || r == 0x7F || (r >= 0x80 && r <= 0x9F) || r == 0x2028 || r == 0x2029 {
			return ErrUsername
		}
	}
	return nil
}

// RegisterInput 是注册的输入。安全问题必填（第 14.1 节：找回密码的唯一途径）。
type RegisterInput struct {
	Username  string
	Password  string
	Invite    string
	Questions []auth.Question
	Answers   []string
}

// Register 创建普通用户。注册开放时不需要邀请；关闭时邀请必须有效。
// 账号、安全问题与邀请的消费在同一事务中完成。工作量证明由调用方（HTTP 层）先行校验。
func (s *Service) Register(ctx context.Context, in RegisterInput) (*store.User, error) {
	username, password, invite := in.Username, in.Password, in.Invite
	if err := CheckUsername(username); err != nil {
		return nil, err
	}
	if err := s.checkNameFree(ctx, username, ""); err != nil {
		return nil, err
	}
	if err := auth.CheckPassword(password); err != nil {
		return nil, err
	}
	// 安全问题可选（反馈 #76）：0–3 个
	if err := auth.CheckQuestions(in.Questions, in.Answers, 0); err != nil {
		return nil, err
	}
	open, err := s.RegistrationOpen(ctx)
	if err != nil {
		return nil, err
	}
	if !open && invite == "" {
		return nil, ErrRegistrationClosed
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	stored, err := hashQuestions(in.Questions, in.Answers)
	if err != nil {
		return nil, err
	}
	u := &store.User{Username: username, PasswordHash: hash, Role: "user"}
	err = s.Store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := tx.Questions().Set(ctx, u.ID, stored); err != nil {
			return err
		}
		if invite != "" {
			// 邀请成功消费即记下邀请关系（需求 R12）；管理员发出的邀请同时记为受邀用户（可自选节点写法，反馈 #46）。
			// 开放注册时邀请可选，无效或过期的邀请不挡住注册，只是不记关系。
			inv, err := tx.Invites().Use(ctx, auth.HashToken(invite), u.ID, s.now())
			switch {
			case err == nil:
				inviter, err := tx.Users().ByID(ctx, inv.CreatedBy)
				if err != nil {
					return err
				}
				u.InvitedBy, u.InvitedByName, u.InviteID = inviter.ID, inviter.Username, inv.ID
				if err := tx.Users().SetInviter(ctx, u.ID, inviter.ID, inviter.Username, inv.ID); err != nil {
					return err
				}
				if inviter.Role == "admin" {
					u.Invited = true
					if err := tx.Users().SetInvited(ctx, u.ID, true); err != nil {
						return err
					}
				}
			case !errors.Is(err, store.ErrNotFound):
				return err
			case !open:
				return ErrRegistrationClosed
			}
		}
		return tx.Audit().Add(ctx, &store.AuditEntry{OwnerID: u.ID, Action: "register"})
	})
	if err != nil {
		return nil, err
	}
	s.notifyInviter(ctx, u)
	return u, nil
}
