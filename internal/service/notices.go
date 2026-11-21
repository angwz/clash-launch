package service

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/angwz/clash-launch/internal/events"
	"github.com/angwz/clash-launch/internal/store"
)

// 站内提醒（需求 R23、R24）：运行中发生、需要用户留意的事。每条提醒进入站内列表（实时推送），
// 用户配置了提醒邮件时同时发邮件。dedup 让同一件事只提醒一次：
//   - 节点源连续两次拉取失败：每一段连续失败提醒一次（以开始失败的时间区分）；
//   - 配置在后台重新生成时由通过变为不通过：每个失败的生成结果提醒一次；
//   - 订阅链接第一次被新的国家或地区、新的客户端拉取（链接的第一个值是用户自己导入，不提醒）。
//
// 提醒是附带动作：写入或发送失败只记日志，不影响拉取、生成与订阅本身。

// notice 记录一条提醒；是新的提醒时推送并发邮件。
func (s *Service) notice(ctx context.Context, owner string, kind Alert, dedup string, detail map[string]string) {
	b, err := json.Marshal(detail)
	if err != nil {
		return
	}
	n := &store.Notification{OwnerID: owner, Kind: string(kind), Dedup: dedup, Detail: b, CreatedAt: s.now()}
	created, err := s.Store.Notifications().Add(ctx, n)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("记录提醒失败", "owner", owner, "kind", kind, "error", err)
		}
		return
	}
	if !created {
		return
	}
	s.push(owner, events.Notice, n.ID)
	// 邮件里只写人能读懂的部分；对象 ID 留给站内列表生成链接
	mail := map[string]string{}
	for k, v := range detail {
		if k != "source" && k != "profile" && k != "linkId" {
			mail[k] = v
		}
	}
	s.notify(ctx, owner, kind, mail)
}

// noticeSourceFailing 在节点源第二次连续拉取失败时提醒（第一次失败常是对方临时故障）。
func (s *Service) noticeSourceFailing(ctx context.Context, owner string, src *store.Source, prev, now *store.Snapshot) {
	if now.LastError == "" || prev == nil || prev.LastError == "" || now.FailingSince.IsZero() {
		return
	}
	s.notice(ctx, owner, AlertSourceFailing, "source:"+src.ID+":"+strconv.FormatInt(now.FailingSince.UnixMilli(), 10),
		map[string]string{"source": src.ID, "name": src.Name, "error": now.LastError})
}

// renderInBackground 在输入变化（节点源拉取、规则源更新、授权变化）后重新生成配置；
// 由通过变为不通过时提醒。用户自己在编辑器里保存的不提醒：页面上已经显示了检查结果。
func (s *Service) renderInBackground(ctx context.Context, owner, profileID string) {
	before, err := s.Store.Renders().Get(ctx, owner, profileID)
	wasOK := err == nil && before.LastStatus == "ok"
	r, _, err := s.RenderProfile(ctx, owner, profileID)
	if err != nil {
		s.Log.Warn("render after input change", "profile", profileID, "error", err)
		return
	}
	if !wasOK || r.LastStatus != "failed" {
		return
	}
	name := profileID
	if p, err := s.Store.Profiles().Get(ctx, owner, profileID); err == nil && p.Name != "" {
		name = p.Name
	}
	s.notice(ctx, owner, AlertCheckFailed, "check:"+profileID+":"+r.LastKey, map[string]string{"profile": profileID, "name": name, "error": r.LastError})
}

// noticeSeen 处理订阅请求中第一次出现的国家与客户端（R23）。经由短链接的请求不计：
// 短链接多是分享给好友的，好友在别的地区使用是正常的。
func (s *Service) noticeSeen(ctx context.Context, reqs []store.SubRequest) {
	var vs []store.SeenValue
	seen := map[string]bool{}
	for _, q := range reqs {
		if q.LinkID == "" || q.ShortID != "" || q.Status >= 400 {
			continue
		}
		for _, kv := range [][2]string{{"country", q.Country}, {"client", q.Client}} {
			k := q.LinkID + "\x00" + kv[0] + "\x00" + kv[1]
			if kv[1] == "" || seen[k] {
				continue
			}
			seen[k] = true
			vs = append(vs, store.SeenValue{LinkID: q.LinkID, OwnerID: q.OwnerID, Kind: kv[0], Value: kv[1], At: q.At})
		}
	}
	if len(vs) == 0 {
		return
	}
	fresh, err := s.Store.Stats().MarkSeen(ctx, vs)
	if err != nil {
		s.Log.Warn("记录链接来源失败", "error", err)
		return
	}
	for _, v := range fresh {
		label := v.LinkID
		profileID := ""
		if l, err := s.Store.Links().Get(ctx, v.OwnerID, v.LinkID); err == nil {
			profileID = l.ProfileID
			label = l.Note
			if p, err := s.Store.Profiles().Get(ctx, v.OwnerID, l.ProfileID); err == nil && label == "" {
				label = p.Name
			}
		}
		s.notice(ctx, v.OwnerID, AlertLinkNew, "link:"+v.LinkID+":"+v.Kind+":"+v.Value,
			map[string]string{"link": label, "linkId": v.LinkID, "profile": profileID, v.Kind: v.Value})
	}
}
