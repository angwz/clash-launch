package store

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"strings"
	"time"
)

// RequestCursor 返回一条请求记录作为分页游标的形式（「时间|ID」）。
func RequestCursor(q SubRequest) string { return strconv.FormatInt(ms(q.At), 10) + "|" + q.ID }

// 统计（反馈 #66–#68），表结构与取值说明见 migrations/*/0007_stats.sql。

// CountDelta 是一次计数增量。Day 为 UTC 日期 YYYYMMDD。
type CountDelta struct {
	Scope, Key string
	Day        int
	N          int64
}

// DayCount 是某一天的计数。
type DayCount struct {
	Key   string
	Day   int
	Count int64
}

// SubRequest 是一次订阅请求的记录。
type SubRequest struct {
	ID      string
	LinkID  string // 空表示令牌无效
	OwnerID string
	At      time.Time
	IP      string
	UA      string
	Client  string
	Status  int
	Bytes   int64
	Kind    string // config | provider | export
	// 归属地（反馈 #74）：记录时由本机地理数据库查出
	Country string // ISO 两位代码
	ASN     string // 如 AS4134
	Org     string
	// ShortID 非空表示经短链接访问
	ShortID string
}

// SubIP 是某条链接上一个 IP 的汇总。
type SubIP struct {
	LinkID  string
	IP      string
	First   time.Time
	Last    time.Time
	OK      int64
	Fail    int64
	Client  string
	OwnerID string
	Country string
	Org     string
}

// StatsRepo 管理统计数据。
type StatsRepo interface {
	// AddCounts 累加计数（同一天同一项合并）。
	AddCounts(ctx context.Context, d []CountDelta) error
	// Daily 返回 scope 下 [from, to] 日期区间内每天的计数；keys 为空表示全部键。
	Daily(ctx context.Context, scope string, keys []string, from, to int) ([]DayCount, error)
	// Totals 返回 scope 下每个键的总数（全部天数之和）；keys 为空表示全部键。
	Totals(ctx context.Context, scope string, keys []string) (map[string]int64, error)
	// AddRequests 写入请求记录，并更新按链接与 IP 的汇总；令牌无效的请求（LinkID 为空）只更新汇总。
	AddRequests(ctx context.Context, rs []SubRequest) error
	// Requests 返回某条链接的请求记录（新的在前），Page.After 为上一页最后一条的 ID。
	Requests(ctx context.Context, owner, linkID string, p Page) ([]SubRequest, error)
	// IPs 返回某条链接的 IP 汇总；owner 为空且 linkID 为空时返回令牌无效的请求来源（管理员）。
	IPs(ctx context.Context, owner, linkID string) ([]SubIP, error)
	// Range 返回某个用户在 [from, to) 内的全部请求记录（时间正序），最多 limit 条；用于订阅分析（反馈 #74）。
	// 第二个返回值表示是否因超过 limit 而截断（此时返回的是区间内最新的 limit 条）。
	Range(ctx context.Context, owner string, from, to time.Time, limit int) ([]SubRequest, bool, error)
	// LastByShort 返回每个短链接最近一次被请求的时间（反馈 #78：短链接列表折叠时显示）。
	LastByShort(ctx context.Context, owner string) (map[string]time.Time, error)
}

func (d *DB) Stats() StatsRepo { return stats{d} }

type stats struct{ d *DB }

func (r stats) AddCounts(ctx context.Context, ds []CountDelta) error {
	if len(ds) == 0 {
		return nil
	}
	return r.d.Tx(ctx, func(s Store) error {
		x := s.(*DB)
		for _, c := range ds {
			// SQLite 与 PostgreSQL 都支持 ON CONFLICT … DO UPDATE
			if _, err := x.exec(ctx, "INSERT INTO stat_counts (scope, key, day, count) VALUES (?,?,?,?) ON CONFLICT (scope, key, day) DO UPDATE SET count = stat_counts.count + excluded.count",
				c.Scope, c.Key, c.Day, c.N); err != nil {
				return err
			}
		}
		return nil
	})
}

// keyFilter 生成 key IN (...) 条件；keys 为空时不过滤。
func keyFilter(keys []string, args []any) (string, []any) {
	if len(keys) == 0 {
		return "", args
	}
	q := " AND key IN ("
	for i, k := range keys {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, k)
	}
	return q + ")", args
}

