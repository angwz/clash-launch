package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

// 短链接与站内消息（反馈 #74），表结构见 migrations/*/0008_shortlinks_messages.sql。

// ShortLink 是一个短链接：/u/<Code> 提供 LinkID 所指配置链接的内容。
type ShortLink struct {
	ID        string
	OwnerID   string
	LinkID    string
	Code      string // 小写
	Note      string
	Disabled  bool
	ExpiresAt time.Time // 零值表示不过期
	// GranteeID 非空表示这是为好友单独生成的短链接（反馈 #75）
	GranteeID string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

// Active 报告短链接此刻是否可用。
func (s *ShortLink) Active(now time.Time) bool {
	return !s.Disabled && (s.ExpiresAt.IsZero() || now.Before(s.ExpiresAt))
}

// ShortLinkRepo 管理短链接。除 ByCode（公开访问用）外都按所有者隔离。
type ShortLinkRepo interface {
	List(ctx context.Context, owner string) ([]ShortLink, error)
	Get(ctx context.Context, owner, id string) (*ShortLink, error)
	// ByCode 按短码查找（不区分所有者，供 /u/<code> 使用）。
	ByCode(ctx context.Context, code string) (*ShortLink, error)
	// Create 新建；短码已被使用时返回 ErrExists。
	Create(ctx context.Context, owner string, s *ShortLink) error
	// Update 修改备注、停用与过期时间（乐观锁）。
	Update(ctx context.Context, owner string, s *ShortLink) error
	Delete(ctx context.Context, owner, id string) error
	Count(ctx context.Context, owner string) (int, error)
}

func (d *DB) ShortLinks() ShortLinkRepo { return shortLinks{d} }

type shortLinks struct{ d *DB }

const shortCols = "id, owner_id, link_id, code, note, disabled, expires_at, created_at, updated_at, version, grantee_id"

func scanShort(row interface{ Scan(...any) error }) (*ShortLink, error) {
	var s ShortLink
	var dis, exp, c, u int64
	if err := row.Scan(&s.ID, &s.OwnerID, &s.LinkID, &s.Code, &s.Note, &dis, &exp, &c, &u, &s.Version, &s.GranteeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s.Disabled = dis != 0
	if exp != 0 {
		s.ExpiresAt = fromMS(exp)
	}
	s.CreatedAt, s.UpdatedAt = fromMS(c), fromMS(u)
	return &s, nil
}

func (r shortLinks) List(ctx context.Context, owner string) ([]ShortLink, error) {
	rows, err := r.d.query(ctx, "SELECT "+shortCols+" FROM short_links WHERE owner_id=? ORDER BY created_at DESC, id DESC", owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ShortLink
	for rows.Next() {
		s, err := scanShort(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r shortLinks) Get(ctx context.Context, owner, id string) (*ShortLink, error) {
	return scanShort(r.d.row(ctx, "SELECT "+shortCols+" FROM short_links WHERE owner_id=? AND id=?", owner, id))
}

func (r shortLinks) ByCode(ctx context.Context, code string) (*ShortLink, error) {
	return scanShort(r.d.row(ctx, "SELECT "+shortCols+" FROM short_links WHERE code=?", strings.ToLower(code)))
}

func (r shortLinks) Create(ctx context.Context, owner string, s *ShortLink) error {
	if s.ID == "" {
		s.ID = NewID()
	}
	now := time.Now()
	s.OwnerID, s.Code, s.CreatedAt, s.UpdatedAt, s.Version = owner, strings.ToLower(s.Code), now, now, 1
	// 只能指向自己的配置链接：同一事务中先确认链接属于 owner，再插入
	return r.d.Tx(ctx, func(st Store) error {
		x := st.(*DB)
		var n int
		if err := x.row(ctx, "SELECT COUNT(*) FROM links WHERE id=? AND owner_id=?", s.LinkID, owner).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		_, err := x.exec(ctx, "INSERT INTO short_links ("+shortCols+") VALUES (?,?,?,?,?,?,?,?,?,1,?)",
			s.ID, owner, s.LinkID, s.Code, s.Note, b2i(s.Disabled), msOrZero(s.ExpiresAt), ms(now), ms(now), s.GranteeID)
		if err != nil && isUnique(err) {
			return ErrExists
		}
		return err
	})
}

func (r shortLinks) Update(ctx context.Context, owner string, s *ShortLink) error {
	now := time.Now()
	res, err := r.d.exec(ctx, "UPDATE short_links SET note=?, disabled=?, expires_at=?, updated_at=?, version=version+1 WHERE owner_id=? AND id=? AND version=?",
		s.Note, b2i(s.Disabled), msOrZero(s.ExpiresAt), ms(now), owner, s.ID, s.Version)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := r.Get(ctx, owner, s.ID); err != nil {
			return err
		}
		return ErrConflict
	}
	s.Version++
	s.UpdatedAt = now
	return nil
}

func (r shortLinks) Delete(ctx context.Context, owner, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM short_links WHERE owner_id=? AND id=?", owner, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r shortLinks) Count(ctx context.Context, owner string) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM short_links WHERE owner_id=?", owner).Scan(&n)
	return n, err
}

func msOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return ms(t)
}

// Message 是一条站内消息。Body 是解密后的内容（JSON，由 service 定义结构）。
type Message struct {
	ID              string
	SenderID        string
	RecipientID     string
	Kind            string // text | nodes | subscription
	Body            []byte
	CreatedAt       time.Time
	ReadAt          time.Time
	SenderHidden    bool
	RecipientHidden bool
	AddedSource     string
}

// MessageRepo 管理站内消息。每个方法都以「当前用户」为参数，只返回该用户作为发件人或收件人、且未隐藏的消息。
type MessageRepo interface {
	Create(ctx context.Context, m *Message) error
	Get(ctx context.Context, user, id string) (*Message, error)
	// Thread 返回 user 与 other 之间的消息（新的在前）；Page.After 为上一页最后一条的「时间|ID」。
	Thread(ctx context.Context, user, other string, p Page) ([]Message, error)
	// Recent 返回 user 最近的消息（新的在前，最多 limit 条），用于会话列表。
	Recent(ctx context.Context, user string, limit int) ([]Message, error)
	// MarkRead 把 other 发给 user 的未读消息标为已读。
	MarkRead(ctx context.Context, user, other string, at time.Time) error
	// Unread 返回 user 的未读消息数（不含回收站中的对话）；UnreadFrom 只算 other 发来的。
	Unread(ctx context.Context, user string) (int, error)
	UnreadFrom(ctx context.Context, user, other string) (int, error)
	// Hide 对 user 隐藏一条消息（发件人与收件人各自独立）。
	Hide(ctx context.Context, user, id string) error
	// SetAdded 记录收件人由附件加入的源。
	SetAdded(ctx context.Context, recipient, id, sourceID string) error
	// SentSince 返回 sender 在 since 之后发出的消息数（发送频率限制）。
	SentSince(ctx context.Context, sender string, since time.Time) (int, error)
}

func (d *DB) Messages() MessageRepo { return messages{d} }

type messages struct{ d *DB }

const messageCols = "id, sender_id, recipient_id, kind, body_enc, created_at, read_at, sender_hidden, recipient_hidden, added_source"

// visibleTo 是「user 能看到」的条件（参数：user, user）。
const visibleTo = "((sender_id=? AND sender_hidden=0) OR (recipient_id=? AND recipient_hidden=0))"

func (r messages) scan(row interface{ Scan(...any) error }) (*Message, error) {
	var m Message
	var enc []byte
	var c, rd, sh, rh int64
	if err := row.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Kind, &enc, &c, &rd, &sh, &rh, &m.AddedSource); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	body, err := r.d.open(enc, "messages", "body_enc", m.ID)
	if err != nil {
		return nil, err
	}
	m.Body = body
	m.CreatedAt = fromMS(c)
	if rd != 0 {
		m.ReadAt = fromMS(rd)
	}
	m.SenderHidden, m.RecipientHidden = sh != 0, rh != 0
	return &m, nil
}

func (r messages) list(rows *sql.Rows, err error) ([]Message, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Message
	for rows.Next() {
		m, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (r messages) Create(ctx context.Context, m *Message) error {
	if m.ID == "" {
		m.ID = NewID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	enc, err := r.d.seal(m.Body, "messages", "body_enc", m.ID)
	if err != nil {
		return err
	}
	return r.d.Tx(ctx, func(st Store) error {
		x := st.(*DB)
		if _, err := x.exec(ctx, "INSERT INTO messages ("+messageCols+") VALUES (?,?,?,?,?,?,0,0,0,?)",
			m.ID, m.SenderID, m.RecipientID, m.Kind, enc, ms(m.CreatedAt), m.AddedSource); err != nil {
			return err
		}
		// 双方都成为（或恢复为）联系人，按这条消息的时间排序（反馈 #75）
		for _, pair := range [][2]string{{m.SenderID, m.RecipientID}, {m.RecipientID, m.SenderID}} {
			if _, err := x.exec(ctx, "INSERT INTO contacts (user_id, peer_id, last_at, deleted_at) VALUES (?,?,?,0) "+
				"ON CONFLICT (user_id, peer_id) DO UPDATE SET last_at = excluded.last_at, deleted_at = 0", pair[0], pair[1], ms(m.CreatedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r messages) Get(ctx context.Context, user, id string) (*Message, error) {
	return r.scan(r.d.row(ctx, "SELECT "+messageCols+" FROM messages WHERE id=? AND "+visibleTo, id, user, user))
}

func (r messages) Thread(ctx context.Context, user, other string, p Page) ([]Message, error) {
	at, id := int64(1<<62), "~"
	if a, b, ok := strings.Cut(p.After, "|"); ok {
		if n, err := strconv.ParseInt(a, 10, 64); err == nil {
			at, id = n, b
		}
	}
	return r.list(r.d.query(ctx, "SELECT "+messageCols+" FROM messages WHERE "+
		"((sender_id=? AND recipient_id=? AND sender_hidden=0) OR (sender_id=? AND recipient_id=? AND recipient_hidden=0)) "+
		"AND (created_at < ? OR (created_at = ? AND id < ?)) ORDER BY created_at DESC, id DESC LIMIT ?",
		user, other, other, user, at, at, id, limitOf(p)))
}

func (r messages) Recent(ctx context.Context, user string, limit int) ([]Message, error) {
	return r.list(r.d.query(ctx, "SELECT "+messageCols+" FROM messages WHERE "+visibleTo+" ORDER BY created_at DESC, id DESC LIMIT ?", user, user, limit))
}

func (r messages) MarkRead(ctx context.Context, user, other string, at time.Time) error {
	_, err := r.d.exec(ctx, "UPDATE messages SET read_at=? WHERE recipient_id=? AND sender_id=? AND read_at=0", ms(at), user, other)
	return err
}

func (r messages) Unread(ctx context.Context, user string) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM messages m WHERE recipient_id=? AND recipient_hidden=0 AND read_at=0 "+
		"AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.user_id=m.recipient_id AND c.peer_id=m.sender_id AND c.deleted_at <> 0)", user).Scan(&n)
	return n, err
}

func (r messages) UnreadFrom(ctx context.Context, user, other string) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM messages WHERE recipient_id=? AND sender_id=? AND recipient_hidden=0 AND read_at=0", user, other).Scan(&n)
	return n, err
}

func (r messages) Hide(ctx context.Context, user, id string) error {
	res, err := r.d.exec(ctx, "UPDATE messages SET sender_hidden = CASE WHEN sender_id=? THEN 1 ELSE sender_hidden END, "+
		"recipient_hidden = CASE WHEN recipient_id=? THEN 1 ELSE recipient_hidden END WHERE id=? AND "+visibleTo, user, user, id, user, user)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r messages) SetAdded(ctx context.Context, recipient, id, sourceID string) error {
	res, err := r.d.exec(ctx, "UPDATE messages SET added_source=? WHERE id=? AND recipient_id=?", sourceID, id, recipient)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r messages) SentSince(ctx context.Context, sender string, since time.Time) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM messages WHERE sender_id=? AND created_at>=?", sender, ms(since)).Scan(&n)
	return n, err
}
