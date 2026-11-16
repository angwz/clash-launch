package api

import (
	"context"
	"testing"
	"time"
)

// 需求 R8：登录设备可以定时退出，只能早于自然到期；可以取消；到点后会话失效；「其他设备」一次设定。
func TestScheduleLogout(t *testing.T) {
	h := newHarness(t, nil)
	c1, _ := h.setup()
	c2 := h.client()
	c2.login("admin", "correct horse battery")
	c3 := h.client()
	c3.login("admin", "correct horse battery")

	type item struct {
		ID        string     `json:"id"`
		Current   bool       `json:"current"`
		ExpiresAt time.Time  `json:"expiresAt"`
		LogoutAt  *time.Time `json:"logoutAt"`
	}
	list := func() []item {
		return decode[struct {
			Items []item `json:"items"`
		}](t, c1.want(200, "GET", "/api/v1/account/sessions", nil)).Items
	}
	var other item
	for _, s := range list() {
		if !s.Current {
			other = s
		}
	}
	path := "/api/v1/account/sessions/" + other.ID + "/logout-at"
	c1.want(422, "PUT", path, map[string]any{"at": time.Now().Add(10 * time.Second)})
	c1.want(422, "PUT", path, map[string]any{"at": other.ExpiresAt.Add(time.Hour)})
	c1.want(404, "PUT", "/api/v1/account/sessions/0000000000000000/logout-at", map[string]any{"at": nil})

	at := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	c1.want(200, "PUT", path, map[string]any{"at": at})
	for _, s := range list() {
		if s.ID == other.ID && (s.LogoutAt == nil || !s.LogoutAt.Equal(at)) {
			t.Fatalf("定时未保存：%+v", s)
		}
	}
	c1.want(200, "PUT", path, map[string]any{"at": nil})
	for _, s := range list() {
		if s.LogoutAt != nil {
			t.Fatalf("取消后仍有定时：%+v", s)
		}
	}

	// 其他设备全部定时：当前设备不受影响
	if r := decode[map[string]int](t, c1.want(200, "PUT", "/api/v1/account/sessions/others/logout-at", map[string]any{"at": at})); r["updated"] != 2 {
		t.Fatalf("其他设备：%v", r)
	}
	for _, s := range list() {
		if (s.LogoutAt != nil) == s.Current {
			t.Fatalf("其他设备定时：%+v", s)
		}
	}
	type me struct {
		LogoutAt *time.Time `json:"logoutAt"`
	}
	if m := decode[me](t, c2.want(200, "GET", "/api/v1/me", nil)); m.LogoutAt == nil {
		t.Error("被定时的设备应在 /me 中看到退出时间")
	}

	// 到点：把定时改到过去（接口不允许，直接写存储），会话随即失效
	ss, err := h.db.Sessions().List(context.Background(), mustUserID(t, h))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ss {
		if !s.LogoutAt.IsZero() {
			if err := h.db.Sessions().SetLogoutAt(context.Background(), s.UserID, s.IDHash, time.Now().Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
		}
	}
	c2.want(401, "GET", "/api/v1/me", nil)
	c3.want(401, "GET", "/api/v1/me", nil)
	c1.want(200, "GET", "/api/v1/me", nil)
}

func mustUserID(t *testing.T, h *harness) string {
	t.Helper()
	u, err := h.db.Users().ByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
