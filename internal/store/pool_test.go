package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/store/storetest"
)

// R14a：PostgreSQL 连接带默认语句超时；DSN 中写了的以 DSN 为准；迁移的事务不受限制。
func TestPoolStatementTimeout(t *testing.T) {
	if os.Getenv(storetest.Env) == "" {
		t.Skip("只在 PostgreSQL 上运行（设置 " + storetest.Env + "）")
	}
	ctx := context.Background()
	dir := t.TempDir()
	k, _, err := LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	dsn := storetest.DSN(t, dir)
	show := func(q querier) string {
		t.Helper()
		var v string
		if err := q.QueryRowContext(ctx, "SHOW statement_timeout").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	d, err := OpenPool(ctx, dsn, k, Pool{StatementTimeout: 7 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if v := show(d.q); v != "7s" {
		t.Fatalf("语句超时应为 7s，得到 %s", v)
	}
	if st := d.root.Stats(); st.MaxOpenConnections != 20 {
		t.Fatalf("默认连接上限应为 20，得到 %d", st.MaxOpenConnections)
	}
	// 迁移使用的事务：SET LOCAL 解除限制，事务结束后恢复
	if err := d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.q.ExecContext(ctx, "SET LOCAL statement_timeout = 0"); err != nil {
			return err
		}
		if v := show(tx.q); v != "0" {
			t.Errorf("事务内应为 0，得到 %s", v)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if v := show(d.q); v != "7s" {
		t.Fatalf("事务结束后应恢复为 7s，得到 %s", v)
	}
	// 超时确实生效：pg_sleep 超过上限被取消
	d2, err := OpenPool(ctx, dsn, k, Pool{StatementTimeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d2.Close() }()
	if _, err := d2.q.ExecContext(ctx, "SELECT pg_sleep(2)"); err == nil || !strings.Contains(err.Error(), "statement timeout") {
		t.Fatalf("超过语句超时应被取消，得到 %v", err)
	}
	// DSN 中的设置优先
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	d3, err := OpenPool(ctx, dsn+sep+"statement_timeout=0", k, Pool{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d3.Close() }()
	if v := show(d3.q); v != "0" {
		t.Fatalf("DSN 中的 statement_timeout 应优先，得到 %s", v)
	}
}
