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
var ErrShortCode = errors.New("service: 短码只能包含小写字母、数字与连字符，3–32 个字符，以字母或数字开头")

var shortCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,31}$`)

// 随机短码的字符表：去掉容易看错的 0 o 1 l i；8 位约 40 比特，配合 /u/ 的试探限速，不可能被逐个猜出。
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
		if !shortCodeRe.MatchString(sl.Code) {
			return ErrShortCode
		}
		return s.Store.ShortLinks().Create(ctx, u.ID, sl)
	}
	// 随机短码：极少数情况下撞上已有短码，换一个再试
	for range 5 {
		sl.Code = randomCode(8)
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
	Kind     string `json:"kind"` // nodes | subscription
	Name     string `json:"name"`
	Content  string `json:"content,omitempty"`
	URL      string `json:"url,omitempty"`
	UA       string `json:"ua,omitempty"`
	Interval int    `json:"interval,omitempty"`
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

// SendMessage 发送一条消息；sourceID 非空时附上发件人的这个节点源。收件人开启自动加入时，
// 附件立即加入收件人的节点页（配额不足等原因加入失败时，消息照常送达，收件人可稍后手动加入）。
func (s *Service) SendMessage(ctx context.Context, from *store.User, to, text, sourceID string) (*store.Message, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > messageMaxText || (text == "" && sourceID == "") {
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
	if sourceID != "" {
		src, err := s.Store.Sources().Get(ctx, from.ID, sourceID)
		if err != nil {
			return nil, err
		}
		a := &Attachment{Name: src.Name}
		if src.Kind == "subscription" {
			a.Kind, a.URL, a.UA, a.Interval = "subscription", src.URL, src.UA, src.Interval
		} else {
			a.Kind, a.Content = "nodes", string(src.Content)
		}
		body.Attach, kind = a, a.Kind
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	m := &store.Message{SenderID: from.ID, RecipientID: rcpt.ID, Kind: kind, Body: raw, CreatedAt: s.now()}
	if err := s.Store.Messages().Create(ctx, m); err != nil {
		return nil, err
	}
	if body.Attach != nil && (pref.AutoAdd == nil || *pref.AutoAdd) {
		if src, err := s.AcceptAttachment(ctx, rcpt, m.ID); err == nil {
			m.AddedSource = src.ID
		} else if s.Log != nil {
			s.Log.Info("message attachment not added automatically", "message", m.ID, "reason", err.Error())
		}
	}
	return m, nil
}

// AcceptAttachment 把一条消息的附件加入收件人的节点页（只能加入一次）。名称后注明来自谁。
func (s *Service) AcceptAttachment(ctx context.Context, u *store.User, id string) (*store.Source, error) {
	m, err := s.Store.Messages().Get(ctx, u.ID, id)
	if err != nil {
		return nil, err
	}
	if m.RecipientID != u.ID {
		return nil, store.ErrNotFound
	}
	if m.AddedSource != "" {
		if src, err := s.Store.Sources().Get(ctx, u.ID, m.AddedSource); err == nil {
			return src, nil
		}
	}
	var body MessageBody
	if err := json.Unmarshal(m.Body, &body); err != nil {
		return nil, fmt.Errorf("消息 %s 的内容无效：%w", id, err)
	}
	if body.Attach == nil {
		return nil, ErrMessage
	}
	sender := "?"
	if su, err := s.Store.Users().ByID(ctx, m.SenderID); err == nil {
		sender = su.Username
	}
	a := body.Attach
	src := &store.Source{Name: a.Name + "（来自 " + sender + "）"}
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
	return src, nil
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

// Threads 返回用户的会话列表（最近的在前）。
func (s *Service) Threads(ctx context.Context, u *store.User) ([]Thread, error) {
	ms, err := s.Store.Messages().Recent(ctx, u.ID, 1000)
	if err != nil {
		return nil, err
	}
	byUser := map[string]*Thread{}
	var order []string
	for _, m := range ms {
		other, mine := m.SenderID, false
		if m.SenderID == u.ID {
			other, mine = m.RecipientID, true
		}
		t := byUser[other]
		if t == nil {
			var body MessageBody
			_ = json.Unmarshal(m.Body, &body)
			t = &Thread{UserID: other, Last: preview(body), LastKind: m.Kind, LastAt: m.CreatedAt, Mine: mine}
			byUser[other] = t
			order = append(order, other)
		}
		if !mine && m.ReadAt.IsZero() {
			t.Unread++
		}
	}
	out := make([]Thread, 0, len(order))
	for _, id := range order {
		t := byUser[id]
		if ou, err := s.Store.Users().ByID(ctx, id); err == nil {
			t.Username = ou.Username
		} else {
			continue // 对方账号已删除：会话随之消失（消息已级联删除）
		}
		out = append(out, *t)
	}
	return out, nil
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
