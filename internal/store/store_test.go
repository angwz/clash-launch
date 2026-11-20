package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/store/storetest"
	"github.com/angwz/clash-launch/migrations"
)

func newDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	k, _, err := LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	d, err := Open(context.Background(), storetest.DSN(t, dir), k)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.Migrate(context.Background(), filepath.Join(dir, "backups")); err != nil {
		t.Fatal(err)
	}
	return d
}

// 两套迁移的逻辑结构（表与列）必须一致（第 17.1 节）。
func TestMigrationsEquivalent(t *testing.T) {
	re := regexp.MustCompile(`(?m)^\s+([a-z_]+)\s+(TEXT|INTEGER|BLOB|BIGINT|BYTEA)\b`)
	tables := regexp.MustCompile(`CREATE TABLE ([a-z_]+)`)
	shape := func(path string) map[string][]string {
		b, err := migrations.FS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]string{}
		for _, part := range strings.Split(string(b), "CREATE TABLE ")[1:] {
			name := tables.FindStringSubmatch("CREATE TABLE " + part)[1]
			for _, m := range re.FindAllStringSubmatch(part, -1) {
				out[name] = append(out[name], m[1])
			}
		}
		return out
	}
	shapeAll := func(dir string) map[string][]string {
		out := map[string][]string{}
		files, _ := fs.Glob(migrations.FS, dir+"/*.sql")
		for _, f := range files {
			for k, v := range shape(f) {
				out[k] = v
			}
		}
		return out
	}
	a, b := shapeAll("sqlite"), shapeAll("postgres")
	if len(a) != len(b) || len(a) != len(Tables) {
		t.Fatalf("表数不一致：sqlite %d、postgres %d、Tables %d", len(a), len(b), len(Tables))
	}
	for name, cols := range a {
		if !slices.Equal(cols, b[name]) {
			t.Errorf("表 %s 的列不一致：%v / %v", name, cols, b[name])
		}
	}
}

