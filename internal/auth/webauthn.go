package auth

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/fxamacker/cbor/v2"
)

// 通行密钥（WebAuthn Level 2）的服务端验证，只覆盖本项目需要的部分（决定 0005）：
//   - 不验证认证器的来源证明：请求 attestation: "none"，收到其他格式时同样忽略 attStmt；
//   - 公钥算法：ES256（-7）、EdDSA/Ed25519（-8）、RS256（-257）；
//   - 要求用户在场（UP）与用户验证（UV）；签名计数不得回退（克隆的认证器）。
// 每一步对应 W3C 规范 §7.1（注册）与 §7.2（认证）的编号，见各函数注释。

// ErrWebAuthn 表示通行密钥的数据无效或验证失败。
var ErrWebAuthn = errors.New("auth: 通行密钥验证失败")

func waErr(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrWebAuthn}, a...)...)
}

// RelyingParty 是依赖方：RPID 为域名（不含端口），Origin 为 scheme://host[:port]。
type RelyingParty struct {
	ID     string
	Name   string
	Origin string
}

// 认证器数据的标志位。
const (
	flagUP = 0x01 // 用户在场
	flagUV = 0x04 // 用户验证
	flagAT = 0x40 // 包含凭据数据
	flagED = 0x80 // 包含扩展
)

// COSE 算法编号。
const (
	AlgES256 = -7
	AlgEdDSA = -8
	AlgRS256 = -257
)

var b64u = base64.RawURLEncoding

// clientData 是 clientDataJSON 中需要检查的字段。
type clientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

// checkClientData 对应 §7.1 步骤 7–10 与 §7.2 步骤 11–14：类型、挑战、来源。
func (rp RelyingParty) checkClientData(raw []byte, typ string, challenge []byte) error {
	var cd clientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return waErr("clientDataJSON 无法解析")
	}
	if cd.Type != typ {
		return waErr("类型为 %q，期望 %q", cd.Type, typ)
	}
	got, err := b64u.DecodeString(cd.Challenge)
	if err != nil || subtle.ConstantTimeCompare(got, challenge) != 1 {
		return waErr("挑战不匹配")
	}
	if cd.Origin != rp.Origin || cd.CrossOrigin {
		return waErr("来源 %q 不匹配", cd.Origin)
	}
	return nil
}

// authData 是解析后的认证器数据。
type authData struct {
	rpIDHash  []byte
	flags     byte
	signCount uint32
	credID    []byte
	publicKey []byte // COSE_Key 原文
}

func parseAuthData(b []byte, wantCred bool) (*authData, error) {
	if len(b) < 37 {
		return nil, waErr("认证器数据过短")
	}
	ad := &authData{rpIDHash: b[:32], flags: b[32], signCount: binary.BigEndian.Uint32(b[33:37])}
	rest := b[37:]
	if ad.flags&flagAT != 0 {
		if len(rest) < 18 {
			return nil, waErr("凭据数据过短")
		}
		n := int(binary.BigEndian.Uint16(rest[16:18])) // 跳过 16 字节的 AAGUID
		rest = rest[18:]
		if n == 0 || n > 1023 || len(rest) < n {
			return nil, waErr("凭据 ID 长度无效")
		}
		ad.credID, rest = rest[:n], rest[n:]
		// 公钥是一个 CBOR 项，其后可能紧跟扩展：用流式解码得到它的长度
		dec := cbor.NewDecoder(bytes.NewReader(rest))
		var raw cbor.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, waErr("公钥无法解析")
		}
		ad.publicKey, rest = rest[:dec.NumBytesRead()], rest[dec.NumBytesRead():]
	} else if wantCred {
		return nil, waErr("缺少凭据数据")
	}
	if ad.flags&flagED == 0 && len(rest) != 0 {
		return nil, waErr("认证器数据有多余的字节")
	}
	return ad, nil
}

// checkAuthData 对应 §7.1 步骤 13–15 与 §7.2 步骤 15–17：RP ID 哈希、UP、UV。
func (rp RelyingParty) checkAuthData(ad *authData) error {
	want := sha256.Sum256([]byte(rp.ID))
	if subtle.ConstantTimeCompare(ad.rpIDHash, want[:]) != 1 {
		return waErr("RP ID 不匹配")
	}
	if ad.flags&flagUP == 0 {
		return waErr("缺少用户在场标志")
	}
	if ad.flags&flagUV == 0 {
		return waErr("缺少用户验证标志")
	}
	return nil
}

// Credential 是注册得到的凭据。
type Credential struct {
	ID        []byte
	PublicKey []byte // COSE_Key
	Alg       int
	SignCount uint32
}

