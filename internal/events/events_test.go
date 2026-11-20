package events

import (
	"errors"
	"sync"
	"testing"
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
	h.Unsubscribe(b)
	if h.Online() != 1 {
		t.Fatalf("注销后在线人数 %d", h.Online())
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
	if h.Online() != 0 {
		t.Fatalf("全部注销后在线人数 %d", h.Online())
	}
}
