package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 用户协议（R40）：注册必须同意当前版本；管理员改写后普通用户被拦截，只能读身份、同意或拒绝、退出；
// 同意与拒绝记入审计；保留期占位符替换为本站设置；管理员不受拦截。
func TestTerms(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.AuditRetention = 180 * 24 * time.Hour
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})

	pub := h.client()
	cur := decode[service.Terms](t, pub.want(200, "GET", "/api/v1/terms", nil))
	if cur.Version != 1 || cur.Custom || !strings.Contains(cur.Texts["zh-CN"], "180 天") || !strings.Contains(cur.Texts["en"], "an unlimited time") {
		t.Fatalf("默认协议：%+v", cur.Version)
	}
	if strings.Contains(cur.Texts["zh-CN"], "{{") {
		t.Fatal("占位符应被替换")
	}

	// 注册：不带或带旧版本都拒绝
	reg := h.client()
	body := registerBody(t, reg, "bobby", "")
	delete(body, "terms")
	reg.want(409, "POST", "/api/v1/register", body)
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bobby", ""))
	bob := h.client()
	bob.login("bobby", "a long enough password")
	me := decode[sessionResp](t, bob.want(200, "GET", "/api/v1/me", nil))
	if me.Terms != (termsState{Accepted: 1, Current: 1}) {
		t.Fatalf("注册时同意的版本：%+v", me.Terms)
	}
	bob.want(200, "GET", "/api/v1/profiles", nil)

	// 管理员改写：版本 +1，bob 被拦截
	admin.want(422, "PUT", "/api/v1/admin/terms", map[string]any{"texts": map[string]string{"zh-CN": "新的协议"}})
	admin.want(422, "PUT", "/api/v1/admin/terms", map[string]any{"texts": map[string]string{"zh-CN": "x", "en": "y", "ja": "z"}})
	bob.want(404, "PUT", "/api/v1/admin/terms", map[string]any{"reset": true})
	if def := decode[adminTerms](t, admin.want(200, "GET", "/api/v1/admin/terms", nil)); !strings.Contains(def.Defaults["zh-CN"], "{{audit_days}}") || !strings.Contains(def.Texts["zh-CN"], "{{audit_days}}") {
		t.Fatal("管理员读到的文本应保留占位符，便于编辑")
	}
	got := decode[service.Terms](t, admin.want(200, "PUT", "/api/v1/admin/terms", map[string]any{"texts": map[string]string{"zh-CN": "# 新协议\n\n保留 {{audit_days}}。", "en": "# New terms"}}))
	if got.Version != 2 || !got.Custom || got.Texts["zh-CN"] != "# 新协议\n\n保留 180 天。" || got.UpdatedAt == nil {
		t.Fatalf("改写后：%+v", got)
	}
	if raw := decode[adminTerms](t, admin.want(200, "GET", "/api/v1/admin/terms", nil)); raw.Texts["zh-CN"] != "# 新协议\n\n保留 {{audit_days}}。" || !raw.Custom {
		t.Fatalf("改写后的原始文本：%+v", raw.Texts)
	}
	resp, b := bob.do("GET", "/api/v1/profiles", nil)
	if resp.StatusCode != 403 || !strings.Contains(string(b), "terms.required") {
		t.Fatalf("新版本未同意时应被拦截：%d %s", resp.StatusCode, b)
	}
	if me := decode[sessionResp](t, bob.want(200, "GET", "/api/v1/me", nil)); !me.Terms.Pending || me.Terms.Current != 2 {
		t.Fatalf("me 应报告待同意：%+v", me.Terms)
	}
	admin.want(200, "GET", "/api/v1/profiles", nil) // 管理员不受拦截

	// 拒绝：只记审计；同意旧版本：409；同意当前版本：放行
	bob.want(204, "POST", "/api/v1/account/terms", map[string]any{"version": 2, "accept": false})
	bob.want(409, "POST", "/api/v1/account/terms", map[string]any{"version": 1, "accept": true})
	st := decode[termsState](t, bob.want(200, "POST", "/api/v1/account/terms", map[string]any{"version": 2, "accept": true}))
	if st.Pending || st.Accepted != 2 {
		t.Fatalf("同意后：%+v", st)
	}
	bob.want(200, "GET", "/api/v1/profiles", nil)
	u, _ := h.db.Users().ByUsername(context.Background(), "bobby")
	var actions []string
	entries, err := h.db.Audit().List(context.Background(), u.ID, store.Page{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		actions = append(actions, e.Action)
	}
	joined := strings.Join(actions, ",")
	if !strings.Contains(joined, "account.terms_accept") || !strings.Contains(joined, "account.terms_decline") {
		t.Fatalf("审计：%v", actions)
	}

	// 恢复默认：再生成一个新版本
	if r := decode[service.Terms](t, admin.want(200, "PUT", "/api/v1/admin/terms", map[string]any{"reset": true})); r.Version != 3 || r.Custom {
		t.Fatalf("恢复默认：%+v", r.Version)
	}
	bob.want(403, "GET", "/api/v1/profiles", nil)
	bob.want(204, "DELETE", "/api/v1/session", nil) // 待同意时仍可退出登录
}
