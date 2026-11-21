package store

import (
	"context"
	"database/sql"
	"fmt"
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
	// PruneRequests 删除 before 之前的请求明细，返回删除的行数。按批删除，每批一个短事务，
	// 不长时间锁表；按天、按链接与按 IP 的汇总（stat_counts、sub_ips）不受影响（需求 R14c）。
	PruneRequests(ctx context.Context, before time.Time) (int64, error)
	// EachRequest 按时间正序逐条回调某个用户在 [from, to) 内、满足 f 的请求（订阅分析，反馈 #74、需求 R14c）。
	// 只填分析用到的列（不含 UA 与字节数），整行来自覆盖索引 sub_requests_analytics，不回表；
	// 没有条数上限，内存由调用方的汇总决定。回调拿到的指针在下一次回调时被复用；fn 返回错误时停止并原样返回。
	EachRequest(ctx context.Context, owner string, from, to time.Time, f RequestFilter, fn func(*SubRequest) error) error
	// LatestRequests 返回某个用户在 [from, to) 内、满足 f 的最新 n 条请求（新的在前，完整列）。
	LatestRequests(ctx context.Context, owner string, from, to time.Time, f RequestFilter, n int) ([]SubRequest, error)
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
	// 对指向本人链接的每个短链接各取一次最大时间（一次索引下探），而不是把全部经短链接的请求分组：
	// 后者在千万行上要数秒（PostgreSQL 17 没有跳跃扫描，见 docs/development/benchmarks.md）。
	// 短链接可能属于别人（授权给好友的），所以经 links 按链接所有者找。short_id <> '' 让部分索引 sub_requests_short 可用。
	rows, err := r.d.query(ctx, "SELECT s.id, (SELECT MAX(q.at) FROM sub_requests q WHERE q.owner_id=? AND q.short_id=s.id AND q.short_id <> '') "+
		"FROM short_links s JOIN links l ON l.id = s.link_id WHERE l.owner_id=?", owner, owner)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at sql.NullInt64 // 从未被请求过的短链接为 NULL，不放进结果（与原来的分组写法一致）
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		if at.Valid {
			out[id] = fromMS(at.Int64)
		}
	}
	return out, rows.Err()
}

// pruneBatch 是每批删除的行数：足够大以减少往返，又不至于让单个事务持锁过久。
const pruneBatch = 10_000

func (r stats) PruneRequests(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for {
		// 子查询按 at 取一批 ID（走 sub_requests_at 索引），两种数据库都支持这种写法
		res, err := r.d.exec(ctx, "DELETE FROM sub_requests WHERE id IN (SELECT id FROM sub_requests WHERE at < ? ORDER BY at LIMIT ?)", ms(before), pruneBatch)
		if err != nil {
			return total, fmt.Errorf("store: 清理订阅请求明细: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < pruneBatch || ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}

// RequestFilter 限定订阅分析的范围；零值表示不过滤。
type RequestFilter struct {
	LinkID  string
	ShortID string
	// Shorts 非 nil 时只取经由这些短链接的请求（某位好友，反馈 #75）；空切片表示没有匹配。
	Shorts []string
}

// where 生成 owner、时间区间与 f 的条件，只由常量片段与占位符拼成。
// 第二个返回值为 false 表示必然没有匹配，不必查询。
func (f RequestFilter) where(owner string, from, to time.Time) (string, []any, bool) {
	q := "owner_id=? AND at>=? AND at<?"
	args := []any{owner, ms(from), ms(to)}
	if f.LinkID != "" {
		q += " AND link_id=?"
		args = append(args, f.LinkID)
	}
	if f.ShortID != "" {
		q += " AND short_id=?"
		args = append(args, f.ShortID)
	}
	if f.Shorts != nil {
		if len(f.Shorts) == 0 {
			return "", nil, false
		}
		q += " AND short_id IN (" + strings.Repeat(",?", len(f.Shorts))[1:] + ")"
		for _, id := range f.Shorts {
			args = append(args, id)
		}
	}
	return q, args, true
}

func (r stats) EachRequest(ctx context.Context, owner string, from, to time.Time, f RequestFilter, fn func(*SubRequest) error) error {
	cond, args, ok := f.where(owner, from, to)
	if !ok {
		return nil
	}
	// 列与 ORDER BY 都与 sub_requests_analytics 一致：按索引顺序读出，不排序、不回表
	rows, err := r.d.query(ctx, "SELECT at, id, status, ip, country, asn, org, client, kind, link_id, short_id FROM sub_requests WHERE "+cond+" ORDER BY at, id", args...)
	if err != nil {
		return fmt.Errorf("store: 读取订阅请求: %w", err)
	}
	defer func() { _ = rows.Close() }()
	q := SubRequest{OwnerID: owner}
	for rows.Next() {
		var at int64
		if err := rows.Scan(&at, &q.ID, &q.Status, &q.IP, &q.Country, &q.ASN, &q.Org, &q.Client, &q.Kind, &q.LinkID, &q.ShortID); err != nil {
			return fmt.Errorf("store: 读取订阅请求: %w", err)
		}
		q.At = fromMS(at)
		if err := fn(&q); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r stats) LatestRequests(ctx context.Context, owner string, from, to time.Time, f RequestFilter, n int) ([]SubRequest, error) {
	cond, args, ok := f.where(owner, from, to)
	if !ok {
		return nil, nil
	}
	rows, err := r.d.query(ctx, "SELECT "+requestCols+" FROM sub_requests WHERE "+cond+" ORDER BY at DESC, id DESC LIMIT ?", append(args, n)...)
	if err != nil {
		return nil, err
	}
	return scanRequests(rows)
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
