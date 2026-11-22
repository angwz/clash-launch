package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/events"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 订阅分析、短链接与站内消息（反馈 #74）。

// linkLabels 返回用户全部配置链接的说明（「配置名 · 链接备注」），按链接 ID 索引。
func (s *Server) linkLabels(ctx context.Context, owner string) (map[string]linkInfo, error) {
	ps, err := s.Svc.Store.Profiles().List(ctx, owner, store.Page{Limit: 200})
	if err != nil {
		return nil, err
	}
	out := map[string]linkInfo{}
	for _, p := range ps {
		ls, err := s.Svc.Store.Links().List(ctx, owner, p.ID)
		if err != nil {
			return nil, err
		}
		for _, l := range ls {
			out[l.ID] = linkInfo{ProfileID: p.ID, ProfileName: p.Name, Note: l.Note}
		}
	}
	return out, nil
}

type linkInfo struct {
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	Note        string `json:"note"`
}

// —— 短链接 ——

type shortDTO struct {
	ID        string     `json:"id"`
	Code      string     `json:"code"`
	URL       string     `json:"url"`
	LinkID    string     `json:"linkId"`
	Target    linkInfo   `json:"target"`
	Note      string     `json:"note"`
	Disabled  bool       `json:"disabled"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	Total     int64      `json:"total"`
	Last      *time.Time `json:"last,omitempty"` // 最近一次被请求的时间
	Series    []int64    `json:"series"`         // 近 30 天每天的请求数（UTC）
	Version   int64      `json:"version"`
	// 为好友生成的短链接（反馈 #75）：Grantee 是对方的用户名，Granted 表示授权仍有效
	Grantee     string `json:"grantee,omitempty"`
	GranteeNick string `json:"granteeNickname,omitempty"` // O12
	GrantID     string `json:"grantId,omitempty"`
	Granted     bool   `json:"granted,omitempty"`
}

// receivedShortDTO 是别人授权给我使用的短链接。
type receivedShortDTO struct {
	GrantID   string    `json:"grantId"`
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	From      string    `json:"from"`
	FromNick  string    `json:"fromNickname,omitempty"` // O12：授权者的昵称
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) listShortLinks(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	items, err := s.Svc.Store.ShortLinks().List(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	labels, err := s.linkLabels(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.Svc.FlushStats(ctx)
	ids := make([]string, 0, len(items))
	for _, x := range items {
		ids = append(ids, x.ID)
	}
	from, to, days := dayRange(time.Now(), 30)
	totals, byDay := map[string]int64{}, map[string][]store.DayCount{}
	if len(ids) > 0 {
		st := s.Svc.Store.Stats()
		if totals, err = st.Totals(ctx, "short", ids); err != nil {
			s.fail(w, err)
			return
		}
		rows, err := st.Daily(ctx, "short", ids, from, to)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, x := range rows {
			byDay[x.Key] = append(byDay[x.Key], x)
		}
	}
	lastAt, err := s.Svc.Store.Stats().LastByShort(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	given, err := s.Svc.Store.Grants().Given(ctx, p.User.ID, false)
	if err != nil {
		s.fail(w, err)
		return
	}
	grantOf := map[string]store.Grant{}
	for _, g := range given {
		if g.Kind == "short" {
			grantOf[g.TargetID] = g
		}
	}
	out := []shortDTO{}
	for _, x := range items {
		d := s.toShort(r, &x, labels[x.LinkID])
		d.Total, d.Series = totals[x.ID], series(byDay[x.ID], days)
		if t, ok := lastAt[x.ID]; ok {
			d.Last = &t
		}
		if x.GranteeID != "" {
			if u, err := s.Svc.Store.Users().ByID(ctx, x.GranteeID); err == nil {
				d.Grantee, d.GranteeNick = u.Username, u.Nickname
			}
			if g, ok := grantOf[x.ID]; ok {
				d.GrantID, d.Granted = g.ID, g.Active()
			}
		}
		out = append(out, d)
	}
	// 别人授权给我的短链接：只能复制与导入，不显示对方配置的细节
	received := []receivedShortDTO{}
	gs, err := s.Svc.Store.Grants().Received(ctx, p.User.ID, true)
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, g := range gs {
		if g.Kind != "short" {
			continue
		}
		sl, err := s.Svc.Store.ShortLinks().Get(ctx, g.OwnerID, g.TargetID)
		if err != nil {
			continue
		}
		from, fromNick := "", ""
		if u, err := s.Svc.Store.Users().ByID(ctx, g.OwnerID); err == nil {
			from, fromNick = u.Username, u.Nickname
		}
		// 显示对方配置的名称（只有名称，不含内容），便于区分多个授权
		name := ""
		if l, err := s.Svc.Store.Links().Get(ctx, g.OwnerID, sl.LinkID); err == nil {
			if pr, err := s.Svc.Store.Profiles().Get(ctx, g.OwnerID, l.ProfileID); err == nil {
				name = pr.Name
			}
		}
		received = append(received, receivedShortDTO{GrantID: g.ID, Code: sl.Code, URL: s.baseURL(r) + "/" + sl.Code, From: from, FromNick: fromNick, Name: name, CreatedAt: g.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "received": received})
}

func (s *Server) toShort(r *http.Request, x *store.ShortLink, target linkInfo) shortDTO {
	d := shortDTO{ID: x.ID, Code: x.Code, URL: s.baseURL(r) + "/" + x.Code, LinkID: x.LinkID, Target: target, Note: x.Note, Disabled: x.Disabled, CreatedAt: x.CreatedAt, Version: x.Version, Series: []int64{}}
	if !x.ExpiresAt.IsZero() {
		d.ExpiresAt = &x.ExpiresAt
	}
	return d
}

type shortReq struct {
	LinkID    string     `json:"linkId"`
	Code      string     `json:"code"`
	Note      *string    `json:"note"`
	Disabled  *bool      `json:"disabled"`
	ExpiresAt *time.Time `json:"expiresAt"`
	// ClearExpiry：改为不过期（PATCH 时 expiresAt 为 null 无法与「不修改」区分）
	ClearExpiry bool  `json:"clearExpiry"`
	Version     int64 `json:"version"`
}

func (s *Server) createShortLink(w http.ResponseWriter, r *http.Request, p *principal) {
	var req shortReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	sl := &store.ShortLink{LinkID: req.LinkID, Code: req.Code}
	if req.Note != nil {
		sl.Note = strings.TrimSpace(*req.Note)
	}
	if req.ExpiresAt != nil {
		sl.ExpiresAt = req.ExpiresAt.UTC()
	}
	if err := s.Svc.CreateShortLink(r.Context(), p.User, sl); err != nil {
		s.fail(w, err)
		return
	}
	labels, _ := s.linkLabels(r.Context(), p.User.ID)
	writeJSON(w, http.StatusCreated, s.toShort(r, sl, labels[sl.LinkID]))
}

func (s *Server) patchShortLink(w http.ResponseWriter, r *http.Request, p *principal) {
	var req shortReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	sl, err := s.Svc.Store.ShortLinks().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if req.Version != 0 {
		sl.Version = req.Version
	}
	if req.Note != nil {
		sl.Note = strings.TrimSpace(*req.Note)
	}
	if req.Disabled != nil {
		if !*req.Disabled && sl.GranteeID != "" {
			// 为好友生成的短链接：授权收回后不能在这里重新启用（重新授权请再发一次）
			if g, err := s.Svc.Store.Grants().ActiveFor(r.Context(), sl.GranteeID, "short", sl.ID); err != nil || !g.Active() {
				s.fail(w, store.ErrConflict)
				return
			}
		}
		sl.Disabled = *req.Disabled
	}
	if req.ExpiresAt != nil {
		sl.ExpiresAt = req.ExpiresAt.UTC()
	}
	if req.ClearExpiry {
		sl.ExpiresAt = time.Time{}
	}
	if err := s.Svc.Store.ShortLinks().Update(r.Context(), p.User.ID, sl); err != nil {
		s.fail(w, err)
		return
	}
	labels, _ := s.linkLabels(r.Context(), p.User.ID)
	writeJSON(w, http.StatusOK, s.toShort(r, sl, labels[sl.LinkID]))
}

func (s *Server) deleteShortLink(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteShortLink(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 订阅分析 ——

// analytics 汇总当前用户的订阅请求。参数：days（1–365，默认 30）、link、short（只看某条链接或短链接）、
// tz（用户没有设置时区时，前端传入浏览器时区）。
func (s *Server) analytics(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	sc, err := s.analyticsScope(r, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	from, to, loc, f, codes, friends, granteeOf := sc.from, sc.to, sc.loc, sc.filter, sc.codes, sc.friends, sc.granteeOf
	// 流式汇总，没有条数上限（需求 R14c）；最近的请求另外按完整列取一页
	z := service.NewAnalyzer(from, to, loc, granteeOf)
	if err := s.Svc.Store.Stats().EachRequest(ctx, p.User.ID, from, to, f, func(q *store.SubRequest) error {
		z.Add(q)
		return nil
	}); err != nil {
		s.fail(w, err)
		return
	}
	recent, err := s.Svc.Store.Stats().LatestRequests(ctx, p.User.ID, from, to, f, service.RecentRequests)
	if err != nil {
		s.fail(w, err)
		return
	}
	a := z.Result(recent)
	s.Svc.Locate(a)
	labels, err := s.linkLabels(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	// 明细保留期（R14c）：所选范围超过它时，前端提示前段不完整
	writeJSON(w, http.StatusOK, map[string]any{"analytics": a, "links": labels, "shorts": codes, "friends": friends, "friendNicknames": sc.friendNicks, "retentionDays": s.Svc.Retention(ctx).RequestDays})
}

// —— 站内消息 ——

type attachDTO struct {
	Kind  string `json:"kind"` // nodes | subscription | grant
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Nodes int    `json:"nodes,omitempty"` // 手动节点的行数（粗略的节点数）
	// 授权（反馈 #75）：Target 为 source 或 short；Active 表示授权仍有效
	Note       string `json:"note,omitempty"` // 配置模板的说明
	Target     string `json:"target,omitempty"`
	GrantID    string `json:"grantId,omitempty"`
	SourceKind string `json:"sourceKind,omitempty"`
	Active     bool   `json:"active,omitempty"`
}

type messageDTO struct {
	ID     string     `json:"id"`
	Mine   bool       `json:"mine"`
	Text   string     `json:"text"`
	Attach *attachDTO `json:"attach,omitempty"`
	// Notice 是系统通知（如授权的对象被删除），前端按 code 生成文案
	Notice      *service.Notice `json:"notice,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	ReadAt      *time.Time      `json:"readAt,omitempty"`
	AddedSource string          `json:"addedSource,omitempty"`
}

