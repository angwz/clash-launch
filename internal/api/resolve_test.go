package api

import "testing"

// O10：对方改用户名后，旧名解析为新名（只对联系人）；陌生人得到 404；名字现在属于某人时原样返回。
func TestResolveFormerName(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "carol", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	carol := h.client()
	carol.login("carol", "a long enough password")
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "你好"})
	bob.want(200, "PUT", "/api/v1/account/username", map[string]string{"username": "robert", "password": "a long enough password"})

	if r := decode[map[string]string](t, admin.want(200, "GET", "/api/v1/messages/resolve/BOB", nil)); r["username"] != "robert" {
		t.Errorf("联系人的旧名：%v", r)
	}
	carol.want(404, "GET", "/api/v1/messages/resolve/bob", nil)
	if r := decode[map[string]string](t, carol.want(200, "GET", "/api/v1/messages/resolve/admin", nil)); r["username"] != "admin" {
		t.Errorf("现在的名字原样返回：%v", r)
	}
	admin.want(404, "GET", "/api/v1/messages/resolve/nobody", nil)
}