// VerifyRegistration 验证 navigator.credentials.create() 的结果（§7.1）。
func (rp RelyingParty) VerifyRegistration(challenge, clientDataJSON, attestationObject []byte) (*Credential, error) {
	if err := rp.checkClientData(clientDataJSON, "webauthn.create", challenge); err != nil {
		return nil, err
	}
	var att struct {
		Fmt      string          `cbor:"fmt"`
		AttStmt  cbor.RawMessage `cbor:"attStmt"`
		AuthData []byte          `cbor:"authData"`
	}
	if err := cbor.Unmarshal(attestationObject, &att); err != nil {
		return nil, waErr("attestationObject 无法解析")
	}
	ad, err := parseAuthData(att.AuthData, true)
	if err != nil {
		return nil, err
	}
	if err := rp.checkAuthData(ad); err != nil {
		return nil, err
	}
	alg, _, err := parseCOSEKey(ad.publicKey)
	if err != nil {
		return nil, err
	}
	// 步骤 19–21（来源证明）按 attestation: "none" 处理：不验证 attStmt，不依赖认证器型号。
	return &Credential{ID: ad.credID, PublicKey: ad.publicKey, Alg: alg, SignCount: ad.signCount}, nil
}

// VerifyAssertion 验证 navigator.credentials.get() 的结果（§7.2），返回新的签名计数。
func (rp RelyingParty) VerifyAssertion(challenge, publicKey []byte, storedCount uint32, clientDataJSON, authenticatorData, signature []byte) (uint32, error) {
	if err := rp.checkClientData(clientDataJSON, "webauthn.get", challenge); err != nil {
		return 0, err
	}
	ad, err := parseAuthData(authenticatorData, false)
	if err != nil {
		return 0, err
	}
	if err := rp.checkAuthData(ad); err != nil {
		return 0, err
	}
	// 步骤 19–20：签名覆盖 authenticatorData ‖ SHA-256(clientDataJSON)
	ch := sha256.Sum256(clientDataJSON)
	signed := append(append([]byte{}, authenticatorData...), ch[:]...)
	alg, pub, err := parseCOSEKey(publicKey)
	if err != nil {
		return 0, err
	}
	if err := verifySignature(alg, pub, signed, signature); err != nil {
		return 0, err
	}
	// 步骤 21：计数都不为 0 时必须增长，否则可能是克隆的认证器
	if (ad.signCount != 0 || storedCount != 0) && ad.signCount <= storedCount {
		return 0, waErr("签名计数没有增长（%d ≤ %d）", ad.signCount, storedCount)
	}
	return ad.signCount, nil
}

// parseCOSEKey 解析 COSE_Key（RFC 9053），返回算法与公钥。
func parseCOSEKey(b []byte) (int, crypto.PublicKey, error) {
	var m map[int]any
	if err := cbor.Unmarshal(b, &m); err != nil {
		return 0, nil, waErr("COSE 公钥无法解析")
	}
	kty, _ := asInt(m[1])
	alg, _ := asInt(m[3])
	switch {
	case alg == AlgES256 && kty == 2:
		crv, _ := asInt(m[-1])
		x, _ := m[-2].([]byte)
		y, _ := m[-3].([]byte)
		if crv != 1 || len(x) != 32 || len(y) != 32 {
			return 0, nil, waErr("ES256 公钥无效")
		}
		// 未压缩点 0x04 ‖ x ‖ y；ParseUncompressedPublicKey 同时检查点在曲线上
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			return 0, nil, waErr("ES256 公钥不在曲线上")
		}
		return alg, pub, nil
	case alg == AlgEdDSA && kty == 1:
		crv, _ := asInt(m[-1])
		x, _ := m[-2].([]byte)
		if crv != 6 || len(x) != ed25519.PublicKeySize {
			return 0, nil, waErr("Ed25519 公钥无效")
		}
		return alg, ed25519.PublicKey(x), nil
	case alg == AlgRS256 && kty == 3:
		n, _ := m[-1].([]byte)
		e, _ := m[-2].([]byte)
		if len(n) < 256 || len(e) == 0 || len(e) > 4 {
			return 0, nil, waErr("RS256 公钥无效（至少 2048 位）")
		}
		ev := 0
		for _, c := range e {
			ev = ev<<8 | int(c)
		}
		return alg, &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: ev}, nil
	}
	return 0, nil, waErr("不支持的公钥算法 %d", alg)
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int64:
		return int(x), true
	case uint64:
		return int(x), true //nolint:gosec // COSE 的整数键值很小
	}
	return 0, false
}

func verifySignature(alg int, pub crypto.PublicKey, data, sig []byte) error {
	switch alg {
	case AlgES256:
		h := sha256.Sum256(data)
		if ecdsa.VerifyASN1(pub.(*ecdsa.PublicKey), h[:], sig) {
			return nil
		}
	case AlgEdDSA:
		if ed25519.Verify(pub.(ed25519.PublicKey), data, sig) {
			return nil
		}
	case AlgRS256:
		h := sha256.Sum256(data)
		if rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, h[:], sig) == nil {
			return nil
		}
	}
	return waErr("签名无效")
}

// B64 编解码 base64url（WebAuthn 的二进制字段在 JSON 中的形式）。
func B64(b []byte) string { return b64u.EncodeToString(b) }

// UnB64 解码 base64url；容忍带填充的输入。
func UnB64(s string) ([]byte, error) {
	return b64u.DecodeString(stripPad(s))
}

func stripPad(s string) string {
	for len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return s
}
