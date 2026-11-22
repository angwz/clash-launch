package api

import (
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/service"
)

// 命令面板搜数据（R34）：索引包含本人的配置、节点源、节点名（与节点源页一致）、规则源、短链接与消息联系人；
// 不包含他人的数据；令牌不能读取（索引含联系人）。
func TestSearchIndex(t *testing.T) {
	h := newHarness(t, nil)
	admin, pid := h.setup()
	ls := decode[linkDTO](t, admin.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{"note": "手机"}))
	admin.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": ls.ID, "code": "home-1", "note": "客厅电视"})
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	bob := h.client()
	bob.login("bob", "a long enough password")
	bob.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": "Bob B"})
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bob", "text": "你好"})

	idx := decode[service.SearchIndex](t, admin.want(200, "GET", "/api/v1/search", nil))
	if len(idx.Profiles) != 1 || idx.Profiles[0].Name != "测试" || len(idx.Sources) != 1 || idx.Sources[0].Name != "我的 VPS" || len(idx.Rules) != 1 {
		t.Fatalf("索引：%+v", idx)
	}
	if len(idx.Nodes) != 2 || !strings.Contains(idx.Nodes[0].Name, "香港 01") || idx.Nodes[0].Source != idx.Sources[0].ID {
		t.Errorf("节点名：%+v", idx.Nodes)
	}
	if len(idx.Shorts) != 1 || idx.Shorts[0].Code != "home-1" || idx.Shorts[0].Note != "客厅电视" {
		t.Errorf("短链接：%+v", idx.Shorts)
	}
	if len(idx.Friends) != 1 || idx.Friends[0].Username != "bob" || idx.Friends[0].Nickname != "Bob B" {
		t.Errorf("联系人：%+v", idx.Friends)
	}

	// 他人：只看到自己的（bob 没有配置与节点，联系人是 admin）
	other := decode[service.SearchIndex](t, bob.want(200, "GET", "/api/v1/search", nil))
	if len(other.Profiles)+len(other.Sources)+len(other.Nodes)+len(other.Shorts) != 0 || len(other.Friends) != 1 || other.Friends[0].Username != "admin" {
		t.Errorf("bob 的索引：%+v", other)
	}

	// 联系人移入回收站后不再出现
	admin.want(204, "DELETE", "/api/v1/messages/with/bob", nil)
	if idx := decode[service.SearchIndex](t, admin.want(200, "GET", "/api/v1/search", nil)); len(idx.Friends) != 0 {
		t.Errorf("回收站中的对话：%+v", idx.Friends)
	}

	// 令牌不可访问
	tok := decode[tokenDTO](t, admin.want(201, "POST", "/api/v1/account/tokens", map[string]any{"name": "只读", "scope": "read"}))
	if c, _ := bearer(h, tok.Token, "GET", "/api/v1/search", ""); c != 403 {
		t.Errorf("令牌读取索引：%d", c)
	}
}
