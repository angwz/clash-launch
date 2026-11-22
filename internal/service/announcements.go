package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/events"
	"github.com/angwz/clash-launch/internal/store"
)

// 站内公告（需求 R32）：管理员发布横幅（维护通知、新功能），可设起止时间；用户关闭后不再出现（换设备也一样）。
// 发布、修改、删除后经实时通道通知所有在线用户重读，公告即时出现或消失。
// 正文是纯文字（至多 500 字），界面原样显示，不解析 HTML 或 Markdown。

// ErrAnnouncement 表示公告的内容或时间无效。
var ErrAnnouncement = errors.New("announcement: invalid")

const announcementMax = 500

// AnnouncementInput 是发布或修改公告的内容；StartsAt 为零表示立即，EndsAt 为零表示一直显示到删除。
type AnnouncementInput struct {
	Text     string
	Level    string
	StartsAt time.Time
	EndsAt   time.Time
}

func (in *AnnouncementInput) check(now time.Time) error {
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" || utf8.RuneCountInString(in.Text) > announcementMax {
		return ErrAnnouncement
	}
	switch in.Level {
	case "":
		in.Level = "info"
	case "info", "warn":
	default:
		return ErrAnnouncement
	}
	if in.StartsAt.IsZero() {
		in.StartsAt = now
	}
	if !in.EndsAt.IsZero() && !in.EndsAt.After(in.StartsAt) {
		return ErrAnnouncement
	}
	return nil
}

// CreateAnnouncement 发布一条公告。
func (s *Service) CreateAnnouncement(ctx context.Context, admin *store.User, in AnnouncementInput) (*store.Announcement, error) {
	now := s.now()
	if err := in.check(now); err != nil {
		return nil, err
	}
	a := &store.Announcement{Text: in.Text, Level: in.Level, StartsAt: in.StartsAt, EndsAt: in.EndsAt, CreatedBy: admin.Username, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.Announcements().Create(ctx, a); err != nil {
		return nil, err
	}
	s.audit(ctx, admin.ID, "admin.announcement.create", map[string]string{"id": a.ID})
	s.Events.PublishAll(events.Event{Type: events.Announce, ID: a.ID})
	return a, nil
}

// UpdateAnnouncement 修改公告的内容或时间。已关闭过它的用户不会因修改而再次看到。
func (s *Service) UpdateAnnouncement(ctx context.Context, admin *store.User, id string, in AnnouncementInput) (*store.Announcement, error) {
	now := s.now()
	if err := in.check(now); err != nil {
		return nil, err
	}
	a := &store.Announcement{ID: id, Text: in.Text, Level: in.Level, StartsAt: in.StartsAt, EndsAt: in.EndsAt, UpdatedAt: now}
	if err := s.Store.Announcements().Update(ctx, a); err != nil {
		return nil, err
	}
	s.audit(ctx, admin.ID, "admin.announcement.update", map[string]string{"id": id})
	s.Events.PublishAll(events.Event{Type: events.Announce, ID: id})
	return a, nil
}

// DeleteAnnouncement 删除公告（关闭记录随之删除）。
func (s *Service) DeleteAnnouncement(ctx context.Context, admin *store.User, id string) error {
	if err := s.Store.Announcements().Delete(ctx, id); err != nil {
		return err
	}
	s.audit(ctx, admin.ID, "admin.announcement.delete", map[string]string{"id": id})
	s.Events.PublishAll(events.Event{Type: events.Announce, ID: id})
	return nil
}
