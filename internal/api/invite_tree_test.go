package api

import (
	"strings"
	"testing"
)

// O11：邀请树列出全部用户与各自的邀请人；只有管理员可见。
func TestInviteTree(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	inv := decode[inviteDTO](t, admin.want(201, "POST", "/api/v1/admin/invites", map[string]any{"hours": 1}))
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", inv.URL[strings.Index(inv.URL, "#")+1:]))
	bob := h.client()
	bob.login("bob", "a long enough password")
	type node struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		InvitedBy string `json:"invitedBy"`
	}
	items := decode[struct {
		Items []node `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/admin/invite-tree", nil)).Items
	ids := map[string]node{}
	for _, n := range items {
		ids[n.Username] = n
	}
	if len(items) != 2 || ids["bob"].InvitedBy != ids["admin"].ID || ids["admin"].InvitedBy != "" {
		t.Errorf("邀请树：%+v", items)
	}
	bob.want(404, "GET", "/api/v1/admin/invite-tree", nil)
}
