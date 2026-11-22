package api

import (
	"context"
	"strings"
	"testing"
)

// 足迹（R10）：各触发点记下「第一次」，重复不再记；订阅拉取得出第一次被拉取、客户端与累计；
// 改名每次都记；站点足迹只给管理员；补记只运行一次并标为补记；随备份导出。
func TestFootprints(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	ctx := context.Background()
	type fp struct {
		Kind       string            `json:"kind"`
		Subject    string            `json:"subject"`
		Detail     map[string]string `json:"detail"`
		Backfilled bool              `json:"backfilled"`
	}
	type resp struct {
		Items []fp `json:"items"`
		Site  []fp `json:"site"`
	}
	kinds := func(items []fp) map[string]int {
		m := map[string]int{}
		for _, x := range items {
			m[x.Kind]++
		}
		return m
	}

	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "电脑"}) // 第二条：不再记
	anon := h.client()
	for range 2 {
		anon.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil, "User-Agent", "clash-verge/v2.0.0")
	}
	h.svc.FlushStats(ctx)

	r := decode[resp](t, admin.want(200, "GET", "/api/v1/account/footprints", nil))
	k := kinds(r.Items)
	for _, want := range []string{"joined", "first_login", "first_source", "first_profile", "first_link", "first_pull", "client"} {
		if k[want] != 1 {
			t.Errorf("足迹 %s：%d 条，期望 1：%+v", want, k[want], r.Items)
		}
	}
	if k["first_check"] != 0 {
		t.Errorf("没有内核时不应记下通过检查：%+v", r.Items)
	}
	if len(r.Site) != 1 || r.Site[0].Kind != "site_start" {
		t.Errorf("站点足迹：%+v", r.Site)
	}

	// 新用户：注册、第一位注册用户；改名与改昵称每次都记；看不到站点足迹
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": "Bob"})
	bob.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": "Bobby"})
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "你好"})
	rb := decode[resp](t, bob.want(200, "GET", "/api/v1/account/footprints", nil))
	kb := kinds(rb.Items)
	if kb["joined"] != 1 || kb["nickname"] != 2 || kb["first_contact"] != 1 || rb.Site != nil {
		t.Errorf("bob 的足迹：%+v %+v", rb.Items, rb.Site)
	}
	ra := decode[resp](t, admin.want(200, "GET", "/api/v1/account/footprints", nil))
	if kinds(ra.Items)["first_message"] != 1 || kinds(ra.Site)["site_first"] != 1 {
		t.Errorf("消息与站点：%+v %+v", ra.Items, ra.Site)
	}

	// 补记：已有的不重复；只运行一次
	if err := h.svc.BackfillFootprints(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.BackfillFootprints(ctx); err != nil {
		t.Fatal(err)
	}
	after := decode[resp](t, admin.want(200, "GET", "/api/v1/account/footprints", nil))
	if len(after.Items) != len(ra.Items) {
		t.Errorf("补记不应重复已有的足迹：%d → %d", len(ra.Items), len(after.Items))
	}

	// 随备份导出
	u, err := h.db.Users().ByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	bk, err := h.svc.BuildBackup(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(bk.Footprints) != len(after.Items) {
		t.Errorf("备份中的足迹：%d，期望 %d", len(bk.Footprints), len(after.Items))
	}
}
