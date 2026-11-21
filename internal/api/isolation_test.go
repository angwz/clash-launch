package api

import (
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/source"
)

// 数据隔离（第 14.3 节「必须有测试」）：按路由表逐条检查——用户 A 的每个资源 ID，用户 B 访问时都得到 404。
// 由路由表驱动：新增带路径参数的路由而没有在 ids 中给出对应的资源时，测试失败，提醒补上。
func TestOwnerIsolationAllRoutes(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Pusher = &source.Pusher{} // 提醒渠道（R31）的路由需要推送可用；测试不发送
	a, pid := h.setup()
	const bpw = "a long enough password"

	// A 的各类资源
	l := decode[linkDTO](t, a.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{}))
	srcs := decode[list[sourceDTO]](t, a.want(200, "GET", "/api/v1/sources", nil))
	rss := decode[list[ruleSourceDTO]](t, a.want(200, "GET", "/api/v1/rule-sources", nil))
	dev := newDevice(t, h.http.URL)
	pb := decode[beginResp](t, a.want(200, "POST", "/api/v1/account/passkeys/begin", nil))
	pk := decode[passkeyDTO](t, a.want(201, "POST", "/api/v1/account/passkeys", map[string]any{"ticket": pb.Ticket, "credential": dev.create(pb.Options)}))
	sess := decode[struct {
		Items []sessionDTO `json:"items"`
	}](t, a.want(200, "GET", "/api/v1/account/sessions", nil))
	inv := decode[inviteDTO](t, a.want(201, "POST", "/api/v1/admin/invites", map[string]any{"hours": 1}))
	p := decode[profileDTO](t, a.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	tpl := decode[templateDTO](t, a.want(201, "POST", "/api/v1/templates", map[string]any{"name": "t", "doc": p.Doc}))
	sl := decode[shortDTO](t, a.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID}))
	tok := decode[tokenDTO](t, a.want(201, "POST", "/api/v1/account/tokens", map[string]any{"name": "t", "scope": "read"}))
	ch := decode[map[string]any](t, a.want(201, "POST", "/api/v1/account/channels", map[string]any{"kind": "webhook", "url": "https://example.com/hook"}))

	// 路由前缀 → A 的资源 ID
	ids := map[string]string{
		"/api/v1/profiles/":         pid,
		"/api/v1/sources/":          srcs.Items[0].ID,
		"/api/v1/rule-sources/":     rss.Items[0].ID,
		"/api/v1/account/passkeys/": pk.ID,
		"/api/v1/account/sessions/": sess.Items[0].ID,
		"/api/v1/admin/invites/":    inv.ID,
		"/api/v1/admin/users/":      firstAdmin(t, h),
		"/api/v1/templates/":        tpl.ID,
		"/api/v1/short-links/":      sl.ID,
		"/api/v1/account/tokens/":   tok.ID,
		"/api/v1/account/channels/": ch["id"].(string),
		"/api/v1/invites/":          inv.ID, // 管理员 A 发出的邀请：B 不能作废
	}

	// B：普通用户
	a.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	b := h.client()
	b.login("bob", bpw)
	// A 发给第三人 carol 的消息：B 既不是发件人也不是收件人
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "carol", ""))
	msg := decode[messageDTO](t, a.want(201, "POST", "/api/v1/messages", map[string]any{"to": "carol", "text": "hi", "source": srcs.Items[0].ID}))
	ids["/api/v1/messages/"] = msg.ID
	gm := decode[messageDTO](t, a.want(201, "POST", "/api/v1/messages", map[string]any{"to": "carol", "source": srcs.Items[0].ID, "mode": "grant"}))
	ids["/api/v1/grants/"] = gm.Attach.GrantID
	// 按调用者自身划定范围的路由：B 只能看到自己参与的会话，不存在「A 的资源」，另由 TestMessages 检查
	scoped := map[string]bool{"GET /api/v1/messages/with/{username}": true}
	// 每条路由一个能通过格式校验的请求体（接口拒绝未知字段），使所有权检查成为决定结果的那一步
	bodies := map[string]any{
		"PATCH /api/v1/account/passkeys/{id}":       map[string]string{"name": "x"},
		"POST /api/v1/account/passkeys/{id}/delete": map[string]string{"password": bpw},
		"PUT /api/v1/sources/{id}":                  map[string]string{"name": "x", "kind": "single", "content": "ss://YWVzLTEyOC1nY206ZXhhbXBsZQ==@192.0.2.40:8388#x"},
		"PUT /api/v1/rule-sources/{id}":             map[string]string{"name": "x", "kind": "toml"},
		"PUT /api/v1/profiles/{id}":                 map[string]string{"doc": p.Doc},
		"POST /api/v1/profiles/{id}/preview":        map[string]string{"doc": p.Doc},
		"POST /api/v1/profiles/{id}/rollback":       map[string]int{"n": 1},
		"POST /api/v1/profiles/{id}/links":          map[string]string{"note": "x"},
		"PATCH /api/v1/profiles/{id}/links/{lid}":   map[string]string{"note": "x"},
		"PATCH /api/v1/templates/{id}":              map[string]string{"name": "x"},
		"PATCH /api/v1/short-links/{id}":            map[string]string{"note": "x"},
		"PUT /api/v1/admin/users/{id}/invite-ban":   map[string]bool{"banned": true},
	}

	checked := 0
	for _, r := range h.srv.Routes() {
		if !strings.Contains(r.Pattern, "{") || !strings.HasPrefix(r.Pattern, "/api/v1/") || strings.Contains(r.Pattern, "{pack}") {
			continue // 公开接口与令牌接口（/s/、/p/、/u/、目录）不属于所有者资源
		}
		if scoped[r.Method+" "+r.Pattern] {
			continue
		}
		path := r.Pattern
		for prefix, id := range ids {
			if strings.HasPrefix(path, prefix+"{id}") {
				path = strings.Replace(path, "{id}", id, 1)
			}
		}
		path = strings.Replace(path, "{lid}", l.ID, 1)
		// 会话按调用者自身的联系人查找：B 与 carol 没有交流过，删除与恢复都应是 404
		path = strings.Replace(path, "{username}", "carol", 1)
		if strings.Contains(path, "{") {
			t.Errorf("%s %s：没有对应的资源 ID，请在 ids 中补上", r.Method, r.Pattern)
			continue
		}
		var send any
		if r.Method != "GET" && r.Method != "DELETE" {
			send = map[string]any{}
			if x, ok := bodies[r.Method+" "+r.Pattern]; ok {
				send = x
			}
		}
		resp, got := b.do(r.Method, path, send, "If-Match", `"v1"`)
		if resp.StatusCode != 404 {
			t.Errorf("%s %s：用户 B 得到 %d，期望 404：%s", r.Method, r.Pattern, resp.StatusCode, got)
		}
		checked++
	}
	if checked < 20 {
		t.Errorf("检查的路由过少：%d", checked)
	}
	// A 的资源仍然完好
	a.want(200, "GET", "/api/v1/profiles/"+pid, nil)
}
