package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Ticket 是短期有效的签名凭证：口令正确后、第二因素完成前，用它把「这个用户已通过第一步」
// 带到下一次请求，而不建立会话。无状态：载荷「用途|主体|过期时间」加 HMAC-SHA256；
// 密钥在进程启动时随机生成，重启后未完成的登录需要重新输入口令。

// ErrTicket 表示凭证无效或过期。
var ErrTicket = errors.New("auth: 凭证无效或已过期")

// Tickets 签发并校验凭证。
type Tickets struct{ key []byte }

// NewTickets 使用随机密钥创建签发器。
func NewTickets() *Tickets { return &Tickets{key: []byte(NewToken(32))} }

// Issue 签发 purpose 用途、主体为 subject、ttl 后过期的凭证。
func (t *Tickets) Issue(purpose, subject string, ttl time.Duration, now time.Time) string {
	payload := purpose + "|" + subject + "|" + strconv.FormatInt(now.Add(ttl).Unix(), 10)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + t.sign(payload)
}

func (t *Tickets) sign(payload string) string {
	m := hmac.New(sha256.New, t.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Verify 校验凭证并返回主体。
func (t *Tickets) Verify(purpose, ticket string, now time.Time) (string, error) {
	dot := strings.LastIndexByte(ticket, '.')
	if dot < 0 {
		return "", ErrTicket
	}
	raw, err := base64.RawURLEncoding.DecodeString(ticket[:dot])
	if err != nil || !hmac.Equal([]byte(t.sign(string(raw))), []byte(ticket[dot+1:])) {
		return "", ErrTicket
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 || parts[0] != purpose {
		return "", ErrTicket
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.Unix() > exp {
		return "", ErrTicket
	}
	return parts[1], nil
}
