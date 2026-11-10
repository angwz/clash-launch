package auth

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// 软件认证器：按规范构造 attestationObject 与断言，覆盖每一项检查。
type softAuthenticator struct {
	rpID   string
	credID []byte
	ec     *ecdsa.PrivateKey
	ed     ed25519.PrivateKey
	count  uint32
}

func (a *softAuthenticator) coseKey() []byte {
	var m map[int]any
	if a.ed != nil {
		m = map[int]any{1: 1, 3: AlgEdDSA, -1: 6, -2: []byte(a.ed.Public().(ed25519.PublicKey))}
	} else {
		x, y := a.ec.X.FillBytes(make([]byte, 32)), a.ec.Y.FillBytes(make([]byte, 32))
		m = map[int]any{1: 2, 3: AlgES256, -1: 1, -2: x, -3: y}
	}
	b, _ := cbor.Marshal(m)
	return b
}

func (a *softAuthenticator) authData(flags byte, withCred bool) []byte {
	h := sha256.Sum256([]byte(a.rpID))
	b := append([]byte{}, h[:]...)
	b = append(b, flags)
	b = binary.BigEndian.AppendUint32(b, a.count)
	if withCred {
		b = append(b, make([]byte, 16)...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(a.credID))) //nolint:gosec
		b = append(b, a.credID...)
		b = append(b, a.coseKey()...)
	}
	return b
}

func clientJSON(typ string, challenge []byte, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": B64(challenge), "origin": origin, "crossOrigin": false})
	return b
}

func (a *softAuthenticator) register(challenge []byte, origin string, flags byte) ([]byte, []byte) {
	att, _ := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": a.authData(flags, true)})
	return clientJSON("webauthn.create", challenge, origin), att
}

func (a *softAuthenticator) assert(challenge []byte, origin string, flags byte) (cd, ad, sig []byte) {
	a.count++
	cd = clientJSON("webauthn.get", challenge, origin)
	ad = a.authData(flags, false)
	h := sha256.Sum256(cd)
	msg := append(append([]byte{}, ad...), h[:]...)
	if a.ed != nil {
		sig = ed25519.Sign(a.ed, msg)
	} else {
		d := sha256.Sum256(msg)
		sig, _ = ecdsa.SignASN1(rand.Reader, a.ec, d[:])
	}
	return cd, ad, sig
}

func TestWebAuthn(t *testing.T) {
	rp := RelyingParty{ID: "cl.example.com", Name: "Clash, Launch!", Origin: "https://cl.example.com"}
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	_, ed, _ := ed25519.GenerateKey(rand.Reader)
	for name, a := range map[string]*softAuthenticator{
		"ES256": {rpID: rp.ID, credID: []byte("cred-ec-0001"), ec: ec},
		"EdDSA": {rpID: rp.ID, credID: []byte("cred-ed-0001"), ed: ed},
	} {
		t.Run(name, func(t *testing.T) {
			ch := []byte("registration-challenge-32-bytes!")
			cd, att := a.register(ch, rp.Origin, flagUP|flagUV|flagAT)
			cred, err := rp.VerifyRegistration(ch, cd, att)
			if err != nil {
				t.Fatalf("注册：%v", err)
			}
			if string(cred.ID) != string(a.credID) {
				t.Fatal("凭据 ID")
			}
			// 注册的反例
			bad := map[string]func() ([]byte, []byte, []byte){
				"挑战不同": func() ([]byte, []byte, []byte) {
					c, at := a.register([]byte("other"), rp.Origin, flagUP|flagUV|flagAT)
					return ch, c, at
				},
				"来源不同": func() ([]byte, []byte, []byte) {
					c, at := a.register(ch, "https://evil.example.com", flagUP|flagUV|flagAT)
					return ch, c, at
				},
				"缺少 UV": func() ([]byte, []byte, []byte) { c, at := a.register(ch, rp.Origin, flagUP|flagAT); return ch, c, at },
			}
			for n, f := range bad {
				c0, c1, c2 := f()
				if _, err := rp.VerifyRegistration(c0, c1, c2); !errors.Is(err, ErrWebAuthn) {
					t.Errorf("%s：应拒绝（%v）", n, err)
				}
			}
			other := RelyingParty{ID: "evil.example.com", Origin: rp.Origin}
			if _, err := other.VerifyRegistration(ch, cd, att); err == nil {
				t.Error("RP ID 不同应拒绝")
			}

			// 认证
			ach := []byte("assertion-challenge")
			cd2, ad2, sig := a.assert(ach, rp.Origin, flagUP|flagUV)
			n, err := rp.VerifyAssertion(ach, cred.PublicKey, cred.SignCount, cd2, ad2, sig)
			if err != nil || n != a.count {
				t.Fatalf("认证：%d %v", n, err)
			}
			// 重放（计数不增长）
			if _, err := rp.VerifyAssertion(ach, cred.PublicKey, n, cd2, ad2, sig); err == nil {
				t.Error("签名计数不增长应拒绝")
			}
			// 篡改签名
			cd3, ad3, sig3 := a.assert(ach, rp.Origin, flagUP|flagUV)
			sig3[len(sig3)-1] ^= 1
			if _, err := rp.VerifyAssertion(ach, cred.PublicKey, n, cd3, ad3, sig3); err == nil {
				t.Error("签名被篡改应拒绝")
			}
			// 类型错误：用注册的 clientData 冒充断言
			cd4, ad4, sig4 := a.assert(ach, rp.Origin, flagUP|flagUV)
			if _, err := rp.VerifyAssertion(ach, cred.PublicKey, n, clientJSON("webauthn.create", ach, rp.Origin), ad4, sig4); err == nil {
				t.Error("类型错误应拒绝")
			}
			_ = cd4
			// 缺少 UV
			cd5, ad5, sig5 := a.assert(ach, rp.Origin, flagUP)
			if _, err := rp.VerifyAssertion(ach, cred.PublicKey, n, cd5, ad5, sig5); err == nil {
				t.Error("缺少 UV 应拒绝")
			}
		})
	}
}

func TestParseCOSEKeyRejects(t *testing.T) {
	bad, _ := cbor.Marshal(map[int]any{1: 2, 3: -35, -1: 2}) // ES384：不支持
	if _, _, err := parseCOSEKey(bad); err == nil {
		t.Error("不支持的算法应拒绝")
	}
	notOnCurve, _ := cbor.Marshal(map[int]any{1: 2, 3: AlgES256, -1: 1, -2: make([]byte, 32), -3: make([]byte, 32)})
	if _, _, err := parseCOSEKey(notOnCurve); err == nil {
		t.Error("不在曲线上的点应拒绝")
	}
	if _, err := parseAuthData(make([]byte, 20), false); err == nil {
		t.Error("过短的认证器数据应拒绝")
	}
}
