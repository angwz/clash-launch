package api

import (
	"net/http"
	"strings"
	"testing"
)

// 反馈 #75：根路径短链接、收件人联想、节点与短链接授权、好友使用统计、删除对话与回收站、审计分页。
func TestGrantsContactsRootShort(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	anon := h.client()
	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))

	// 根路径短链接与保留路径
	admin.want(422, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "admin"})
	sl := decode[shortDTO](t, admin.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "Family"}))
	if !strings.HasSuffix(sl.URL, "/family") {
		t.Fatalf("短链接地址：%s", sl.URL)
	}
	_, full := anon.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	if r, b := anon.do("GET", "/FAMILY", nil); r.StatusCode != http.StatusOK || string(b) != string(full) {
		t.Fatalf("根路径短链接：%d", r.StatusCode)
	}
	if r, _ := anon.do("GET", "/u/family", nil); r.StatusCode != http.StatusOK {
		t.Errorf("旧式 /u/ 仍然有效：%d", r.StatusCode)
	}
	if r, _ := anon.do("GET", "/settings", nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("保留路径交给前端（测试中没有前端，应为 404）：%d", r.StatusCode)
	}

	// 收件人联想
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bobby", ""))
	bob := h.client()
	bob.login("bobby", "a long enough password")
	type hits struct {
		Items []struct {
			Username string `json:"username"`
			Accepts  bool   `json:"accepts"`
		} `json:"items"`
	}
	if hs := decode[hits](t, admin.want(200, "GET", "/api/v1/users/search?q=BO", nil)); len(hs.Items) != 1 || hs.Items[0].Username != "bobby" || !hs.Items[0].Accepts {
		t.Errorf("联想：%+v", hs)
	}
	if hs := decode[hits](t, admin.want(200, "GET", "/api/v1/users/search?q=a%25", nil)); len(hs.Items) != 0 {
		t.Errorf("通配符应按字面量处理：%+v", hs)
	}

	// 节点授权：对方只能使用，看不到内容，不能修改；生成配置时可以用
	srcs := decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/sources", nil))
	src := srcs.Items[0]
	gm := decode[messageDTO](t, admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "text": "给你用", "source": src.ID, "mode": "grant"}))
	if gm.Attach == nil || gm.Attach.Kind != "grant" || !gm.Attach.Active {
		t.Fatalf("授权消息：%+v", gm.Attach)
	}
	bs := decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/sources", nil))
	if len(bs.Items) != 1 || bs.Items[0].Grant == nil || bs.Items[0].Grant.From != "admin" || bs.Items[0].Content != "" {
		t.Fatalf("对方节点页应出现只读的授权节点：%+v", bs.Items)
	}
	bob.want(404, "PUT", "/api/v1/sources/"+src.ID, map[string]string{"name": "x", "kind": "batch", "content": "x"}, "If-Match", `"v1"`)
	doc := "clash-launch/profile: v1\nname: b\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n  - {ref: " + src.ID + ", mode: provider}\n"
	bp := decode[profileDTO](t, bob.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	pv := decode[previewResp](t, bob.want(200, "POST", "/api/v1/profiles/"+bp.ID+"/preview", map[string]string{"doc": doc}))
	if !strings.Contains(pv.YAML, "192.0.2.1") || strings.Contains(pv.YAML, "proxy-providers") {
		t.Fatalf("授权节点应写入对方的配置（不交给客户端）：\n%s", pv.YAML)
	}

	// 短链接授权：为对方生成新的短链接，使用情况按好友统计
	sm := decode[messageDTO](t, admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "short": sl.ID, "mode": "grant"}))
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "short": sl.ID, "mode": "share"})
	type recv struct {
		Received []struct {
			URL  string `json:"url"`
			From string `json:"from"`
		} `json:"received"`
	}
	rv := decode[recv](t, bob.want(200, "GET", "/api/v1/short-links", nil))
	if len(rv.Received) != 1 || rv.Received[0].From != "admin" || strings.HasSuffix(rv.Received[0].URL, "/family") {
		t.Fatalf("对方应收到一个新的短链接：%+v", rv)
	}
	friendPath := strings.TrimPrefix(rv.Received[0].URL, h.http.URL)
	anon.do("GET", friendPath, nil, "User-Agent", "FlClash/0.8")
	anon.do("GET", friendPath, nil, "User-Agent", "FlClash/0.8")
	type an struct {
		Analytics struct {
			Totals struct {
				Requests int `json:"requests"`
			} `json:"totals"`
			Friends []struct {
				UserID   string   `json:"userId"`
				Requests int      `json:"requests"`
				IPs      int      `json:"ips"`
				Clients  []string `json:"clients"`
			} `json:"friends"`
		} `json:"analytics"`
		Friends map[string]string `json:"friends"`
	}
	a := decode[an](t, admin.want(200, "GET", "/api/v1/analytics", nil))
	if len(a.Analytics.Friends) != 1 || a.Analytics.Friends[0].Requests != 2 || a.Analytics.Friends[0].Clients[0] != "FlClash" || a.Friends[a.Analytics.Friends[0].UserID] != "bobby" {
		t.Fatalf("好友使用统计：%+v %+v", a.Analytics.Friends, a.Friends)
	}
	only := decode[an](t, admin.want(200, "GET", "/api/v1/analytics?friend="+a.Analytics.Friends[0].UserID, nil))
	if only.Analytics.Totals.Requests != 2 {
		t.Errorf("只看好友：%+v", only.Analytics.Totals)
	}

	// 删除对话：放入回收站，收回双方之间的全部授权
	admin.want(204, "DELETE", "/api/v1/messages/with/bobby", nil)
	if r, _ := anon.do("GET", friendPath, nil); r.StatusCode == http.StatusOK {
		t.Error("删除对话后，为好友生成的短链接应失效")
	}
	bs = decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/sources", nil))
	if len(bs.Items) != 0 {
		t.Errorf("删除对话后，授权的节点应收回：%+v", bs.Items)
	}
	th := decode[map[string]any](t, admin.want(200, "GET", "/api/v1/messages/with/bobby", nil))
	if th["deletedAt"] == nil || len(th["items"].([]any)) != 0 {
		t.Errorf("回收站中的会话不返回消息：%v", th)
	}
	del := decode[map[string][]map[string]any](t, admin.want(200, "GET", "/api/v1/messages/deleted", nil))
	if len(del["items"]) != 1 || del["items"][0]["username"] != "bobby" || del["items"][0]["purgeAt"] == nil {
		t.Errorf("回收站：%v", del)
	}
	ts := decode[map[string][]map[string]any](t, admin.want(200, "GET", "/api/v1/messages", nil))
	if len(ts["items"]) != 0 {
		t.Errorf("删除后不在会话列表中：%v", ts)
	}
	// 对方再发消息：对话自动恢复（授权不恢复）
	bob.want(201, "POST", "/api/v1/messages", map[string]any{"to": "admin", "text": "还在吗"})
	ts = decode[map[string][]map[string]any](t, admin.want(200, "GET", "/api/v1/messages", nil))
	if len(ts["items"]) != 1 {
		t.Errorf("对方来信后应恢复：%v", ts)
	}
	ms := decode[struct {
		Items []messageDTO `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/messages/with/bobby", nil))
	if len(ms.Items) != 3 || ms.Items[1].ID != sm.ID || ms.Items[1].Attach.Active {
		t.Errorf("恢复后历史完整，授权显示已收回：%+v", ms.Items)
	}
	admin.want(204, "POST", "/api/v1/messages/with/bobby/restore", nil)

	// 审计日志按页码分页
	pg := decode[struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
		Page  int              `json:"page"`
	}](t, admin.want(200, "GET", "/api/v1/admin/audit?pageNo=2&limit=2", nil))
	if pg.Page != 2 || len(pg.Items) != 2 || pg.Total < 4 {
		t.Errorf("审计分页：%d/%d %d", pg.Page, pg.Total, len(pg.Items))
	}
}
