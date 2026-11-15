package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// 联系人、回收站与授权（反馈 #75），表结构见 migrations/*/0009_contacts_grants.sql。

// Contact 是一个联系人（交流过的人）。DeletedAt 非零表示对话在回收站中。
type Contact struct {
	UserID    string
	PeerID    string
	LastAt    time.Time
	DeletedAt time.Time
}

// ContactRepo 管理联系人。都以「当前用户」为第一个参数，只操作本人的那一行。
type ContactRepo interface {
	// List 返回联系人（最近交流的在前）；deleted 为 true 时返回回收站中的。
	List(ctx context.Context, user string, deleted bool) ([]Contact, error)
	Get(ctx context.Context, user, peer string) (*Contact, error)
	// SetDeleted 把对话放入回收站（at 非零）或恢复（at 为零）。
	SetDeleted(ctx context.Context, user, peer string, at time.Time) error
	// Purge 永久清除在回收站中超过 before 的对话：这些对话的消息对本人隐藏，联系人移除。返回清除的个数。
	Purge(ctx context.Context, before time.Time) (int, error)
}

func (d *DB) Contacts() ContactRepo { return contacts{d} }

type contacts struct{ d *DB }

func scanContact(row interface{ Scan(...any) error }) (*Contact, error) {
	var c Contact
	var l, del int64
	if err := row.Scan(&c.UserID, &c.PeerID, &l, &del); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.LastAt = fromMS(l)
	if del != 0 {
		c.DeletedAt = fromMS(del)
	}
	return &c, nil
}

func (r contacts) List(ctx context.Context, user string, deleted bool) ([]Contact, error) {
	cond := "deleted_at = 0"
	if deleted {
		cond = "deleted_at <> 0"
	}
	rows, err := r.d.query(ctx, "SELECT user_id, peer_id, last_at, deleted_at FROM contacts WHERE user_id=? AND "+cond+" ORDER BY last_at DESC, peer_id", user)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r contacts) Get(ctx context.Context, user, peer string) (*Contact, error) {
	return scanContact(r.d.row(ctx, "SELECT user_id, peer_id, last_at, deleted_at FROM contacts WHERE user_id=? AND peer_id=?", user, peer))
}

func (r contacts) SetDeleted(ctx context.Context, user, peer string, at time.Time) error {
	res, err := r.d.exec(ctx, "UPDATE contacts SET deleted_at=? WHERE user_id=? AND peer_id=?", msOrZero(at), user, peer)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r contacts) Purge(ctx context.Context, before time.Time) (int, error) {
	var n int
	err := r.d.Tx(ctx, func(st Store) error {
		x := st.(*DB)
		rows, err := x.query(ctx, "SELECT user_id, peer_id FROM contacts WHERE deleted_at <> 0 AND deleted_at < ?", ms(before))
		if err != nil {
			return err
		}
		var pairs [][2]string
		for rows.Next() {
			var p [2]string
			if err := rows.Scan(&p[0], &p[1]); err != nil {
				_ = rows.Close()
				return err
			}
			pairs = append(pairs, p)
		}
		_ = rows.Close()
		for _, p := range pairs {
			if _, err := x.exec(ctx, "UPDATE messages SET sender_hidden=1 WHERE sender_id=? AND recipient_id=?", p[0], p[1]); err != nil {
				return err
			}
			if _, err := x.exec(ctx, "UPDATE messages SET recipient_hidden=1 WHERE recipient_id=? AND sender_id=?", p[0], p[1]); err != nil {
				return err
			}
			if _, err := x.exec(ctx, "DELETE FROM contacts WHERE user_id=? AND peer_id=?", p[0], p[1]); err != nil {
				return err
			}
		}
		n = len(pairs)
		return nil
	})
	return n, err
}

// Grant 是一项授权。
type Grant struct {
	ID        string
	OwnerID   string
	GranteeID string
	Kind      string // source | short
	TargetID  string
	CreatedAt time.Time
	RevokedAt time.Time
}

// Active 报告授权是否有效。
func (g *Grant) Active() bool { return g.RevokedAt.IsZero() }

