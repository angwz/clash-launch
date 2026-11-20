package store

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/migrations"
)

// 反馈 #77：节点相关的数据在数据库中没有明文。按正常流程写入带有标记的数据，
// 然后直接读取数据库里每一张表的每一列，确认标记都不出现。
func TestNoPlaintextAtRest(t *testing.T) {
	ctx := context.Background()
	d := newDB(t)
	u := &User{Username: "alice", PasswordHash: "h", Role: "user"}
	v := &User{Username: "bob", PasswordHash: "h", Role: "user"}
	for _, x := range []*User{u, v} {
		if err := d.Users().Create(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	marks := []string{"MARKurlTOKEN", "MARKnodePASS", "MARKheaderVAL", "MARKsnapNODE", "MARKuserinfo", "MARKdocNODE", "MARKdiagPREVIEW", "MARKrenderERR", "MARKyamlNODE", "MARKlinkTOKEN", "MARKmsgTEXT", "MARKtplDOC"}
	src := &Source{Name: "订阅", Kind: "subscription", URL: "https://example.com/sub?token=MARKurlTOKEN", Headers: map[string]string{"Authorization": "Bearer MARKheaderVAL"}, Content: []byte("ss://MARKnodePASS@192.0.2.1:8388")}
	if err := d.Sources().Create(ctx, u.ID, src); err != nil {
		t.Fatal(err)
	}
	if err := d.Snapshots().Put(ctx, u.ID, &Snapshot{Key: "src/" + src.ID, Content: []byte("MARKsnapNODE"), Hash: "h", Headers: map[string]string{"subscription-userinfo": "upload=1; MARKuserinfo"}, FetchedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p := &Profile{Name: "配置", Doc: []byte("sources: [MARKdocNODE]")}
	if err := d.Profiles().Create(ctx, u.ID, p, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Renders().Put(ctx, u.ID, &Render{ProfileID: p.ID, GoodYAML: []byte("proxies: [MARKyamlNODE]"), GoodHeaders: map[string]string{"subscription-userinfo": "MARKuserinfo"},
		LastError: "MARKrenderERR", LastDiags: []byte(`[{"code":"node.unrecognized","args":{"preview":"MARKdiagPREVIEW"}}]`), GoodAt: time.Now(), LastAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := d.Links().Create(ctx, u.ID, &Link{ProfileID: p.ID, Token: "MARKlinkTOKEN-0123456789", TokenHash: "th"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Messages().Create(ctx, &Message{SenderID: u.ID, RecipientID: v.ID, Kind: "text", Body: []byte(`{"text":"MARKmsgTEXT"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := d.Templates().Create(ctx, u.ID, &Template{Name: "模板", Doc: []byte("MARKtplDOC")}); err != nil {
		t.Fatal(err)
	}

	// 直接读数据库：每张表的每一列
	for _, table := range append(Tables, "schema_migrations") {
		rows, err := d.root.QueryContext(ctx, "SELECT * FROM "+table)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, v := range vals {
				s := fmt.Sprint(v)
				if b, ok := v.([]byte); ok {
					s = string(b)
				}
				for _, m := range marks {
					if strings.Contains(s, m) {
						t.Errorf("%s.%s 中有明文 %s", table, cols[i], m)
					}
				}
			}
		}
		_ = rows.Close()
	}

	// 读回来仍然完整
	snap, err := d.Snapshots().Get(ctx, u.ID, "src/"+src.ID)
	if err != nil || !strings.Contains(snap.Headers["subscription-userinfo"], "MARKuserinfo") {
		t.Errorf("快照响应头：%+v %v", snap, err)
	}
	rd, err := d.Renders().Get(ctx, u.ID, p.ID)
	if err != nil || rd.LastError != "MARKrenderERR" || !strings.Contains(string(rd.LastDiags), "MARKdiagPREVIEW") || rd.GoodHeaders["subscription-userinfo"] != "MARKuserinfo" {
		t.Errorf("渲染结果：%+v %v", rd, err)
	}
}

// 迁移 0010 之前的明文：启动迁移后被加密并清除，读取结果不变。
func TestEncryptLegacy(t *testing.T) {
	ctx := context.Background()
	d := newDB(t)
	u := &User{Username: "carol", PasswordHash: "h", Role: "user"}
	if err := d.Users().Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	p := &Profile{Name: "p", Doc: []byte("x")}
	if err := d.Profiles().Create(ctx, u.ID, p, ""); err != nil {
		t.Fatal(err)
	}
	// 模拟旧版本写入的明文（经 d.exec：占位符按数据库换算，PostgreSQL 用 $1）
	if _, err := d.exec(ctx, "INSERT INTO snapshots (key, owner_id, headers) VALUES ('k1', ?, ?)", u.ID, `{"subscription-userinfo":"LEGACYinfo"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.exec(ctx, "INSERT INTO renders (profile_id, owner_id, good_headers, last_error, last_diags) VALUES (?, ?, ?, ?, ?)", p.ID, u.ID, `{"x":"LEGACYhdr"}`, "LEGACYerr", `[{"m":"LEGACYdiag"}]`); err != nil {
		t.Fatal(err)
	}
	n, err := d.encryptLegacy(ctx)
	if err != nil || n != 4 {
		t.Fatalf("应加密 4 个单元：%d %v", n, err)
	}
	var h, ge, le, ld string
	_ = d.root.QueryRowContext(ctx, "SELECT headers FROM snapshots WHERE key='k1'").Scan(&h)
	_ = d.root.QueryRowContext(ctx, "SELECT good_headers, last_error, last_diags FROM renders WHERE profile_id=?", p.ID).Scan(&ge, &le, &ld)
	if strings.Contains(h+ge+le+ld, "LEGACY") {
		t.Errorf("明文应清除：%q %q %q %q", h, ge, le, ld)
	}
	if rd, err := d.Renders().Get(ctx, u.ID, p.ID); err != nil || rd.LastError != "LEGACYerr" || rd.GoodHeaders["x"] != "LEGACYhdr" || !strings.Contains(string(rd.LastDiags), "LEGACYdiag") {
		t.Errorf("读回：%+v %v", rd, err)
	}
	if n, _ := d.encryptLegacy(ctx); n != 0 {
		t.Errorf("再次执行不应有待处理的行：%d", n)
	}
}

// 每一个 *_enc 列都在密钥轮换的列表里：新增加密列时忘了登记，轮换主密钥后就会留下旧密钥的密文。
func TestEncryptedColumnsCovered(t *testing.T) {
	files, _ := fs.Glob(migrations.FS, "sqlite/*.sql")
	create := regexp.MustCompile(`(?s)CREATE TABLE (\w+) \((.*?)\n\)`)
	col := regexp.MustCompile(`(?m)^\s+(\w+_enc)\s`)
	alter := regexp.MustCompile(`ALTER TABLE (\w+) ADD COLUMN (\w+_enc)\s`)
	found := map[string]bool{}
	for _, f := range files {
		b, _ := fs.ReadFile(migrations.FS, f)
		for _, m := range create.FindAllStringSubmatch(string(b), -1) {
			for _, c := range col.FindAllStringSubmatch(m[2], -1) {
				found[m[1]+"."+c[1]] = true
			}
		}
		for _, m := range alter.FindAllStringSubmatch(string(b), -1) {
			found[m[1]+"."+m[2]] = true
		}
	}
	listed := map[string]bool{}
	for _, e := range encrypted {
		listed[e.table+"."+e.col] = true
	}
	if len(found) < 10 {
		t.Fatalf("没有解析出加密列：%v", found)
	}
	for c := range found {
		if !listed[c] {
			t.Errorf("加密列 %s 不在密钥轮换列表 encrypted 中", c)
		}
	}
}
