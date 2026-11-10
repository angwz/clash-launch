package api

import (
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
)

// TOTP：确认后才启用；登录需要第二步且不提前建立会话；同一时间步不能重放；恢复码一次性；关闭需要口令。
func TestTOTPFlow(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	const pw = "correct horse battery"

	st := decode[map[string]string](t, admin.want(200, "POST", "/api/v1/account/totp/setup", nil))
	secret := st["secret"]
	admin.want(401, "POST", "/api/v1/account/totp/enable", map[string]string{"code": "000000"})
	now := time.Now()
	code, _ := auth.TOTPCode(secret, auth.TOTPStep(now))
	codes := decode[codesResp](t, admin.want(200, "POST", "/api/v1/account/totp/enable", map[string]string{"code": code})).RecoveryCodes
	if len(codes) != 10 {
		t.Fatalf("恢复码：%v", codes)
	}
	admin.want(409, "POST", "/api/v1/account/totp/setup", nil)

	// 口令正确只得到凭证，不建立会话
	c := h.client()
	login := func() string {
		r := decode[mfaResp](t, c.want(200, "POST", "/api/v1/session", map[string]string{"username": "admin", "password": pw}))
		if len(r.MFA) == 0 || r.MFA[0] != "totp" || r.Ticket == "" {
			t.Fatalf("应要求第二步：%+v", r)
		}
		return r.Ticket
	}
	ticket := login()
	c.want(401, "GET", "/api/v1/me", nil)
	c.want(401, "POST", "/api/v1/session/totp", map[string]string{"ticket": "forged.x", "code": code})
	// 启用时用过的时间步不能再用
	c.want(401, "POST", "/api/v1/session/totp", map[string]string{"ticket": ticket, "code": code})
	next, _ := auth.TOTPCode(secret, auth.TOTPStep(now)+1)
	sess := decode[sessionResp](t, c.want(200, "POST", "/api/v1/session/totp", map[string]string{"ticket": ticket, "code": next}))
	c.csrf = sess.CSRF
	c.want(200, "GET", "/api/v1/me", nil)

	// 恢复码：一次性，大小写与连字符不影响
	c2 := h.client()
	ticket = decode[mfaResp](t, c2.want(200, "POST", "/api/v1/session", map[string]string{"username": "admin", "password": pw})).Ticket
	c2.want(200, "POST", "/api/v1/session/totp", map[string]string{"ticket": ticket, "code": codes[0]})
	c3 := h.client()
	ticket = decode[mfaResp](t, c3.want(200, "POST", "/api/v1/session", map[string]string{"username": "admin", "password": pw})).Ticket
	c3.want(401, "POST", "/api/v1/session/totp", map[string]string{"ticket": ticket, "code": codes[0]})
	if st := decode[map[string]any](t, admin.want(200, "GET", "/api/v1/account/totp", nil)); st["remainingCodes"] != float64(9) {
		t.Errorf("剩余恢复码：%v", st)
	}

	// 重新生成与关闭都需要口令
	admin.want(403, "POST", "/api/v1/account/totp/recovery-codes", map[string]string{"password": "wrong"})
	admin.want(200, "POST", "/api/v1/account/totp/recovery-codes", map[string]string{"password": pw})
	admin.want(403, "POST", "/api/v1/account/totp/disable", map[string]string{"password": "wrong"})
	admin.want(204, "POST", "/api/v1/account/totp/disable", map[string]string{"password": pw})
	c4 := h.client()
	c4.login("admin", pw)
	c4.want(200, "GET", "/api/v1/me", nil)
}
