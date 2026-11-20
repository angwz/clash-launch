package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/angwz/clash-launch/internal/events"
)

// 实时通道（需求 R22）：GET /api/v1/events 升级为 WebSocket，服务器把当前用户的数据变化事件推给浏览器。
//
// 安全：
//   - 认证沿用会话 Cookie（authed）；握手时校验 Origin（与写请求同一规则，含多域名 R9），
//     防止别的网站借用户的登录状态建立连接（CSWSH）。
//   - 事件只含类型与对象 ID，数据仍经 API 读取，权限只在 API 一处检查。
//   - 长连接期间每 30 秒重新核对会话：注销、过期、定时退出或账号停用后立即关闭连接。
//   - 浏览器不需要发消息：读取上限 1 KiB，只处理控制帧。
const (
	eventsCheckEvery = 30 * time.Second
	eventsWriteWait  = 10 * time.Second
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
	ctx := c.CloseRead(r.Context()) // 处理 ping/close 控制帧；对方关闭时 ctx 结束

	send := func(e events.Event) error {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		wctx, cancel := context.WithTimeout(ctx, eventsWriteWait)
		defer cancel()
		return c.Write(wctx, websocket.MessageText, b)
	}
	if send(events.Event{Type: "hello"}) != nil {
		return
	}
	tick := time.NewTicker(eventsCheckEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
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
