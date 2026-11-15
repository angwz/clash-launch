package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// 反馈 #76：安全问题可选、模板分享、一键导入计入统计、用户分页、节点概况。
func TestOptionalQuestionsTemplatesImportsPaging(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()

	// 不设安全问题也能注册；只设 1 个时，答对这 1 个即可找回；找回页面总是显示 3 个问题
	none := registerBody(t, anon, "nina", "")
	none["questions"], none["answers"] = []any{}, []string{}
	anon.want(201, "POST", "/api/v1/register", none)
	one := registerBody(t, anon, "olga", "")
	one["questions"], one["answers"] = []map[string]any{{"builtin": 4}}, []string{"Example Corp"}
	anon.want(201, "POST", "/api/v1/register", one)
	type qs struct {
		Questions []map[string]any `json:"questions"`
	}
	q := decode[qs](t, anon.want(200, "POST", "/api/v1/recovery/questions", map[string]string{"username": "olga"}))
	if len(q.Questions) != 3 || q.Questions[0]["builtin"] != float64(4) {
		t.Fatalf("找回问题应补足为 3 个且真实问题在前：%+v", q)
	}
	rec := func(user string, answers []string) map[string]any {
		return map[string]any{"username": user, "answers": answers, "password": "a brand new password 1", "pow": solvePoW(t, anon, "recover")}
	}
	anon.want(403, "POST", "/api/v1/recovery", rec("olga", []string{"wrong", "x1", "x2"}))
	anon.want(204, "POST", "/api/v1/recovery", rec("olga", []string{"example corp", "x1", "x2"}))
	anon.want(403, "POST", "/api/v1/recovery", rec("nina", []string{"a1", "b2", "c3"})) // 没有问题的账号不能用问题找回

	// 模板分享：对方得到一份副本
	nina := h.client()
	nina.login("nina", "a long enough password")
	p := decode[profileDTO](t, admin.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	tpl := decode[templateDTO](t, admin.want(201, "POST", "/api/v1/templates", map[string]any{"name": "家用", "note": "路由器", "doc": p.Doc}))
	admin.want(422, "POST", "/api/v1/messages", map[string]any{"to": "nina", "template": tpl.ID, "mode": "grant"})
	m := decode[messageDTO](t, admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "nina", "template": tpl.ID}))
	if m.Attach == nil || m.Attach.Kind != "template" || m.Attach.Note != "路由器" {
		t.Fatalf("模板附件：%+v", m.Attach)
	}
	ts := string(nina.want(200, "GET", "/api/v1/templates", nil))
	if !strings.Contains(ts, "家用（来自 admin）") {
		t.Errorf("对方应收到模板副本：%s", ts)
	}

	// 一键导入：点击时上报，记为 import，客户端名称来自菜单
	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "电脑"}))
	admin.want(400, "POST", "/api/v1/imports", map[string]string{"link": l.ID, "client": "nope"})
	nina.want(404, "POST", "/api/v1/imports", map[string]string{"link": l.ID, "client": "verge"})
	admin.want(204, "POST", "/api/v1/imports", map[string]string{"link": l.ID, "client": "verge"})
	reqs := decode[list[requestDTO]](t, admin.want(200, "GET", "/api/v1/profiles/"+pid+"/links/"+l.ID+"/requests", nil))
	if len(reqs.Items) != 1 || reqs.Items[0].Kind != "import" || reqs.Items[0].Client != "Clash Verge Rev" {
		t.Errorf("一键导入的记录：%+v", reqs.Items)
	}

	// 用户分页与搜索
	type up struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	pg := decode[up](t, admin.want(200, "GET", "/api/v1/admin/users?pageNo=1&limit=2", nil))
	if pg.Total != 3 || len(pg.Items) != 2 {
		t.Errorf("用户分页：%d %d", pg.Total, len(pg.Items))
	}
	if s := decode[up](t, admin.want(200, "GET", "/api/v1/admin/users?pageNo=1&q=OLG", nil)); s.Total != 1 || s.Items[0]["username"] != "olga" {
		t.Errorf("用户搜索：%+v", s)
	}

	// 节点概况：可用节点数与协议分布
	srcs := decode[struct {
		Items []json.RawMessage `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/sources", nil))
	var first struct {
		Summary struct {
			Nodes int `json:"nodes"`
			Types []struct {
				Type  string `json:"type"`
				Count int    `json:"count"`
			} `json:"types"`
		} `json:"summary"`
	}
	_ = json.Unmarshal(srcs.Items[0], &first)
	if first.Summary.Nodes != 2 || len(first.Summary.Types) != 1 || first.Summary.Types[0].Type != "ss" {
		t.Errorf("节点概况：%+v", first.Summary)
	}
}

func TestParseUserinfo(t *testing.T) {
	var s sourceSummary
	parseUserinfo("upload=1024; download=2048; total=1073741824; expire=1893456000", &s)
	if s.Upload != 1024 || s.Download != 2048 || s.Total != 1<<30 || s.Expire == nil || s.Expire.Year() != 2030 {
		t.Errorf("%+v", s)
	}
}
