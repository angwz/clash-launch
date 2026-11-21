package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/angwz/clash-launch/internal/events"
	"github.com/angwz/clash-launch/internal/version"
)

// 实时通道（需求 R22）：GET /api/v1/events 升级为 WebSocket，服务器把当前用户的数据变化事件推给浏览器。
//
// 安全：
//   - 认证沿用会话 Cookie（authed）；握手时校验 Origin（与写请求同一规则，含多域名 R9），
//     防止别的网站借用户的登录状态建立连接（CSWSH）。
//   - 事件只含类型与对象 ID，数据仍经 API 读取，权限只在 API 一处检查。
//   - 长连接期间每 30 秒重新核对会话：注销、过期、定时退出或账号停用后立即关闭连接。
//   - 浏览器只发两种消息，读取上限 1 KiB，其他内容忽略：
//     {"type":"typing","to":"<用户名>"}：只转发给互为联系人、且双方都没有把对话放进回收站的人，每条连接每 2 秒最多一次；
//     {"type":"ping","id":"<任意短串>"}：回 pong（带回 id，供浏览器测往返延迟），每条连接每秒最多一次。
//   - 在线人数目前只告诉管理员（管理员可设置是否向普通用户显示，见 backlog R11）。
const (
	eventsCheckEvery = 30 * time.Second
	eventsWriteWait  = 10 * time.Second
	typingEvery      = 2 * time.Second
	pingEvery        = time.Second
)

func (s *Server) eventsStream(w http.ResponseWriter, r *http.Request, p *principal) {
	if s.Svc.Events == nil {
		problem(w, http.StatusServiceUnavailable, "events.unavailable", "realtime events are not enabled")
		return
	}
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "auth.origin", "cross-origin websocket is not allowed")
		return
	}
	sub, err := s.Svc.Events.Subscribe(p.User.ID)
	if errors.Is(err, events.ErrTooMany) {
		problem(w, http.StatusTooManyRequests, "events.too_many", "too many realtime connections")
		return
	}
	defer s.Svc.Events.Unsubscribe(sub)
	// http.Server 的读写超时（30/60 秒）会留在被接管的连接上，升级前清除；之后由心跳与写超时管理
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	// Origin 已按本服务的规则校验，跳过库自带的同源检查（它不认识多域名配置）
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return // Accept 已写出错误响应
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(1 << 10)
	online := func() int {
		if p.User.Role == "admin" {
			return s.Svc.Events.Online()
		}
		return 0
	}
	// 读循环：处理控制帧、「正在输入」与 ping；对方关闭或读取出错时结束整个连接。
	// 回应经 replies 交给写循环发送，写操作只在一个 goroutine 里进行。
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	replies := make(chan events.Event, 4)
	go func() {
		defer cancel()
		var lastTyping, lastPing time.Time
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			var m struct {
				Type string `json:"type"`
				To   string `json:"to"`
				ID   string `json:"id"`
			}
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			switch m.Type {
			case events.Typing:
				if time.Since(lastTyping) >= typingEvery && s.forwardTyping(ctx, p, m.To) {
					lastTyping = time.Now()
				}
			case "ping":
				if time.Since(lastPing) < pingEvery || len(m.ID) > 32 {
					continue
				}
				lastPing = time.Now()
				select {
				case replies <- events.Event{Type: events.Pong, ID: m.ID, Online: online()}:
				default: // 写循环忙：丢弃，浏览器把这次当作超时
				}
			}
		}
	}()

	send := func(e events.Event) error {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		wctx, cancel := context.WithTimeout(ctx, eventsWriteWait)
		defer cancel()
		return c.Write(wctx, websocket.MessageText, b)
	}
	if send(events.Event{Type: events.Hello, Version: version.Version, Online: online()}) != nil {
		return
	}
	tick := time.NewTicker(eventsCheckEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-replies:
			if send(e) != nil {
				return
			}
		case e := <-sub.C:
			if sub.Lagged() {
				e = events.Event{Type: events.Resync}
			}
			if send(e) != nil {
				return
			}
		case <-tick.C:
			if _, err := s.principal(r); err != nil {
				_ = c.Close(websocket.StatusPolicyViolation, "session ended")
				return
			}
			pctx, cancel := context.WithTimeout(ctx, eventsWriteWait)
			err := c.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// forwardTyping 把「正在输入」转给对方；返回是否转发。只有双方互为联系人、且都没有把对话放进回收站时才转发，
// 不能借此打扰陌生人或探测账号是否存在（不论结果都不回应）。
func (s *Server) forwardTyping(ctx context.Context, p *principal, to string) bool {
	if to == "" {
		return false
	}
	peer, err := s.Svc.Store.Users().ByUsername(ctx, to)
	if err != nil || peer.ID == p.User.ID || !s.Svc.Mutual(ctx, p.User.ID, peer.ID) {
		return false
	}
	s.Svc.Events.Publish(peer.ID, events.Event{Type: events.Typing, ID: p.User.Username})
	return true
}
