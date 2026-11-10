package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/angwz/clash-launch/internal/auth"
)

// webAuthnDevice 是测试用的软件认证器（ES256）：按服务端给的选项生成注册与断言。
type webAuthnDevice struct {
	t      *testing.T
	origin string
	rpID   string
	key    *ecdsa.PrivateKey
	credID []byte
	userID []byte
	count  uint32
}

func newDevice(t *testing.T, publicURL string) *webAuthnDevice {
	u, _ := url.Parse(publicURL)
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &webAuthnDevice{t: t, origin: u.Scheme + "://" + u.Host, rpID: u.Hostname(), key: k, credID: id}
}

func (d *webAuthnDevice) authData(withCred bool) []byte {
	h := sha256.Sum256([]byte(d.rpID))
	b := append(append([]byte{}, h[:]...), 0x01|0x04|map[bool]byte{true: 0x40}[withCred])
	b = binary.BigEndian.AppendUint32(b, d.count)
	if withCred {
		b = append(b, make([]byte, 16)...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(d.credID)))
		b = append(b, d.credID...)
		pk, _ := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: d.key.X.FillBytes(make([]byte, 32)), -3: d.key.Y.FillBytes(make([]byte, 32))})
		b = append(b, pk...)
	}
	return b
}

func (d *webAuthnDevice) clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": d.origin})
	return b
}

func (d *webAuthnDevice) create(opts map[string]any) map[string]string {
	d.userID, _ = auth.UnB64(opts["user"].(map[string]any)["id"].(string))
	att, _ := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": d.authData(true)})
	return map[string]string{"clientDataJSON": auth.B64(d.clientData("webauthn.create", opts["challenge"].(string))), "attestationObject": auth.B64(att)}
}

func (d *webAuthnDevice) get(opts map[string]any) map[string]string {
	d.count++
	cd := d.clientData("webauthn.get", opts["challenge"].(string))
	ad := d.authData(false)
	h := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), h[:]...))
	sig, _ := ecdsa.SignASN1(rand.Reader, d.key, digest[:])
	return map[string]string{"id": auth.B64(d.credID), "clientDataJSON": auth.B64(cd), "authenticatorData": auth.B64(ad), "signature": auth.B64(sig), "userHandle": auth.B64(d.userID)}
}

func TestPasskeys(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	const pw = "correct horse battery"
	dev := newDevice(t, h.http.URL)

	b := decode[beginResp](t, admin.want(200, "POST", "/api/v1/account/passkeys/begin", nil))
	if b.Options["authenticatorSelection"].(map[string]any)["userVerification"] != "required" {
		t.Errorf("应要求用户验证：%v", b.Options)
	}
	admin.want(201, "POST", "/api/v1/account/passkeys", map[string]any{"ticket": b.Ticket, "name": "测试设备", "credential": dev.create(b.Options)})
	// 同一个注册凭证不能再用
	admin.want(401, "POST", "/api/v1/account/passkeys", map[string]any{"ticket": b.Ticket, "name": "x", "credential": dev.create(b.Options)})
	list := decode[struct {
		Items     []passkeyDTO `json:"items"`
		Available bool         `json:"available"`
	}](t, admin.want(200, "GET", "/api/v1/account/passkeys", nil))
	if len(list.Items) != 1 || !list.Available || list.Items[0].Name != "测试设备" {
		t.Fatalf("列表：%+v", list)
	}

	// 免密码登录；同一断言不能重放
	c := h.client()
	lb := decode[beginResp](t, c.want(200, "POST", "/api/v1/session/passkey/begin", map[string]string{}))
	assertion := dev.get(lb.Options)
	c.csrf = decode[sessionResp](t, c.want(200, "POST", "/api/v1/session/passkey", map[string]any{"ticket": lb.Ticket, "credential": assertion})).CSRF
	c.want(200, "GET", "/api/v1/me", nil)
	h.client().want(401, "POST", "/api/v1/session/passkey", map[string]any{"ticket": lb.Ticket, "credential": assertion})

	// 只有通行密钥时，口令登录不要求第二步
	plain := h.client()
	plain.login("admin", pw)

	// 启用 TOTP 后，第二步可以用通行密钥代替验证码
	st := decode[map[string]string](t, admin.want(200, "POST", "/api/v1/account/totp/setup", nil))
	code, _ := auth.TOTPCode(st["secret"], auth.TOTPStep(time.Now()))
	admin.want(200, "POST", "/api/v1/account/totp/enable", map[string]string{"code": code})
	m := h.client()
	mr := decode[mfaResp](t, m.want(200, "POST", "/api/v1/session", map[string]string{"username": "admin", "password": pw}))
	if len(mr.MFA) != 2 || mr.MFA[1] != "passkey" {
		t.Fatalf("第二步方式：%v", mr.MFA)
	}
	mb := decode[beginResp](t, m.want(200, "POST", "/api/v1/session/passkey/begin", map[string]string{"ticket": mr.Ticket}))
	if allow := mb.Options["allowCredentials"].([]any); len(allow) != 1 {
		t.Errorf("第二步应只允许该用户的凭据：%v", allow)
	}
	m.want(200, "POST", "/api/v1/session/passkey", map[string]any{"ticket": mb.Ticket, "credential": dev.get(mb.Options)})

	// 别人的通行密钥不能用作该用户的第二步
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "mallory", ""))
	mal := h.client()
	mal.login("mallory", "a long enough password")
	malDev := newDevice(t, h.http.URL)
	mb2 := decode[beginResp](t, mal.want(200, "POST", "/api/v1/account/passkeys/begin", nil))
	mal.want(201, "POST", "/api/v1/account/passkeys", map[string]any{"ticket": mb2.Ticket, "credential": malDev.create(mb2.Options)})
	x := h.client()
	xr := decode[mfaResp](t, x.want(200, "POST", "/api/v1/session", map[string]string{"username": "admin", "password": pw}))
	xb := decode[beginResp](t, x.want(200, "POST", "/api/v1/session/passkey/begin", map[string]string{"ticket": xr.Ticket}))
	x.want(401, "POST", "/api/v1/session/passkey", map[string]any{"ticket": xb.Ticket, "credential": malDev.get(xb.Options)})

	// 删除需要口令
	id := list.Items[0].ID
	admin.want(403, "POST", "/api/v1/account/passkeys/"+id+"/delete", map[string]string{"password": "wrong"})
	mal.want(404, "POST", "/api/v1/account/passkeys/"+id+"/delete", map[string]string{"password": "a long enough password"})
	admin.want(204, "POST", "/api/v1/account/passkeys/"+id+"/delete", map[string]string{"password": pw})
}
