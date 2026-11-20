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
