package api

import (
	"net/http"
	"strings"
	"testing"
)

// 反馈 #74：短链接提供与原链接相同的内容并计入统计；订阅分析汇总请求；站内消息与附件自动加入。
func TestShortLinksAnalyticsMessages(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	anon := h.client()
	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))

	// 短链接
	if b := admin.want(422, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "a"}); !strings.Contains(string(b), "short.code") {
		t.Errorf("短码太短应拒绝：%s", b)
	}
	sl := decode[shortDTO](t, admin.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "Home-1", "note": "家里"}))
	if sl.Code != "home-1" || !strings.HasSuffix(sl.URL, "/home-1") || strings.Contains(sl.URL, "/u/") || sl.Target.Note != "手机" {
		t.Fatalf("短链接：%+v", sl)
	}
	admin.want(409, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "HOME-1"})
	rnd := decode[shortDTO](t, admin.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID}))
	if len(rnd.Code) != 5 {
		t.Errorf("随机短码应为 5 位：%q", rnd.Code)
	}
	_, full := anon.do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	resp, body := anon.do("GET", "/u/HOME-1", nil, "User-Agent", "mihomo/1.19.33")
	if resp.StatusCode != http.StatusOK || string(body) != string(full) {
		t.Fatalf("短链接应返回与原链接相同的内容：%d", resp.StatusCode)
	}
	if r, _ := anon.do("GET", "/u/home-1?download=1", nil); !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment") {
		t.Error("短链接也支持导出")
	}
	list := decode[list[shortDTO]](t, admin.want(200, "GET", "/api/v1/short-links", nil))
	for _, x := range list.Items {
		if x.ID == sl.ID && (x.Total != 2 || len(x.Series) != 30) {
			t.Errorf("短链接的请求数：%+v", x)
		}
	}
	admin.want(200, "PATCH", "/api/v1/short-links/"+sl.ID, map[string]any{"disabled": true})
	if r, _ := anon.do("GET", "/u/home-1", nil); r.StatusCode != http.StatusNotFound {
		t.Errorf("停用后应为 404：%d", r.StatusCode)
	}

	// 订阅分析
	type an struct {
		Analytics struct {
			Totals struct {
				Requests int `json:"requests"`
				IPs      int `json:"ips"`
			} `json:"totals"`
			Kinds  []struct{ Key string } `json:"kinds"`
			Shorts []struct{ Key string } `json:"shorts"`
			Days   []string               `json:"days"`
			Recent []struct {
				ShortID string `json:"shortId"`
			} `json:"recent"`
		} `json:"analytics"`
		Links  map[string]linkInfo `json:"links"`
		Shorts map[string]string   `json:"shorts"`
	}
	a := decode[an](t, admin.want(200, "GET", "/api/v1/analytics?days=7&tz=Asia/Shanghai", nil))
	// 原链接 1 次、短链接 2 次（含导出）；停用后的 404 不属于任何链接，不计入
	if a.Analytics.Totals.Requests != 3 || len(a.Analytics.Days) != 7 || a.Shorts[sl.ID] != "home-1" || a.Links[l.ID].Note != "手机" {
		t.Errorf("分析：%+v", a)
	}
	only := decode[an](t, admin.want(200, "GET", "/api/v1/analytics?short="+sl.ID, nil))
	if only.Analytics.Totals.Requests != 2 {
		t.Errorf("只看短链接：%+v", only.Analytics.Totals)
	}

	// 猜错短码：同一 IP 30 次后限速
	for range 30 {
		anon.do("GET", "/u/no-such-code", nil)
	}
	if r, _ := anon.do("GET", "/u/"+rnd.Code, nil); r.StatusCode != http.StatusTooManyRequests {
		t.Errorf("猜错过多后应限速：%d", r.StatusCode)
	}

	// 站内消息
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon2 := h.client()
	anon2.want(201, "POST", "/api/v1/register", registerBody(t, anon2, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	srcs := decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/sources", nil))
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "  "})
	admin.want(404, "POST", "/api/v1/messages", map[string]any{"to": "nobody", "text": "hi"})
	admin.want(404, "POST", "/api/v1/messages", map[string]any{"to": "admin", "text": "自己"})
	m := decode[messageDTO](t, admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "BOB", "text": "送你节点 🎁", "source": srcs.Items[0].ID}))
	if m.Attach == nil || m.Attach.Kind != "nodes" || m.Attach.Nodes != 2 {
		t.Fatalf("附件：%+v", m.Attach)
	}
	if n := decode[map[string]int](t, bob.want(200, "GET", "/api/v1/messages/unread", nil))["count"]; n != 1 {
		t.Errorf("未读：%d", n)
	}
	bs := decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/sources", nil))
	if len(bs.Items) != 1 || !strings.Contains(bs.Items[0].Name, "来自 admin") || !strings.Contains(bs.Items[0].Content, "192.0.2.1") {
		t.Fatalf("附件应自动加入收件人的节点页：%+v", bs.Items)
	}
	th := decode[struct {
		User  map[string]string `json:"user"`
		Items []messageDTO      `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/messages/with/admin", nil))
	if len(th.Items) != 1 || th.Items[0].Mine || th.Items[0].Text != "送你节点 🎁" || th.Items[0].AddedSource != bs.Items[0].ID {
		t.Fatalf("会话：%+v", th)
	}
	if n := decode[map[string]int](t, bob.want(200, "GET", "/api/v1/messages/unread", nil))["count"]; n != 0 {
		t.Errorf("打开会话后应已读：%d", n)
	}
	// 再次接受返回同一个源，不重复加入
	acc := decode[map[string]string](t, bob.want(200, "POST", "/api/v1/messages/"+m.ID+"/accept", nil))
	if acc["source"] != bs.Items[0].ID {
		t.Errorf("重复接受：%v", acc)
	}
	admin.want(404, "POST", "/api/v1/messages/"+m.ID+"/accept", nil) // 发件人不能接受
	// 关闭自动加入：消息照常送达，附件等待手动加入
	bob.want(200, "PUT", "/api/v1/account/prefs", map[string]any{"autoAdd": false})
	m2 := decode[messageDTO](t, admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "source": srcs.Items[0].ID}))
	th = decode[struct {
		User  map[string]string `json:"user"`
		Items []messageDTO      `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/messages/with/admin", nil))
	if th.Items[0].ID != m2.ID || th.Items[0].AddedSource != "" {
		t.Errorf("关闭自动加入后不应加入：%+v", th.Items[0])
	}
	// 屏蔽与不接收：统一返回 404
	bob.want(200, "PUT", "/api/v1/account/prefs", map[string]any{"blocked": []string{th.User["id"]}})
	admin.want(404, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "hi"})
	bob.want(200, "PUT", "/api/v1/account/prefs", map[string]any{"inbox": "off"})
	admin.want(404, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "hi"})
	// 会话列表与隐藏
	ts := decode[struct {
		Items []struct {
			Username string `json:"username"`
			Unread   int    `json:"unread"`
		} `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/messages", nil))
	if len(ts.Items) != 1 || ts.Items[0].Username != "bob" {
		t.Errorf("会话列表：%+v", ts.Items)
	}
	bob.want(204, "DELETE", "/api/v1/messages/"+m.ID, nil)
	th = decode[struct {
		User  map[string]string `json:"user"`
		Items []messageDTO      `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/messages/with/bob", nil))
	if len(th.Items) != 2 {
		t.Errorf("收件人隐藏不影响发件人：%d", len(th.Items))
	}
}
