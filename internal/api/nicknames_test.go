package api

import (
	"strings"
	"testing"
)

// O12：授权与短链接的另一方、收到的节点与短链接、订阅分析的好友都带上昵称（用户名照旧，界面显示「昵称 @用户名」）。
func TestNicknamesEverywhere(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	admin.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": "管理员小A"})
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": "Bob B"})

	srcs := decode[list[sourceDTO]](t, admin.want(200, "GET", "/api/v1/sources", nil))
	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "x"}))
	sl := decode[shortDTO](t, admin.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "nick-1"}))
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "source": srcs.Items[0].ID, "mode": "grant"})
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "short": sl.ID, "mode": "grant"})

	// 授权者一方：授权列表、为好友生成的短链接、订阅分析的好友
	if b := string(admin.want(200, "GET", "/api/v1/grants", nil)); !strings.Contains(b, `"peerNickname":"Bob B"`) {
		t.Errorf("授权列表：%s", b)
	}
	if b := string(admin.want(200, "GET", "/api/v1/short-links", nil)); !strings.Contains(b, `"granteeNickname":"Bob B"`) {
		t.Errorf("短链接：%s", b)
	}
	if b := string(admin.want(200, "GET", "/api/v1/analytics?days=7", nil)); !strings.Contains(b, `"friendNicknames":{`) || !strings.Contains(b, `"Bob B"`) {
		t.Errorf("订阅分析：%s", b)
	}
	// 被授权一方：收到的节点与短链接
	if b := string(bob.want(200, "GET", "/api/v1/sources", nil)); !strings.Contains(b, `"fromNickname":"管理员小A"`) {
		t.Errorf("收到的节点：%s", b)
	}
	if b := string(bob.want(200, "GET", "/api/v1/short-links", nil)); !strings.Contains(b, `"fromNickname":"管理员小A"`) {
		t.Errorf("收到的短链接：%s", b)
	}
}
