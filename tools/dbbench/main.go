// dbbench 在 PostgreSQL 上造千万级数据，测量存储层热点查询的执行计划与耗时（需求 R14b）。
//
// 用法：
//
//	go run ./tools/dbbench -dsn postgres://postgres:pw@127.0.0.1:55432/postgres?sslmode=disable [-requests 10000000] [-keep]
//	go run ./tools/dbbench -dsn … -db cl_bench_<时间>   # 复用 -keep 保留的库：只执行新迁移并重新测量，不再灌数据
//
// 每次运行新建一个空库（cl_bench_<时间>），执行全部迁移后用 generate_series 灌数据，
// 再对每条热点查询运行 3 次 EXPLAIN (ANALYZE, BUFFERS)，取执行时间的中位数，输出 Markdown 表格。
// 查询语句与 internal/store 中的写法保持一致（占位符换成 $n）；store 改了 SQL 时同步这里。
//
// 数据只用文档地址（192.0.2.0/24）与明显虚构的名称；不读写任何真实数据。
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/angwz/clash-launch/internal/store"
)

type query struct {
	name string
	sql  string
	args []any
	note string // 对应 internal/store 中的方法
}

func main() {
	dsn := flag.String("dsn", "", "可以建库的 PostgreSQL 地址")
	requests := flag.Int("requests", 10_000_000, "订阅请求明细（sub_requests）的行数")
	keep := flag.Bool("keep", false, "结束后保留测试库")
	reuse := flag.String("db", "", "复用已灌好数据的库（-keep 保留的），跳过建库与灌数据")
	flag.Parse()
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "需要 -dsn")
		os.Exit(2)
	}
	if err := run(*dsn, *requests, *keep, *reuse); err != nil {
		fmt.Fprintln(os.Stderr, "dbbench:", err)
		os.Exit(1)
	}
}

