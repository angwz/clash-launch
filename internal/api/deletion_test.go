package api

import (
	"net/http"
	"strings"
	"testing"
)

// 需求 R2：删除授权出去的节点源、配置（连带为好友生成的短链接）后，授权收回、对方的配置不再含这些节点，
// 对方在对话里收到系统通知；消息里的授权卡片显示已收回。
func TestDeleteRevokesGrantsAndNotifies(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	anon := h.client()
	l := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	sl := decode[shortDTO](t, admin.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "family"}))

	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bobby", ""))
	bob := h.client()
	bob.login("bobby", "a long enough password")

	type sources struct {
		Items []sourceDTO `json:"items"`
	}
	src := decode[sources](t, admin.want(200, "GET", "/api/v1/sources", nil)).Items[0]
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "source": src.ID, "mode": "grant"})
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "short": sl.ID, "mode": "grant"})
	type recv struct {
		Received []struct {
			URL string `json:"url"`
		} `json:"received"`
	}
	rv := decode[recv](t, bob.want(200, "GET", "/api/v1/short-links", nil))
	if len(rv.Received) != 1 {
		t.Fatalf("对方应收到一个短链接：%+v", rv)
	}
	friendPath := strings.TrimPrefix(rv.Received[0].URL, h.http.URL)
	if r, _ := anon.do("GET", friendPath, nil); r.StatusCode != http.StatusOK {
		t.Fatalf("删除前短链接可用：%d", r.StatusCode)
	}
	doc := "clash-launch/profile: v1\nname: b\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n  - {ref: " + src.ID + ", mode: provider}\n"
	bp := decode[profileDTO](t, bob.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))

	type thread struct {
		Items []messageDTO `json:"items"`
	}
	notices := func() []string {
		var out []string
		for _, m := range decode[thread](t, bob.want(200, "GET", "/api/v1/messages/with/admin", nil)).Items {
			if m.Notice != nil {
				out = append(out, m.Notice.Code+":"+m.Notice.Target+":"+m.Notice.Name)
			}
		}
		return out
	}

	// 删除节点源：对方节点页不再出现它，配置不再含它的节点，收到通知
	admin.want(204, "DELETE", "/api/v1/sources/"+src.ID, nil)
	if bs := decode[sources](t, bob.want(200, "GET", "/api/v1/sources", nil)); len(bs.Items) != 0 {
		t.Errorf("对方节点页仍有已删除的节点源：%+v", bs.Items)
	}
	if pv := decode[previewResp](t, bob.want(200, "POST", "/api/v1/profiles/"+bp.ID+"/preview", map[string]string{"doc": doc})); strings.Contains(pv.YAML, "192.0.2.1") {
		t.Errorf("对方的配置仍含已删除节点源的节点")
	}
	if ns := notices(); len(ns) != 1 || ns[0] != "grant_deleted:source:"+src.Name {
		t.Fatalf("节点源删除通知：%v", ns)
	}

	// 删除配置：订阅链接与短链接级联删除；为好友生成的短链接收回授权并通知
	admin.want(204, "DELETE", "/api/v1/profiles/"+pid, nil)
	if r, _ := anon.do("GET", friendPath, nil); r.StatusCode == http.StatusOK {
		t.Error("删除配置后，为好友生成的短链接应失效")
	}
	if rv := decode[recv](t, bob.want(200, "GET", "/api/v1/short-links", nil)); len(rv.Received) != 0 {
		t.Errorf("对方仍看到已删除的短链接：%+v", rv)
	}
	ns := notices()
	if len(ns) != 2 || !strings.HasPrefix(ns[0], "grant_deleted:short:") {
		t.Fatalf("短链接删除通知：%v", ns)
	}
	for _, m := range decode[thread](t, bob.want(200, "GET", "/api/v1/messages/with/admin", nil)).Items {
		if m.Attach != nil && m.Attach.Kind == "grant" && m.Attach.Active {
			t.Errorf("授权卡片应显示已收回：%+v", m.Attach)
		}
	}
}
