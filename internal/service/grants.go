package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/events"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/store"
)

// 授权、联系人与回收站（反馈 #75）。

// contactRetention 是删除的对话在回收站中保存的时间。
const contactRetention = 365 * 24 * time.Hour

// usesSource 返回「配置引用了这个节点源」的判断，用于重新生成受影响的配置。
func usesSource(id string) func(*profile.Profile) bool {
	return func(p *profile.Profile) bool {
		for _, r := range p.Sources {
			if r.Ref == id {
				return true
			}
		}
		return false
	}
}

// RevokeGrant 收回一项授权：授权者与被授权者都可以操作（被授权者收回即「移除」）。
// 短链接授权：为对方生成的短链接立即停用（记录保留，统计不丢）；节点授权：重新生成对方用到它的配置，
// 下发的配置中不再包含这些节点。已收回时不报错。
func (s *Service) RevokeGrant(ctx context.Context, u *store.User, id string) error {
	g, err := s.Store.Grants().Get(ctx, u.ID, id)
	if err != nil {
		return err
	}
	if !g.Active() {
		return nil
	}
	if err := s.Store.Grants().Revoke(ctx, u.ID, id, s.now()); err != nil {
		return err
	}
	s.push(g.OwnerID, events.Grant, g.ID)
	s.push(g.GranteeID, events.Grant, g.ID)
	switch g.Kind {
	case "short":
		if sl, err := s.Store.ShortLinks().Get(ctx, g.OwnerID, g.TargetID); err == nil && !sl.Disabled {
			sl.Disabled = true
			if err := s.Store.ShortLinks().Update(ctx, g.OwnerID, sl); err != nil {
				return err
			}
		}
	case "source":
		s.rerenderUsing(ctx, g.GranteeID, usesSource(g.TargetID))
	}
	s.audit(ctx, u.ID, "grant.revoke", map[string]string{"kind": g.Kind})
	return nil
}

// GrantedSource 是别人授权给我的节点源（只读软链接）。
type GrantedSource struct {
	Grant  store.Grant
	Source *store.Source
	From   string // 授权者的用户名
}

// GrantedSources 返回用户收到的有效节点授权。授权者已删除节点源的不列出。
func (s *Service) GrantedSources(ctx context.Context, u *store.User) ([]GrantedSource, error) {
	gs, err := s.Store.Grants().Received(ctx, u.ID, true)
	if err != nil {
		return nil, err
	}
	var out []GrantedSource
	names := map[string]string{}
	for _, g := range gs {
		if g.Kind != "source" {
			continue
		}
		src, err := s.Store.Sources().Get(ctx, g.OwnerID, g.TargetID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if _, ok := names[g.OwnerID]; !ok {
			if ou, err := s.Store.Users().ByID(ctx, g.OwnerID); err == nil {
				names[g.OwnerID] = ou.Username
			}
		}
		out = append(out, GrantedSource{Grant: g, Source: src, From: names[g.OwnerID]})
	}
	return out, nil
}

// resolveSource 查找配置引用的节点源：先找自己的，再找别人授权给自己的（反馈 #75）。
// 返回节点源与它的所有者（快照按所有者保存）；granted 为 true 时，这是只读的授权节点。
func (s *Service) resolveSource(ctx context.Context, owner, id string) (src *store.Source, srcOwner string, granted bool, err error) {
	src, err = s.Store.Sources().Get(ctx, owner, id)
	if err == nil || !errors.Is(err, store.ErrNotFound) {
		return src, owner, false, err
	}
	g, gerr := s.Store.Grants().ActiveFor(ctx, owner, "source", id)
	if gerr != nil {
		return nil, "", false, err // 没有授权：仍然报告「不存在」
	}
	src, err = s.Store.Sources().Get(ctx, g.OwnerID, id)
	return src, g.OwnerID, true, err
}

// rerenderGrantees 在授权者的节点源更新后，重新生成被授权者用到它的配置。
func (s *Service) rerenderGrantees(ctx context.Context, owner, sourceID string) {
	gs, err := s.Store.Grants().ByTarget(ctx, owner, "source", sourceID)
	if err != nil {
		return
	}
	for _, g := range gs {
		s.rerenderUsing(ctx, g.GranteeID, usesSource(sourceID))
	}
}

// —— 联系人与回收站 ——

// Thread 的列表来自联系人：交流过的人永久保留，按最近交流时间排序。

// Threads 返回用户的会话列表（最近交流的在前），每个会话附最后一条消息的预览与未读数。
func (s *Service) Threads(ctx context.Context, u *store.User) ([]Thread, error) {
	cs, err := s.Store.Contacts().List(ctx, u.ID, false)
	if err != nil {
		return nil, err
	}
	out := make([]Thread, 0, len(cs))
	for _, c := range cs {
		peer, err := s.Store.Users().ByID(ctx, c.PeerID)
		if err != nil {
			continue
		}
		t := Thread{UserID: c.PeerID, Username: peer.Username, Nickname: peer.Nickname, LastAt: c.LastAt}
		if ms, err := s.Store.Messages().Thread(ctx, u.ID, c.PeerID, store.Page{Limit: 1}); err == nil && len(ms) > 0 {
			var body MessageBody
			_ = json.Unmarshal(ms[0].Body, &body)
			t.Last, t.LastKind, t.Mine = preview(body), ms[0].Kind, ms[0].SenderID == u.ID
		}
		if n, err := s.Store.Messages().UnreadFrom(ctx, u.ID, c.PeerID); err == nil {
			t.Unread = n
		}
		t.Online = s.PeerOnline(ctx, u.ID, c.PeerID)
		out = append(out, t)
	}
	return out, nil
}

// Mutual 报告两人是否互为联系人，且双方都没有把对话放进回收站。
// 在线状态与「正在输入」只在这种关系下可见：陌生人、被删除对话的人无从得知你是否在线。
func (s *Service) Mutual(ctx context.Context, a, b string) bool {
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		c, err := s.Store.Contacts().Get(ctx, pair[0], pair[1])
		if err != nil || !c.DeletedAt.IsZero() {
			return false
		}
	}
	return true
}

