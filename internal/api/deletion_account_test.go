package api

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 注销账户（R20）：
//   - 需要当前密码；发起后所有会话失效，冷静期内再次登录即撤销；
//   - 冷静期满后注销：订阅失效、不能登录、给别人的授权收回并通知对方、对话保留并显示占位名、
//     原用户名保留不能被注册、deleted- 前缀不能注册；
//   - 管理员立即注销普通用户；最后一个管理员不能注销。
func TestAccountDeletion(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	ctx := context.Background()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	const bobPW = "a long enough password"
	bob := h.client()
	bob.login("bob", bobPW)
	src := decode[sourceDTO](t, bob.want(201, "POST", "/api/v1/sources", map[string]any{"name": "bob 的节点", "kind": "batch", "content": "- {name: 首尔 01, type: ss, server: 192.0.2.61, port: 8388, cipher: aes-128-gcm, password: example-pw}\n"}))
	doc := "clash-launch/profile: v1\nname: bob 的配置\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n  - {ref: " + src.ID + "}\n"
	p := decode[profileDTO](t, bob.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	// 删除保护不阻止注销
	bob.want(200, "PUT", "/api/v1/profiles/"+p.ID+"/protection", map[string]bool{"protected": true})
	l := decode[linkDTO](t, bob.want(201, "POST", "/api/v1/profiles/"+p.ID+"/links", map[string]string{"note": "手机"}))
	bob.want(201, "POST", "/api/v1/messages", map[string]any{"to": "admin", "source": src.ID, "mode": "grant"})
	subPath := strings.TrimPrefix(l.URL, h.http.URL)
	anon := h.client()
	if r, _ := anon.do("GET", subPath, nil); r.StatusCode != 200 {
		t.Fatalf("注销前订阅可用：%d", r.StatusCode)
	}

	bob.want(403, "POST", "/api/v1/account/delete", map[string]string{"password": "wrong password!"})
	bob.want(200, "POST", "/api/v1/account/delete", map[string]string{"password": bobPW})
	bob.want(401, "GET", "/api/v1/me", nil)
	if r, _ := anon.do("GET", subPath, nil); r.StatusCode != 200 {
		t.Errorf("冷静期内订阅照常可用：%d", r.StatusCode)
	}
	// 冷静期内登录即撤销
	r, b := bob.do("POST", "/api/v1/session", map[string]string{"username": "bob", "password": bobPW})
	if r.StatusCode != 200 || !strings.Contains(string(b), `"deletionCanceled":true`) {
		t.Fatalf("登录应撤销注销：%d %s", r.StatusCode, b)
	}
	u, _ := h.db.Users().ByUsername(ctx, "bob")
	if !u.DeletionAt.IsZero() {
		t.Fatal("撤销后不应再有计划注销时间")
	}
	bob.login("bob", bobPW)
	bob.want(200, "POST", "/api/v1/account/delete", map[string]string{"password": bobPW})

	// 冷静期满
	if err := h.db.Users().SetDeletion(ctx, u.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	h.svc.RunDueDeletions(ctx)
	gone, err := h.db.Users().ByID(ctx, u.ID)
	if err != nil || gone.DeletedAt.IsZero() || !strings.HasPrefix(gone.Username, "deleted-") || gone.Nickname != "" {
		t.Fatalf("应改为墓碑：%+v %v", gone, err)
	}
	if r, _ := anon.do("GET", subPath, nil); r.StatusCode != 404 {
		t.Errorf("注销后订阅应失效：%d", r.StatusCode)
	}
	if r, _ := h.client().do("POST", "/api/v1/session", map[string]string{"username": "bob", "password": bobPW}); r.StatusCode != 401 {
		t.Errorf("注销后不能登录：%d", r.StatusCode)
	}
	// 对话保留；授权收回并有通知
	thread := string(admin.want(200, "GET", "/api/v1/messages/with/"+gone.Username, nil))
	if !strings.Contains(thread, "grant_deleted") {
		t.Errorf("对方应收到授权收回的通知，对话保留：%s", thread)
	}
	// 原用户名保留；deleted- 前缀不能注册
	if r, b := reg.do("POST", "/api/v1/register", registerBody(t, reg, "bob", "")); r.StatusCode != 409 || !strings.Contains(string(b), "account.name_reserved") {
		t.Errorf("原用户名应保留：%d %s", r.StatusCode, b)
	}
	if r, _ := reg.do("POST", "/api/v1/register", registerBody(t, reg, "deleted-abc", "")); r.StatusCode != 422 {
		t.Errorf("deleted- 前缀不能注册：%d", r.StatusCode)
	}

	// 管理员：最后一个管理员不能注销；立即注销普通用户
	admin.want(409, "POST", "/api/v1/account/delete", map[string]string{"password": "correct horse battery"})
	admin.want(409, "DELETE", "/api/v1/admin/users/"+firstAdmin(t, h), nil)
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "carol", ""))
	carol, _ := h.db.Users().ByUsername(ctx, "carol")
	admin.want(204, "DELETE", "/api/v1/admin/users/"+carol.ID, nil)
	admin.want(409, "DELETE", "/api/v1/admin/users/"+carol.ID, nil)
	if r, _ := h.client().do("POST", "/api/v1/session", map[string]string{"username": "carol", "password": bobPW}); r.StatusCode != 401 {
		t.Errorf("被管理员注销后不能登录：%d", r.StatusCode)
	}
	if !strings.Contains(string(admin.want(200, "GET", "/api/v1/admin/audit", nil)), "admin.user.delete") {
		t.Error("审计缺少 admin.user.delete")
	}
}
