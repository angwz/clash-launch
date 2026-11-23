package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// 决定 0010：默认使用 PostgreSQL。没有设置 DATABASE_URL 时不再自动建 SQLite 库；
// 数据目录里已有旧的 SQLite 库时继续使用并提示迁移；显式设置的地址照常使用。
func TestDatabaseDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLASH_LAUNCH_DATA_DIR", dir)
	t.Setenv("CLASH_LAUNCH_DATABASE_URL", "")
	t.Setenv("CLASH_LAUNCH_CONFIG", "")
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 没有配置、也没有旧库：读配置不报错（目录权限检查要先完成），打开数据库时报错并说明怎么设置
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.DatabaseURL != "" || c.LegacySQLite {
		t.Fatalf("不应自动选择 SQLite：%q", c.DatabaseURL)
	}
	if _, err := openStore(context.Background(), c, quiet); !errors.Is(err, errNoDatabase) {
		t.Fatalf("应报告没有配置数据库：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacySQLiteFile)); !os.IsNotExist(err) {
		t.Fatal("不应创建 SQLite 文件")
	}

	// 已有旧版默认位置的 SQLite 库：继续使用
	if err := os.WriteFile(filepath.Join(dir, legacySQLiteFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !c.LegacySQLite || c.DatabaseURL != "sqlite://"+filepath.ToSlash(filepath.Join(dir, legacySQLiteFile)) {
		t.Fatalf("应继续使用旧库：%+v", c.DatabaseURL)
	}

	// 显式设置：照常使用，不当作旧库
	t.Setenv("CLASH_LAUNCH_DATABASE_URL", "postgres://clash:example-pw@db.example.com:5432/clash")
	c, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.LegacySQLite || c.DatabaseURL != "postgres://clash:example-pw@db.example.com:5432/clash" {
		t.Fatalf("显式设置：%+v", c.DatabaseURL)
	}
}
