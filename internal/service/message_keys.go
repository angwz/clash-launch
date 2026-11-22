package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/store"
)

// 站内消息的端到端加密（需求 R46，决定 0009）。服务器只保存公钥与包裹后的私钥，不能解开；
// 这里只做格式与归属的校验：包裹与加密都在浏览器里完成（web/src/lib/e2e.ts）。

// ErrMessageKey 表示密钥或加密消息的格式不对，或引用的密钥不是当前的。
var ErrMessageKey = errors.New("message key: invalid")

const (
	maxPublicKey = 200  // P-256 的 SPKI 为 91 字节，Base64URL 约 122 个字符
	maxWrapped   = 1024 // 包裹后的私钥（JSON：盐、次数、IV、密文）
	maxCipher    = 8192 // 加密后的消息正文（Base64URL）：原文至多 2000 字
)

// WrappedKey 是包裹后的私钥。服务器只检查它是 JSON 对象且不超过长度上限。
type WrappedKey = json.RawMessage

// MessageKeyInput 是上传的密钥。Replace 为真或还没有密钥时生成新密钥（新的编号）；否则只更新当前密钥的包裹。
type MessageKeyInput struct {
	KeyID     string // 更新包裹时：当前密钥的编号
	PublicKey string
	Wrapped   WrappedKey
	Recovery  WrappedKey // 可为空；更新包裹时为空表示不改
	Replace   bool
}

func checkWrapped(w WrappedKey, required bool) error {
	if len(w) == 0 {
		if required {
			return ErrMessageKey
		}
		return nil
	}
	var obj map[string]any
	if len(w) > maxWrapped || json.Unmarshal(w, &obj) != nil {
		return ErrMessageKey
	}
	return nil
}

func checkB64(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil
}

// SetMessageKey 生成或更新消息密钥。需要当前密码：有了会话也不能悄悄换掉别人的密钥，让之后的消息发给攻击者的公钥。
func (s *Service) SetMessageKey(ctx context.Context, u *store.User, password string, in MessageKeyInput) (*store.MessageKey, error) {
	if !auth.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrWrongPassword
	}
	if err := checkWrapped(in.Wrapped, true); err != nil {
		return nil, err
	}
	if err := checkWrapped(in.Recovery, false); err != nil {
		return nil, err
	}
	cur, err := s.Store.MessageKeys().Active(ctx, u.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if cur == nil || in.Replace {
		if !checkB64(in.PublicKey, maxPublicKey) {
			return nil, ErrMessageKey
		}
		k := &store.MessageKey{UserID: u.ID, KeyID: store.NewID(), PublicKey: in.PublicKey, Wrapped: string(in.Wrapped), Recovery: string(in.Recovery), CreatedAt: s.now()}
		if err := s.Store.MessageKeys().Replace(ctx, k); err != nil {
			return nil, err
		}
		action := "account.message_key"
		if cur != nil {
			action = "account.message_key_reset"
		}
		s.audit(ctx, u.ID, action, nil)
		return k, nil
	}
	if in.KeyID != cur.KeyID {
		return nil, ErrMessageKey // 包裹的是另一把（已被替换的）密钥
	}
	var rec *string
	if len(in.Recovery) > 0 {
		r := string(in.Recovery)
		rec = &r
	}
	if err := s.Store.MessageKeys().Rewrap(ctx, u.ID, cur.KeyID, string(in.Wrapped), rec); err != nil {
		return nil, err
	}
	return s.Store.MessageKeys().Active(ctx, u.ID)
}

// EncText 是端到端加密的消息正文（决定 0009 #4）。服务器不解密，只校验格式与密钥归属。
type EncText struct {
	V         int    `json:"v"`
	SenderKey string `json:"sk"` // 发件人的密钥编号
	RcptKey   string `json:"rk"` // 收件人的密钥编号
	IV        string `json:"iv"`
	CT        string `json:"ct"`
}

// checkEnc 校验加密正文：版本、长度，且两把密钥都是双方当前的密钥（防止发给已作废的密钥，对方解不开）。
func (s *Service) checkEnc(ctx context.Context, e *EncText, from, to string) error {
	if e.V != 1 || !checkB64(e.IV, 32) || !checkB64(e.CT, maxCipher) {
		return ErrMessageKey
	}
	sk, err := s.Store.MessageKeys().Active(ctx, from)
	if err != nil || sk.KeyID != e.SenderKey {
		return ErrMessageKey
	}
	rk, err := s.Store.MessageKeys().Active(ctx, to)
	if err != nil || rk.KeyID != e.RcptKey {
		return ErrMessageKey
	}
	return nil
}