func (r stats) Daily(ctx context.Context, scope string, keys []string, from, to int) ([]DayCount, error) {
	cond, args := keyFilter(keys, []any{scope, from, to})
	rows, err := r.d.query(ctx, "SELECT key, day, count FROM stat_counts WHERE scope=? AND day>=? AND day<=?"+cond+" ORDER BY day, key", args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []DayCount
	for rows.Next() {
		var c DayCount
		if err := rows.Scan(&c.Key, &c.Day, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r stats) Totals(ctx context.Context, scope string, keys []string) (map[string]int64, error) {
	cond, args := keyFilter(keys, []any{scope})
	rows, err := r.d.query(ctx, "SELECT key, SUM(count) FROM stat_counts WHERE scope=?"+cond+" GROUP BY key", args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var n int64
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

func (r stats) AddRequests(ctx context.Context, rs []SubRequest) error {
	if len(rs) == 0 {
		return nil
	}
	return r.d.Tx(ctx, func(s Store) error {
		x := s.(*DB)
		for _, q := range rs {
			at := ms(q.At)
			if q.LinkID != "" {
				if q.ID == "" {
					q.ID = NewID()
				}
				if _, err := x.exec(ctx, "INSERT INTO sub_requests ("+requestCols+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
					q.ID, q.LinkID, q.OwnerID, at, q.IP, q.UA, q.Client, q.Status, q.Bytes, q.Kind, q.Country, q.ASN, q.Org, q.ShortID); err != nil {
					return err
				}
			}
			ok, fail := 0, 0
			if q.Status < 400 {
				ok = 1
			} else {
				fail = 1
			}
			if _, err := x.exec(ctx, "INSERT INTO sub_ips (link_id, ip, owner_id, first_at, last_at, ok, fail, client, country, org) VALUES (?,?,?,?,?,?,?,?,?,?) "+
				"ON CONFLICT (link_id, ip) DO UPDATE SET last_at = excluded.last_at, ok = sub_ips.ok + excluded.ok, fail = sub_ips.fail + excluded.fail, client = excluded.client, country = excluded.country, org = excluded.org",
				q.LinkID, q.IP, q.OwnerID, at, at, ok, fail, q.Client, q.Country, q.Org); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r stats) Requests(ctx context.Context, owner, linkID string, p Page) ([]SubRequest, error) {
	// 按请求时间倒序；游标为上一页最后一条的「时间|ID」（同一毫秒内的 ID 不保证有序，不能单独用 ID 分页）
	at, id := int64(1<<62), "~"
	if a, b, ok := strings.Cut(p.After, "|"); ok {
		if n, err := strconv.ParseInt(a, 10, 64); err == nil {
			at, id = n, b
		}
	}
	rows, err := r.d.query(ctx, "SELECT "+requestCols+" FROM sub_requests WHERE owner_id=? AND link_id=? AND (at < ? OR (at = ? AND id < ?)) ORDER BY at DESC, id DESC LIMIT ?",
		owner, linkID, at, at, id, limitOf(p))
	if err != nil {
		return nil, err
	}
	return scanRequests(rows)
}

const requestCols = "id, link_id, owner_id, at, ip, ua, client, status, bytes, kind, country, asn, org, short_id"

func scanRequests(rows *sql.Rows) ([]SubRequest, error) {
	defer func() { _ = rows.Close() }()
	var out []SubRequest
	for rows.Next() {
		var q SubRequest
		var at int64
		if err := rows.Scan(&q.ID, &q.LinkID, &q.OwnerID, &at, &q.IP, &q.UA, &q.Client, &q.Status, &q.Bytes, &q.Kind, &q.Country, &q.ASN, &q.Org, &q.ShortID); err != nil {
			return nil, err
		}
		q.At = fromMS(at)
		out = append(out, q)
	}
	return out, rows.Err()
}

func (r stats) LastByShort(ctx context.Context, owner string) (map[string]time.Time, error) {
	rows, err := r.d.query(ctx, "SELECT short_id, MAX(at) FROM sub_requests WHERE owner_id=? AND short_id <> '' GROUP BY short_id", owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = fromMS(at)
	}
	return out, rows.Err()
}

func (r stats) Range(ctx context.Context, owner string, from, to time.Time, limit int) ([]SubRequest, bool, error) {
	// 取区间内最新的 limit+1 条（倒序），再翻转为正序；多出一条说明被截断
	rows, err := r.d.query(ctx, "SELECT "+requestCols+" FROM sub_requests WHERE owner_id=? AND at>=? AND at<? ORDER BY at DESC, id DESC LIMIT ?",
		owner, ms(from), ms(to), limit+1)
	if err != nil {
		return nil, false, err
	}
	out, err := scanRequests(rows)
	if err != nil {
		return nil, false, err
	}
	cut := len(out) > limit
	if cut {
		out = out[:limit]
	}
	slices.Reverse(out)
	return out, cut, nil
}

func (r stats) IPs(ctx context.Context, owner, linkID string) ([]SubIP, error) {
	rows, err := r.d.query(ctx, "SELECT link_id, ip, owner_id, first_at, last_at, ok, fail, client, country, org FROM sub_ips WHERE owner_id=? AND link_id=? ORDER BY last_at DESC LIMIT 1000",
		owner, linkID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []SubIP
	for rows.Next() {
		var x SubIP
		var f, l int64
		if err := rows.Scan(&x.LinkID, &x.IP, &x.OwnerID, &f, &l, &x.OK, &x.Fail, &x.Client, &x.Country, &x.Org); err != nil {
			return nil, err
		}
		x.First, x.Last = fromMS(f), fromMS(l)
		out = append(out, x)
	}
	return out, rows.Err()
}
