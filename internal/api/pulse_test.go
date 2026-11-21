package api

import (
	"testing"

	"github.com/angwz/clash-launch/internal/events"
)

// 在线指示灯（R11）：轮询接口返回版本、未读数与内核是否可用；在线人数默认只给管理员，
// 管理员打开设置后普通用户也能看到（轮询接口与实时通道的 hello 一致）。
func TestPulse(t *testing.T) {
	h := newHarness(t, nil) // 没有内核：kernel 为 false，hello 带 unavailable
	admin, _ := h.setup()
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	reg.want(201, "POST", "/api/v1/register", registerBody(t, reg, "bobby", ""))
	bob := h.client()
	bob.login("bobby", "a long enough password")

	ac, _, err := admin.dialEvents(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ac.CloseNow() }()
	if e := readEvent(t, ac, events.Hello); e.Kernel != "unavailable" {
		t.Fatalf("内核不可用时 hello 应告知：%+v", e)
	}

	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "text": "在吗"})
	got := decode[pulseDTO](t, bob.want(200, "GET", "/api/v1/pulse", nil))
	if got.Version == "" || got.Unread != 1 || got.Kernel || got.Online != 0 {
		t.Fatalf("普通用户默认看不到在线人数：%+v", got)
	}
	if a := decode[pulseDTO](t, admin.want(200, "GET", "/api/v1/pulse", nil)); a.Online != 1 {
		t.Fatalf("管理员应看到在线人数 1：%+v", a)
	}

	bob.want(404, "PUT", "/api/v1/admin/pulse", map[string]bool{"showOnline": true})
	if s := decode[pulseSettings](t, admin.want(200, "GET", "/api/v1/admin/pulse", nil)); s.ShowOnline {
		t.Fatal("默认不向普通用户显示")
	}
	if s := decode[pulseSettings](t, admin.want(200, "PUT", "/api/v1/admin/pulse", map[string]bool{"showOnline": true})); !s.ShowOnline {
		t.Fatal("设置未生效")
	}
	if got := decode[pulseDTO](t, bob.want(200, "GET", "/api/v1/pulse", nil)); got.Online != 1 {
		t.Fatalf("打开设置后普通用户应看到在线人数：%+v", got)
	}
	bc, _, err := bob.dialEvents(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bc.CloseNow() }()
	if e := readEvent(t, bc, events.Hello); e.Online != 2 {
		t.Fatalf("hello 也应带在线人数：%+v", e)
	}
	h.client().want(401, "GET", "/api/v1/pulse", nil)
}
