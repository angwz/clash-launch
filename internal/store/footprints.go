package store

import (
	"context"
	"time"
)

// 足迹（需求 R10），表结构见 migrations/*/0025_footprints.sql。

// Footprint 是一条足迹。UserID 为空表示站点本身。
type Footprint struct {
	UserID     string
	Kind       string
	Subject    string // 同一类中的区分：国家代码、阈值、客户端名称……；「第一次」类为空
	At         time.Time
	Detail     []byte // JSON
	Backfilled bool
}

// FootprintRepo 管理足迹。
type FootprintRepo interface {
	// Add 写入一条足迹；同一 (用户, 类别, 区分) 已有时忽略并返回 false。
	Add(ctx context.Context, f *Footprint) (bool, error)
	// List 返回用户的全部足迹（按时间先后）。
	List(ctx context.Context, user string) ([]Footprint, error)
	// Count 返回用户某一类足迹的条数（如去过的国家数）。
	Count(ctx context.Context, user, kind string) (int, error)
}

func (d *DB) Footprints() FootprintRepo { return footprints{d} }

type footprints struct{ d *DB }

func (r footprints) Add(ctx context.Context, f *Footprint) (bool, error) {
	if len(f.Detail) == 0 {
		f.Detail = []byte("{}")
	}
	res, err := r.d.exec(ctx, "INSERT INTO footprints (user_id, kind, subject, at, detail, backfilled) VALUES (?,?,?,?,?,?) ON CONFLICT (user_id, kind, subject) DO NOTHING",
		f.UserID, f.Kind, f.Subject, ms(f.At), string(f.Detail), b2i(f.Backfilled))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r footprints) List(ctx context.Context, user string) ([]Footprint, error) {
	rows, err := r.d.query(ctx, "SELECT user_id, kind, subject, at, detail, backfilled FROM footprints WHERE user_id=? ORDER BY at, kind, subject", user)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Footprint
	for rows.Next() {
		var f Footprint
		var at int64
		var detail string
		var bf int
		if err := rows.Scan(&f.UserID, &f.Kind, &f.Subject, &at, &detail, &bf); err != nil {
			return nil, err
		}
		f.At, f.Detail, f.Backfilled = fromMS(at), []byte(detail), bf != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r footprints) Count(ctx context.Context, user, kind string) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM footprints WHERE user_id=? AND kind=?", user, kind).Scan(&n)
	return n, err
}