func TestOwnerIsolationAndCrypto(t *testing.T) {
	ctx := context.Background()
	d := newDB(t)
	alice, bob := &User{Username: "alice", PasswordHash: "x", Role: "admin"}, &User{Username: "Bob", PasswordHash: "x", Role: "user"}
	for _, u := range []*User{alice, bob} {
		if err := d.Users().Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Users().Create(ctx, &User{Username: " BOB ", PasswordHash: "x", Role: "user"}); !errors.Is(err, ErrExists) {
		t.Errorf("用户名应不区分大小写地唯一：%v", err)
	}
	src := &Source{Name: "机场 A", Kind: "subscription", URL: "https://sub.example.com/x?token=secret-token", Headers: map[string]string{"Authorization": "Bearer t"}}
	if err := d.Sources().Create(ctx, alice.ID, src); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Sources().Get(ctx, bob.ID, src.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("他人的源应返回 not found：%v", err)
	}
	if err := d.Sources().Delete(ctx, bob.ID, src.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("他人不能删除：%v", err)
	}
	got, err := d.Sources().Get(ctx, alice.ID, src.ID)
	if err != nil || got.URL != src.URL || got.Headers["Authorization"] != "Bearer t" {
		t.Fatalf("解密不符：%+v %v", got, err)
	}
	// 数据库中不出现明文
	var raw []byte
	if err := d.row(ctx, "SELECT url_enc FROM sources WHERE id=?", src.ID).Scan(&raw); err != nil { // d.row 按方言改写占位符
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-token") {
		t.Error("URL 以明文存储")
	}
	// 密文被搬到另一行后无法解密（AAD 绑定行 ID）
	other := &Source{Name: "B", Kind: "single", Content: []byte("x")}
	_ = d.Sources().Create(ctx, alice.ID, other)
	if _, err := d.exec(ctx, "UPDATE sources SET url_enc=? WHERE id=?", raw, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Sources().Get(ctx, alice.ID, other.ID); !errors.Is(err, ErrDecrypt) {
		t.Errorf("搬移的密文应无法解密：%v", err)
	}
	// 乐观锁
	stale := *got
	got.Name = "新名字"
	if err := d.Sources().Update(ctx, alice.ID, got); err != nil {
		t.Fatal(err)
	}
	if err := d.Sources().Update(ctx, alice.ID, &stale); !errors.Is(err, ErrConflict) {
		t.Errorf("过期版本应冲突：%v", err)
	}
	stale.ID = "missing"
	if err := d.Sources().Update(ctx, alice.ID, &stale); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的资源应返回 not found：%v", err)
	}
	// 方案与版本
	p := &Profile{Name: "P", Doc: []byte("clash-launch/profile: v1\n")}
	if err := d.Profiles().Create(ctx, alice.ID, p, "create"); err != nil {
		t.Fatal(err)
	}
	p.Doc = []byte("clash-launch/profile: v1\nname: P2\n")
	if err := d.Profiles().Update(ctx, alice.ID, p, "edit"); err != nil {
		t.Fatal(err)
	}
	revs, err := d.Profiles().Revisions(ctx, alice.ID, p.ID, Page{})
	if err != nil || len(revs) != 2 || revs[0].N != 2 {
		t.Fatalf("版本：%+v %v", revs, err)
	}
	if _, err := d.Profiles().Revisions(ctx, bob.ID, p.ID, Page{}); err != nil {
		t.Fatal(err)
	} else if r, _ := d.Profiles().Revisions(ctx, bob.ID, p.ID, Page{}); len(r) != 0 {
		t.Error("他人不能看到版本")
	}
	// 链接按令牌哈希查找
	l := &Link{ProfileID: p.ID, Token: "tok", TokenHash: "h1"}
	if err := d.Links().Create(ctx, alice.ID, l); err != nil {
		t.Fatal(err)
	}
	if x, err := d.Links().ByTokenHash(ctx, "h1"); err != nil || x.Token != "tok" {
		t.Fatalf("按哈希查找：%+v %v", x, err)
	}
}

func TestKeyringRotateAndBackupCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	env := func(string) string { return "" }
	k, ks, err := LoadKeyring(dir, env)
	if err != nil || !ks.Generated {
		t.Fatalf("应生成主密钥：%+v %v", ks, err)
	}
	if st, err := os.Stat(filepath.Join(dir, "master.key")); err != nil || st.Size() == 0 {
		t.Fatal("密钥文件未写入")
	}
	blob, _ := k.Seal([]byte("hello"), "a")
	id, err := AppendKey(filepath.Join(dir, "master.key"), k)
	if err != nil || k.Current() != id {
		t.Fatal(err)
	}
	k2, ks2, err := LoadKeyring(dir, env)
	if err != nil || ks2.Generated || k2.Current() != id {
		t.Fatalf("重新加载：%+v %v", ks2, err)
	}
	if pt, err := k2.Open(blob, "a"); err != nil || string(pt) != "hello" {
		t.Fatal("轮换后旧密文应仍可解密")
	}
	src, err := Open(ctx, "sqlite:"+filepath.Join(dir, "a.db"), k2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	if _, err := src.Migrate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	u := &User{Username: "u", PasswordHash: "h", Role: "admin"}
	_ = src.Users().Create(ctx, u)
	_ = src.Sources().Create(ctx, u.ID, &Source{Name: "s", Kind: "single", Content: []byte("ss://x")})
	if err := src.Backup(ctx, filepath.Join(dir, "backup.db")); err != nil {
		t.Fatal(err)
	}
	dst, err := Open(ctx, "sqlite:"+filepath.Join(dir, "b.db"), k2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := dst.Migrate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	stats, err := Copy(ctx, src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].Rows != 1 || stats[2].Rows != 1 {
		t.Errorf("复制行数：%+v", stats)
	}
	s, err := dst.Sources().List(ctx, u.ID, Page{})
	if err != nil || len(s) != 1 || string(s[0].Content) != "ss://x" {
		t.Fatalf("复制后应可解密：%+v %v", s, err)
	}
	if _, err := Copy(ctx, src, dst); err == nil {
		t.Error("目标非空时应拒绝复制")
	}
}

// 邀请：只能使用一次；过期后不可用；不过期（expires_at=0）的邀请一直有效直到被使用。
func TestInvites(t *testing.T) {
	ctx := context.Background()
	d := newDB(t)
	admin := &User{Username: "admin", PasswordHash: "x", Role: "admin"}
	if err := d.Users().Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	never := &Invite{TokenHash: "h-never", CreatedBy: admin.ID, Note: "n"}
	soon := &Invite{TokenHash: "h-soon", CreatedBy: admin.ID, ExpiresAt: now.Add(time.Hour)}
	for _, inv := range []*Invite{never, soon} {
		if err := d.Invites().Create(ctx, inv); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Invites().Use(ctx, "h-soon", "u1", now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Errorf("过期邀请应不可用：%v", err)
	}
	got, err := d.Invites().Use(ctx, "h-never", "u1", now.Add(1000*time.Hour))
	if err != nil || got.UsedBy != "u1" || got.UsedAt.IsZero() || !got.ExpiresAt.IsZero() {
		t.Fatalf("使用邀请：%+v %v", got, err)
	}
	if _, err := d.Invites().Use(ctx, "h-never", "u2", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("邀请只能用一次：%v", err)
	}
	all, err := d.Invites().List(ctx, Page{})
	if err != nil || len(all) != 2 {
		t.Fatalf("列表：%d %v", len(all), err)
	}
	if err := d.Invites().Delete(ctx, soon.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.Invites().Delete(ctx, soon.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除应 404：%v", err)
	}
}

// 防止降级：数据库被更新的版本迁移过（有本程序不认识的迁移）时，Migrate 拒绝继续，数据保持不变；
// 已是最新时重复 Migrate 不做任何事（每次启动都会调用）。
func TestMigrateRefusesNewerSchema(t *testing.T) {
	d := newDB(t)
	ctx := context.Background()
	if done, err := d.Migrate(ctx, ""); err != nil || len(done) != 0 {
		t.Fatalf("重复迁移应无操作：%v %v", done, err)
	}
	if _, err := d.root.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (9999, 0)"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Migrate(ctx, ""); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("应拒绝更新版本的数据库：%v", err)
	}
}

// 统计（反馈 #66、#67）：计数按天合并并可求总数；请求逐条记录，按 IP 汇总成功与失败；令牌无效的请求只汇总。
func TestStats(t *testing.T) {
	d := newDB(t)
	ctx := context.Background()
	st := d.Stats()
	if err := st.AddCounts(ctx, []CountDelta{{"pv", "editor", 20261114, 2}, {"pv", "editor", 20261114, 3}, {"pv", "help", 20261115, 1}}); err != nil {
		t.Fatal(err)
	}
	tot, err := st.Totals(ctx, "pv", nil)
	if err != nil || tot["editor"] != 5 || tot["help"] != 1 {
		t.Fatalf("总数：%v %v", tot, err)
	}
	days, err := st.Daily(ctx, "pv", []string{"editor"}, 20261101, 20261130)
	if err != nil || len(days) != 1 || days[0].Count != 5 {
		t.Fatalf("按天：%v %v", days, err)
	}
	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	rs := []SubRequest{
		{LinkID: "l1", OwnerID: "u1", At: at, IP: "192.0.2.1", Client: "mihomo", Status: 200, Kind: "config"},
		{LinkID: "l1", OwnerID: "u1", At: at.Add(time.Hour), IP: "192.0.2.1", Client: "mihomo", Status: 304, Kind: "config"},
		{LinkID: "l1", OwnerID: "u1", At: at.Add(2 * time.Hour), IP: "192.0.2.2", Client: "clash-verge", Status: 429, Kind: "config"},
		{IP: "192.0.2.9", At: at, Status: 404},
	}
	if err := st.AddRequests(ctx, rs); err != nil {
		t.Fatal(err)
	}
	list, err := st.Requests(ctx, "u1", "l1", Page{Limit: 10})
	if err != nil || len(list) != 3 || list[0].IP != "192.0.2.2" {
		t.Fatalf("请求记录：%+v %v", list, err)
	}
	if other, _ := st.Requests(ctx, "u2", "l1", Page{Limit: 10}); len(other) != 0 {
		t.Error("其他用户不能看到这条链接的记录")
	}
	ips, err := st.IPs(ctx, "u1", "l1")
	if err != nil || len(ips) != 2 {
		t.Fatalf("IP 汇总：%+v %v", ips, err)
	}
	for _, x := range ips {
		if x.IP == "192.0.2.1" && (x.OK != 2 || x.Fail != 0 || !x.First.Equal(at)) {
			t.Errorf("192.0.2.1：%+v", x)
		}
		if x.IP == "192.0.2.2" && (x.OK != 0 || x.Fail != 1) {
			t.Errorf("192.0.2.2：%+v", x)
		}
	}
	bad, err := st.IPs(ctx, "", "")
	if err != nil || len(bad) != 1 || bad[0].Fail != 1 {
		t.Errorf("令牌无效的来源：%+v %v", bad, err)
	}
}

// R14c：过期的请求明细按批删除，较新的明细与按 IP 的汇总保留。
func TestPruneRequests(t *testing.T) {
	d := newDB(t)
	ctx := context.Background()
	st := d.Stats()
	cut := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	old := pruneBatch + 7 // 超过一批，覆盖循环
	rs := make([]SubRequest, 0, old+3)
	for i := 0; i < old; i++ {
		rs = append(rs, SubRequest{LinkID: "l1", OwnerID: "u1", At: cut.Add(-time.Duration(i+1) * time.Minute), IP: "192.0.2.1", Status: 200, Kind: "config"})
	}
	for i := 0; i < 3; i++ {
		rs = append(rs, SubRequest{LinkID: "l1", OwnerID: "u1", At: cut.Add(time.Duration(i) * time.Minute), IP: "192.0.2.2", Status: 200, Kind: "config"})
	}
	if err := st.AddRequests(ctx, rs); err != nil {
		t.Fatal(err)
	}
	n, err := st.PruneRequests(ctx, cut)
	if err != nil || n != int64(old) {
		t.Fatalf("应删除 %d 行，得到 %d %v", old, n, err)
	}
	left, _, err := st.Range(ctx, "u1", cut.Add(-365*24*time.Hour), cut.Add(time.Hour), 100)
	if err != nil || len(left) != 3 {
		t.Fatalf("应剩 3 条较新的明细，得到 %d %v", len(left), err)
	}
	ips, err := st.IPs(ctx, "u1", "l1")
	if err != nil || len(ips) != 2 {
		t.Fatalf("按 IP 的汇总应保留，得到 %d %v", len(ips), err)
	}
}

// R14d：按页浏览审计记录时，总数只数到上限加一，用于判断「还有更多」。
func TestAuditPagedCountCap(t *testing.T) {
	d := newDB(t)
	ctx := context.Background()
	for i := 0; i < 15; i++ {
		if err := d.Audit().Add(ctx, &AuditEntry{OwnerID: "u1", Action: "session.login"}); err != nil {
			t.Fatal(err)
		}
	}
	list, n, err := d.Audit().Paged(ctx, "u1", 0, 10, 12)
	if err != nil || len(list) != 10 || n != 13 {
		t.Fatalf("上限 12 时应数到 13（表示还有更多），得到 %d 条、总数 %d、%v", len(list), n, err)
	}
	if _, n, _ := d.Audit().Paged(ctx, "u1", 10, 10, 100); n != 15 {
		t.Fatalf("未超过上限时应为真实总数 15，得到 %d", n)
	}
	if _, n, _ := d.Audit().PagedAll(ctx, 0, 10, 5); n != 6 {
		t.Fatalf("全站：上限 5 时应数到 6，得到 %d", n)
	}
}
