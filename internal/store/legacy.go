package store

import (
	"context"
	"fmt"
)

// encryptLegacy 把迁移 0010 之前以明文保存的节点相关数据加密，并清除明文（反馈 #77）。
// 每次启动在迁移之后执行；没有待处理的行时只是一次查询。在一个事务中完成，失败时整体回滚。
func (d *DB) encryptLegacy(ctx context.Context) (int, error) {
	type job struct {
		table, pk, col, enc, empty, aad string
	}
	jobs := []job{
		{"snapshots", "key", "headers", "headers_enc", "{}", "headers"},
		{"renders", "profile_id", "good_headers", "good_headers_enc", "{}", "good_headers"},
		{"renders", "profile_id", "last_diags", "last_diags_enc", "[]", "last_diags"},
		{"renders", "profile_id", "last_error", "last_error_enc", "", "last_error"},
	}
	n := 0
	err := d.Tx(ctx, func(st Store) error {
		x := st.(*DB)
		for _, j := range jobs {
			rows, err := x.query(ctx, "SELECT "+j.pk+", "+j.col+" FROM "+j.table+" WHERE "+j.enc+" IS NULL AND "+j.col+" <> ?", j.empty) //nolint:gosec // 表名与列名来自上面的常量
			if err != nil {
				return err
			}
			type item struct{ id, val string }
			var items []item
			for rows.Next() {
				var it item
				if err := rows.Scan(&it.id, &it.val); err != nil {
					_ = rows.Close()
					return err
				}
				items = append(items, it)
			}
			_ = rows.Close()
			for _, it := range items {
				blob, err := x.sealStr(it.val, j.table, j.aad, it.id)
				if err != nil {
					return err
				}
				if _, err := x.exec(ctx, "UPDATE "+j.table+" SET "+j.enc+"=?, "+j.col+"=? WHERE "+j.pk+"=?", blob, j.empty, it.id); err != nil { //nolint:gosec // 同上
					return fmt.Errorf("加密 %s.%s %s: %w", j.table, j.col, it.id, err)
				}
				n++
			}
		}
		return nil
	})
	return n, err
}
