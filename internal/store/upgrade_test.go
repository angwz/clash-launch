package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/migrations"
)

// 反馈 #73：用旧版本程序建的、已有数据的数据库，升级到最新版本后数据完整可用，再复制到新库同样完整。
//
// 做法：只执行到迁移 0003 模拟旧版本，按当时的表结构写入用户、节点源（加密）、配置与链接，
// 并写入一条已使用的邀请（0004 的数据迁移会据此把用户标记为受邀）；然后升级到最新版本，
// 检查自动备份、数据迁移结果与全部数据，最后用 Copy 复制到一个新库再检查一遍。
func TestUpgradePreservesData(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	k, _, err := LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	old, err := Open(ctx, "sqlite:"+filepath.Join(dir, "old.db"), k)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = old.Close() }()
	if _, err := old.migrateTo(ctx, "", 3); err != nil {
		t.Fatal(err)
	}
	// 4219e64 之前建的库多一列 sources.allow_direct（见 RetiredColumns）：模拟它
	if _, err := old.root.ExecContext(ctx, "ALTER TABLE sources ADD COLUMN allow_direct INTEGER NOT NULL DEFAULT 0"); err != nil {
		t.Fatal(err)
	}

	// 0003 时代的 users 表没有 invited 与 prefs 列：按当时的结构直接写入
	now := ms(time.Now())
	if _, err := old.root.ExecContext(ctx, "INSERT INTO users (id, username, username_norm, password_hash, role, created_at, updated_at) VALUES ('u1', 'Alice', 'alice', 'h', 'user', ?, ?), ('a1', 'admin', 'admin', 'h', 'admin', ?, ?)", now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := old.root.ExecContext(ctx, "INSERT INTO invites (id, token_hash, created_by, created_at, used_at, used_by) VALUES ('i1', 'th', 'a1', ?, ?, 'u1')", now, now); err != nil {
		t.Fatal(err)
	}
	src := &Source{Name: "订阅", Kind: "single", Content: []byte("ss://example")}
	if err := old.Sources().Create(ctx, "u1", src); err != nil {
		t.Fatal(err)
	}
	// profiles 表后来加了 protected 列（R55）：按 0003 时代的列直接写入
	prof := &Profile{ID: NewID(), Name: "配置", Doc: []byte("clash-launch/profile: v1\nname: 配置\n")}
	pdoc, err := old.seal(prof.Doc, "profiles", "doc", prof.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.root.ExecContext(ctx, "INSERT INTO profiles (id, owner_id, name, doc_enc, created_at, updated_at, version) VALUES (?,?,?,?,?,?,1)",
		prof.ID, "u1", prof.Name, pdoc, now, now); err != nil {
		t.Fatal(err)
	}
	// links 表后来加了 limits 列（R42）：按 0003 时代的列直接写入
	link := &Link{ID: NewID(), ProfileID: prof.ID, Token: "token-example-0123456789abcdef", TokenHash: "hash-example", Note: "手机"}
	enc, err := old.sealStr(link.Token, "links", "token", link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.root.ExecContext(ctx, "INSERT INTO links (id, owner_id, profile_id, token_enc, token_hash, note, created_at, last_access, last_ua, version) VALUES (?,?,?,?,?,?,?,0,'',1)",
		link.ID, "u1", link.ProfileID, enc, link.TokenHash, link.Note, now); err != nil {
		t.Fatal(err)
	}

	// 版本不同的两个库不能直接复制
	latest, err := Open(ctx, "sqlite:"+filepath.Join(dir, "latest.db"), k)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = latest.Close() }()
	if _, err := latest.Migrate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Copy(ctx, old, latest); !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("版本不同应拒绝复制：%v", err)
	}

	backups := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backups, 0o700); err != nil {
		t.Fatal(err)
	}
	done, err := old.Migrate(ctx, backups)
	if err != nil {
		t.Fatalf("升级：%v", err)
	}
	if len(done) == 0 || done[0] != 4 {
		t.Fatalf("应从 0004 开始升级：%v", done)
	}
	if bs, _ := filepath.Glob(filepath.Join(backups, "before-migrate-*.db")); len(bs) != 1 {
		t.Errorf("升级前应自动备份：%v", bs)
	}

	check := func(name string, d *DB) {
		t.Helper()
		u, err := d.Users().ByID(ctx, "u1")
		if err != nil || u.Username != "Alice" || !u.Invited {
			t.Errorf("%s：用户 %+v %v（0004 应把用过邀请的用户标为受邀）", name, u, err)
		}
		if p, err := d.Users().Prefs(ctx, "u1"); err != nil || string(p) != "{}" {
			t.Errorf("%s：偏好 %q %v", name, p, err)
		}
		ss, err := d.Sources().List(ctx, "u1", Page{})
		if err != nil || len(ss) != 1 || string(ss[0].Content) != "ss://example" {
			t.Errorf("%s：节点源 %+v %v", name, ss, err)
		}
		p, err := d.Profiles().Get(ctx, "u1", prof.ID)
		if err != nil || string(p.Doc) != string(prof.Doc) {
			t.Errorf("%s：配置 %+v %v", name, p, err)
		}
		l, err := d.Links().ByTokenHash(ctx, "hash-example")
		if err != nil || l.Token != link.Token || l.Note != "手机" {
			t.Errorf("%s：链接 %+v %v", name, l, err)
		}
	}
	check("升级后", old)

	fresh, err := Open(ctx, "sqlite:"+filepath.Join(dir, "new.db"), k)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fresh.Close() }()
	if _, err := fresh.Migrate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	stats, err := Copy(ctx, old, fresh)
	if err != nil {
		t.Fatalf("复制：%v", err)
	}
	var skipped []string
	for _, s := range stats {
		skipped = append(skipped, s.Skipped...)
	}
	if len(skipped) != 1 || skipped[0] != "sources.allow_direct" {
		t.Errorf("应只跳过登记过的废弃列：%v", skipped)
	}
	check("复制后", fresh)

	// 未登记的多余列：拒绝复制，不静默丢弃
	if _, err := old.root.ExecContext(ctx, "ALTER TABLE profiles ADD COLUMN unknown_extra TEXT NOT NULL DEFAULT ''"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(ctx, "sqlite:"+filepath.Join(dir, "other.db"), k)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if _, err := other.Migrate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Copy(ctx, old, other); !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("未登记的多余列应拒绝复制：%v", err)
	}
}

// 迁移只向前：已发布的迁移文件不得修改（反馈 #73）。migrations/SUMS 记录每个文件的 SHA-256；
// 改动已有文件或新增文件而不登记都会失败。新增迁移时把下面打印的行追加到 SUMS。
func TestMigrationsFrozen(t *testing.T) {
	b, err := os.ReadFile("../../migrations/SUMS")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			t.Fatalf("SUMS 格式错误：%q", line)
		}
		want[f[1]] = f[0]
	}
	files, _ := fs.Glob(migrations.FS, "*/*.sql")
	for _, f := range files {
		body, _ := fs.ReadFile(migrations.FS, f)
		sum := sha256.Sum256(body)
		got := hex.EncodeToString(sum[:])
		switch w, ok := want[f]; {
		case !ok:
			t.Errorf("新迁移 %s 未登记，请在 migrations/SUMS 追加：\n%s  %s", f, got, f)
		case w != got:
			t.Errorf("已发布的迁移 %s 被修改了。迁移只向前：请撤销修改，改为新增一个迁移（见 docs/guide/data.md）", f)
		}
		delete(want, f)
	}
	for f := range want {
		t.Errorf("已发布的迁移 %s 被删除了", f)
	}
}
