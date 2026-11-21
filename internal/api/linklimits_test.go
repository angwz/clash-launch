package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 订阅链接访问限制（R42）：限定客户端、国家或地区、每天次数；只改出现的字段；无效取值 422；
// 被挡住的请求照常记入订阅分析；清空后恢复。
func TestLinkLimits(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	ctx := context.Background()
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	path := "/api/v1/profiles/" + pid + "/links/" + l.ID
	patch := func(body any, code int) linkDTO {
		t.Helper()
		ls := decode[map[string][]linkDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid+"/links", nil))["items"]
		var cur linkDTO
		for _, x := range ls {
			if x.ID == l.ID {
				cur = x
			}
		}
		b := c.want(code, "PATCH", path, body, "If-Match", `"v`+itoa(int(cur.Version))+`"`)
		if code != 200 {
			return linkDTO{}
		}
		return decode[linkDTO](t, b)
	}
	fetch := func(ua string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", l.URL, nil)
		req.Header.Set("User-Agent", ua)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode, resp.Header.Get("Retry-After")
	}

	patch(map[string]any{"limits": map[string]any{"countries": []string{"CHN"}}}, 422)
	patch(map[string]any{"limits": map[string]any{"clients": []string{"clash verge/1.0"}}}, 422)
	patch(map[string]any{"limits": map[string]any{"perDay": -1}}, 422)
	got := patch(map[string]any{"limits": map[string]any{"clients": []string{" Stash ", "stash", "mihomo"}, "perDay": 2}}, 200)
	if len(got.Limits.Clients) != 2 || got.Limits.PerDay != 2 || got.Note != "手机" {
		t.Fatalf("规范化后：%+v", got)
	}
	if got = patch(map[string]any{"note": "新备注"}, 200); got.Limits.PerDay != 2 || got.Note != "新备注" {
		t.Fatalf("只改备注不应清掉限制：%+v", got)
	}

	if code, _ := fetch("clash-verge/v2.0"); code != 403 {
		t.Fatalf("不允许的客户端：%d", code)
	}
	for i := 0; i < 2; i++ {
		if code, _ := fetch("Stash/2.5"); code != 200 {
			t.Fatalf("第 %d 次允许的拉取：%d", i+1, code)
		}
	}
	code, retry := fetch("Stash/2.5")
	if code != 429 || retry == "" {
		t.Fatalf("超过每天次数：%d %q", code, retry)
	}

	// 地区：测试环境没有地理库，来源查不到地区，按不符合处理
	patch(map[string]any{"limits": map[string]any{"countries": []string{"jp"}}}, 200)
	if code, _ := fetch("Stash/2.5"); code != 403 {
		t.Fatalf("查不到地区时应挡住：%d", code)
	}
	// 清空后恢复
	if got = patch(map[string]any{"limits": map[string]any{}}, 200); len(got.Limits.Countries) != 0 {
		t.Fatalf("清空：%+v", got.Limits)
	}
	if code, _ := fetch("clash-verge/v2.0"); code != 200 {
		t.Fatalf("清空后：%d", code)
	}

	// 被挡住的请求记入订阅分析
	h.svc.FlushStats(ctx)
	rs, err := h.db.Stats().LatestRequests(ctx, firstAdmin(t, h), time.Now().Add(-time.Hour), time.Now().Add(time.Minute), store.RequestFilter{LinkID: l.ID}, 20)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[int]int{}
	for _, q := range rs {
		statuses[q.Status]++
	}
	if statuses[403] != 2 || statuses[429] != 1 || statuses[200] != 3 {
		t.Fatalf("请求记录的状态码：%v", statuses)
	}
}