func run(base string, requests int, keep bool, reuse string) error {
	ctx := context.Background()
	admin, err := sql.Open("pgx", base)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	name := reuse
	if name == "" {
		name = "cl_bench_" + time.Now().Format("20060102150405")
		if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
			return fmt.Errorf("建库: %w", err)
		}
	} else if !benchDB.MatchString(name) {
		return fmt.Errorf("-db 只接受本工具建的库（cl_bench_<时间>），得到 %q", name)
	}
	if !keep && reuse == "" {
		defer func() { _, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") }()
	}
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	u.Path = "/" + name
	dsn := u.String()

	// 迁移：与服务启动时相同的路径
	dir, err := os.MkdirTemp("", "dbbench-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	keys, _, err := store.LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		return err
	}
	st, err := store.OpenPool(ctx, dsn, keys, store.Pool{StatementTimeout: time.Hour})
	if err != nil {
		return err
	}
	if _, err := st.Migrate(ctx, ""); err != nil {
		return fmt.Errorf("迁移: %w", err)
	}
	_ = st.Close()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UnixMilli()
	if reuse == "" {
		if err := seed(ctx, db, requests, now); err != nil {
			return err
		}
	} else if _, err := db.ExecContext(ctx, "ANALYZE"); err != nil { // 新迁移建的索引需要最新的统计信息
		return err
	}

	var ver string
	_ = db.QueryRowContext(ctx, "SHOW server_version").Scan(&ver)
	fmt.Printf("# 存储层基准（PostgreSQL %s）\n\n", ver)
	if err := printCounts(ctx, db); err != nil {
		return err
	}
	day := int64(86_400_000)
	qs := []query{
		{"订阅分析：重度用户近 30 天明细", "SELECT id, link_id, owner_id, at, ip, ua, client, status, bytes, kind, country, asn, org, short_id FROM sub_requests WHERE owner_id=$1 AND at>=$2 AND at<$3 ORDER BY at DESC, id DESC LIMIT $4",
			[]any{"u1", now - 30*day, now + 1, 50_000}, "Stats.RequestsBetween（analytics.go 读进内存聚合，上限 5 万行）"},
		{"订阅分析：普通用户近 30 天明细", "SELECT id, link_id, owner_id, at, ip, ua, client, status, bytes, kind, country, asn, org, short_id FROM sub_requests WHERE owner_id=$1 AND at>=$2 AND at<$3 ORDER BY at DESC, id DESC LIMIT $4",
			[]any{"u5000", now - 30*day, now + 1, 50_000}, "同上"},
		{"链接请求记录第一页", "SELECT id, link_id, owner_id, at, ip, ua, client, status, bytes, kind, country, asn, org, short_id FROM sub_requests WHERE owner_id=$1 AND link_id=$2 AND (at < $3 OR (at = $3 AND id < $4)) ORDER BY at DESC, id DESC LIMIT $5",
			[]any{"u1", "l1", now + 1, "~", 50}, "Stats.Requests（键集分页）"},
		{"短链接最近访问（50 个短链接）", "SELECT s.id, (SELECT MAX(q.at) FROM sub_requests q WHERE q.owner_id=$1 AND q.short_id=s.id AND q.short_id <> '') FROM (SELECT 's'||g AS id FROM generate_series(0, 49) g) s",
			[]any{"u1"}, "Stats.LastByShort（逐个短链接取最大时间；库里没有 short_links，用 50 个编号代替）"},
		{"清理一批过期明细（只计划，不执行）", "EXPLAIN DELETE FROM sub_requests WHERE id IN (SELECT id FROM sub_requests WHERE at < $1 ORDER BY at LIMIT 10000)", []any{now - 170*day}, "Stats.PruneRequests（每批 1 万行）"},
		{"审计：个人总数（数到上限）", "SELECT COUNT(*) FROM (SELECT 1 FROM audit WHERE owner_id=$1 LIMIT $2) t", []any{"u1", 10_001}, "Audit.Paged 的计数（上限 1 万）"},
		{"审计：个人第 1 页", "SELECT id, owner_id, at, action, detail FROM audit WHERE owner_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3", []any{"u1", 10, 0}, "Audit.Page"},
		{"审计：个人第 1000 页", "SELECT id, owner_id, at, action, detail FROM audit WHERE owner_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3", []any{"u1", 10, 9_990}, "Audit.Page（OFFSET）"},
		{"审计：全站总数（数到上限）", "SELECT COUNT(*) FROM (SELECT 1 FROM audit LIMIT $1) t", []any{50_001}, "Audit.PagedAll 的计数（上限 5 万）"},
		{"审计：全站最后可达的第 1000 页", "SELECT id, owner_id, at, action, detail FROM audit ORDER BY id DESC LIMIT $1 OFFSET $2", []any{50, 49_950}, "Audit.PagedAll（OFFSET 上限 49950）"},
		{"未读消息数", "SELECT COUNT(*) FROM messages m WHERE recipient_id=$1 AND recipient_hidden=0 AND read_at=0 AND NOT EXISTS (SELECT 1 FROM contacts c WHERE c.user_id=m.recipient_id AND c.peer_id=m.sender_id AND c.deleted_at <> 0)",
			[]any{"u1"}, "Messages.Unread"},
		{"与某人的未读数", "SELECT COUNT(*) FROM messages WHERE recipient_id=$1 AND sender_id=$2 AND recipient_hidden=0 AND read_at=0", []any{"u1", "u2"}, "Messages.UnreadFrom"},
		{"会话第一页", "SELECT id FROM (SELECT * FROM (SELECT id, created_at FROM messages WHERE sender_id=$1 AND recipient_id=$2 AND sender_hidden=0 AND (created_at < $3 OR (created_at = $3 AND id < $4)) ORDER BY created_at DESC, id DESC LIMIT $5) " +
			"UNION ALL SELECT * FROM (SELECT id, created_at FROM messages WHERE sender_id=$2 AND recipient_id=$1 AND recipient_hidden=0 AND (created_at < $3 OR (created_at = $3 AND id < $4)) ORDER BY created_at DESC, id DESC LIMIT $5)) t ORDER BY created_at DESC, id DESC LIMIT $5",
			[]any{"u1", "u2", now + 1, "~", 50}, "Messages.Thread（两个方向各取一页再合并）"},
		{"最近消息（会话列表预览）", "SELECT id FROM (SELECT * FROM (SELECT id, created_at FROM messages WHERE sender_id=$1 AND sender_hidden=0 ORDER BY created_at DESC, id DESC LIMIT $2) " +
			"UNION ALL SELECT * FROM (SELECT id, created_at FROM messages WHERE recipient_id=$1 AND recipient_hidden=0 ORDER BY created_at DESC, id DESC LIMIT $2)) t ORDER BY created_at DESC, id DESC LIMIT $2", []any{"u1", 20}, "Messages.Recent（同上）"},
		{"发送频率检查", "SELECT COUNT(*) FROM messages WHERE sender_id=$1 AND created_at>=$2", []any{"u1", now - 3_600_000}, "Messages.SentSince"},
		{"用户搜索：用户名前缀或昵称任一段", "SELECT id FROM users WHERE username_norm LIKE $1 ESCAPE '!' OR nickname_norm LIKE $2 ESCAPE '!' ORDER BY CASE WHEN username_norm LIKE $1 ESCAPE '!' THEN 0 ELSE 1 END, username_norm LIMIT $3",
			[]any{"user12%", "%a3f%", 10}, "Users.Search"},
		{"管理页用户列表第 1000 页（含总数）", "SELECT id FROM users WHERE username_norm LIKE $1 ESCAPE '!' ORDER BY username_norm LIMIT $2 OFFSET $3", []any{"%", 50, 49_950}, "Users.Page（OFFSET）"},
	}
	fmt.Println("\n| 查询 | 中位耗时 | 计划要点 | 对应方法 |")
	fmt.Println("| --- | ---: | --- | --- |")
	for _, q := range qs {
		ms, plan, err := measure(ctx, db, q)
		if err != nil {
			return fmt.Errorf("%s: %w", q.name, err)
		}
		t := fmt.Sprintf("%.1f ms", ms)
		if ms < 0 {
			t = "—"
		}
		fmt.Printf("| %s | %s | %s | %s |\n", q.name, t, plan, q.note)
	}
	if keep {
		fmt.Printf("\n测试库已保留：%s\n", name)
	}
	return nil
}