// GrantRepo 管理授权。授权者与被授权者都能读取与收回；其他人一律 ErrNotFound。
type GrantRepo interface {
	Create(ctx context.Context, g *Grant) error
	// Get 返回 user 作为授权者或被授权者的一项授权。
	Get(ctx context.Context, user, id string) (*Grant, error)
	// Given 返回 user 给出的授权；Received 返回 user 收到的授权。activeOnly 只返回有效的。
	Given(ctx context.Context, user string, activeOnly bool) ([]Grant, error)
	Received(ctx context.Context, user string, activeOnly bool) ([]Grant, error)
	// ActiveFor 返回被授权者对某个目标的有效授权（用于生成配置时解析软链接）。
	ActiveFor(ctx context.Context, grantee, kind, target string) (*Grant, error)
	// ByTarget 返回某个目标上的全部有效授权（授权者更新节点后重新生成对方的配置）。
	ByTarget(ctx context.Context, owner, kind, target string) ([]Grant, error)
	// Revoke 收回一项授权（授权者或被授权者都可以）；已收回时不报错。
	Revoke(ctx context.Context, user, id string, at time.Time) error
	// Between 返回 a 与 b 之间（任一方向）的有效授权。
	Between(ctx context.Context, a, b string) ([]Grant, error)
}

func (d *DB) Grants() GrantRepo { return grants{d} }

type grants struct{ d *DB }

const grantCols = "id, owner_id, grantee_id, kind, target_id, created_at, revoked_at"

func scanGrant(row interface{ Scan(...any) error }) (*Grant, error) {
	var g Grant
	var c, rv int64
	if err := row.Scan(&g.ID, &g.OwnerID, &g.GranteeID, &g.Kind, &g.TargetID, &c, &rv); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	g.CreatedAt = fromMS(c)
	if rv != 0 {
		g.RevokedAt = fromMS(rv)
	}
	return &g, nil
}

func (r grants) list(rows *sql.Rows, err error) ([]Grant, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Grant
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r grants) Create(ctx context.Context, g *Grant) error {
	if g.ID == "" {
		g.ID = NewID()
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now()
	}
	_, err := r.d.exec(ctx, "INSERT INTO grants ("+grantCols+") VALUES (?,?,?,?,?,?,0)", g.ID, g.OwnerID, g.GranteeID, g.Kind, g.TargetID, ms(g.CreatedAt))
	return err
}

func (r grants) Get(ctx context.Context, user, id string) (*Grant, error) {
	return scanGrant(r.d.row(ctx, "SELECT "+grantCols+" FROM grants WHERE id=? AND (owner_id=? OR grantee_id=?)", id, user, user))
}

func activeCond(activeOnly bool) string {
	if activeOnly {
		return " AND revoked_at = 0"
	}
	return ""
}

func (r grants) Given(ctx context.Context, user string, activeOnly bool) ([]Grant, error) {
	return r.list(r.d.query(ctx, "SELECT "+grantCols+" FROM grants WHERE owner_id=?"+activeCond(activeOnly)+" ORDER BY created_at DESC", user))
}

func (r grants) Received(ctx context.Context, user string, activeOnly bool) ([]Grant, error) {
	return r.list(r.d.query(ctx, "SELECT "+grantCols+" FROM grants WHERE grantee_id=?"+activeCond(activeOnly)+" ORDER BY created_at DESC", user))
}

func (r grants) ActiveFor(ctx context.Context, grantee, kind, target string) (*Grant, error) {
	return scanGrant(r.d.row(ctx, "SELECT "+grantCols+" FROM grants WHERE grantee_id=? AND kind=? AND target_id=? AND revoked_at = 0 ORDER BY created_at DESC LIMIT 1", grantee, kind, target))
}

func (r grants) ByTarget(ctx context.Context, owner, kind, target string) ([]Grant, error) {
	return r.list(r.d.query(ctx, "SELECT "+grantCols+" FROM grants WHERE owner_id=? AND kind=? AND target_id=? AND revoked_at = 0", owner, kind, target))
}

func (r grants) Revoke(ctx context.Context, user, id string, at time.Time) error {
	if _, err := r.Get(ctx, user, id); err != nil {
		return err
	}
	_, err := r.d.exec(ctx, "UPDATE grants SET revoked_at=? WHERE id=? AND revoked_at = 0", ms(at), id)
	return err
}

func (r grants) Between(ctx context.Context, a, b string) ([]Grant, error) {
	return r.list(r.d.query(ctx, "SELECT "+grantCols+" FROM grants WHERE revoked_at = 0 AND ((owner_id=? AND grantee_id=?) OR (owner_id=? AND grantee_id=?))", a, b, b, a))
}
