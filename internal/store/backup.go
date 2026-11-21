package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// 个人备份与还原（需求 R21）的存储部分：还原前的自动留底、按备份重建修订历史、跨用户检查 ID 是否已被占用。

// UserBackup 是一份由服务器保存的个人备份（目前只有还原前的自动留底）。Data 是备份的明文 JSON，加密存储。
type UserBackup struct {
	ID        string
	OwnerID   string
	Kind      string // pre_restore
	Data      []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

// BackupRepo 管理服务器保存的个人备份。
type BackupRepo interface {
	// Put 保存一份备份；同一用户同一 kind 只保留最新一份。
	Put(ctx context.Context, b *UserBackup) error
	// Get 返回未过期的备份；没有或已过期时返回 ErrNotFound。
	Get(ctx context.Context, owner, kind string, now time.Time) (*UserBackup, error)
	Delete(ctx context.Context, owner, kind string) error
	// Purge 删除已过期的备份，返回删除的条数。
	Purge(ctx context.Context, now time.Time) (int64, error)
}

func (d *DB) Backups() BackupRepo { return backups{d} }

type backups struct{ d *DB }

func (r backups) Put(ctx context.Context, b *UserBackup) error {
	if b.ID == "" {
		b.ID = NewID()
	}
	enc, err := r.d.seal(b.Data, "user_backups", "data", b.ID)
	if err != nil {
		return err
	}
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM user_backups WHERE owner_id=? AND kind=?", b.OwnerID, b.Kind); err != nil {
			return err
		}
		_, err := tx.exec(ctx, "INSERT INTO user_backups (id, owner_id, kind, created_at, expires_at, data_enc) VALUES (?,?,?,?,?,?)",
			b.ID, b.OwnerID, b.Kind, ms(b.CreatedAt), ms(b.ExpiresAt), enc)
		return err
	})
}

func (r backups) Get(ctx context.Context, owner, kind string, now time.Time) (*UserBackup, error) {
	b := UserBackup{OwnerID: owner, Kind: kind}
	var enc []byte
	var c, e int64
	err := r.d.row(ctx, "SELECT id, created_at, expires_at, data_enc FROM user_backups WHERE owner_id=? AND kind=? AND expires_at>?", owner, kind, ms(now)).Scan(&b.ID, &c, &e, &enc)
	if err != nil {
		return nil, notFound(err)
	}
	if b.Data, err = r.d.open(enc, "user_backups", "data", b.ID); err != nil {
		return nil, err
	}
	b.CreatedAt, b.ExpiresAt = fromMS(c), fromMS(e)
	return &b, nil
}

func (r backups) Delete(ctx context.Context, owner, kind string) error {
	_, err := r.d.exec(ctx, "DELETE FROM user_backups WHERE owner_id=? AND kind=?", owner, kind)
	return err
}

func (r backups) Purge(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.d.exec(ctx, "DELETE FROM user_backups WHERE expires_at<=?", ms(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// idTables 是可以用 Taken 检查的表：还原到另一台服务器（或另一个账号）时，备份里的 ID 可能已被别人使用。
var idTables = []string{"sources", "rule_sources", "profiles", "templates", "links", "short_links", "notify_channels"}

// Taken 报告 table 中是否已有这个 ID（不区分所有者）。在事务中先查后插：PostgreSQL 的事务里
// 一条失败的语句会让整个事务作废，不能靠捕获唯一约束冲突来判断。
func (d *DB) Taken(ctx context.Context, table, id string) (bool, error) {
	if !slices.Contains(idTables, table) {
		return false, fmt.Errorf("%w：%s", ErrBackupTable, table)
	}
	var n int
	err := d.row(ctx, "SELECT COUNT(*) FROM "+table+" WHERE id=?", id).Scan(&n) // 表名来自常量列表 idTables
	return n > 0, err
}

// SetRevisions 用备份中的修订历史替换方案现有的历史，并把方案版本设为 version（还原，需求 R21）。
// 修订按 N 写入；没有修订时只设置版本。须在事务中调用，方案必须属于 owner。
func (r profiles) SetRevisions(ctx context.Context, owner, profileID string, version int64, revs []Revision) error {
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		res, err := tx.exec(ctx, "UPDATE profiles SET version=? WHERE owner_id=? AND id=?", version, owner, profileID)
		if err := affected(res, err, ErrNotFound); err != nil {
			return err
		}
		if _, err := tx.exec(ctx, "DELETE FROM revisions WHERE owner_id=? AND profile_id=?", owner, profileID); err != nil {
			return err
		}
		for _, v := range revs {
			id := NewID()
			enc, err := tx.seal(v.Doc, "revisions", "doc", id)
			if err != nil {
				return err
			}
			if _, err := tx.exec(ctx, "INSERT INTO revisions (id, profile_id, owner_id, n, doc_enc, note, created_at) VALUES (?,?,?,?,?,?,?)",
				id, profileID, owner, v.N, enc, v.Note, ms(v.CreatedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

// ErrBackupTable 是 Taken 收到未登记表名时的错误类别（供调用方区分编程错误）。
var ErrBackupTable = errors.New("store: 不支持的表")
