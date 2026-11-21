package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/angwz/clash-launch/internal/events"
)

// dialEvents 以 c 的登录状态连接实时通道；origin 为空时用服务自己的地址。
func (c *client) dialEvents(t *testing.T, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	if origin == "" {
		origin = c.h.http.URL
	}
	hdr := http.Header{"Origin": {origin}}
	u, _ := http.NewRequest("GET", c.h.http.URL, nil)
	for _, ck := range c.c.Jar.Cookies(u.URL) {
		hdr.Add("Cookie", ck.Name+"="+ck.Value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return websocket.Dial(ctx, "ws"+strings.TrimPrefix(c.h.http.URL, "http")+"/api/v1/events", &websocket.DialOptions{HTTPHeader: hdr})
}

func readEvent(t *testing.T, conn *websocket.Conn, want string) events.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, b, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("等待 %s 事件：%v", want, err)
		}
		var e events.Event
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatal(err)
		}
		if e.Type == want {
			return e
		}
	}
}

// R22：实时通道推送自己的数据变化；别人收不到；跨站连接被拒绝；未登录被拒绝。
func TestEventsStream(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	p := decode[profileDTO](t, c.want(200, "GET", "/api/v1/profiles/"+pid, nil))
	conn, _, err := c.dialEvents(t, "")
	if err != nil {
		t.Fatalf("连接实时通道：%v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	readEvent(t, conn, "hello")

	// 订阅被拉取 → sub 事件，ID 为链接 ID
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{}))
	h.client().do("GET", strings.TrimPrefix(l.URL, h.http.URL), nil)
	if e := readEvent(t, conn, events.Sub); e.ID != l.ID {
		t.Fatalf("sub 事件的 ID 应为链接 %s，得到 %s", l.ID, e.ID)
	}
	// 保存节点源 → source 事件，随后配置重新生成 → render 事件
	srcID := regexp.MustCompile(`sources:\n  - ref: (\S+)`).FindStringSubmatch(p.Doc)[1]
	src := decode[sourceDTO](t, c.want(200, "GET", "/api/v1/sources/"+srcID, nil))
	c.want(200, "PUT", "/api/v1/sources/"+srcID, map[string]any{"name": src.Name, "kind": "batch",
		"content": "- {name: 实时 09, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example-pw}\n"}, "If-Match", `"v`+itoa(int(src.Version))+`"`)
	readEvent(t, conn, events.Source)
	if e := readEvent(t, conn, events.Render); e.ID != pid {
		t.Fatalf("render 事件的 ID 应为配置 %s，得到 %s", pid, e.ID)
	}

	// 跨站与未登录
	if _, resp, err := c.dialEvents(t, "https://evil.example.com"); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("跨站连接应被拒绝（403），得到 %v %v", resp, err)
	}
	if _, resp, err := h.client().dialEvents(t, ""); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("未登录应被拒绝（401），得到 %v %v", resp, err)
	}
}

// R22：「正在输入」只转发给互为联系人的人；陌生人收不到。
func TestEventsTyping(t *testing.T) {
	h := newHarness(t, nil)
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
	bc, _, err := bob.dialEvents(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bc.CloseNow() }()
	readEvent(t, ac, "hello")
	readEvent(t, bc, "hello")
	typing := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := ac.Write(ctx, websocket.MessageText, []byte(`{"type":"typing","to":"bobby"}`)); err != nil {
			t.Fatal(err)
		}
	}

	// 还不是联系人：不转发
	typing()
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	_, b, err := bc.Read(ctx)
	cancel()
	if err == nil {
		t.Fatalf("陌生人不应收到「正在输入」，收到 %s", b)
	}
	// 读超时会关闭连接（coder/websocket 的约定），重新连接
	bc, _, err = bob.dialEvents(t, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bc.CloseNow() }()
	readEvent(t, bc, "hello")

	// 发过消息后互为联系人：转发，ID 为发送者的用户名
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "text": "在吗"})
	typing()
	if e := readEvent(t, bc, events.Typing); e.ID != "admin" {
		t.Fatalf("typing 事件的 ID 应为 admin，得到 %q", e.ID)
	}
}

