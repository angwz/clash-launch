package store

import (
	"context"
	"time"
)

// 站内公告（需求 R32），表结构见 migrations/*/0027_announcements.sql。

// Announcement 是一条公告。EndsAt 为零表示一直显示到删除。
type Announcement struct {
	ID        string
	Text      string
	Level     string // info | warn
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AnnouncementRepo 管理公告与用户的关闭记录。
type AnnouncementRepo interface {
	Create(ctx context.Context, a *Announcement) error
	Update(ctx context.Context, a *Announcement) error
	Delete(ctx context.Context, id string) error
	// All 返回全部公告（新的在前），管理页使用。
	All(ctx context.Context) ([]Announcement, error)
	// Active 返回此刻生效、且 user 没有关闭的公告（新的在前）。
	Active(ctx context.Context, user string, now time.Time) ([]Announcement, error)
	// Dismiss 记下 user 关闭了公告；公告不存在时返回 ErrNotFound。
	Dismiss(ctx context.Context, user, id string, at time.Time) error
}

func (d *DB) Announcements() AnnouncementRepo { return announcements{d} }

type announcements struct{ d *DB }

const announcementCols = "id, text, level, starts_at, ends_at, created_by, created_at, updated_at"

func scanAnnouncements(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Announcement, error) {
	var out []Announcement
	for rows.Next() {
		var a Announcement
		var s, e, c, u int64
		if err := rows.Scan(&a.ID, &a.Text, &a.Level, &s, &e, &a.CreatedBy, &c, &u); err != nil {
			return nil, err
		}
		a.StartsAt, a.CreatedAt, a.UpdatedAt = fromMS(s), fromMS(c), fromMS(u)
		if e != 0 {
			a.EndsAt = fromMS(e)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r announcements) Create(ctx context.Context, a *Announcement) error {
	if a.ID == "" {
		a.ID = NewID()
	}
	_, err := r.d.exec(ctx, "INSERT INTO announcements ("+announcementCols+") VALUES (?,?,?,?,?,?,?,?)",
		a.ID, a.Text, a.Level, ms(a.StartsAt), ms(a.EndsAt), a.CreatedBy, ms(a.CreatedAt), ms(a.UpdatedAt))
	return err
}

func (r announcements) Update(ctx context.Context, a *Announcement) error {
	res, err := r.d.exec(ctx, "UPDATE announcements SET text=?, level=?, starts_at=?, ends_at=?, updated_at=? WHERE id=?",
		a.Text, a.Level, ms(a.StartsAt), ms(a.EndsAt), ms(a.UpdatedAt), a.ID)
	return affected(res, err, ErrNotFound)
}

func (r announcements) Delete(ctx context.Context, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM announcements WHERE id=?", id)
	return affected(res, err, ErrNotFound)
}

func (r announcements) All(ctx context.Context) ([]Announcement, error) {
	rows, err := r.d.query(ctx, "SELECT "+announcementCols+" FROM announcements ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAnnouncements(rows)
}

func (r announcements) Active(ctx context.Context, user string, now time.Time) ([]Announcement, error) {
	rows, err := r.d.query(ctx, "SELECT "+announcementCols+" FROM announcements a WHERE starts_at<=? AND (ends_at=0 OR ends_at>?) "+
		"AND NOT EXISTS (SELECT 1 FROM announcement_dismissals x WHERE x.announcement_id=a.id AND x.user_id=?) ORDER BY created_at DESC, id DESC",
		ms(now), ms(now), user)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanAnnouncements(rows)
}

func (r announcements) Dismiss(ctx context.Context, user, id string, at time.Time) error {
	var n int
	if err := r.d.row(ctx, "SELECT COUNT(*) FROM announcements WHERE id=?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	_, err := r.d.exec(ctx, "INSERT INTO announcement_dismissals (user_id, announcement_id, at) VALUES (?,?,?) ON CONFLICT (user_id, announcement_id) DO NOTHING", user, id, ms(at))
	return err
}
