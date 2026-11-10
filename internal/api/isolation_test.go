package api

import (
	"strings"
	"testing"
)

// 数据隔离（第 14.3 节「必须有测试」）：按路由表逐条检查——用户 A 的每个资源 ID，用户 B 访问时都得到 404。
// 由路由表驱动：新增带路径参数的路由而没有在 ids 中给出对应的资源时，测试失败，提醒补上。
func TestOwnerIsolationAllRoutes(t *testing.T) {
	h := newHarness(t, nil)
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

	// 路由前缀 → A 的资源 ID
	ids := map[string]string{
		"/api/v1/profiles/":         pid,
		"/api/v1/sources/":          srcs.Items[0].ID,
		"/api/v1/rule-sources/":     rss.Items[0].ID,
		"/api/v1/account/passkeys/": pk.ID,
		"/api/v1/account/sessions/": sess.Items[0].ID,
		"/api/v1/admin/invites/":    inv.ID,
		"/api/v1/admin/users/":      firstAdmin(t, h),
	}

	// B：普通用户
	a.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	b := h.client()
	b.login("bob", bpw)
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
	}

	checked := 0
	for _, r := range h.srv.Routes() {
		if !strings.Contains(r.Pattern, "{") || !strings.HasPrefix(r.Pattern, "/api/v1/") || strings.Contains(r.Pattern, "{pack}") {
			continue // 公开接口与令牌接口（/s/、/p/、目录）不属于所有者资源
		}
		path := r.Pattern
		for prefix, id := range ids {
			if strings.HasPrefix(path, prefix+"{id}") {
				path = strings.Replace(path, "{id}", id, 1)
			}
		}
		path = strings.Replace(path, "{lid}", l.ID, 1)
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
