package store

import (
	"context"
	"time"
)

// 站内消息的加密密钥（需求 R46，决定 0009），表结构见 migrations/*/0028_message_keys.sql。

// MessageKey 是用户的一把消息密钥。Wrapped 与 Recovery 是包裹后的私钥（JSON，服务器不能解开）。
type MessageKey struct {
	UserID    string
	KeyID     string
	PublicKey string
	Wrapped   string
	Recovery  string
	Active    bool
	CreatedAt time.Time
}

// MessageKeyRepo 管理消息密钥。
type MessageKeyRepo interface {
	// Active 返回用户当前的密钥；没有时返回 ErrNotFound。
	Active(ctx context.Context, user string) (*MessageKey, error)
	// All 返回用户的全部密钥（新的在前），含已不再使用的旧公钥。
	All(ctx context.Context, user string) ([]MessageKey, error)
	// Replace 把 k 设为用户当前的密钥，旧密钥标为不再使用（在一个事务中）。
	Replace(ctx context.Context, k *MessageKey) error
	// Rewrap 更新当前密钥的包裹（改密码、生成恢复密钥）；recovery 为 nil 时不改。
	Rewrap(ctx context.Context, user, keyID, wrapped string, recovery *string) error
}

func (d *DB) MessageKeys() MessageKeyRepo { return messageKeys{d} }

type messageKeys struct{ d *DB }

const messageKeyCols = "user_id, key_id, public_key, wrapped, recovery, active, created_at"

func scanMessageKey(row interface{ Scan(...any) error }) (*MessageKey, error) {
	var k MessageKey
	var active int
	var c int64
	if err := row.Scan(&k.UserID, &k.KeyID, &k.PublicKey, &k.Wrapped, &k.Recovery, &active, &c); err != nil {
		return nil, notFound(err)
	}
	k.Active, k.CreatedAt = active != 0, fromMS(c)
	return &k, nil
}

func (r messageKeys) Active(ctx context.Context, user string) (*MessageKey, error) {
	return scanMessageKey(r.d.row(ctx, "SELECT "+messageKeyCols+" FROM message_keys WHERE user_id=? AND active=1", user))
}

func (r messageKeys) All(ctx context.Context, user string) ([]MessageKey, error) {
	rows, err := r.d.query(ctx, "SELECT "+messageKeyCols+" FROM message_keys WHERE user_id=? ORDER BY created_at DESC, key_id DESC", user)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []MessageKey
	for rows.Next() {
		k, err := scanMessageKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

func (r messageKeys) Replace(ctx context.Context, k *MessageKey) error {
	return r.d.Tx(ctx, func(st Store) error {
		x := st.(*DB)
		// 旧密钥只留公钥：私钥的包裹一并清掉（用不上了，也不应继续留在服务器上）
		if _, err := x.exec(ctx, "UPDATE message_keys SET active=0, wrapped='', recovery='' WHERE user_id=?", k.UserID); err != nil {
			return err
		}
		_, err := x.exec(ctx, "INSERT INTO message_keys ("+messageKeyCols+") VALUES (?,?,?,?,?,1,?)", k.UserID, k.KeyID, k.PublicKey, k.Wrapped, k.Recovery, ms(k.CreatedAt))
		return err
	})
}

func (r messageKeys) Rewrap(ctx context.Context, user, keyID, wrapped string, recovery *string) error {
	if recovery != nil {
		res, err := r.d.exec(ctx, "UPDATE message_keys SET wrapped=?, recovery=? WHERE user_id=? AND key_id=? AND active=1", wrapped, *recovery, user, keyID)
		return affected(res, err, ErrNotFound)
	}
	res, err := r.d.exec(ctx, "UPDATE message_keys SET wrapped=? WHERE user_id=? AND key_id=? AND active=1", wrapped, user, keyID)
	return affected(res, err, ErrNotFound)
}
