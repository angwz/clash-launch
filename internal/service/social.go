package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/store"
)

// 短链接与站内消息（反馈 #74）。

// —— 短链接 ——

// ErrShortCode 表示自定义短码不合规。
var ErrShortCode = errors.New("service: 短码只能包含小写字母、数字与连字符，3–32 个字符，以字母或数字开头，且不能是保留的路径")

var shortCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,31}$`)

// 随机短码的字符表：去掉容易看错的 0 o 1 l i。从 5 位起（约 2,800 万种），撞上已有短码时逐步加长；
// 配合「同一 IP 一小时猜错 30 次即限速」，逐个猜测不可行（反馈 #75）。
const shortAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func randomCode(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("service: 随机数不可用：" + err.Error())
	}
	for i := range b {
		b[i] = shortAlphabet[int(b[i])%len(shortAlphabet)]
	}
	return string(b)
}

// CreateShortLink 新建短链接。code 为空时随机生成；数量与配置链接共用同一个上限。
func (s *Service) CreateShortLink(ctx context.Context, u *store.User, sl *store.ShortLink) error {
	l, err := s.Limits(ctx, u)
	if err != nil {
		return err
	}
	if l.Links >= 0 {
		n, err := s.Store.ShortLinks().Count(ctx, u.ID)
		if err != nil {
			return err
		}
		if n >= l.Links {
			return fmt.Errorf("%w：short links %d/%d", ErrQuota, n, l.Links)
		}
	}
	if utf8.RuneCountInString(sl.Note) > 100 {
		return fmt.Errorf("%w：备注最多 100 个字符", ErrShortCode)
	}
	if sl.Code != "" {
		sl.Code = strings.ToLower(strings.TrimSpace(sl.Code))
		if !shortCodeRe.MatchString(sl.Code) || IsReservedCode(sl.Code) {
			return ErrShortCode
		}
		return s.Store.ShortLinks().Create(ctx, u.ID, sl)
	}
	// 随机短码：撞上已有短码或保留路径时换一个，每两次失败加长一位
	for i := range 12 {
		sl.Code = randomCode(5 + i/2)
		if IsReservedCode(sl.Code) {
			continue
		}
		err := s.Store.ShortLinks().Create(ctx, u.ID, sl)
		if !errors.Is(err, store.ErrExists) {
			return err
		}
		sl.ID = ""
	}
	return fmt.Errorf("service: 生成短码失败")
}

// —— 站内消息 ——

// ErrRecipient 表示收件人不存在、不接收消息或屏蔽了发件人。三种情况不区分，避免借此探测账号。
var ErrRecipient = errors.New("service: 收件人不存在或不接收消息")

// ErrMessage 表示消息内容不合规。
var ErrMessage = errors.New("service: 消息不能为空，文字最多 2000 个字符")

const (
	messageMaxText = 2000
	// 每个账号每小时最多发送的消息数（管理员不限）
	messagesPerHour = 60
)

// MessageBody 是加密保存的消息内容。
type MessageBody struct {
	Text   string      `json:"text,omitempty"`
	Attach *Attachment `json:"attach,omitempty"`
}

// Attachment 是随消息发送的节点：手动节点的原文，或订阅地址。不包含发件人设置的请求头（可能含凭据）。
type Attachment struct {
	Kind     string `json:"kind"` // nodes | subscription | grant
	Name     string `json:"name"`
	Content  string `json:"content,omitempty"`
	URL      string `json:"url,omitempty"`
	UA       string `json:"ua,omitempty"`
	Interval int    `json:"interval,omitempty"`
	// 授权（反馈 #75）：Target 为 source 或 short；GrantID 指向 grants 表；SourceKind 是被授权节点的类型
	// 配置模板（反馈 #76）：说明与模板文档
	Note       string `json:"note,omitempty"`
	Doc        string `json:"doc,omitempty"`
	Target     string `json:"target,omitempty"`
	GrantID    string `json:"grantId,omitempty"`
	SourceKind string `json:"sourceKind,omitempty"`
}

// inboxPrefs 是收件人在偏好中的消息设置（与 api/prefs.go 的字段一致）。
type inboxPrefs struct {
	Inbox   string   `json:"inbox"`   // "" 接收 | off 不接收
	AutoAdd *bool    `json:"autoAdd"` // 收到的节点自动加入节点页；默认开
	Blocked []string `json:"blocked"` // 屏蔽的用户 ID
}

func (s *Service) inbox(ctx context.Context, userID string) (inboxPrefs, error) {
	var p inboxPrefs
	b, err := s.Store.Users().Prefs(ctx, userID)
	if err != nil {
		return p, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &p); err != nil {
			return p, fmt.Errorf("读取 %s 的偏好：%w", userID, err)
		}
	}
	return p, nil
}

// AttachRequest 是发送消息时附带的内容（反馈 #75）：
//   - Source + Mode=share：复制一份节点给对方，对方可以随意修改；
//   - Source + Mode=grant：授权对方只能使用这个节点（软链接，随你更新，可随时收回）；
//   - Short（只能授权）：为对方单独生成一个新的短链接，指向同一份配置，使用情况在订阅分析中按好友统计。
type AttachRequest struct {
	Source   string
	Short    string
	Template string // 配置模板（只能分享，反馈 #76）
	Mode     string // share | grant
}

// SendMessage 发送一条消息，可以附带节点（分享或授权）或短链接（授权）。收件人开启自动加入时，
// 分享的附件立即加入收件人的节点页（配额不足等原因加入失败时，消息照常送达，收件人可稍后手动加入）；
// 授权不占对方配额，直接出现在对方的节点页或短链接页。
func (s *Service) SendMessage(ctx context.Context, from *store.User, to, text string, at AttachRequest) (*store.Message, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > messageMaxText || (text == "" && at.Source == "" && at.Short == "" && at.Template == "") {
		return nil, ErrMessage
	}
	if at.Mode == "" {
		at.Mode = "share"
	}
	if (at.Mode != "share" && at.Mode != "grant") || (at.Short != "" && (at.Mode != "grant" || at.Source != "")) || (at.Template != "" && (at.Mode != "share" || at.Source != "" || at.Short != "")) {
		return nil, ErrMessage
	}
	rcpt, err := s.Store.Users().ByUsername(ctx, strings.TrimSpace(to))
	if errors.Is(err, store.ErrNotFound) || (err == nil && (rcpt.ID == from.ID || rcpt.Disabled)) {
		return nil, ErrRecipient
	}
	if err != nil {
		return nil, err
	}
	pref, err := s.inbox(ctx, rcpt.ID)
	if err != nil {
		return nil, err
	}
	if pref.Inbox == "off" || contains(pref.Blocked, from.ID) {
		return nil, ErrRecipient
	}
	if from.Role != "admin" {
		n, err := s.Store.Messages().SentSince(ctx, from.ID, s.now().Add(-time.Hour))
		if err != nil {
			return nil, err
		}
		if n >= messagesPerHour {
			return nil, &WaitError{Wait: time.Hour}
		}
	}
	body := MessageBody{Text: text}
	kind := "text"
	var grant *store.Grant
	switch {
	case at.Source != "":
		src, err := s.Store.Sources().Get(ctx, from.ID, at.Source)
		if err != nil {
			return nil, err
		}
		a := &Attachment{Name: src.Name, SourceKind: src.Kind}
		if at.Mode == "grant" {
			grant = &store.Grant{OwnerID: from.ID, GranteeID: rcpt.ID, Kind: "source", TargetID: src.ID}
			a.Kind, a.Target = "grant", "source"
		} else if src.Kind == "subscription" {
			a.Kind, a.URL, a.UA, a.Interval = "subscription", src.URL, src.UA, src.Interval
		} else {
			a.Kind, a.Content = "nodes", string(src.Content)
		}
		body.Attach, kind = a, a.Kind
	case at.Template != "":
		tpl, err := s.Store.Templates().Get(ctx, from.ID, at.Template)
		if err != nil {
			return nil, err
		}
		body.Attach, kind = &Attachment{Kind: "template", Name: tpl.Name, Note: tpl.Note, Doc: string(tpl.Doc)}, "template"
	case at.Short != "":
		orig, err := s.Store.ShortLinks().Get(ctx, from.ID, at.Short)
		if err != nil || orig.GranteeID != "" {
			return nil, store.ErrNotFound // 只能从自己用的短链接授权，授权出去的短链接不能再转授
		}
		derived := &store.ShortLink{LinkID: orig.LinkID, GranteeID: rcpt.ID, Note: rcpt.Username}
		if err := s.CreateShortLink(ctx, from, derived); err != nil {
			return nil, err
		}
		grant = &store.Grant{OwnerID: from.ID, GranteeID: rcpt.ID, Kind: "short", TargetID: derived.ID}
		name := orig.Note
		if name == "" {
			name = orig.Code
		}
		body.Attach, kind = &Attachment{Kind: "grant", Target: "short", Name: name}, "grant"
	}
	if grant != nil {
		grant.CreatedAt = s.now()
		if err := s.Store.Grants().Create(ctx, grant); err != nil {
			return nil, err
		}
		body.Attach.GrantID = grant.ID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	m := &store.Message{SenderID: from.ID, RecipientID: rcpt.ID, Kind: kind, Body: raw, CreatedAt: s.now()}
	if err := s.Store.Messages().Create(ctx, m); err != nil {
		if grant != nil {
			_ = s.RevokeGrant(ctx, from, grant.ID)
		}
		return nil, err
	}
	if grant != nil {
		s.audit(ctx, from.ID, "grant.create", map[string]string{"to": rcpt.Username, "kind": grant.Kind, "name": body.Attach.Name})
		if grant.Kind == "source" {
			s.rerenderUsing(ctx, rcpt.ID, usesSource(grant.TargetID))
		}
	} else if body.Attach != nil && (pref.AutoAdd == nil || *pref.AutoAdd) {
		if added, err := s.AcceptAttachment(ctx, rcpt, m.ID); err == nil {
			m.AddedSource = added.ID
		} else if s.Log != nil {
			s.Log.Info("message attachment not added automatically", "message", m.ID, "reason", err.Error())
		}
	}
	return m, nil
}

// Added 是收件人由附件得到的东西：节点源（source）或配置模板（template）。
type Added struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AcceptAttachment 把一条消息的附件加入收件人的节点页（节点）或模板（配置模板，反馈 #76），只加入一次。
// 名称后注明来自谁。授权不需要接受（它本来就出现在对方那里）。
func (s *Service) AcceptAttachment(ctx context.Context, u *store.User, id string) (*Added, error) {
	m, err := s.Store.Messages().Get(ctx, u.ID, id)
	if err != nil {
		return nil, err
	}
	if m.RecipientID != u.ID {
		return nil, store.ErrNotFound
	}
	var body MessageBody
	if err := json.Unmarshal(m.Body, &body); err != nil {
		return nil, fmt.Errorf("消息 %s 的内容无效：%w", id, err)
	}
	if body.Attach == nil || body.Attach.Kind == "grant" {
		return nil, ErrMessage
	}
	a := body.Attach
	if m.AddedSource != "" {
		if a.Kind == "template" {
			if t, err := s.Store.Templates().Get(ctx, u.ID, m.AddedSource); err == nil {
				return &Added{Kind: "template", ID: t.ID, Name: t.Name}, nil
			}
		} else if src, err := s.Store.Sources().Get(ctx, u.ID, m.AddedSource); err == nil {
			return &Added{Kind: "source", ID: src.ID, Name: src.Name}, nil
		}
	}
	sender := "?"
	if su, err := s.Store.Users().ByID(ctx, m.SenderID); err == nil {
		sender = su.Username
	}
	name := a.Name + "（来自 " + sender + "）"
	if a.Kind == "template" {
		// 模板中引用的节点源属于发件人，对收件人不存在：用模板新建配置时会跳过并提示（已有行为）
		if err := s.CheckStorage(ctx, u, int64(len(a.Doc))); err != nil {
			return nil, err
		}
		t := &store.Template{Name: name, Note: a.Note, Doc: []byte(a.Doc)}
		if err := s.Store.Templates().Create(ctx, u.ID, t); err != nil {
			return nil, err
		}
		if err := s.Store.Messages().SetAdded(ctx, u.ID, id, t.ID); err != nil {
			return nil, err
		}
		s.audit(ctx, u.ID, "message.template_added", map[string]string{"from": sender, "template": t.Name})
		return &Added{Kind: "template", ID: t.ID, Name: t.Name}, nil
	}
	src := &store.Source{Name: name}
	if a.Kind == "subscription" {
		src.Kind, src.URL, src.UA = "subscription", a.URL, a.UA
		// 拉取间隔不低于收件人的配额
		if err := s.CheckInterval(ctx, u, a.Interval); err == nil {
			src.Interval = a.Interval
		}
	} else {
		src.Kind, src.Content = "batch", []byte(a.Content)
	}
	if err := s.CheckCreate(ctx, u, QuotaSource, int64(len(src.Content))); err != nil {
		return nil, err
	}
	if err := s.Store.Sources().Create(ctx, u.ID, src); err != nil {
		return nil, err
	}
	if err := s.Store.Messages().SetAdded(ctx, u.ID, id, src.ID); err != nil {
		return nil, err
	}
	s.audit(ctx, u.ID, "message.source_added", map[string]string{"from": sender, "source": src.Name})
	return &Added{Kind: "source", ID: src.ID, Name: src.Name}, nil
}

// Thread 是一个会话的摘要。
type Thread struct {
	UserID   string    `json:"userId"`
	Username string    `json:"username"`
	Last     string    `json:"last"`     // 最后一条的文字预览
	LastKind string    `json:"lastKind"` // text | nodes | subscription
	LastAt   time.Time `json:"lastAt"`
	Mine     bool      `json:"mine"` // 最后一条是自己发的
	Unread   int       `json:"unread"`
}

func preview(b MessageBody) string {
	t := b.Text
	if t == "" && b.Attach != nil {
		t = b.Attach.Name
	}
	if r := []rune(t); len(r) > 60 {
		t = string(r[:60]) + "…"
	}
	return t
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
