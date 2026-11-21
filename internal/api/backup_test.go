package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// 个人备份与还原（R21）：
//   - 备份需要当前密码与至少 10 个字符的口令；文件是密文，不含节点密码的明文；
//   - 还原前比较数量；口令错误 422；还原整体覆盖：之后新增的节点源消失、删掉的模板回来、改过的配置名恢复；
//   - 订阅链接与短链接按原令牌、原短码恢复，客户端的订阅不断；
//   - 还原后不再存在的节点源，授权被收回并通知对方（R2）；
//   - 撤销回到还原之前；
//   - 同一份备份还原到另一个账号：ID、令牌、短码都已被占用，换新的并在结果中列出，配置照常生成。
func TestBackupRestore(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	const pw = "correct horse battery"
	anon := h.client()
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	c.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "mybackup"})
	tpl := decode[templateDTO](t, c.want(201, "POST", "/api/v1/templates", map[string]any{"name": "模板甲", "doc": decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil)).Doc}))
	subPath := strings.TrimPrefix(l.URL, h.http.URL)
	sub := func(path string) (int, string) {
		r, b := anon.do("GET", path, nil)
		return r.StatusCode, string(b)
	}
	if code, _ := sub(subPath); code != 200 {
		t.Fatalf("订阅：%d", code)
	}

	c.want(403, "POST", "/api/v1/account/backup", map[string]string{"password": "wrong password!", "passphrase": "backup passphrase"})
	c.want(422, "POST", "/api/v1/account/backup", map[string]string{"password": pw, "passphrase": "short"})
	r, raw := c.do("POST", "/api/v1/account/backup", map[string]string{"password": pw, "passphrase": "backup passphrase"})
	if r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Disposition"), ".backup") {
		t.Fatalf("备份：%d %s", r.StatusCode, raw)
	}
	file := string(raw)
	if strings.Contains(file, "example-pw") || strings.Contains(file, "香港") || !strings.Contains(file, "clash-launch/backup-encrypted") {
		t.Fatalf("备份文件应是密文")
	}

	// 备份之后的修改：新增节点源并授权给 bob、删掉模板、改配置名
	c.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	extra := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{"name": "之后新增", "kind": "batch", "content": "- {name: 新加坡 09, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example-pw}\n"}))
	c.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "source": extra.ID, "mode": "grant"})
	c.want(204, "DELETE", "/api/v1/templates/"+tpl.ID, nil)
	p := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	c.want(200, "PUT", "/api/v1/profiles/"+pid, map[string]any{"doc": strings.Replace(p.Doc, "name: 测试", "name: 改过的名字", 1)}, "If-Match", `"v`+itoa(int(p.Version))+`"`)

	c.want(422, "POST", "/api/v1/account/restore/preview", map[string]string{"file": file, "passphrase": "not the passphrase"})
	c.want(422, "POST", "/api/v1/account/restore/preview", map[string]string{"file": "{}", "passphrase": "backup passphrase"})
	type summary struct {
		Sources, Profiles, Templates, Links, ShortLinks, Revisions int
	}
	pv := decode[struct {
		Current summary `json:"current"`
		Backup  summary `json:"backup"`
	}](t, c.want(200, "POST", "/api/v1/account/restore/preview", map[string]string{"file": file, "passphrase": "backup passphrase"}))
	if pv.Current.Sources != pv.Backup.Sources+1 || pv.Current.Templates != 0 || pv.Backup.Templates != 1 || pv.Backup.Links != 1 || pv.Backup.ShortLinks != 1 {
		t.Errorf("比较：%+v", pv)
	}
	c.want(403, "POST", "/api/v1/account/restore", map[string]string{"file": file, "passphrase": "backup passphrase", "password": "wrong password!"})
	rep := decode[struct {
		NewTokens []string `json:"newTokens"`
		NewCodes  []string `json:"newCodes"`
		Revoked   int      `json:"revoked"`
	}](t, c.want(200, "POST", "/api/v1/account/restore", map[string]string{"file": file, "passphrase": "backup passphrase", "password": pw}))
	if len(rep.NewTokens) != 0 || len(rep.NewCodes) != 0 || rep.Revoked != 1 {
		t.Errorf("同账号还原：%+v", rep)
	}
	srcs := decode[struct {
		Items []sourceDTO `json:"items"`
	}](t, c.want(200, "GET", "/api/v1/sources", nil)).Items
	for _, s := range srcs {
		if s.ID == extra.ID {
			t.Error("备份之后新增的节点源应被覆盖掉")
		}
	}
	if !strings.Contains(string(c.want(200, "GET", "/api/v1/templates", nil)), "模板甲") {
		t.Error("删掉的模板应回来")
	}
	if got := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil)); got.Name != "测试" {
		t.Errorf("配置名应恢复：%s", got.Name)
	}
	if code, _ := sub(subPath); code != 200 {
		t.Errorf("订阅链接应继续可用：%d", code)
	}
	if code, _ := sub("/u/mybackup"); code != 200 {
		t.Errorf("短链接应继续可用：%d", code)
	}
	if !strings.Contains(string(bob.want(200, "GET", "/api/v1/messages/with/admin", nil)), "grant_deleted") {
		t.Error("被收回授权的一方应收到通知")
	}

	// 撤销：回到还原之前（新增的节点源回来）
	if !strings.Contains(string(c.want(200, "GET", "/api/v1/account/restore/undo", nil)), `"available":true`) {
		t.Fatal("应可以撤销")
	}
	c.want(200, "POST", "/api/v1/account/restore/undo", map[string]string{"password": pw})
	if !strings.Contains(string(c.want(200, "GET", "/api/v1/sources", nil)), "之后新增") {
		t.Error("撤销后新增的节点源应回来")
	}
	if !strings.Contains(string(c.want(200, "GET", "/api/v1/account/restore/undo", nil)), `"available":false`) {
		t.Error("撤销后留底应删除")
	}

	// 还原到另一个账号：全部 ID、令牌与短码都被 admin 占用
	brep := decode[struct {
		NewTokens []string `json:"newTokens"`
		NewCodes  []string `json:"newCodes"`
	}](t, bob.want(200, "POST", "/api/v1/account/restore", map[string]string{"file": file, "passphrase": "backup passphrase", "password": "a long enough password"}))
	if len(brep.NewTokens) != 1 || len(brep.NewCodes) != 1 || !strings.HasPrefix(brep.NewCodes[0], "mybackup → ") {
		t.Errorf("跨账号还原：%+v", brep)
	}
	bps := decode[struct {
		Items []profileDTO `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/profiles", nil)).Items
	if len(bps) != 1 || bps[0].ID == pid || bps[0].Render == nil || bps[0].Render.Status != "ok" {
		t.Fatalf("bob 的配置：%+v", bps)
	}
	bl := decode[struct {
		Items []linkDTO `json:"items"`
	}](t, bob.want(200, "GET", "/api/v1/profiles/"+bps[0].ID+"/links", nil)).Items
	if len(bl) != 1 || bl[0].URL == l.URL {
		t.Fatalf("bob 的链接应换新令牌：%+v", bl)
	}
	resp, err := http.Get(bl[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "香港 01") {
		t.Errorf("bob 的订阅：%d", resp.StatusCode)
	}
	// admin 的订阅不受影响
	if code, _ := sub(subPath); code != 200 {
		t.Errorf("admin 的订阅应不受影响：%d", code)
	}
}