// PeerOnline 报告 me 能否看到 peer 在线（需求 R22）：互为联系人且对方此刻有实时连接。
func (s *Service) PeerOnline(ctx context.Context, me, peer string) bool {
	return s.Events.IsOnline(peer) && s.Mutual(ctx, me, peer)
}

// AnnouncePresence 把 user 的上线或下线通知给能看到它的联系人。由事件中心在独立的 goroutine 中调用。
func (s *Service) AnnouncePresence(user string, online bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	u, err := s.Store.Users().ByID(ctx, user)
	if err != nil {
		return
	}
	cs, err := s.Store.Contacts().List(ctx, user, false)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("在线状态：读取联系人失败", "user", user, "error", err)
		}
		return
	}
	state := "off"
	if online {
		state = "on"
	}
	for _, c := range cs {
		if s.Mutual(ctx, user, c.PeerID) {
			s.Events.Publish(c.PeerID, events.Event{Type: events.Presence, ID: u.Username, State: state})
		}
	}
}

// DeletedThread 是回收站中的会话。
type DeletedThread struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	DeletedAt time.Time `json:"deletedAt"`
	PurgeAt   time.Time `json:"purgeAt"`
}

// DeletedThreads 返回回收站中的会话。
func (s *Service) DeletedThreads(ctx context.Context, u *store.User) ([]DeletedThread, error) {
	cs, err := s.Store.Contacts().List(ctx, u.ID, true)
	if err != nil {
		return nil, err
	}
	out := make([]DeletedThread, 0, len(cs))
	for _, c := range cs {
		peer, err := s.Store.Users().ByID(ctx, c.PeerID)
		if err != nil {
			continue
		}
		out = append(out, DeletedThread{UserID: c.PeerID, Username: peer.Username, DeletedAt: c.DeletedAt, PurgeAt: c.DeletedAt.Add(contactRetention)})
	}
	return out, nil
}

// peerByName 按用户名找到对方，且对方必须是自己的联系人。
func (s *Service) peerByName(ctx context.Context, u *store.User, name string) (*store.User, error) {
	peer, err := s.Store.Users().ByUsername(ctx, strings.TrimSpace(name))
	if err != nil {
		return nil, store.ErrNotFound
	}
	if _, err := s.Store.Contacts().Get(ctx, u.ID, peer.ID); err != nil {
		return nil, err
	}
	return peer, nil
}

// DeleteThread 删除与某人的对话：放入回收站保存一年，标记已读，并收回双方之间的全部授权。
// 对方的列表不受影响；任何一方再发消息时，对话自动恢复。
func (s *Service) DeleteThread(ctx context.Context, u *store.User, name string) error {
	peer, err := s.peerByName(ctx, u, name)
	if err != nil {
		return err
	}
	if err := s.Store.Contacts().SetDeleted(ctx, u.ID, peer.ID, s.now()); err != nil {
		return err
	}
	if err := s.Store.Messages().MarkRead(ctx, u.ID, peer.ID, s.now()); err != nil {
		return err
	}
	gs, err := s.Store.Grants().Between(ctx, u.ID, peer.ID)
	if err != nil {
		return err
	}
	for _, g := range gs {
		if err := s.RevokeGrant(ctx, u, g.ID); err != nil {
			return err
		}
	}
	return nil
}

// RestoreThread 从回收站恢复对话（被收回的授权不会恢复）。
func (s *Service) RestoreThread(ctx context.Context, u *store.User, name string) error {
	peer, err := s.peerByName(ctx, u, name)
	if err != nil {
		return err
	}
	return s.Store.Contacts().SetDeleted(ctx, u.ID, peer.ID, time.Time{})
}

// PurgeThreads 永久清除回收站中超过一年的对话（每天执行一次）。
func (s *Service) PurgeThreads(ctx context.Context) {
	n, err := s.Store.Contacts().Purge(ctx, s.now().Add(-contactRetention))
	if err != nil && s.Log != nil {
		s.Log.Error("清理回收站失败", "error", err)
	} else if n > 0 && s.Log != nil {
		s.Log.Info("清理回收站", "threads", n)
	}
}

// UserHit 是用户名联想的一项。
type UserHit struct {
	Username string `json:"username"`
	Nickname string `json:"nickname,omitempty"` // 需求 R5
	Accepts  bool   `json:"accepts"`            // 是否接收站内消息
}

// SearchUsers 按用户名前缀或昵称中的任一段联想（不区分大小写，最多 8 个），用于消息的收件人输入框。
// 不包含自己与已停用的账号。
func (s *Service) SearchUsers(ctx context.Context, u *store.User, q string) ([]UserHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []UserHit{}, nil
	}
	us, err := s.Store.Users().Search(ctx, q, 9)
	if err != nil {
		return nil, err
	}
	out := make([]UserHit, 0, len(us))
	for _, x := range us {
		if x.ID == u.ID || x.Disabled || len(out) == 8 {
			continue
		}
		p, err := s.inbox(ctx, x.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, UserHit{Username: x.Username, Nickname: x.Nickname, Accepts: p.Inbox != "off"})
	}
	return out, nil
}
