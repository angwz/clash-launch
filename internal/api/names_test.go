package api

import (
	"context"
	"strings"
	"testing"
)

// 需求 R4、R5：昵称可含空格、可被搜索；改用户名需要密码、180 天一次，旧名保留期内别人不能注册。
func TestNicknameAndRename(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")

	// 昵称：首尾空白去掉，内部空格保留；按其中一段可以搜到
	me := decode[userDTO](t, admin.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": "  Ann  Lee "}))
	if me.Nickname != "Ann  Lee" {
		t.Fatalf("昵称：%q", me.Nickname)
	}
	admin.want(422, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": strings.Repeat("长", 33)})
	type hits struct {
		Items []struct {
			Username string `json:"username"`
			Nickname string `json:"nickname"`
		} `json:"items"`
	}
	if hs := decode[hits](t, bob.want(200, "GET", "/api/v1/users/search?q=LEE", nil)); len(hs.Items) != 1 || hs.Items[0].Username != "admin" || hs.Items[0].Nickname != "Ann  Lee" {
		t.Errorf("按昵称搜索：%+v", hs)
	}

	// 改用户名
	bob.want(403, "PUT", "/api/v1/account/username", map[string]string{"username": "robert", "password": "wrong password!"})
	bob.want(409, "PUT", "/api/v1/account/username", map[string]string{"username": "admin", "password": "a long enough password"})
	u := decode[userDTO](t, bob.want(200, "PUT", "/api/v1/account/username", map[string]string{"username": "robert", "password": "a long enough password"}))
	if u.Username != "robert" || u.NextRenameAt == nil {
		t.Fatalf("改名后：%+v", u)
	}
	if r, b := bob.do("PUT", "/api/v1/account/username", map[string]string{"username": "rob", "password": "a long enough password"}); r.StatusCode != 409 || !strings.Contains(string(b), "account.rename_wait") {
		t.Errorf("180 天内不能再改：%d %s", r.StatusCode, b)
	}
	again := h.client()
	again.login("robert", "a long enough password")
	if r, b := reg.do("POST", "/api/v1/register", registerBody(t, reg, "BOB", "")); r.StatusCode != 409 || !strings.Contains(string(b), "account.name_reserved") {
		t.Errorf("旧名在保留期内不能被别人注册：%d %s", r.StatusCode, b)
	}
}

// 需求 R12：人人可邀请（名额、注册时长、收回邀请权）；可用多次；记录邀请关系并通知邀请人；
// 普通用户的邀请不赋予「受邀用户」权限。
func TestInvitesForEveryone(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": false})
	bob := h.client()
	bob.login("bob", "a long enough password")

	// 默认注册满 7 天才能邀请
	if r, b := bob.do("POST", "/api/v1/invites", map[string]any{"maxUses": 2}); r.StatusCode != 403 || !strings.Contains(string(b), "invite.too_new") {
		t.Fatalf("新账号不能邀请：%d %s", r.StatusCode, b)
	}
	admin.want(200, "PUT", "/api/v1/admin/invite-policy", map[string]int{"perMonth": 2, "minAgeDays": 0})
	bob.want(422, "POST", "/api/v1/invites", map[string]any{"maxUses": 101})
	inv := decode[inviteDTO](t, bob.want(201, "POST", "/api/v1/invites", map[string]any{"maxUses": 2, "note": "群里"}))
	tok := inv.URL[strings.Index(inv.URL, "#")+1:]

	anon := h.client()
	type who struct {
		Username string `json:"username"`
	}
	if w := decode[who](t, anon.want(200, "POST", "/api/v1/register/inviter", map[string]string{"token": tok})); w.Username != "bob" {
		t.Errorf("注册页显示邀请人：%+v", w)
	}
	anon.want(404, "POST", "/api/v1/register/inviter", map[string]string{"token": "nope"})

	for _, name := range []string{"c1", "c2"} {
		c := h.client()
		c.want(201, "POST", "/api/v1/register", registerBody(t, c, name, tok))
	}
	c3 := h.client()
	c3.want(403, "POST", "/api/v1/register", registerBody(t, c3, "c3", tok))

	c1, err := h.db.Users().ByUsername(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if c1.Invited || c1.InvitedByName != "bob" || c1.InviteID != inv.ID {
		t.Errorf("邀请关系与权限：invited=%v by=%q invite=%q", c1.Invited, c1.InvitedByName, c1.InviteID)
	}

	type mine struct {
		Quota struct {
			Used    int  `json:"used"`
			Allowed bool `json:"allowed"`
		} `json:"quota"`
		Items    []inviteDTO `json:"items"`
		Invitees []struct {
			Username string `json:"username"`
		} `json:"invitees"`
	}
	m := decode[mine](t, bob.want(200, "GET", "/api/v1/invites", nil))
	if m.Quota.Used != 1 || len(m.Items) != 1 || m.Items[0].Status != "used" || strings.Join(m.Items[0].UsedBy, ",") != "c1,c2" || len(m.Invitees) != 2 {
		t.Fatalf("我的邀请：%+v", m)
	}
	// 邀请人收到通知，双方成为联系人
	type thread struct {
		Items []messageDTO `json:"items"`
	}
	th := decode[thread](t, bob.want(200, "GET", "/api/v1/messages/with/c1", nil))
	if len(th.Items) != 1 || th.Items[0].Notice == nil || th.Items[0].Notice.Code != "invite_joined" {
		t.Errorf("邀请人的通知：%+v", th.Items)
	}

	// 名额：最近 30 天 2 个
	second := decode[inviteDTO](t, bob.want(201, "POST", "/api/v1/invites", map[string]any{}))
	if r, b := bob.do("POST", "/api/v1/invites", map[string]any{}); r.StatusCode != 403 || !strings.Contains(string(b), "invite.quota") {
		t.Errorf("名额用完：%d %s", r.StatusCode, b)
	}
	// 收回邀请权：未用的邀请随之作废
	bu, _ := h.db.Users().ByUsername(context.Background(), "bob")
	admin.want(204, "PUT", "/api/v1/admin/users/"+bu.ID+"/invite-ban", map[string]bool{"banned": true})
	m = decode[mine](t, bob.want(200, "GET", "/api/v1/invites", nil))
	for _, x := range m.Items {
		if x.ID == second.ID && x.Status != "revoked" {
			t.Errorf("收回邀请权后未用的邀请应作废：%+v", x)
		}
	}
	if m.Quota.Allowed {
		t.Error("收回后不能再邀请")
	}
	// 管理员不受名额限制，管理员的邀请赋予受邀权限
	admin.want(201, "POST", "/api/v1/invites", map[string]any{})
	al := decode[list[inviteDTO]](t, admin.want(200, "GET", "/api/v1/admin/invites", nil))
	if len(al.Items) < 3 || al.Items[len(al.Items)-1].CreatedBy != "bob" {
		t.Errorf("管理员视图显示发出者：%+v", al.Items)
	}
}
