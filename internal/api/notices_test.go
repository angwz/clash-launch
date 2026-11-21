package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// 站内提醒（R23、R24）：
//   - 节点源连续两次拉取失败提醒一次，同一段连续失败不重复；
//   - 配置在后台重新生成时由通过变为不通过提醒；
//   - 订阅链接第一次被新的客户端拉取时提醒（第一个客户端不算），同一客户端不重复；
//   - 列表、未读数与标为已读。
func TestNotices(t *testing.T) {
	var fail atomic.Bool
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("proxies:\n  - {name: n1, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example}\n"))
	}))
	defer sub.Close()
	h := newHarness(t, nil)
	c, pid := h.setup()
	ctx := context.Background()
	notices := func() (items []noticeDTO, unread int) {
		t.Helper()
		r := decode[struct {
			Items  []noticeDTO `json:"items"`
			Unread int         `json:"unread"`
		}](t, c.want(200, "GET", "/api/v1/notifications", nil))
		return r.Items, r.Unread
	}
	kinds := func() string {
		items, _ := notices()
		var ks []string
		for _, n := range items {
			ks = append(ks, n.Kind)
		}
		return strings.Join(ks, ",")
	}

	// R24 节点源：成功 → 失败一次（不提醒）→ 再失败（提醒）→ 第三次失败（不重复）
	src := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{"name": "机场", "kind": "subscription", "url": sub.URL + "/sub"}))
	c.want(200, "POST", "/api/v1/sources/"+src.ID+"/refresh", nil)
	fail.Store(true)
	c.want(200, "POST", "/api/v1/sources/"+src.ID+"/refresh", nil)
	if k := kinds(); k != "" {
		t.Fatalf("第一次失败不提醒：%s", k)
	}
	c.want(200, "POST", "/api/v1/sources/"+src.ID+"/refresh", nil)
	c.want(200, "POST", "/api/v1/sources/"+src.ID+"/refresh", nil)
	items, unread := notices()
	if len(items) != 1 || items[0].Kind != "source_failing" || items[0].Detail["name"] != "机场" || items[0].Detail["source"] != src.ID || unread != 1 {
		t.Fatalf("连续失败：%+v %d", items, unread)
	}

	// R24 配置检查：配置当前通过；在存储里写入无法解析的原始编辑（模拟输入变化导致失败），再改节点源触发后台重新生成
	p, err := h.db.Profiles().Get(ctx, firstAdmin(t, h), pid)
	if err != nil {
		t.Fatal(err)
	}
	p.Doc = append(p.Doc, []byte("raw: \"{{ not yaml\"\n")...)
	if err := h.db.Profiles().Update(ctx, p.OwnerID, p, "test"); err != nil {
		t.Fatal(err)
	}
	srcs := decode[list[sourceDTO]](t, c.want(200, "GET", "/api/v1/sources", nil))
	var batch sourceDTO
	for _, x := range srcs.Items {
		if x.Kind == "batch" {
			batch = x
		}
	}
	full := decode[sourceDTO](t, c.want(200, "GET", "/api/v1/sources/"+batch.ID, nil))
	c.want(200, "PUT", "/api/v1/sources/"+batch.ID, map[string]any{"name": "我的 VPS 改名", "kind": "batch", "content": full.Content}, "If-Match", `"v`+itoa(int(full.Version))+`"`)
	if k := kinds(); !strings.Contains(k, "check_failed") {
		t.Fatalf("后台重新生成失败应提醒：%s", k)
	}

	// R23 新客户端：第一个客户端是基线，第二个提醒，重复的不再提醒
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	fetch := func(ua string) {
		req, _ := http.NewRequest("GET", l.URL, nil)
		req.Header.Set("User-Agent", ua)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		h.svc.FlushStats(ctx)
	}
	before := len(strings.Split(kinds(), ","))
	fetch("clash-verge/v2.0")
	fetch("clash-verge/v2.1")
	if n := len(strings.Split(kinds(), ",")); n != before {
		t.Fatalf("第一个客户端不提醒：%s", kinds())
	}
	fetch("Stash/2.5")
	fetch("Stash/2.6")
	items, _ = notices()
	if items[0].Kind != "link_new" || items[0].Detail["client"] != "Stash" || items[0].Detail["link"] != "手机" || items[0].Detail["linkId"] != l.ID {
		t.Fatalf("新客户端：%+v", items[0])
	}
	if n := len(strings.Split(kinds(), ",")); n != before+1 {
		t.Fatalf("同一客户端只提醒一次：%s", kinds())
	}

	// 已读
	c.want(204, "POST", "/api/v1/notifications/read", map[string]any{"ids": []string{items[0].ID}})
	_, unread = notices()
	if unread != len(items)-1 {
		t.Fatalf("标一条已读后：%d", unread)
	}
	c.want(204, "POST", "/api/v1/notifications/read", map[string]any{})
	if _, unread = notices(); unread != 0 {
		t.Fatalf("全部已读后：%d", unread)
	}
	// 别人看不到
	h.client().want(401, "GET", "/api/v1/notifications", nil)
}
