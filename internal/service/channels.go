package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/source"
	"github.com/angwz/clash-launch/internal/store"
)

// 提醒渠道（需求 R31，决定 0008）：除邮件外，把提醒发到用户本人配置的 Webhook、Telegram、Bark、ntfy。
// 与邮件相同：测试消息送达后才启用，修改设置回到未验证；设置整体加密存储。
// 接口不返回令牌、设备 key、签名密钥与 Webhook 地址（地址常含密钥），只返回「已设置」与主机名；
// 编辑时这些字段留空表示保留原值。

// MaxChannels 是每个用户的渠道上限。
const MaxChannels = 5

var (
	// ErrChannelsUnavailable 表示服务没有配置推送（Pusher 为 nil）。
	ErrChannelsUnavailable = errors.New("service: 提醒渠道不可用")
	// ErrChannelLimit 表示渠道数量已达上限。
	ErrChannelLimit = fmt.Errorf("service: 每个账号最多 %d 个提醒渠道", MaxChannels)
	// ErrChannelName 表示名称无效。
	ErrChannelName = errors.New("service: 渠道名称最多 40 个字符")
)

// ChannelInput 是用户提交的渠道设置。
type ChannelInput struct {
	Name string `json:"name"`
	source.Push
}

// ChannelView 是返回给用户的渠道（不含密钥）。
type ChannelView struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	Host       string     `json:"host,omitempty"` // webhook 只返回主机名
	URL        string     `json:"url,omitempty"`  // bark、ntfy 的服务器地址
	Chat       string     `json:"chat,omitempty"`
	Topic      string     `json:"topic,omitempty"`
	HasToken   bool       `json:"hasToken"`
	HasKey     bool       `json:"hasKey"`
	HasSecret  bool       `json:"hasSecret"`
	Enabled    bool       `json:"enabled"`
	VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
	LastAt     *time.Time `json:"lastAt,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func channelPush(c *store.Channel) (source.Push, error) {
	var p source.Push
	if err := json.Unmarshal(c.Config, &p); err != nil {
		return p, fmt.Errorf("渠道 %s 的设置无法读取：%w", c.ID, err)
	}
	p.Kind = c.Kind
	return p, nil
}

func channelView(c *store.Channel) ChannelView {
	v := ChannelView{ID: c.ID, Kind: c.Kind, Name: c.Name, Enabled: c.Enabled, VerifiedAt: timePtr(c.VerifiedAt), LastAt: timePtr(c.LastAt), LastError: c.LastError, CreatedAt: c.CreatedAt}
	p, err := channelPush(c)
	if err != nil {
		v.LastError = err.Error()
		return v
	}
	v.Chat, v.Topic, v.HasToken, v.HasKey, v.HasSecret = p.Chat, p.Topic, p.Token != "", p.Key != "", p.Secret != ""
	if c.Kind == source.PushWebhook {
		if u, err := url.Parse(p.URL); err == nil {
			v.Host = u.Host
		}
	} else {
		v.URL = p.URL
	}
	return v
}

// ListChannels 返回本人的渠道。
func (s *Service) ListChannels(ctx context.Context, userID string) ([]ChannelView, error) {
	cs, err := s.Store.Channels().List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelView, 0, len(cs))
	for i := range cs {
		out = append(out, channelView(&cs[i]))
	}
	return out, nil
}

func channelName(name, kind string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = kind
	}
	if utf8.RuneCountInString(name) > 40 || strings.ContainsAny(name, "\r\n") {
		return "", ErrChannelName
	}
	return name, nil
}

// CreateChannel 新建一个渠道（未验证、未启用）；需要再发送测试消息。
func (s *Service) CreateChannel(ctx context.Context, userID string, in ChannelInput) (*ChannelView, error) {
	if s.Pusher == nil {
		return nil, ErrChannelsUnavailable
	}
	n, err := s.Store.Channels().Count(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= MaxChannels {
		return nil, ErrChannelLimit
	}
	p := trimPush(in.Push)
	name, err := channelName(in.Name, p.Kind)
	if err != nil {
		return nil, err
	}
	if err := s.Pusher.CheckPush(p); err != nil {
		return nil, err
	}
	cfg, err := json.Marshal(p) //nolint:gosec // 设置整体加密后存储（config_enc），不会以明文离开服务
	if err != nil {
		return nil, err
	}
	c := &store.Channel{OwnerID: userID, Kind: p.Kind, Name: name, Config: cfg, CreatedAt: s.now()}
	if err := s.Store.Channels().Create(ctx, c); err != nil {
		return nil, err
	}
	s.audit(ctx, userID, "channel.create", map[string]string{"id": c.ID, "kind": c.Kind})
	v := channelView(c)
	return &v, nil
}

// UpdateChannel 修改渠道；种类不能改。令牌、key、密钥与 Webhook 地址留空时保留原值。修改后回到未验证。
func (s *Service) UpdateChannel(ctx context.Context, userID, id string, in ChannelInput) (*ChannelView, error) {
	if s.Pusher == nil {
		return nil, ErrChannelsUnavailable
	}
	c, err := s.Store.Channels().Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	old, err := channelPush(c)
	if err != nil {
		return nil, err
	}
	p := trimPush(in.Push)
	p.Kind = c.Kind
	keep := func(v *string, o string) {
		if *v == "" {
			*v = o
		}
	}
	keep(&p.Token, old.Token)
	keep(&p.Key, old.Key)
	keep(&p.Secret, old.Secret)
	if c.Kind == source.PushWebhook {
		keep(&p.URL, old.URL)
	}
	name, err := channelName(in.Name, c.Kind)
	if err != nil {
		return nil, err
	}
	if err := s.Pusher.CheckPush(p); err != nil {
		return nil, err
	}
	if c.Config, err = json.Marshal(p); err != nil { //nolint:gosec // 同上：加密存储
		return nil, err
	}
	c.Name, c.Enabled, c.VerifiedAt, c.LastAt, c.LastError = name, false, time.Time{}, time.Time{}, ""
	if err := s.Store.Channels().Update(ctx, c); err != nil {
		return nil, err
	}
	s.audit(ctx, userID, "channel.update", map[string]string{"id": c.ID, "kind": c.Kind})
	v := channelView(c)
	return &v, nil
}

// DeleteChannel 删除渠道。
func (s *Service) DeleteChannel(ctx context.Context, userID, id string) error {
	if err := s.Store.Channels().Delete(ctx, userID, id); err != nil {
		return err
	}
	s.audit(ctx, userID, "channel.delete", map[string]string{"id": id})
	return nil
}

// TestChannel 发送测试消息；送达后启用。与测试邮件共用每个用户每小时 5 次的限额。
func (s *Service) TestChannel(ctx context.Context, userID, id string) (*ChannelView, error) {
	if s.Pusher == nil {
		return nil, ErrChannelsUnavailable
	}
	c, err := s.Store.Channels().Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	g, now := s.g(), s.now()
	if w := g.smtpTest.Wait(userID, now); w > 0 {
		return nil, &WaitError{Wait: w}
	}
	g.smtpTest.Fail(userID, now) // 每次尝试都计数
	u, err := s.Store.Users().ByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	p, err := channelPush(c)
	if err != nil {
		return nil, err
	}
	title, body := alertText(langOf(u), "test", nil)
	sendErr := s.Pusher.Send(ctx, p, source.PushMessage{Kind: "test", Title: title, Text: body, Time: now.UTC()})
	c.LastAt, c.LastError = now, ""
	if sendErr != nil {
		c.LastError = sendErr.Error()
	} else {
		c.Enabled, c.VerifiedAt = true, now
	}
	if err := s.Store.Channels().Update(ctx, c); err != nil {
		return nil, err
	}
	if sendErr != nil {
		return nil, sendErr
	}
	s.audit(ctx, userID, "channel.verified", map[string]string{"id": c.ID, "kind": c.Kind})
	v := channelView(c)
	return &v, nil
}

func trimPush(p source.Push) source.Push {
	for _, f := range []*string{&p.Kind, &p.URL, &p.Token, &p.Chat, &p.Key, &p.Topic, &p.Secret} {
		*f = strings.TrimSpace(*f)
	}
	return p
}

// pushAlert 把一条提醒发给本人全部已启用的渠道（在 sendAlert 的后台 goroutine 中调用）。
// 每个渠道的结果记在渠道上；失败只记日志，不影响其他渠道与邮件。
func (s *Service) pushAlert(ctx context.Context, userID string, kind Alert, title, body string, detail map[string]string, at time.Time) {
	cs, err := s.Store.Channels().List(ctx, userID)
	if err != nil {
		if s.Log != nil {
			s.Log.Warn("读取提醒渠道失败", "user", userID, "error", err)
		}
		return
	}
	for i := range cs {
		c := &cs[i]
		if !c.Enabled {
			continue
		}
		p, err := channelPush(c)
		if err == nil {
			err = s.Pusher.Send(ctx, p, source.PushMessage{Kind: string(kind), Title: title, Text: body, Detail: detail, Time: at.UTC()})
		}
		msg := ""
		if err != nil {
			msg = err.Error()
			if s.Log != nil {
				s.Log.Warn("提醒推送失败", "user", userID, "channel", c.ID, "kind", kind, "error", err)
			}
		}
		if err := s.Store.Channels().Result(ctx, c.ID, s.now(), msg); err != nil && s.Log != nil {
			s.Log.Warn("记录推送结果失败", "channel", c.ID, "error", err)
		}
	}
}
