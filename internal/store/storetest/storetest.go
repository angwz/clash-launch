// Package storetest 为测试提供数据库地址：默认是临时目录中的 SQLite 文件；
// 设置 CLASH_LAUNCH_TEST_POSTGRES（一个可以建库的 PostgreSQL 地址，如
// postgres://postgres:pw@localhost:5432/postgres?sslmode=disable）时，为每个测试新建一个空库，
// 测试结束后删除。这样同一套存储与 API 测试在两种数据库上都运行，迁移与 SQL 方言差异由 CI 覆盖。
package storetest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL 驱动（建库与删库用）
)

// Env 是 PostgreSQL 测试地址的环境变量名。
const Env = "CLASH_LAUNCH_TEST_POSTGRES"

// DSN 返回本测试使用的数据库地址。dir 是测试的临时目录（SQLite 文件放在这里）。
func DSN(t testing.TB, dir string) string {
	t.Helper()
	base := os.Getenv(Env)
	if base == "" {
		return "sqlite:" + filepath.Join(dir, "t.db")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("%s 无效: %v", Env, err)
	}
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("连接 PostgreSQL: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "cl_test_" + hex.EncodeToString(b) // 只含小写字母、数字与下划线，可直接用作标识符
	if _, err := admin.ExecContext(context.Background(), "CREATE DATABASE "+name); err != nil {
		_ = admin.Close()
		t.Fatalf("建测试库: %v", err)
	}
	t.Cleanup(func() {
		// 测试中的连接在此之前已由各自的 Cleanup 关闭（Cleanup 后进先出）；WITH (FORCE) 兜底断开残留连接
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("删除测试库 %s: %v", name, err)
		}
		_ = admin.Close()
	})
	u.Path = "/" + name
	return u.String()
}
