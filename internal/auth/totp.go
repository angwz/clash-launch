package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 的默认算法；认证器应用普遍只支持 SHA-1，HMAC-SHA1 在此用途下仍是安全的
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP 两步验证（RFC 6238）：HMAC-SHA1、30 秒、6 位。验证时允许前后各一个时间步的时钟偏差；
// 同一时间步只能使用一次（由存储层的 last_step 保证，见 store.TOTPRepo.Advance）。

const (
	totpPeriod = 30
	totpDigits = 6
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret 生成 160 位随机密钥（base32，无填充）。
func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err) // 系统随机数不可用时无法安全继续
	}
	return b32.EncodeToString(b)
}

// TOTPStep 返回时刻 t 所在的时间步。
func TOTPStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// TOTPCode 计算密钥在时间步 step 的验证码。
func TOTPCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("auth: TOTP 密钥无效: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // 时间步非负
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1_000_000), nil
}

// VerifyTOTP 检查验证码，返回匹配的时间步；不匹配时 ok 为 false。比较为常数时间。
func VerifyTOTP(secret, code string, now time.Time) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != totpDigits {
		return 0, false
	}
	cur := TOTPStep(now)
	for _, s := range []int64{cur, cur - 1, cur + 1} {
		want, err := TOTPCode(secret, s)
		if err == nil && hmac.Equal([]byte(want), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}

// TOTPURI 生成认证器应用使用的 otpauth:// 地址（Key Uri Format）。
func TOTPURI(secret, issuer, account string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// NewRecoveryCodes 生成 n 个一次性恢复码（形如 abcd-efgh，32 位熵），返回原文与存储用的哈希。
func NewRecoveryCodes(n int) (codes, hashes []string) {
	enc := base32.NewEncoding("abcdefghijkmnpqrstuvwxyz23456789").WithPadding(base32.NoPadding) // 去掉易混淆的 l、o、0、1
	for range n {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		s := enc.EncodeToString(b)
		code := s[:4] + "-" + s[4:]
		codes = append(codes, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes
}

// HashRecoveryCode 规范化（小写、去掉空白与连字符）后取 SHA-256。
func HashRecoveryCode(code string) string {
	c := strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	h := sha256.Sum256([]byte(c))
	return hex.EncodeToString(h[:])
}
