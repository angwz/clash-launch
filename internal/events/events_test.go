package events

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHubRoutesPerUser(t *testing.T) {
	h := New(2)
	a, _ := h.Subscribe("alice")
	b, _ := h.Subscribe("bob")
	h.Publish("alice", Event{Type: Render, ID: "p1"})
	if got := <-a.C; got.Type != Render || got.ID != "p1" {
		t.Fatalf("alice 收到 %+v", got)
	}
	select {
	case e := <-b.C:
		t.Fatalf("bob 不应收到 alice 的事件：%+v", e)
	default:
	}
	if h.Online() != 2 {
		t.Fatalf("在线人数 %d", h.Online())
	}
	h.grace = 20 * time.Millisecond
	h.Unsubscribe(b)
	if h.Online() != 2 || !h.IsOnline("bob") {
		t.Fatalf("宽限期内仍算在线：%d", h.Online())
	}
	time.Sleep(100 * time.Millisecond)
	if h.Online() != 1 || h.IsOnline("bob") {
		t.Fatalf("宽限期过后在线人数 %d", h.Online())
	}
}

func TestHubLimitAndLag(t *testing.T) {
	h := New(1)
	s, err := h.Subscribe("u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Subscribe("u"); !errors.Is(err, ErrTooMany) {
		t.Fatalf("超过上限应返回 ErrTooMany，得到 %v", err)
	}
	// 缓冲写满后发布不阻塞，并标记落后
	for i := 0; i < bufferSize+10; i++ {
		h.Publish("u", Event{Type: Sub})
	}
	if !s.Lagged() {
		t.Fatal("缓冲满后应标记落后")
	}
	if s.Lagged() {
		t.Fatal("Lagged 应在读取后清除")
	}
	var nilHub *Hub
	nilHub.Publish("u", Event{Type: Sub}) // nil 安全
}

func TestHubConcurrent(t *testing.T) {
	h := New(0)
	h.grace = time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _ := h.Subscribe("u")
			for j := 0; j < 100; j++ {
				h.Publish("u", Event{Type: Message})
			}
			h.Unsubscribe(s)
		}()
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond) // 等宽限期过去
	if h.Online() != 0 {
		t.Fatalf("全部注销后在线人数 %d", h.Online())
	}
}

// 上线、下线只在第一条连接与最后一条连接（加宽限期）时通知；宽限期内重连不产生任何通知。
func TestHubPresence(t *testing.T) {
	h := New(4)
	h.grace = 50 * time.Millisecond
	got := make(chan string, 8)
	h.OnPresence(func(u string, on bool) {
		if on {
			got <- u + ":on"
		} else {
			got <- u + ":off"
		}
	})
	next := func() string {
		t.Helper()
		select {
		case v := <-got:
			return v
		case <-time.After(2 * time.Second):
			t.Fatal("没有收到通知")
			return ""
		}
	}
	quiet := func(d time.Duration) {
		t.Helper()
		select {
		case v := <-got:
			t.Fatalf("不应有通知：%s", v)
		case <-time.After(d):
		}
	}
	a1, _ := h.Subscribe("alice")
	if v := next(); v != "alice:on" {
		t.Fatalf("第一条连接：%s", v)
	}
	a2, _ := h.Subscribe("alice") // 第二个标签页
	h.Unsubscribe(a1)
	quiet(80 * time.Millisecond)
	h.Unsubscribe(a2)
	a3, _ := h.Subscribe("alice") // 刷新页面：宽限期内重连
	quiet(80 * time.Millisecond)
	h.Unsubscribe(a3)
	if v := next(); v != "alice:off" {
		t.Fatalf("最后一条断开并过了宽限期：%s", v)
	}
	if h.IsOnline("alice") || h.Online() != 0 {
		t.Fatal("下线后不应再算在线")
	}
	_, _ = h.Subscribe("alice")
	if v := next(); v != "alice:on" {
		t.Fatalf("再次上线：%s", v)
	}
}