// toMessage 转换消息；授权附件查询授权当前是否有效（收发双方都能读取这项授权）。
func (s *Server) toMessage(ctx context.Context, me string, m *store.Message) messageDTO {
	var b service.MessageBody
	_ = json.Unmarshal(m.Body, &b)
	d := messageDTO{ID: m.ID, Mine: m.SenderID == me, Text: b.Text, Notice: b.Notice, CreatedAt: m.CreatedAt, ReadAt: tp(m.ReadAt)}
	if !d.Mine {
		d.AddedSource = m.AddedSource
	}
	if a := b.Attach; a != nil {
		d.Attach = &attachDTO{Kind: a.Kind, Name: a.Name, URL: a.URL, Note: a.Note, Target: a.Target, GrantID: a.GrantID, SourceKind: a.SourceKind}
		if a.Kind == "grant" {
			if g, err := s.Svc.Store.Grants().Get(ctx, me, a.GrantID); err == nil {
				d.Attach.Active = g.Active()
			}
		}
		if a.Kind == "nodes" {
			for _, line := range strings.Split(a.Content, "\n") {
				if strings.TrimSpace(line) != "" {
					d.Attach.Nodes++
				}
			}
		}
	}
	return d
}

func (s *Server) listThreads(w http.ResponseWriter, r *http.Request, p *principal) {
	ts, err := s.Svc.Threads(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNil(ts)})
}

