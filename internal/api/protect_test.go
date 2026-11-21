package api

import (
	"fmt"
	"strings"
	"testing"
)

// 删除保护（R55）：开启后删除接口返回 409，开关不改变版本、写入审计；关闭后可以删除；别人的配置 404。
func TestProfileProtection(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	before := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	if before.Protected {
		t.Fatal("新配置默认不保护")
	}
	if r := string(c.want(200, "PUT", "/api/v1/profiles/"+pid+"/protection", map[string]bool{"protected": true})); !strings.Contains(r, `"protected":true`) {
		t.Errorf("开启：%s", r)
	}
	after := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	if !after.Protected || after.Version != before.Version {
		t.Errorf("开启后：protected=%v version %d→%d", after.Protected, before.Version, after.Version)
	}
	if r := string(c.want(409, "DELETE", "/api/v1/profiles/"+pid, nil)); !strings.Contains(r, "profile.protected") {
		t.Errorf("删除受保护的配置：%s", r)
	}
	// 保存文档不会清掉保护
	after.Doc += "\n"
	c.want(200, "PUT", "/api/v1/profiles/"+pid, map[string]any{"doc": after.Doc}, "If-Match", fmt.Sprintf(`"v%d"`, after.Version))
	if !decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil)).Protected {
		t.Error("保存后保护应保留")
	}
	audit := string(c.want(200, "GET", "/api/v1/account/audit", nil))
	if !strings.Contains(audit, "profile.protect") {
		t.Errorf("审计缺少 profile.protect：%s", audit)
	}

	c.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(404, "PUT", "/api/v1/profiles/"+pid+"/protection", map[string]bool{"protected": false})

	c.want(200, "PUT", "/api/v1/profiles/"+pid+"/protection", map[string]bool{"protected": false})
	c.want(204, "DELETE", "/api/v1/profiles/"+pid, nil)
	if !strings.Contains(string(c.want(200, "GET", "/api/v1/account/audit", nil)), "profile.unprotect") {
		t.Error("审计缺少 profile.unprotect")
	}
}
