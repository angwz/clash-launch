package api

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 数据保留设置与订阅分析导出（R43、R39）：
//   - 网页上的保留期优先于环境变量，可恢复为环境变量；超出范围 422；清理任务按生效值执行；只有管理员可见；
//   - 数据量统计包含全部业务表；
//   - CSV 导出：明细与按天汇总，UTF-8 BOM，公式起始字符被转义（CSV 注入）。
func TestRetentionAndExport(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	ctx := context.Background()
	h.svc.RequestRetention = 90 * 24 * time.Hour // 相当于环境变量 CLASH_LAUNCH_SUB_REQUEST_DAYS=90

	if r := string(c.want(200, "GET", "/api/v1/admin/retention", nil)); !strings.Contains(r, `"requestDays":90`) || !strings.Contains(r, `"requestFromSetting":false`) {
		t.Errorf("默认沿用环境变量：%s", r)
	}
	c.want(422, "PUT", "/api/v1/admin/retention", map[string]any{"auditDays": 99999, "requestDays": 7})
	if r := string(c.want(200, "PUT", "/api/v1/admin/retention", map[string]any{"auditDays": 30, "requestDays": 7})); !strings.Contains(r, `"requestDays":7`) || !strings.Contains(r, `"auditDays":30`) || !strings.Contains(r, `"requestFromSetting":true`) {
		t.Errorf("设置后：%s", r)
	}
	// 清理按生效值：10 天前的明细被删，2 天前的保留
	old, recent := time.Now().Add(-10*24*time.Hour), time.Now().Add(-2*24*time.Hour)
	owner := firstAdmin(t, h)
	if err := h.db.Stats().AddRequests(ctx, []store.SubRequest{{ID: "r-old", LinkID: "link-x", OwnerID: owner, At: old, IP: "192.0.2.1", Client: "old", Status: 200, Kind: "config"}, {ID: "r-new", LinkID: "link-x", OwnerID: owner, At: recent, IP: "192.0.2.2", Client: "new", Status: 200, Kind: "config"}}); err != nil {
		t.Fatal(err)
	}
	h.svc.PruneRequests(ctx)
	var kept []string
	_ = h.db.Stats().EachRequest(ctx, owner, time.Now().Add(-30*24*time.Hour), time.Now(), store.RequestFilter{}, func(q *store.SubRequest) error {
		kept = append(kept, q.ID)
		return nil
	})
	if strings.Join(kept, ",") != "r-new" {
		t.Errorf("按 7 天清理后：%v", kept)
	}
	if r := string(c.want(200, "PUT", "/api/v1/admin/retention", map[string]any{"auditDays": nil, "requestDays": nil})); !strings.Contains(r, `"requestDays":90`) || !strings.Contains(r, `"requestFromSetting":false`) {
		t.Errorf("恢复为环境变量：%s", r)
	}
	if u := string(c.want(200, "GET", "/api/v1/admin/usage", nil)); !strings.Contains(u, `"table":"users"`) || !strings.Contains(u, `"table":"sub_requests"`) || !strings.Contains(u, `"total"`) {
		t.Errorf("数据量：%s", u)
	}

	// 导出：一条恶意 User-Agent 的请求
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	h.client().do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil, "User-Agent", "=HYPERLINK(\"https://example.com\")/1.0")
	r, body := c.do("GET", "/api/v1/analytics/export?days=7&kind=requests", nil)
	if r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "text/csv") || !strings.HasPrefix(string(body), "\xEF\xBB\xBF") {
		t.Fatalf("导出：%d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xEF\xBB\xBF"))).ReadAll()
	if err != nil || len(rows) < 2 || rows[0][0] != "time" {
		t.Fatalf("CSV：%v %v", err, rows)
	}
	found := false
	for _, row := range rows[1:] {
		if row[2] == "手机" && strings.HasPrefix(row[5], "'=") {
			found = true
		}
	}
	if !found {
		t.Errorf("恶意客户端名应被转义：%v", rows)
	}
	r, body = c.do("GET", "/api/v1/analytics/export?days=7&kind=daily", nil)
	daily, _ := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xEF\xBB\xBF"))).ReadAll()
	if r.StatusCode != 200 || len(daily) != 8 || daily[0][0] != "date" {
		t.Errorf("按天汇总应有表头加 7 天：%d %v", len(daily), daily)
	}

	c.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(404, "GET", "/api/v1/admin/retention", nil)
	bob.want(404, "GET", "/api/v1/admin/usage", nil)
	// 导出只含本人的请求
	_, bb := bob.do("GET", "/api/v1/analytics/export?days=7", nil)
	if strings.Contains(string(bb), "手机") {
		t.Error("不应导出别人的请求")
	}
}