func (s *Server) unreadMessages(w http.ResponseWriter, r *http.Request, p *principal) {
	n, err := s.Svc.Store.Messages().Unread(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

// threadWith 返回与某个用户的消息（新的在前，分页），并把对方发来的消息标为已读。
func (s *Server) threadWith(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	other, err := s.Svc.Store.Users().ByUsername(ctx, r.PathValue("username"))
	if err != nil {
		s.fail(w, store.ErrNotFound)
		return
	}
	// 对话在回收站中：不返回消息，界面显示「已删除」与恢复按钮（反馈 #75）
	if c, err := s.Svc.Store.Contacts().Get(ctx, p.User.ID, other.ID); err == nil && !c.DeletedAt.IsZero() {
		writeJSON(w, http.StatusOK, map[string]any{"user": map[string]string{"id": other.ID, "username": other.Username, "nickname": other.Nickname, "avatar": other.Avatar}, "items": []messageDTO{}, "deletedAt": c.DeletedAt, "purgeAt": c.DeletedAt.Add(365 * 24 * time.Hour)})
		return
	}
	items, err := s.Svc.Store.Messages().Thread(ctx, p.User.ID, other.ID, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	// 已读回执（R22）：只在确有未读时通知对方，避免两边打开的会话互相触发重读
	unread, _ := s.Svc.Store.Messages().UnreadFrom(ctx, p.User.ID, other.ID)
	if err := s.Svc.Store.Messages().MarkRead(ctx, p.User.ID, other.ID, time.Now()); err != nil {
		s.fail(w, err)
		return
	}
	if unread > 0 {
		s.Svc.Events.Publish(other.ID, events.Event{Type: events.Message, ID: p.User.ID})
		s.Svc.Events.Publish(p.User.ID, events.Event{Type: events.Message, ID: other.ID}) // 自己其他标签页的未读数
	}
	out := list[messageDTO]{Items: []messageDTO{}}
	for i := range items {
		out.Items = append(out.Items, s.toMessage(ctx, p.User.ID, &items[i]))
	}
	if len(items) > 0 && len(items) == pageSize(r) {
		last := items[len(items)-1]
		out.Next = strconv.FormatInt(last.CreatedAt.UnixMilli(), 10) + "|" + last.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": map[string]any{"id": other.ID, "username": other.Username, "nickname": other.Nickname, "avatar": other.Avatar, "online": s.Svc.PeerOnline(ctx, p.User.ID, other.ID)}, "items": out.Items, "next": out.Next})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		To       string `json:"to"`
		Text     string `json:"text"`
		Source   string `json:"source"`
		Short    string `json:"short"`
		Template string `json:"template"`
		Mode     string `json:"mode"` // share | grant
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	m, err := s.Svc.SendMessage(r.Context(), p.User, req.To, req.Text, service.AttachRequest{Source: req.Source, Short: req.Short, Template: req.Template, Mode: req.Mode})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.toMessage(r.Context(), p.User.ID, m))
}

func (s *Server) acceptMessage(w http.ResponseWriter, r *http.Request, p *principal) {
	src, err := s.Svc.AcceptAttachment(r.Context(), p.User, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"kind": src.Kind, "source": src.ID, "name": src.Name})
}

func (s *Server) hideMessage(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Messages().Hide(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
