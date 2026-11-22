package api

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/service"
)

// 头像与默认昵称（R19）：注册时随机分配预置头像与按语言的昵称；换预置、随机换、上传（裁剪缩放、重新编码）；
// 图片需要登录才能读取；无效的标识与图片被拒绝；补分配只处理从未分配的账号。
func TestAvatars(t *testing.T) {
	h := newHarness(t, nil)
	admin, _ := h.setup()
	ctx := context.Background()
	me := decode[sessionResp](t, admin.want(200, "GET", "/api/v1/me", nil)).User
	if !service.ValidPreset(me.Avatar) || me.Nickname == "" {
		t.Fatalf("管理员应分配到预置头像与昵称：%+v", me)
	}

	// 中文界面注册：中文昵称
	admin.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	reg := h.client()
	body := registerBody(t, reg, "bob", "")
	body["lang"] = "zh-CN"
	reg.want(201, "POST", "/api/v1/register", body)
	bob := h.client()
	bob.login("bob", "a long enough password")
	b := decode[sessionResp](t, bob.want(200, "GET", "/api/v1/me", nil)).User
	if !service.ValidPreset(b.Avatar) || !strings.ContainsFunc(b.Nickname, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }) {
		t.Errorf("中文界面注册：%+v", b)
	}

	// 换预置、随机换、无效的标识
	if r := decode[map[string]string](t, bob.want(200, "PUT", "/api/v1/account/avatar", map[string]string{"preset": "p:comet-2"})); r["avatar"] != "p:comet-2" {
		t.Errorf("换预置：%v", r)
	}
	if r := decode[map[string]string](t, bob.want(200, "PUT", "/api/v1/account/avatar", map[string]string{})); r["avatar"] == "p:comet-2" || !service.ValidPreset(r["avatar"]) {
		t.Errorf("随机换：%v", r)
	}
	for _, bad := range []string{"p:comet-0", "p:comet-7", "p:dragon-1", "u:abc", "p:comet-02"} {
		bob.want(422, "PUT", "/api/v1/account/avatar", map[string]string{"preset": bad})
	}

	// 上传：800×400 的 PNG（带透明）→ 256×256 PNG；不透明的 → JPEG
	img := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	for y := range 400 {
		for x := range 800 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 200, A: 200})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	up := decode[map[string]string](t, bob.want(200, "POST", "/api/v1/account/avatar/upload", rawBody(buf.Bytes())))
	if !strings.HasPrefix(up["avatar"], "u:") {
		t.Fatalf("上传：%v", up)
	}
	resp, data := admin.do("GET", "/api/v1/avatars/"+b.ID, nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("读取头像：%d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	got, err := png.Decode(bytes.NewReader(data))
	if err != nil || got.Bounds().Dx() != 256 || got.Bounds().Dy() != 256 {
		t.Errorf("头像应为 256×256：%v %v", err, got.Bounds())
	}
	opaque := image.NewRGBA(image.Rect(0, 0, 100, 120))
	buf.Reset()
	_ = jpeg.Encode(&buf, opaque, nil)
	bob.want(200, "POST", "/api/v1/account/avatar/upload", rawBody(buf.Bytes()))
	resp, data = admin.do("GET", "/api/v1/avatars/"+b.ID, nil)
	if c, err := jpeg.DecodeConfig(bytes.NewReader(data)); resp.Header.Get("Content-Type") != "image/jpeg" || err != nil || c.Width != 100 || c.Height != 100 {
		t.Errorf("小于 256 的图只裁不放大：%s %v %+v", resp.Header.Get("Content-Type"), err, c)
	}
	if r, _ := h.client().do("GET", "/api/v1/avatars/"+b.ID, nil); r.StatusCode != 401 {
		t.Errorf("未登录读取头像：%d", r.StatusCode)
	}
	bob.want(422, "POST", "/api/v1/account/avatar/upload", rawBody([]byte("not an image")))
	bob.want(413, "POST", "/api/v1/account/avatar/upload", rawBody(bytes.Repeat([]byte{0}, 2<<20+10)))
	// 换回预置：图片删除
	bob.want(200, "PUT", "/api/v1/account/avatar", map[string]string{"preset": "p:moon-1"})
	if r, _ := admin.do("GET", "/api/v1/avatars/"+b.ID, nil); r.StatusCode != 404 {
		t.Errorf("换回预置后图片应删除：%d", r.StatusCode)
	}

	// 昵称建议
	if n := decode[map[string]string](t, bob.want(200, "GET", "/api/v1/account/nickname/suggest?lang=en", nil))["nickname"]; !strings.Contains(n, " ") {
		t.Errorf("英文昵称建议：%q", n)
	}

	// 补分配：只处理头像为空的账号；已清空昵称、已有头像的账号不受影响
	bob.want(200, "PUT", "/api/v1/account/nickname", map[string]string{"nickname": ""})
	u, _ := h.db.Users().ByUsername(ctx, "bob")
	if err := h.db.Users().SetAvatar(ctx, u.ID, ""); err != nil {
		t.Fatal(err)
	}
	a, _ := h.db.Users().ByUsername(ctx, "admin")
	if err := h.svc.AssignMissingAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	u2, _ := h.db.Users().ByUsername(ctx, "bob")
	a2, _ := h.db.Users().ByUsername(ctx, "admin")
	if !service.ValidPreset(u2.Avatar) || u2.Nickname == "" || a2.Avatar != a.Avatar || a2.Nickname != a.Nickname {
		t.Errorf("补分配：bob=%q/%q admin=%q→%q", u2.Avatar, u2.Nickname, a.Avatar, a2.Avatar)
	}
}

// rawBody 让 client.do 把字节原样作为请求体。
type rawBody []byte
