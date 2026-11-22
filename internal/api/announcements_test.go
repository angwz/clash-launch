package api

import (
	"strings"
	"testing"
	"time"
)

// 站内公告（R32）：管理员发布、修改、删除；普通用户只能读取生效中的并关闭；关闭后不再出现；
// 尚未开始与已过期的不显示；内容与时间校验；普通用户不能管理。
func TestAnnouncements(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	type item struct {
		ID        string `json:"id"`
		Text      string `json:"text"`
		Level     string `json:"level"`
		CreatedBy string `json:"createdBy"`
	}
	type items struct {
		Items []item `json:"items"`
	}

	now := time.Now().UTC()
	a := decode[item](t, admin.want(201, "POST", "/api/v1/admin/announcements", map[string]any{"text": "  今晚 23:00 维护 10 分钟  ", "level": "warn"}))
	admin.want(201, "POST", "/api/v1/admin/announcements", map[string]any{"text": "明天才开始", "startsAt": now.Add(24 * time.Hour)})
	admin.want(201, "POST", "/api/v1/admin/announcements", map[string]any{"text": "已过期", "startsAt": now.Add(-2 * time.Hour), "endsAt": now.Add(-time.Hour)})
	for _, bad := range []map[string]any{{"text": ""}, {"text": strings.Repeat("长", 501)}, {"text": "x", "level": "error"}, {"text": "x", "startsAt": now, "endsAt": now.Add(-time.Minute)}} {
		admin.want(422, "POST", "/api/v1/admin/announcements", bad)
	}
	bob.want(404, "POST", "/api/v1/admin/announcements", map[string]any{"text": "x"}) // 管理接口对普通用户不可见
	bob.want(404, "GET", "/api/v1/admin/announcements", nil)

	got := decode[items](t, bob.want(200, "GET", "/api/v1/announcements", nil)).Items
	if len(got) != 1 || got[0].Text != "今晚 23:00 维护 10 分钟" || got[0].Level != "warn" || got[0].CreatedBy != "" {
		t.Fatalf("生效中的公告：%+v", got)
	}
	if all := decode[items](t, admin.want(200, "GET", "/api/v1/admin/announcements", nil)).Items; len(all) != 3 || all[2].CreatedBy != "admin" {
		t.Errorf("管理页：%+v", all)
	}

	// 修改后仍显示；关闭后对 bob 消失，管理员仍能看到
	admin.want(200, "PUT", "/api/v1/admin/announcements/"+a.ID, map[string]any{"text": "改到 23:30", "level": "warn"})
	bob.want(204, "POST", "/api/v1/announcements/"+a.ID+"/dismiss", nil)
	bob.want(204, "POST", "/api/v1/announcements/"+a.ID+"/dismiss", nil) // 重复关闭无害
	bob.want(404, "POST", "/api/v1/announcements/missing/dismiss", nil)
	if got := decode[items](t, bob.want(200, "GET", "/api/v1/announcements", nil)).Items; len(got) != 0 {
		t.Errorf("关闭后：%+v", got)
	}
	if got := decode[items](t, admin.want(200, "GET", "/api/v1/announcements", nil)).Items; len(got) != 1 || got[0].Text != "改到 23:30" {
		t.Errorf("管理员没有关闭：%+v", got)
	}
	admin.want(204, "DELETE", "/api/v1/admin/announcements/"+a.ID, nil)
	admin.want(404, "DELETE", "/api/v1/admin/announcements/"+a.ID, nil)
	if got := decode[items](t, admin.want(200, "GET", "/api/v1/announcements", nil)).Items; len(got) != 0 {
		t.Errorf("删除后：%+v", got)
	}
}
