package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
func (d *DB) Migrate(ctx context.Context, backupDir string) ([]int, error) {
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
	for _, f := range files {
		v, err := versionOf(f)
		if err != nil {
			return nil, err
		}
		if !applied[v] {
			pending = append(pending, f)
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
var Tables = []string{"users", "sessions", "sources", "rule_sources", "snapshots", "profiles", "revisions", "links", "renders", "audit", "settings", "invites"}

// CopyStats 是一张表的复制结果。
type CopyStats struct {
	Table    string
	Rows     int
	Checksum string
}

// Copy 把 src 中的全部业务数据按表流式复制到 dst（dst 必须已迁移且为空），并校验行数与校验和。
// 用于从 SQLite 迁到 PostgreSQL（clash-launch db copy）。密文原样复制：两边使用同一把主密钥。
func Copy(ctx context.Context, src, dst *DB) ([]CopyStats, error) {
	var stats []CopyStats
	for _, t := range Tables {
		var n int
		if err := dst.root.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil {
			return stats, err
		}
		if n > 0 {
			return stats, fmt.Errorf("store: 目标表 %s 不为空", t)
		}
	}
	err := dst.Tx(ctx, func(s Store) error {
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
		sum, rows, err := tableChecksum(ctx, dst, st.Table)
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

func copyTable(ctx context.Context, src, dst *DB, table string) (CopyStats, error) {
	rows, err := src.root.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY 1") //nolint:gosec // 表名来自常量列表 Tables
	if err != nil {
		return CopyStats{}, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return CopyStats{}, err
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	insert := "INSERT INTO " + table + " (" + strings.Join(cols, ",") + ") VALUES (" + ph + ")"
	h := sha256.New()
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return CopyStats{}, err
		}
		normalize(vals)
		hashRow(h, vals)
		if _, err := dst.exec(ctx, insert, vals...); err != nil {
			return CopyStats{}, err
		}
		n++
	}
	return CopyStats{Table: table, Rows: n, Checksum: hex.EncodeToString(h.Sum(nil))}, rows.Err()
}

func tableChecksum(ctx context.Context, d *DB, table string) (string, int, error) {
	rows, err := d.root.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY 1") //nolint:gosec // 同上
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = rows.Close() }()
	cols, _ := rows.Columns()
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