// R14d：短链接列表的「最近请求」改为逐个短链接取最大时间；被请求过的有时间，没被请求过的没有。
func TestShortLinkLastSeen(t *testing.T) {
	h := newHarness(t, nil)
	c, pid := h.setup()
	l := decode[linkDTO](t, c.want(201, "POST", "/api/v1/profiles/"+pid+"/links", map[string]string{}))
	used := decode[shortDTO](t, c.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "used1"}))
	idle := decode[shortDTO](t, c.want(201, "POST", "/api/v1/short-links", map[string]any{"linkId": l.ID, "code": "idle1"}))
	if r, _ := h.client().do("GET", strings.TrimPrefix(used.URL, h.http.URL), nil); r.StatusCode != http.StatusOK {
		t.Fatalf("经短链接拉取：%d", r.StatusCode)
	}
	list := decode[list[shortDTO]](t, c.want(200, "GET", "/api/v1/short-links", nil))
	got := map[string]*time.Time{}
	for _, x := range list.Items {
		got[x.ID] = x.Last
	}
	if got[used.ID] == nil || got[idle.ID] != nil {
		t.Fatalf("最近请求：用过的应有时间、没用过的应为空，得到 used=%v idle=%v", got[used.ID], got[idle.ID])
	}
}

// R22：联系人上线、下线即时通知（陌生人收不到）；会话列表与会话页带在线状态；
// ping 原样带回 ID；hello 带服务器版本；在线人数只告诉管理员（R11）。
func TestEventsPresenceAndPing(t *testing.T) {
	h := newHarness(t, nil)
	h.svc.Events.SetOfflineGrace(200 * time.Millisecond)
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
	if e := readEvent(t, ac, events.Hello); e.Version == "" || e.Online < 1 {
		t.Fatalf("hello 应带版本与在线人数（管理员）：%+v", e)
	}

	// 还不是联系人：bob 上线，admin 收不到
	bc, _, err := bob.dialEvents(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if e := readEvent(t, bc, events.Hello); e.Online != 0 {
		t.Fatalf("普通用户不应看到在线人数：%+v", e)
	}
	ping := func(c *websocket.Conn, id string) events.Event {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"ping","id":"`+id+`"}`)); err != nil {
			t.Fatal(err)
		}
		return readEvent(t, c, events.Pong)
	}
	if e := ping(bc, "b1"); e.ID != "b1" || e.Online != 0 {
		t.Fatalf("pong：%+v", e)
	}
	if e := ping(ac, "a1"); e.ID != "a1" || e.Online != 2 {
		t.Fatalf("管理员的 pong 应带在线人数 2：%+v", e)
	}
	_ = bc.CloseNow()
	time.Sleep(1100 * time.Millisecond) // bob 下线（宽限期 200 毫秒），也避开 ping 每秒一次的限制
	// 陌生人的上线、下线都不应通知 admin：下一条消息必须直接是这次 ping 的回应
	{
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := ac.Write(ctx, websocket.MessageText, []byte(`{"type":"ping","id":"a2"}`)); err != nil {
			t.Fatal(err)
		}
		_, b, err := ac.Read(ctx)
		cancel()
		if err != nil || !strings.Contains(string(b), `"pong"`) {
			t.Fatalf("陌生人的在线状态不应推送：%s %v", b, err)
		}
	}

	// 互为联系人之后：bob 上线时 admin 收到 presence on，列表与会话页显示在线
	admin.want(201, "POST", "/api/v1/messages", map[string]any{"to": "bobby", "text": "在吗"})
	bc, _, err = bob.dialEvents(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if e := readEvent(t, ac, events.Presence); e.ID != "bobby" || e.State != "on" {
		t.Fatalf("上线通知：%+v", e)
	}
	threads := decode[struct {
		Items []struct {
			Username string `json:"username"`
			Online   bool   `json:"online"`
		} `json:"items"`
	}](t, admin.want(200, "GET", "/api/v1/messages", nil))
	if len(threads.Items) != 1 || !threads.Items[0].Online {
		t.Fatalf("会话列表应显示 bobby 在线：%+v", threads.Items)
	}
	with := decode[struct {
		User struct {
			Online bool `json:"online"`
		} `json:"user"`
	}](t, admin.want(200, "GET", "/api/v1/messages/with/bobby", nil))
	if !with.User.Online {
		t.Fatal("会话页应显示对方在线")
	}
	_ = bc.CloseNow()
	if e := readEvent(t, ac, events.Presence); e.ID != "bobby" || e.State != "off" {
		t.Fatalf("下线通知：%+v", e)
	}
}
