package api

import (
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/service"
)

// 配额与管理：数量、拉取间隔与存储配额；管理员只看到元数据；停用、重置口令、调整配额；审计；不能停用自己。
func TestQuotaAndAdmin(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.QuotaDefaults = service.QuotaDefaults{Sources: 2, Links: 1, MinInterval: 10 * time.Minute}
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "quinn", ""))
	u := h.client()
	u.login("quinn", "a long enough password")

	node := "ss://YWVzLTEyOC1nY206ZXhhbXBsZQ==@192.0.2.30:8388#secret-node-name"
	src := func(name string, interval int) map[string]any {
		return map[string]any{"name": name, "kind": "single", "content": node, "interval": interval}
	}
	u.want(201, "POST", "/api/v1/sources", src("a", 0))
	if r, b := u.do("POST", "/api/v1/sources", src("b", 60)); r.StatusCode != 403 || !strings.Contains(string(b), "quota.exceeded") {
		t.Errorf("拉取间隔短于配额应拒绝：%d %s", r.StatusCode, b)
	}
	u.want(201, "POST", "/api/v1/sources", src("b", 900))
	u.want(403, "POST", "/api/v1/sources", src("c", 0))
	q := decode[map[string]map[string]any](t, u.want(200, "GET", "/api/v1/account/quota", nil))
	if q["limits"]["sources"] != float64(2) || q["usage"]["sources"] != float64(2) {
		t.Errorf("配额与用量：%v", q)
	}

	// 管理员：只看到元数据与用量，看不到内容
	users := string(admin.want(200, "GET", "/api/v1/admin/users", nil))
	if !strings.Contains(users, `"username":"quinn"`) || strings.Contains(users, "secret-node-name") || strings.Contains(users, "192.0.2.30") {
		t.Errorf("管理员视图：%s", users)
	}
	list := decode[struct {
		Items []service.AdminUser `json:"items"`
	}](t, []byte(users))
	var id string
	for _, x := range list.Items {
		if x.Username == "quinn" {
			id = x.ID
			if x.Usage.Sources != 2 {
				t.Errorf("用量：%+v", x.Usage)
			}
		}
	}
	admin.want(204, "PUT", "/api/v1/admin/users/"+id+"/quota", map[string]any{"sources": 3, "profiles": -1, "links": -1, "minInterval": -1, "storage": -1})
	u.want(201, "POST", "/api/v1/sources", src("c", 0))
	admin.want(400, "PUT", "/api/v1/admin/users/"+id+"/quota", map[string]any{"sources": -5, "profiles": -1, "links": -1, "minInterval": -1, "storage": -1})

	// 普通用户看不到管理接口
	u.want(404, "GET", "/api/v1/admin/users", nil)
	u.want(404, "GET", "/api/v1/admin/status", nil)

	// 停用：会话立即失效、不能登录；不能停用自己
	admin.want(204, "PATCH", "/api/v1/admin/users/"+id, map[string]bool{"disabled": true})
	u.want(401, "GET", "/api/v1/me", nil)
	h.client().want(401, "POST", "/api/v1/session", map[string]string{"username": "quinn", "password": "a long enough password"})
	admin.want(409, "PATCH", "/api/v1/admin/users/"+firstAdmin(t, h), map[string]bool{"disabled": true})
	admin.want(204, "PATCH", "/api/v1/admin/users/"+id, map[string]bool{"disabled": false})
	admin.want(204, "POST", "/api/v1/admin/users/"+id+"/password", map[string]string{"password": "reset by the admin 1"})
	u2 := h.client()
	u2.login("quinn", "reset by the admin 1")

	// 审计：本人与管理员视图
	if mine := string(u2.want(200, "GET", "/api/v1/account/audit", nil)); !strings.Contains(mine, "account.password_reset_by_admin") || !strings.Contains(mine, "login.ok") {
		t.Errorf("本人审计：%s", mine)
	}
	if all := string(admin.want(200, "GET", "/api/v1/admin/audit", nil)); !strings.Contains(all, "admin.user.disable") || !strings.Contains(all, `"user":"admin"`) {
		t.Errorf("管理员审计：%s", all)
	}
	if st := string(admin.want(200, "GET", "/api/v1/admin/status", nil)); !strings.Contains(st, `"keys"`) || !strings.Contains(st, `"database":"`+h.db.Dialect()+`"`) {
		t.Errorf("系统状态：%s", st)
	}
}