// seed 用 generate_series 灌数据。分布：u1 是重度用户（订阅明细的 20%、大量审计与消息），
// 其余均匀分布在 1 万个用户上；时间均匀落在最近 180 天。
func seed(ctx context.Context, db *sql.DB, requests int, now int64) error {
	steps := []struct{ name, sql string }{
		{"用户 10 万", `INSERT INTO users (id, username, username_norm, password_hash, role, created_at, updated_at, nickname_norm)
			SELECT 'u'||g, 'user'||g, 'user'||g, 'x', 'user', $1, $1, 'n'||substr(md5(g::text), 1, 8) FROM generate_series(1, 100000) g`},
		{fmt.Sprintf("订阅明细 %d", requests), `INSERT INTO sub_requests (id, link_id, owner_id, at, ip, ua, client, status, bytes, kind, country, asn, org, short_id)
			SELECT lpad(to_hex(g), 12, '0'),
			       CASE WHEN g % 5 = 0 THEN 'l'||(g % 3 + 1) ELSE 'l'||(g % 30000 + 10) END,
			       CASE WHEN g % 5 = 0 THEN 'u1' ELSE 'u'||(g % 10000 + 2) END,
			       $1 - (g::bigint * 1555) % 15552000000,
			       '192.0.2.'||(g % 254 + 1), 'mihomo/1.19.33', 'mihomo', CASE WHEN g % 97 = 0 THEN 304 ELSE 200 END, 40000,
			       'config', (ARRAY['CN','HK','JP','SG','US'])[g % 5 + 1], 'AS64500', 'Example Net',
			       CASE WHEN g % 5 = 0 AND g % 7 = 0 THEN 's'||(g % 50) ELSE '' END
			FROM generate_series(1, ` + strconv.Itoa(requests) + `) g`},
		{"审计 200 万", `INSERT INTO audit (id, owner_id, at, action, detail)
			SELECT lpad(to_hex(g), 12, '0'), CASE WHEN g % 10 = 0 THEN 'u1' ELSE 'u'||(g % 100000 + 1) END,
			       $1 - (2000000 - g)::bigint * 7000, 'session.login', '{}'
			FROM generate_series(1, 2000000) g`},
		{"消息 100 万", `INSERT INTO messages (id, sender_id, recipient_id, kind, body_enc, created_at, read_at)
			SELECT lpad(to_hex(g), 12, '0'),
			       CASE WHEN g % 10 = 0 THEN 'u2' WHEN g % 10 = 1 THEN 'u1' ELSE 'u'||(g % 99000 + 3) END,
			       CASE WHEN g % 10 = 0 THEN 'u1' WHEN g % 10 = 1 THEN 'u2' ELSE 'u'||((g * 7) % 99000 + 3) END,
			       'text', '\x00'::bytea, $1 - (1000000 - g)::bigint * 15000, CASE WHEN g % 3 = 0 THEN 0 ELSE $1 END
			FROM generate_series(1, 1000000) g`},
		{"联系人", `INSERT INTO contacts (user_id, peer_id, last_at) VALUES ('u1', 'u2', $1), ('u2', 'u1', $1)`},
		// VACUUM 建立可见性映射：生产环境由 autovacuum 完成；不做的话仅索引扫描会回表，计数类查询被高估
		{"VACUUM 与统计信息", `VACUUM ANALYZE`},
	}
	for _, s := range steps {
		start := time.Now()
		var err error
		if strings.Contains(s.sql, "$1") {
			_, err = db.ExecContext(ctx, s.sql, now)
		} else {
			_, err = db.ExecContext(ctx, s.sql)
		}
		if err != nil {
			return fmt.Errorf("灌数据（%s）: %w", s.name, err)
		}
		fmt.Fprintf(os.Stderr, "已灌入 %s（%s）\n", s.name, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

func printCounts(ctx context.Context, db *sql.DB) error {
	fmt.Println("| 表 | 行数 | 占用（含索引） |")
	fmt.Println("| --- | ---: | ---: |")
	for _, t := range []string{"sub_requests", "audit", "messages", "users"} {
		var n int64
		var size string
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), pg_size_pretty(pg_total_relation_size('"+t+"')) FROM "+t).Scan(&n, &size); err != nil {
			return err
		}
		fmt.Printf("| `%s` | %d | %s |\n", t, n, size)
	}
	return nil
}

var (
	benchDB  = regexp.MustCompile(`^cl_bench_[0-9]+$`)
	execTime = regexp.MustCompile(`Execution Time: ([0-9.]+) ms`)
	nodeLine = regexp.MustCompile(`^\s*(?:->\s*)?([A-Z][A-Za-z ]+?)(?: using (\S+))?(?: on (\S+))?\s+\(`)
)

// measure 运行 3 次 EXPLAIN (ANALYZE, BUFFERS)，返回执行时间的中位数与计划的主要节点。
// 语句本身以 EXPLAIN 开头时只取计划、不执行（用于删除类语句），耗时返回 -1。
func measure(ctx context.Context, db *sql.DB, q query) (float64, string, error) {
	planOnly := strings.HasPrefix(q.sql, "EXPLAIN ")
	stmt := "EXPLAIN (ANALYZE, BUFFERS) " + q.sql //nolint:gosec // 语句来自本文件中的常量列表
	if planOnly {
		stmt = q.sql
	}
	var times []float64
	var plan []string
	for i := 0; i < 3; i++ {
		rows, err := db.QueryContext(ctx, stmt, q.args...)
		if err != nil {
			return 0, "", err
		}
		plan = plan[:0]
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				_ = rows.Close()
				return 0, "", err
			}
			if m := execTime.FindStringSubmatch(line); m != nil {
				v, _ := strconv.ParseFloat(m[1], 64)
				times = append(times, v)
			}
			if m := nodeLine.FindStringSubmatch(line); m != nil {
				n := strings.TrimSpace(m[1])
				if m[2] != "" {
					n += " · " + m[2]
				}
				plan = append(plan, n)
			}
		}
		_ = rows.Close()
		if planOnly {
			break
		}
	}
	if len(plan) > 4 {
		plan = plan[:4]
	}
	desc := strings.Join(plan, " → ")
	if planOnly {
		return -1, desc, nil
	}
	sort.Float64s(times)
	return times[len(times)/2], desc, nil
}
