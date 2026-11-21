package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/migrations"
)

// Migrate 执行尚未执行的迁移（只向前）。SQLite 下，若有待执行的迁移且数据库已有数据，
// 先把数据库备份到 backupDir（升级前自动备份，第 18.3 节）。返回执行的迁移版本。
// ErrNewerSchema 表示数据库由更新的程序版本迁移过，当前程序不能安全使用它。
var ErrNewerSchema = errors.New("store: 数据库版本比程序新")

func (d *DB) Migrate(ctx context.Context, backupDir string) ([]int, error) {
	done, err := d.migrateTo(ctx, backupDir, 0)
	if err != nil {
		return done, err
	}
	// 迁移 0010 之前的明文在这里加密（反馈 #77）；之后的启动没有待处理的行
	if _, err := d.encryptLegacy(ctx); err != nil {
		return done, fmt.Errorf("store: 加密旧数据：%w", err)
	}
	return done, nil
}

// migrateTo 执行到版本 upto 为止的迁移（0 表示全部）。只供测试模拟「旧版本建的数据库」。
func (d *DB) migrateTo(ctx context.Context, backupDir string, upto int) ([]int, error) {
	if _, err := d.root.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT NOT NULL PRIMARY KEY, applied_at BIGINT NOT NULL)"); err != nil {
		return nil, err
	}
	applied := map[int]bool{}
	rows, err := d.root.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			_ = rows.Close()
			return nil, err
		}
		applied[v] = true
	}
	_ = rows.Close()
	files, err := fs.Glob(migrations.FS, d.dialect+"/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var pending []string
	known := map[int]bool{}
	for _, f := range files {
		v, err := versionOf(f)
		if err != nil {
			return nil, err
		}
		known[v] = true
		if !applied[v] && (upto == 0 || v <= upto) {
			pending = append(pending, f)
		}
	}
	// 防止降级：数据库里有本程序不认识的迁移，说明它被更新的版本升级过。旧程序在新表结构上运行
	// 可能写坏数据，所以拒绝启动，提示换回新版本或恢复迁移前的自动备份。
	for v := range applied {
		if !known[v] {
			return nil, fmt.Errorf("%w：数据库已被更新的版本升级（迁移 %d），当前程序较旧。请使用新版本镜像，或从 backups 目录恢复升级前的备份", ErrNewerSchema, v)
		}
	}
	if len(pending) == 0 {
		return nil, nil
	}
	if len(applied) > 0 && d.dialect == "sqlite" && backupDir != "" {
		name := filepath.Join(backupDir, "before-migrate-"+time.Now().UTC().Format("20060102T150405Z")+".db")
		if err := d.Backup(ctx, name); err != nil {
			return nil, fmt.Errorf("store: 迁移前备份失败: %w", err)
		}
	}
	var done []int
	for _, f := range pending {
		v, _ := versionOf(f)
		body, err := fs.ReadFile(migrations.FS, f)
		if err != nil {
			return nil, err
		}
		err = d.Tx(ctx, func(s Store) error {
			tx := s.(*DB)
			if tx.dialect == "postgres" {
				// 迁移可能搬移大表，不受连接上的语句超时限制；SET LOCAL 只作用于这个事务
				if _, err := tx.q.ExecContext(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
			}
			for _, stmt := range splitSQL(string(body)) {
				if _, err := tx.q.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
			}
			_, err := tx.exec(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", v, ms(time.Now()))
			return err
		})
		if err != nil {
			return done, err
		}
		done = append(done, v)
	}
	return done, nil
}

func versionOf(f string) (int, error) {
	base := filepath.Base(f)
	n, _, ok := strings.Cut(base, "_")
	v, err := strconv.Atoi(n)
	if !ok || err != nil {
		return 0, fmt.Errorf("store: 迁移文件名无效：%s", f)
	}
	return v, nil
}

// splitSQL 按语句结尾的分号拆分；迁移脚本中不使用含分号的字符串字面量。
func splitSQL(s string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "--") || t == "" {
			continue
		}
		cur.WriteString(line + "\n")
		if strings.HasSuffix(t, ";") {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

// Backup 生成一致的数据库快照（SQLite：VACUUM INTO，在线进行）。PostgreSQL 请使用 pg_dump。
func (d *DB) Backup(ctx context.Context, path string) error {
	if d.dialect != "sqlite" {
		return fmt.Errorf("store: PostgreSQL 请使用其自带的备份工具（pg_dump）")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("store: 备份文件已存在：%s", path)
	}
	_, err := d.root.ExecContext(ctx, "VACUUM INTO ?", path)
	return err
}

// Tables 是按依赖顺序排列的业务表（db copy 按此顺序复制）。
var Tables = []string{"users", "sessions", "sources", "rule_sources", "snapshots", "profiles", "revisions", "links", "short_links", "renders", "audit", "settings", "invites", "user_questions", "user_totp", "recovery_codes", "passkeys", "user_smtp", "user_quotas", "known_devices", "templates", "stat_counts", "sub_requests", "sub_ips", "messages", "contacts", "grants", "username_history", "invite_uses", "access_tokens"}

// CopyStats 是一张表的复制结果。
type CopyStats struct {
	Table    string
	Rows     int
	Checksum string
	// Skipped 是源库中跳过的已废弃列（见 RetiredColumns）。
	Skipped []string
}

// RetiredColumns 是已经不再使用、但可能还留在旧数据库里的列（表.列 → 原因）。
//
// 迁移只向前，已执行过的迁移不得修改（TestMigrationsFrozen 锁定）。但在这条规则建立之前，
// 提交 4219e64 曾直接从 0001 中删掉 sources.allow_direct（出站代理随决定 0003 取消），
// 因此在那之前建的数据库多这一列。程序读写都写明列名，不受影响；db copy 复制时按列名对应，
// 只有登记在这里的列可以跳过，其他对不上的列一律报错，不静默丢弃数据。
var RetiredColumns = map[string]string{
	"sources.allow_direct": "出站代理已取消（决定 0003）；该列只在 4219e64 之前建的数据库中存在",
}

// ErrSchemaMismatch 表示源库与目标库的迁移版本或表结构对不上。
var ErrSchemaMismatch = errors.New("store: 源库与目标库的表结构不一致")

// Copy 把 src 中的全部业务数据按表流式复制到 dst（dst 必须已迁移且为空），并校验行数与校验和。
// 用于在数据库之间搬迁（SQLite → PostgreSQL、SQLite → SQLite 等，clash-launch db copy）。
// 两边必须处于同一迁移版本：源库较旧时先用本版本程序执行 db migrate（会先自动备份）。
// 按列名对应复制；密文原样复制，两边使用同一把主密钥。
func Copy(ctx context.Context, src, dst *DB) ([]CopyStats, error) {
	sv, err := appliedVersions(ctx, src)
	if err != nil {
		return nil, err
	}
	dv, err := appliedVersions(ctx, dst)
	if err != nil {
		return nil, err
	}
	if sv != dv {
		return nil, fmt.Errorf("%w：源库迁移到 %s，目标库迁移到 %s。请先用同一版本的程序对两边执行 db migrate", ErrSchemaMismatch, sv, dv)
	}
	var stats []CopyStats
	for _, t := range Tables {
		var n int
		if err := dst.root.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil { // 表名来自常量列表 Tables
			return stats, err
		}
		if n > 0 {
			return stats, fmt.Errorf("store: 目标表 %s 不为空", t)
		}
	}
	err = dst.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		for _, t := range Tables {
			st, err := copyTable(ctx, src, tx, t)
			if err != nil {
				return fmt.Errorf("%s: %w", t, err)
			}
			stats = append(stats, st)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, st := range stats {
		cols, err := columnsOf(ctx, dst, st.Table)
		if err != nil {
			return stats, err
		}
		sum, rows, err := tableChecksum(ctx, dst, st.Table, cols)
		if err != nil {
			return stats, err
		}
		if sum != st.Checksum || rows != st.Rows {
			return stats, fmt.Errorf("store: %s 复制后校验不一致（行数 %d/%d）", st.Table, rows, st.Rows)
		}
		stats[i].Checksum = sum
	}
	return stats, nil
}

// appliedVersions 返回已执行的迁移版本（逗号分隔，升序），用于比较两个库是否同一版本。
func appliedVersions(ctx context.Context, d *DB) (string, error) {
	rows, err := d.root.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return "", fmt.Errorf("store: 读取迁移版本（数据库是否已初始化？）：%w", err)
	}
	defer func() { _ = rows.Close() }()
	var vs []string
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		vs = append(vs, strconv.Itoa(v))
	}
	if len(vs) == 0 {
		return "（无）", rows.Err()
	}
	return strings.Join(vs, ","), rows.Err()
}

// columnsOf 返回表的列名（按表定义顺序）。走 d.q：在事务中时用事务自己的连接
// （SQLite 只有一个连接，绕开事务另取连接会一直等待）。
func columnsOf(ctx context.Context, d *DB, table string) ([]string, error) {
	rows, err := d.q.QueryContext(ctx, "SELECT * FROM "+table+" WHERE 1=0") // 表名来自常量列表 Tables
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return rows.Columns()
}

func copyTable(ctx context.Context, src, dst *DB, table string) (CopyStats, error) {
	scols, err := columnsOf(ctx, src, table)
	if err != nil {
		return CopyStats{}, err
	}
	dcols, err := columnsOf(ctx, dst, table)
	if err != nil {
		return CopyStats{}, err
	}
	// 按列名对应：目标库的每一列源库都必须有；源库多出的列只允许是登记过的废弃列
	have := map[string]bool{}
	for _, c := range scols {
		have[c] = true
	}
	want := map[string]bool{}
	for _, c := range dcols {
		want[c] = true
		if !have[c] {
			return CopyStats{}, fmt.Errorf("%w：源库缺少列 %s.%s", ErrSchemaMismatch, table, c)
		}
	}
	st := CopyStats{Table: table}
	for _, c := range scols {
		if want[c] {
			continue
		}
		if _, ok := RetiredColumns[table+"."+c]; !ok {
			return CopyStats{}, fmt.Errorf("%w：源库多出列 %s.%s，且不是已登记的废弃列", ErrSchemaMismatch, table, c)
		}
		st.Skipped = append(st.Skipped, table+"."+c)
	}
	list := strings.Join(dcols, ",")
	rows, err := src.root.QueryContext(ctx, "SELECT "+list+" FROM "+table+" ORDER BY 1") //nolint:gosec // 表名与列名来自数据库自身的表定义
	if err != nil {
		return CopyStats{}, err
	}
	defer func() { _ = rows.Close() }()
	ph := strings.TrimSuffix(strings.Repeat("?,", len(dcols)), ",")
	insert := "INSERT INTO " + table + " (" + list + ") VALUES (" + ph + ")"
	h := sha256.New()
	for rows.Next() {
		vals := make([]any, len(dcols))
		ptrs := make([]any, len(dcols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return st, err
		}
		normalize(vals)
		hashRow(h, vals)
		if _, err := dst.exec(ctx, insert, vals...); err != nil {
			return st, err
		}
		st.Rows++
	}
	st.Checksum = hex.EncodeToString(h.Sum(nil))
	return st, rows.Err()
}

func tableChecksum(ctx context.Context, d *DB, table string, cols []string) (string, int, error) {
	rows, err := d.root.QueryContext(ctx, "SELECT "+strings.Join(cols, ",")+" FROM "+table+" ORDER BY 1") //nolint:gosec // 同上
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = rows.Close() }()
	h := sha256.New()
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", 0, err
		}
		normalize(vals)
		hashRow(h, vals)
		n++
	}
	return hex.EncodeToString(h.Sum(nil)), n, rows.Err()
}

// normalize 统一两种驱动返回的类型（整数为 int64，文本为 string，二进制为 []byte）。
func normalize(vals []any) {
	for i, v := range vals {
		switch x := v.(type) {
		case int32:
			vals[i] = int64(x)
		case int:
			vals[i] = int64(x)
		}
	}
}

func hashRow(h interface{ Write([]byte) (int, error) }, vals []any) {
	for _, v := range vals {
		switch x := v.(type) {
		case nil:
			_, _ = h.Write([]byte("N|"))
		case []byte:
			_, _ = fmt.Fprintf(h, "B%d:%x|", len(x), x)
		default:
			_, _ = fmt.Fprintf(h, "%T:%v|", x, x)
		}
	}
	_, _ = h.Write([]byte("\n"))
}
