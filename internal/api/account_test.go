package api

import (
	"net/http"
	"strings"
	"testing"
)

// 账号安全（第 14.1 节）：找回不泄露用户名、答对 2 个即可、成功后会话失效、连续失败后锁定；
// 工作量证明不可伪造与重放；登录退避；修改安全问题与口令需要当前口令；会话列表与注销。
func TestAccountSecurity(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})

	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "carol", ""))

	// 工作量证明：缺失、伪造、重放都被拒绝
	body := registerBody(t, anon, "dave", "")
	body["pow"] = map[string]string{"challenge": "forged.sig", "nonce": "1"}
	anon.want(400, "POST", "/api/v1/register", body)
	body = registerBody(t, anon, "erin", "")
	anon.want(201, "POST", "/api/v1/register", body)
	body["username"] = "frank"
	if r, b := anon.do("POST", "/api/v1/register", body); r.StatusCode != 400 || !strings.Contains(string(b), "pow.invalid") {
		t.Errorf("重放的工作量证明应被拒绝：%d %s", r.StatusCode, b)
	}
	// 安全问题必填
	body = registerBody(t, anon, "gina", "")
	body["questions"] = []map[string]any{{"builtin": 0}, {"builtin": 0}, {"builtin": 1}}
	anon.want(422, "POST", "/api/v1/register", body)

	// 找回的问题：已有用户返回真实问题；不存在的用户返回稳定的诱饵问题，形状相同
	qs := func(user string) string {
		return string(anon.want(200, "POST", "/api/v1/recovery/questions", map[string]string{"username": user}))
	}
	if real := qs("carol"); !strings.Contains(real, "我的第一辆自行车") {
		t.Errorf("真实问题：%s", real)
	}
	if a, b := qs("nobody"), qs("NOBODY"); a != b || !strings.Contains(a, `"builtin"`) {
		t.Errorf("诱饵问题应稳定：%s %s", a, b)
	}

	// carol 登录两个设备
	c1, c2 := h.client(), h.client()
	c1.login("carol", "a long enough password")
	c2.login("carol", "a long enough password")

	recoverBody := func(user string, answers []string) map[string]any {
		return map[string]any{"username": user, "answers": answers, "password": "brand new password 2", "pow": solvePoW(t, anon, "recover")}
	}
	// 只答对 1 个：失败
	anon.want(403, "POST", "/api/v1/recovery", recoverBody("carol", []string{"beijing", "wrong", "wrong"}))
	// 答对 2 个（大小写、空白、全角不影响）：成功，全部会话失效
	anon.want(204, "POST", "/api/v1/recovery", recoverBody("carol", []string{"  ＢＥＩＪＩＮＧ ", "mr.   whiskers", "wrong"}))
	c1.want(401, "GET", "/api/v1/me", nil)
	c2.want(401, "GET", "/api/v1/me", nil)
	c1.login("carol", "brand new password 2")

	// 不存在的用户：与答错同样的结果
	anon.want(403, "POST", "/api/v1/recovery", recoverBody("nobody", []string{"a1", "b2", "c3"}))
	// 连续失败 5 次后锁定 15 分钟
	for i := 0; i < 5; i++ {
		anon.do("POST", "/api/v1/recovery", recoverBody("erin", []string{"x1", "y2", "z3"}))
	}
	if r, _ := anon.do("POST", "/api/v1/recovery", recoverBody("erin", []string{"beijing", "mr. whiskers", "红色"})); r.StatusCode != 429 || r.Header.Get("Retry-After") == "" {
		t.Errorf("连续失败后应锁定（即使答案正确）：%d", r.StatusCode)
	}

	// 登录退避：同一账号连续失败 3 次后要求等待
	bad := h.client()
	for i := 0; i < 3; i++ {
		bad.want(401, "POST", "/api/v1/session", map[string]string{"username": "erin", "password": "wrong password!"})
	}
	bad.want(401, "POST", "/api/v1/session", map[string]string{"username": "erin", "password": "wrong password!"})
	if r, _ := bad.do("POST", "/api/v1/session", map[string]string{"username": "erin", "password": "a long enough password"}); r.StatusCode != http.StatusTooManyRequests {
		t.Errorf("退避期内即使口令正确也应等待：%d", r.StatusCode)
	}

	// 修改安全问题需要当前口令
	newQs := map[string]any{"password": "wrong", "questions": []map[string]any{{"builtin": 2}, {"builtin": 3}, {"builtin": 4}}, "answers": []string{"aa", "bb", "cc"}}
	c1.want(403, "PUT", "/api/v1/account/questions", newQs)
	newQs["password"] = "brand new password 2"
	c1.want(204, "PUT", "/api/v1/account/questions", newQs)
	if got := string(c1.want(200, "GET", "/api/v1/account/questions", nil)); !strings.Contains(got, `"builtin":4`) || strings.Contains(got, "answer") {
		t.Errorf("安全问题（不应含答案）：%s", got)
	}

	// 会话：列出与注销；修改口令时注销其他设备
	c3 := h.client()
	c3.login("carol", "brand new password 2")
	list := decode[struct {
		Items []sessionDTO `json:"items"`
	}](t, c1.want(200, "GET", "/api/v1/account/sessions", nil))
	if len(list.Items) != 2 {
		t.Fatalf("会话数：%+v", list.Items)
	}
	var other string
	for _, s := range list.Items {
		if !s.Current {
			other = s.ID
		}
	}
	c1.want(204, "DELETE", "/api/v1/account/sessions/"+other, nil)
	c3.want(401, "GET", "/api/v1/me", nil)
	c1.want(404, "DELETE", "/api/v1/account/sessions/0000000000000000", nil)

	c4 := h.client()
	c4.login("carol", "brand new password 2")
	c1.want(403, "PUT", "/api/v1/account/password", map[string]any{"current": "nope", "password": "third password here"})
	c1.want(204, "PUT", "/api/v1/account/password", map[string]any{"current": "brand new password 2", "password": "third password here", "logoutOthers": true})
	c4.want(401, "GET", "/api/v1/me", nil)
	c1.want(200, "GET", "/api/v1/me", nil)
}
