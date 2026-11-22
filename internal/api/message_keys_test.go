package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

// R46（决定 0009）：消息密钥需要密码才能设置；加密消息只能用双方当前的密钥，服务器原样转交密文；
// 改密码时与新的包裹一起更新，包裹不对时整个修改作废；重新生成后旧公钥保留、旧私钥的包裹删除。
func TestMessageKeys(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	const apw = "correct horse battery"
	const bpw = "a long enough password"
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", bpw)
	pub := func() string {
		b := make([]byte, 91)
		_, _ = rand.Read(b)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	b64 := func(n int) string { return base64.RawURLEncoding.EncodeToString(make([]byte, n)) }
	type key struct {
		KeyID   string         `json:"keyId"`
		Active  bool           `json:"active"`
		Wrapped map[string]any `json:"wrapped"`
	}

	if b := string(bob.want(200, "GET", "/api/v1/account/message-key", nil)); !strings.Contains(b, `"key":null`) {
		t.Fatalf("还没有密钥：%s", b)
	}
	bob.want(403, "PUT", "/api/v1/account/message-key", map[string]any{"password": "wrong password!!", "publicKey": pub(), "wrapped": map[string]any{"v": 1}})
	bob.want(422, "PUT", "/api/v1/account/message-key", map[string]any{"password": bpw, "publicKey": "not base64!", "wrapped": map[string]any{"v": 1}})
	bk := decode[key](t, bob.want(200, "PUT", "/api/v1/account/message-key", map[string]any{"password": bpw, "publicKey": pub(), "wrapped": map[string]any{"v": 1, "ct": "a"}}))
	ak := decode[key](t, admin.want(200, "PUT", "/api/v1/account/message-key", map[string]any{"password": apw, "publicKey": pub(), "wrapped": map[string]any{"v": 1}}))

	// 加密消息：服务器原样转交，正文为空；会话列表标为加密
	enc := map[string]any{"v": 1, "sk": ak.KeyID, "rk": bk.KeyID, "iv": b64(12), "ct": b64(40)}
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "enc": enc})
	got := string(bob.want(200, "GET", "/api/v1/messages/with/admin", nil))
	if !strings.Contains(got, `"ct":"`+b64(40)+`"`) || !strings.Contains(got, `"text":""`) {
		t.Errorf("收件人看到的消息：%s", got)
	}
	if b := string(bob.want(200, "GET", "/api/v1/messages", nil)); !strings.Contains(b, `"lastEnc":true`) || !strings.Contains(b, `"last":""`) {
		t.Errorf("会话列表：%s", b)
	}
	// 密钥不是当前的、同时带明文、格式不对：拒绝
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "bob", "enc": map[string]any{"v": 1, "sk": ak.KeyID, "rk": "old", "iv": b64(12), "ct": b64(8)}})
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "明文", "enc": enc})
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "bob", "enc": map[string]any{"v": 2, "sk": ak.KeyID, "rk": bk.KeyID, "iv": b64(12), "ct": b64(8)}})

	// 改密码：包裹的不是当前密钥时整个修改作废（旧密码仍然有效）；正确时新包裹与新密码一起生效
	bob.want(422, "PUT", "/api/v1/account/password", map[string]any{"current": bpw, "password": "another long password", "messageKey": map[string]any{"keyId": "nope", "wrapped": map[string]any{"v": 1, "ct": "new"}}})
	h.client().login("bob", bpw)
	bob.want(204, "PUT", "/api/v1/account/password", map[string]any{"current": bpw, "password": "another long password", "messageKey": map[string]any{"keyId": bk.KeyID, "wrapped": map[string]any{"v": 1, "ct": "new"}}})
	if k := decode[struct{ Key key }](t, bob.want(200, "GET", "/api/v1/account/message-key", nil)).Key; k.Wrapped["ct"] != "new" {
		t.Errorf("改密码后的包裹：%+v", k)
	}

	// 重新生成：旧公钥保留、不再使用，旧私钥的包裹删除；发往旧密钥的消息被拒绝
	nk := decode[key](t, bob.want(200, "PUT", "/api/v1/account/message-key", map[string]any{"password": "another long password", "publicKey": pub(), "wrapped": map[string]any{"v": 1}, "replace": true}))
	keys := decode[struct{ Items []key }](t, admin.want(200, "GET", "/api/v1/users/bob/message-keys", nil)).Items
	if len(keys) != 2 || keys[0].KeyID != nk.KeyID || !keys[0].Active || keys[1].Active || keys[1].Wrapped != nil {
		t.Errorf("密钥历史：%+v", keys)
	}
	all, _ := h.db.MessageKeys().All(context.Background(), mustUserIDByName(t, h, "bob"))
	for _, k := range all {
		if !k.Active && (k.Wrapped != "" || k.Recovery != "") {
			t.Errorf("旧密钥的私钥包裹应删除：%+v", k)
		}
	}
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "bob", "enc": enc})
}

func mustUserIDByName(t *testing.T, h *harness, name string) string {
	t.Helper()
	u, err := h.db.Users().ByUsername(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
