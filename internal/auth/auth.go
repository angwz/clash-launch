// Package auth 提供账号安全的基础构件：口令哈希（argon2id）、口令策略、随机令牌与其哈希、常数时间比较。
// 会话与 CSRF 的 HTTP 细节在 internal/api。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2 参数：OWASP Password Storage Cheat Sheet 推荐的 argon2id 配置之一（m=19 MiB, t=2, p=1）。
// 取舍与依据见 docs/decisions/0002。参数写在哈希串中，日后调高不影响已有哈希的验证。
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrWeakPassword 表示口令不满足策略。
var ErrWeakPassword = errors.New("auth: 口令不满足要求")

// 口令策略（反馈 #17：个人项目，口令可以简单）：至少 MinPasswordLen 个字符，且包含至少 MinPasswordClasses 类字符。
// 弱口令的风险由登录退避与按 IP 限流兜底（见 backoff.go）。
const (
	MinPasswordLen     = 5
	MinPasswordClasses = 2
)

// CheckPassword 检查口令策略：长度、字符类别（小写字母、大写字母、数字、其他字符各算一类），
// 并拒绝随程序发布的常见口令表中的口令（不区分大小写，不联网）。
func CheckPassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return fmt.Errorf("%w：至少 %d 个字符", ErrWeakPassword, MinPasswordLen)
	}
	if PasswordClasses(pw) < MinPasswordClasses {
		return fmt.Errorf("%w：至少包含 %d 类字符（小写字母、大写字母、数字、其他字符）", ErrWeakPassword, MinPasswordClasses)
	}
	if len(pw) > 1024 {
		return fmt.Errorf("%w：过长", ErrWeakPassword)
	}
	if IsCommonPassword(pw) {
		return fmt.Errorf("%w：这是常见口令", ErrWeakPassword)
	}
	return nil
}

// PasswordClasses 返回口令包含的字符类别数：ASCII 小写字母、ASCII 大写字母、数字、其他字符（含空格与中文）。
func PasswordClasses(pw string) int {
	var lower, upper, digit, other bool
	for _, r := range pw {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			other = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			n++
		}
	}
	return n
}

// HashPassword 生成 PHC 格式的 argon2id 哈希：$argon2id$v=19$m=…,t=…,p=…$盐$哈希。
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// VerifyPassword 校验口令（常数时间比较）。哈希格式错误时返回 false。
func VerifyPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 || m > 1<<22 || t > 64 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[4])
	want, err2 := enc.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 || len(want) > 128 {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want))) //nolint:gosec // 长度已限制在 128 以内
	return subtle.ConstantTimeCompare(got, want) == 1
}

// DummyVerify 在用户不存在时执行一次同等开销的计算，避免以响应时间区分「用户不存在」与「口令错误」。
func DummyVerify(pw string) {
	_ = argon2.IDKey([]byte(pw), make([]byte, saltLen), argonTime, argonMemory, argonThreads, argonKeyLen)
}

// NewToken 返回 n 字节随机数的 base64url 编码（无填充）。订阅链接令牌为 32 字节（256 位，43 个字符）。
func NewToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: 随机数不可用：" + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken 返回令牌的 SHA-256（十六进制），用于存储与查找。
func HashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

// Equal 以常数时间比较两个字符串。
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// LogToken 返回只含前 6 个字符的令牌，用于日志（第 13.2 节）。
func LogToken(tok string) string {
	if len(tok) <= 6 {
		return "***"
	}
	return tok[:6] + "…"
}
