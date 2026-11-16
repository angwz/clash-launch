package service

import (
	"context"
	"encoding/json"

	"github.com/angwz/clash-launch/internal/store"
)

// 删除授权出去的对象（需求 R2）：节点源、短链接，以及会连带删除短链接的配置与订阅链接。
//
// 删除前先收回授权，再删除对象，最后通知对方并重新生成对方受影响的配置。顺序的理由：
//   - 先收回：授权记录保留，消息里的授权卡片显示「已收回」，统计不丢；
//   - 对方的配置重新生成时，节点源已不可解析，下发的配置不再包含这些节点（与手动收回授权相同）；
//   - 通知是一条 kind = notice 的消息，由授权者发给被授权者，出现在双方的对话里。
// 通知或重新生成失败只记日志：删除本身已经完成，不能因为附带动作失败而回滚或报错。

// Notice 是系统通知的内容（消息 kind = notice）。前端按 Code 生成本地化文案。
type Notice struct {
	Code   string `json:"code"`   // grant_deleted：授权的对象已被所有者删除
	Target string `json:"target"` // source | short
	Name   string `json:"name"`
}

// DeleteSource 删除节点源，并收回它的全部授权。
func (s *Service) DeleteSource(ctx context.Context, u *store.User, id string) error {
	src, err := s.Store.Sources().Get(ctx, u.ID, id)
	if err != nil {
		return err
	}
	gs, err := s.Store.Grants().ByTarget(ctx, u.ID, "source", id)
	if err != nil {
		return err
	}
	if err := s.revokeAll(ctx, u, gs); err != nil {
		return err
	}
	if err := s.Store.Sources().Delete(ctx, u.ID, id); err != nil {
		return err
	}
	for _, g := range gs {
		s.rerenderUsing(ctx, g.GranteeID, usesSource(id))
		s.notifyDeleted(ctx, u, g.GranteeID, "source", src.Name)
	}
	return nil
}

// DeleteShortLink 删除短链接；为好友生成的短链接同时收回授权并通知对方。
func (s *Service) DeleteShortLink(ctx context.Context, u *store.User, id string) error {
	sl, err := s.Store.ShortLinks().Get(ctx, u.ID, id)
	if err != nil {
		return err
	}
	gs, err := s.shortGrants(ctx, u, []store.ShortLink{*sl})
	if err != nil {
		return err
	}
	if err := s.revokeAll(ctx, u, gs); err != nil {
		return err
	}
	if err := s.Store.ShortLinks().Delete(ctx, u.ID, id); err != nil {
		return err
	}
	s.notifyShorts(ctx, u, gs, []store.ShortLink{*sl})
	return nil
}

// DeleteProfile 删除配置。它的订阅链接与指向这些链接的短链接随之级联删除，
// 其中为好友生成的短链接先收回授权，删除后通知对方。
func (s *Service) DeleteProfile(ctx context.Context, u *store.User, id string) error {
	if _, err := s.Store.Profiles().Get(ctx, u.ID, id); err != nil {
		return err
	}
	links, err := s.Store.Links().List(ctx, u.ID, id)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ID)
	}
	return s.deleteWithShorts(ctx, u, ids, func() error { return s.Store.Profiles().Delete(ctx, u.ID, id) })
}

// DeleteLink 删除一条订阅链接；指向它的、为好友生成的短链接处理同 DeleteProfile。
func (s *Service) DeleteLink(ctx context.Context, u *store.User, linkID string) error {
	return s.deleteWithShorts(ctx, u, []string{linkID}, func() error { return s.Store.Links().Delete(ctx, u.ID, linkID) })
}

// deleteWithShorts 收回指向 linkIDs 的授权短链接，执行删除（级联删除短链接），再通知对方。
func (s *Service) deleteWithShorts(ctx context.Context, u *store.User, linkIDs []string, del func() error) error {
	all, err := s.Store.ShortLinks().List(ctx, u.ID)
	if err != nil {
		return err
	}
	var affected []store.ShortLink
	for _, sl := range all {
		if sl.GranteeID != "" && contains(linkIDs, sl.LinkID) {
			affected = append(affected, sl)
		}
	}
	gs, err := s.shortGrants(ctx, u, affected)
	if err != nil {
		return err
	}
	if err := s.revokeAll(ctx, u, gs); err != nil {
		return err
	}
	if err := del(); err != nil {
		return err
	}
	s.notifyShorts(ctx, u, gs, affected)
	return nil
}

// shortGrants 返回这些短链接上的有效授权。
func (s *Service) shortGrants(ctx context.Context, u *store.User, sls []store.ShortLink) ([]store.Grant, error) {
	var out []store.Grant
	for _, sl := range sls {
		if sl.GranteeID == "" {
			continue
		}
		gs, err := s.Store.Grants().ByTarget(ctx, u.ID, "short", sl.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, gs...)
	}
	return out, nil
}

func (s *Service) revokeAll(ctx context.Context, u *store.User, gs []store.Grant) error {
	now := s.now()
	for _, g := range gs {
		if err := s.Store.Grants().Revoke(ctx, u.ID, g.ID, now); err != nil {
			return err
		}
		s.audit(ctx, u.ID, "grant.revoke", map[string]string{"kind": g.Kind, "reason": "deleted"})
	}
	return nil
}

// notifyShorts 按授权找到对应的短链接名称（备注优先，否则短码）并通知对方。
func (s *Service) notifyShorts(ctx context.Context, u *store.User, gs []store.Grant, sls []store.ShortLink) {
	for _, g := range gs {
		name := ""
		for _, sl := range sls {
			if sl.ID == g.TargetID {
				name = sl.Code
			}
		}
		s.notifyDeleted(ctx, u, g.GranteeID, "short", name)
	}
}

// notifyDeleted 给被授权者发一条系统通知。不受发送频率限制：这是系统动作，不是用户发言。
func (s *Service) notifyDeleted(ctx context.Context, owner *store.User, grantee, target, name string) {
	raw, err := json.Marshal(MessageBody{Notice: &Notice{Code: "grant_deleted", Target: target, Name: name}})
	if err == nil {
		err = s.Store.Messages().Create(ctx, &store.Message{SenderID: owner.ID, RecipientID: grantee, Kind: "notice", Body: raw, CreatedAt: s.now()})
	}
	if err != nil && s.Log != nil {
		s.Log.Warn("deletion notice not sent", "owner", owner.ID, "grantee", grantee, "err", err.Error())
	}
}
