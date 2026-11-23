package store

import (
	"context"
	"testing"
	"time"
)

// 数据量统计（R43）：刚写入的小表也要报出实际行数。PostgreSQL 的估算来自 ANALYZE，新表的 reltuples 为 -1，
// 小表常为 0；界面隐藏 0 行的表，原来在 PostgreSQL 上刚注册的用户表会被当成空表（反馈 #115 的实测）。
func TestUsageCountsSmallTables(t *testing.T) {
	d := newDB(t)
	ctx := context.Background()
	if err := d.Users().Create(ctx, &User{ID: NewID(), Username: "usage-test", PasswordHash: "x", Role: "user", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	u, err := d.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tu := range u.Tables {
		if tu.Table == "users" {
			if tu.Rows != 1 || tu.Estimated {
				t.Fatalf("users 应精确计为 1 行：%+v（%s）", tu, u.Dialect)
			}
			return
		}
	}
	t.Fatal("缺少 users 表")
}
