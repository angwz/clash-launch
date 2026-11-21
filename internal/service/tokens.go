package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 个人访问令牌（需求 R25）：供脚本更新节点源、触发重新生成、读取统计。
//
//   - 令牌形如 clt_<43 位随机串>（32 字节），只以 SHA-256 哈希保存，明文只在创建时返回一次；
//     固定前缀便于密钥扫描工具识别泄露。
//   - 权限分 read（只能读取）与 write（还能修改）；能访问的接口范围见 api.tokenAllowed，
//     账号安全相关的接口（密码、两步验证、会话、令牌本身）与管理接口一律不开放。
//   - 只能在浏览器会话中创建与吊销：令牌不能再生成令牌。创建时账号已经通过了两步验证策略（R41）。
//   - 找回密码、管理员重设密码时吊销全部令牌（与会话一起失效）；账号停用后令牌随之不可用。
const (
	TokenPrefix    = "clt_"
	tokenMaxCount  = 20
	tokenMaxName   = 64
	tokenTouchStep = time.Minute
)

var (
	// ErrTokenInvalid 表示令牌的名称、权限或有效期无效。
	ErrTokenInvalid = errors.New("service: 令牌的名称、权限或有效期无效")
	// ErrTokenLimit 表示令牌数已达上限。
	ErrTokenLimit = errors.New("service: 令牌数量已达上限")
)

// TokenInput 是创建令牌的输入。Days 为 0 表示不过期。
type TokenInput struct {
	Name  string
	Scope string
	Days  int
}

// CreateToken 生成令牌，返回记录与只显示一次的明文。
func (s *Service) CreateToken(ctx context.Context, u *store.User, in TokenInput) (*store.AccessToken, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > tokenMaxName || (in.Scope != "read" && in.Scope != "write") || in.Days < 0 || in.Days > 3650 {
		return nil, "", ErrTokenInvalid
	}
	n, err := s.Store.Tokens().Count(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	if n >= tokenMaxCount {
		return nil, "", ErrTokenLimit
	}
	plain := TokenPrefix + auth.NewToken(32)
	now := s.now()
	t := &store.AccessToken{OwnerID: u.ID, Name: name, TokenHash: auth.HashToken(plain), Scope: in.Scope, CreatedAt: now}
	if in.Days > 0 {
		t.ExpiresAt = now.Add(time.Duration(in.Days) * 24 * time.Hour)
	}
	if err := s.Store.Tokens().Create(ctx, t); err != nil {
		return nil, "", err
	}
	s.audit(ctx, u.ID, "token.create", map[string]string{"name": name, "scope": in.Scope})
	s.notify(ctx, u.ID, AlertToken, map[string]string{"name": name, "scope": in.Scope})
	return t, plain, nil
}

// RevokeToken 吊销本人的一个令牌。
func (s *Service) RevokeToken(ctx context.Context, u *store.User, id string) error {
	ts, err := s.Store.Tokens().List(ctx, u.ID)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if t.ID == id {
			if err := s.Store.Tokens().Delete(ctx, u.ID, id); err != nil {
				return err
			}
			s.audit(ctx, u.ID, "token.revoke", map[string]string{"name": t.Name})
			return nil
		}
	}
	return store.ErrNotFound
}

// TokenPrincipal 校验 Bearer 令牌，返回令牌与它的所有者。过期、所有者停用或不存在时返回 ErrNotFound。
// 最近使用时间至多每分钟写一次，避免每个请求都写库。
func (s *Service) TokenPrincipal(ctx context.Context, plain, ip string) (*store.AccessToken, *store.User, error) {
	if !strings.HasPrefix(plain, TokenPrefix) {
		return nil, nil, store.ErrNotFound
	}
	t, err := s.Store.Tokens().ByHash(ctx, auth.HashToken(plain))
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	if !t.ExpiresAt.IsZero() && now.After(t.ExpiresAt) {
		return nil, nil, store.ErrNotFound
	}
	u, err := s.Store.Users().ByID(ctx, t.OwnerID)
	if err != nil || u.Disabled {
		return nil, nil, store.ErrNotFound
	}
	if now.Sub(t.LastUsedAt) >= tokenTouchStep {
		if err := s.Store.Tokens().Touch(ctx, t.ID, now, ip); err != nil && s.Log != nil {
			s.Log.Warn("记录令牌使用时间失败", "error", err)
		}
	}
	return t, u, nil
}
