// Package events 是进程内的实时事件中心（需求 R22）：服务层在数据变化时按用户发布事件，
// 实时通道（internal/api 的 WebSocket）把事件推给这个用户已连接的浏览器。
//
// 设计要点：
//   - 事件只说明「什么变了」（类型与对象 ID），不携带数据本身；浏览器收到后照常经 API 重新读取，
//     权限检查只在 API 一处，事件通道不会成为绕过授权的旁路。
//   - 发布从不阻塞：每个连接有固定大小的缓冲，满了就丢弃并标记「落后」，连接随后发出 resync，
//     让浏览器把所有数据重读一遍。慢连接不会拖慢渲染、拉取等发布方。
//   - 在线状态：用户的第一条连接建立时算上线；最后一条断开后再等 offlineGrace 才算下线，
//     刷新页面、切换网络造成的短暂断开不会让联系人看到一下线一上线。
//   - 只在单个进程内分发。多实例部署时需要改为 PostgreSQL 的 LISTEN/NOTIFY（见 backlog R22）。
package events

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// 事件类型。前端按类型决定刷新哪些数据（web/src/lib/realtime.svelte.ts）。
const (
	Render    = "render"    // 配置重新生成（ID 为配置 ID）：检查结果、订阅内容可能变了
	Source    = "source"    // 节点源拉取或保存（ID 为节点源 ID）
	Rules     = "rules"     // 规则源拉取或保存（ID 为规则源 ID）
	Message   = "message"   // 收到或发出消息、已读状态变化
	Sub       = "sub"       // 订阅链接或短链接被客户端拉取（ID 为链接 ID）
	Grant     = "grant"     // 授权建立或收回
	Resync    = "resync"    // 连接落后、丢过事件：浏览器应重读全部数据
	Typing    = "typing"    // 联系人正在输入（ID 为对方的用户名）；只转发、不保存
	Presence  = "presence"  // 联系人上线或下线（ID 为对方的用户名，State 为 on 或 off）
	Hello     = "hello"     // 连接建立：附服务器版本（新版本提示）与在线人数
	Pong      = "pong"      // 回应浏览器的 ping（ID 原样带回，用于测往返延迟），附在线人数
	Notice    = "notice"    // 新的站内提醒（R23、R24；ID 为提醒 ID）
	Footprint = "footprint" // 新记下的足迹（R10；ID 为类别，State 为区分）
	Announce  = "announce"  // 站内公告发布、修改或删除（R32）：浏览器重读公告
)

// Event 是推给浏览器的一条通知。
type Event struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	State   string `json:"state,omitempty"`   // presence：on | off
	Online  int    `json:"online,omitempty"`  // hello、pong：在线人数（只发给有权看的人）
	Version string `json:"version,omitempty"` // hello：服务器版本
	Kernel  string `json:"kernel,omitempty"`  // hello：服务端内核检查不可用时为 unavailable（指示灯降级）
}

// ErrTooMany 表示这个用户的连接数已达上限。
var ErrTooMany = errors.New("events: too many connections for this user")

// bufferSize 是每个连接的事件缓冲。浏览器处理事件只是触发一次重读，正常情况下远用不满。
const bufferSize = 64

// offlineGrace 是最后一条连接断开后、判定下线前的等待时间。
const offlineGrace = 15 * time.Second

// Subscription 是一个连接的订阅。
type Subscription struct {
	C      chan Event
	user   string
	lagged atomic.Bool
}

// Lagged 报告并清除「丢过事件」标记。
func (s *Subscription) Lagged() bool { return s.lagged.Swap(false) }

// Hub 按用户分发事件。零值不可用，用 New 构造；nil *Hub 的方法都是空操作，便于在测试与工具中省略。
type Hub struct {
	mu       sync.Mutex
	subs     map[string]map[*Subscription]struct{}
	leaving  map[string]*time.Timer // 已断开最后一条连接、还在宽限期内的用户
	perUser  int
	grace    time.Duration
	presence func(user string, online bool)
}

// New 构造事件中心；perUser 是每个用户同时保持的连接数上限（多个标签页、多台设备）。
func New(perUser int) *Hub {
	return &Hub{subs: map[string]map[*Subscription]struct{}{}, leaving: map[string]*time.Timer{}, perUser: perUser, grace: offlineGrace}
}

// OnPresence 登记上线、下线的回调（在独立的 goroutine 中调用，不持有锁）。只应在开始服务前调用一次。
func (h *Hub) OnPresence(fn func(user string, online bool)) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.presence = fn
}

// Subscribe 为用户登记一个连接。
func (h *Hub) Subscribe(user string) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[user]
	if h.perUser > 0 && len(set) >= h.perUser {
		return nil, ErrTooMany
	}
	if set == nil {
		set = map[*Subscription]struct{}{}
		h.subs[user] = set
		if t := h.leaving[user]; t != nil {
			// 宽限期内重新连上：对联系人来说从未下线
			t.Stop()
			delete(h.leaving, user)
		} else {
			h.announce(user, true)
		}
	}
	s := &Subscription{C: make(chan Event, bufferSize), user: user}
	set[s] = struct{}{}
	return s, nil
}

// Unsubscribe 注销连接。之后不再向 s.C 发送；s.C 不关闭，避免与并发的 Publish 竞争。
func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.subs[s.user]
	if set == nil {
		return
	}
	delete(set, s)
	if len(set) > 0 {
		return
	}
	delete(h.subs, s.user)
	var t *time.Timer
	t = time.AfterFunc(h.grace, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		// 只有仍是这一个计时器时才下线：期间重连又断开会换成新的计时器
		if h.leaving[s.user] == t {
			delete(h.leaving, s.user)
			h.announce(s.user, false)
		}
	})
	h.leaving[s.user] = t
}

// announce 在持有锁时调用：回调放到新的 goroutine，避免回调里的数据库查询拖住连接的建立与关闭。
func (h *Hub) announce(user string, online bool) {
	if fn := h.presence; fn != nil {
		go fn(user, online)
	}
}

// Publish 把事件发给用户的全部连接，不阻塞。
func (h *Hub) Publish(user string, e Event) {
	if h == nil || user == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[user] {
		select {
		case s.C <- e:
		default:
			s.lagged.Store(true)
		}
	}
}

// PublishAll 把事件发给所有在线用户的全部连接，不阻塞（站内公告，R32）。
func (h *Hub) PublishAll(e Event) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, set := range h.subs {
		for s := range set {
			select {
			case s.C <- e:
			default:
				s.lagged.Store(true)
			}
		}
	}
}

// IsOnline 报告用户是否在线（有连接，或刚断开、仍在宽限期内）。
func (h *Hub) IsOnline(user string) bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.subs[user] != nil || h.leaving[user] != nil
}

// Online 返回在线的用户数（含宽限期内的），供在线指示灯 R11 使用。
func (h *Hub) Online() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs) + len(h.leaving)
}

// SetOfflineGrace 调整判定下线前的等待时间（默认 15 秒）；只应在开始服务前调用，供测试缩短等待。
func (h *Hub) SetOfflineGrace(d time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.grace = d
}
