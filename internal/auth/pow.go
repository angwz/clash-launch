package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 工作量证明（第 14.1 节）：注册与找回前，浏览器计算一个小难题（约 1 秒），挡住批量请求；
// 不依赖任何第三方验证码服务。
//
// 挑战是无状态的：载荷「用途|过期时间|随机数」加 HMAC-SHA256 签名，密钥在进程启动时随机生成
// （重启后未完成的挑战失效，不影响已完成的注册）。客户端找出 nonce，使 SHA-256(挑战 ":" nonce)
// 的前 Bits 位为 0；服务端校验签名、用途、过期，并记住已用过的挑战直到过期，防止重放。

// ErrPoW 表示工作量证明无效、过期或已被使用。
var ErrPoW = errors.New("auth: 工作量证明无效或已过期")

// PoW 签发并校验挑战。零值不可用，用 NewPoW 创建。
type PoW struct {
	key  []byte
	Bits int
	TTL  time.Duration

	mu   sync.Mutex
	used map[string]time.Time // 已使用的挑战 → 过期时间
}

// NewPoW 创建签发器；bits 是要求的前导零位数（默认 20，约一百万次哈希）。
func NewPoW(bits int) *PoW {
	if bits <= 0 {
		bits = 20
	}
	return &PoW{key: []byte(NewToken(32)), Bits: bits, TTL: 5 * time.Minute, used: map[string]time.Time{}}
}

// Challenge 签发一个用于 purpose 的挑战。
func (p *PoW) Challenge(purpose string, now time.Time) string {
	payload := purpose + "|" + strconv.FormatInt(now.Add(p.TTL).Unix(), 10) + "|" + NewToken(12) + "|" + strconv.Itoa(p.Bits)
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + p.sign(payload)
}

func (p *PoW) sign(payload string) string {
	m := hmac.New(sha256.New, p.key)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Verify 校验 challenge 与 nonce；成功后该挑战不能再次使用。
func (p *PoW) Verify(purpose, challenge, nonce string, now time.Time) error {
	dot := strings.LastIndexByte(challenge, '.')
	if dot < 0 || len(nonce) == 0 || len(nonce) > 20 {
		return ErrPoW
	}
	raw, err := base64.RawURLEncoding.DecodeString(challenge[:dot])
	if err != nil {
		return ErrPoW
	}
	payload := string(raw)
	if !hmac.Equal([]byte(p.sign(payload)), []byte(challenge[dot+1:])) {
		return ErrPoW
	}
	parts := strings.Split(payload, "|")
	if len(parts) != 4 || parts[0] != purpose {
		return ErrPoW
	}
	exp, err1 := strconv.ParseInt(parts[1], 10, 64)
	need, err2 := strconv.Atoi(parts[3])
	if err1 != nil || err2 != nil || now.Unix() > exp {
		return ErrPoW
	}
	if _, err := strconv.ParseUint(nonce, 10, 64); err != nil {
		return ErrPoW
	}
	if LeadingZeroBits(sha256.Sum256([]byte(challenge+":"+nonce))) < need {
		return ErrPoW
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, e := range p.used { // 顺带清理过期记录；数量受签发速率与 TTL 限制
		if now.After(e) {
			delete(p.used, k)
		}
	}
	if _, dup := p.used[challenge]; dup {
		return ErrPoW
	}
	p.used[challenge] = time.Unix(exp, 0)
	return nil
}

// LeadingZeroBits 返回哈希的前导零位数。
func LeadingZeroBits(h [32]byte) int {
	n := 0
	for i := 0; i < len(h); i += 8 {
		v := binary.BigEndian.Uint64(h[i : i+8])
		if v != 0 {
			return n + bits.LeadingZeros64(v)
		}
		n += 64
	}
	return n
}

// Solve 在服务端求解（只用于测试与命令行工具；浏览器用 Web Worker 求解）。
func Solve(challenge string, bits int) string {
	for i := uint64(0); ; i++ {
		s := strconv.FormatUint(i, 10)
		if LeadingZeroBits(sha256.Sum256([]byte(challenge+":"+s))) >= bits {
			return s
		}
	}
}
