// Package events 是进程内的实时事件中心（需求 R22）：服务层在数据变化时按用户发布事件，
// 实时通道（internal/api 的 WebSocket）把事件推给这个用户已连接的浏览器。
//
// 设计要点：
//   - 事件只说明「什么变了」（类型与对象 ID），不携带数据本身；浏览器收到后照常经 API 重新读取，
//     权限检查只在 API 一处，事件通道不会成为绕过授权的旁路。
//   - 发布从不阻塞：每个连接有固定大小的缓冲，满了就丢弃并标记「落后」，连接随后发出 resync，
//     让浏览器把所有数据重读一遍。慢连接不会拖慢渲染、拉取等发布方。
//   - 只在单个进程内分发。多实例部署时需要改为 PostgreSQL 的 LISTEN/NOTIFY（见 backlog R22）。
package events

import (
	"errors"
	"sync"
	"sync/atomic"
)

// 事件类型。前端按类型决定刷新哪些数据（web/src/lib/realtime.ts）。
const (
	Render  = "render"  // 配置重新生成（ID 为配置 ID）：检查结果、订阅内容可能变了
	Source  = "source"  // 节点源拉取或保存（ID 为节点源 ID）
	Rules   = "rules"   // 规则源拉取或保存（ID 为规则源 ID）
	Message = "message" // 收到或发出消息、已读状态变化
	Sub     = "sub"     // 订阅链接或短链接被客户端拉取（ID 为链接 ID）
	Grant   = "grant"   // 授权建立或收回
	Resync  = "resync"  // 连接落后、丢过事件：浏览器应重读全部数据
)

// Event 是推给浏览器的一条通知。
type Event struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

// ErrTooMany 表示这个用户的连接数已达上限。
var ErrTooMany = errors.New("events: too many connections for this user")

// bufferSize 是每个连接的事件缓冲。浏览器处理事件只是触发一次重读，正常情况下远用不满。
const bufferSize = 64

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
	mu      sync.Mutex
	subs    map[string]map[*Subscription]struct{}
	perUser int
}

// New 构造事件中心；perUser 是每个用户同时保持的连接数上限（多个标签页、多台设备）。
func New(perUser int) *Hub {
	return &Hub{subs: map[string]map[*Subscription]struct{}{}, perUser: perUser}
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
	}
	s := &Subscription{C: make(chan Event, bufferSize), user: user}
	set[s] = struct{}{}
	return s, nil
}

// Unsubscribe 注销连接。之后不再向 s.C 发送；s.C 不关闭，避免与并发的 Publish 竞争。
func (h *Hub) Unsubscribe(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.subs[s.user]; set != nil {
		delete(set, s)
		if len(set) == 0 {
			delete(h.subs, s.user)
		}
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

// Online 返回当前至少有一个连接的用户数（供在线指示灯 R11 使用）。
func (h *Hub) Online() int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
