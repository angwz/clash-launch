package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL 驱动（pgx）
	_ "modernc.org/sqlite"             // SQLite 驱动（纯 Go，免 CGO）
)

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DB 是 Store 的 SQL 实现（SQLite 与 PostgreSQL 共用；差异只在占位符与驱动）。
type DB struct {
	root    *sql.DB
	q       querier
	dialect string // sqlite | postgres
	keys    *Keyring
}

// Open 按 DSN 打开数据库：sqlite:///data/clash-launch.db（或 sqlite:相对路径）、postgres://…。
// 打开后不自动迁移；调用 Migrate。
func Open(ctx context.Context, dsn string, keys *Keyring) (*DB, error) {
	dialect, driver, conn, err := parseDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, conn)
	if err != nil {
		return nil, err
	}
	if dialect == "sqlite" {
		db.SetMaxOpenConns(1) // SQLite 单写者：串行化写入，避免 SQLITE_BUSY；读多写少的规模下足够
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: 连接数据库: %w", err)
	}
	return &DB{root: db, q: db, dialect: dialect, keys: keys}, nil
}

func parseDSN(dsn string) (dialect, driver, conn string, err error) {
	switch {
	case strings.HasPrefix(dsn, "sqlite:"):
		p := strings.TrimPrefix(dsn, "sqlite:")
		p = strings.TrimPrefix(p, "//")
		if p == "" {
			return "", "", "", errors.New("store: sqlite DSN 缺少文件路径")
		}
		if !strings.HasPrefix(p, "/") && filepath.VolumeName(p) == "" && p != ":memory:" {
			p = filepath.Clean(p)
		}
		q := url.Values{}
		for _, pr := range []string{"journal_mode(WAL)", "foreign_keys(1)", "busy_timeout(5000)", "synchronous(NORMAL)"} {
			q.Add("_pragma", pr)
		}
		return "sqlite", "sqlite", "file:" + p + "?" + q.Encode(), nil
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return "postgres", "pgx", dsn, nil
	}
	return "", "", "", fmt.Errorf("store: 不支持的数据库地址（应以 sqlite: 或 postgres:// 开头）")
}

// Dialect 返回数据库类型。
func (d *DB) Dialect() string { return d.dialect }

// Close 关闭数据库。
func (d *DB) Close() error { return d.root.Close() }

// Raw 返回底层连接（迁移、备份与 db copy 使用）。
func (d *DB) Raw() *sql.DB { return d.root }

// Keys 返回密钥环。
func (d *DB) Keys() *Keyring { return d.keys }

// Tx 在事务中执行 fn。
func (d *DB) Tx(ctx context.Context, fn func(Store) error) error {
	if _, nested := d.q.(*sql.Tx); nested {
		return fn(d)
	}
	tx, err := d.root.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	inner := &DB{root: d.root, q: tx, dialect: d.dialect, keys: d.keys}
	if err := fn(inner); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// rebind 把 ? 占位符改为 PostgreSQL 的 $n。
func (d *DB) rebind(q string) string {
	if d.dialect != "postgres" {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (d *DB) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return d.q.ExecContext(ctx, d.rebind(q), args...)
}

func (d *DB) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return d.q.QueryContext(ctx, d.rebind(q), args...)
}

func (d *DB) row(ctx context.Context, q string, args ...any) *sql.Row {
	return d.q.QueryRowContext(ctx, d.rebind(q), args...)
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}

func fromMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (d *DB) seal(b []byte, table, col, id string) ([]byte, error) {
	return d.keys.Seal(b, table+"."+col+":"+id)
}

func (d *DB) open(b []byte, table, col, id string) ([]byte, error) {
	return d.keys.Open(b, table+"."+col+":"+id)
}

func (d *DB) sealStr(s, table, col, id string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return d.seal([]byte(s), table, col, id)
}

func (d *DB) openStr(b []byte, table, col, id string) (string, error) {
	pt, err := d.open(b, table, col, id)
	return string(pt), err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// affected 检查 UPDATE/DELETE 是否命中一行；没有命中时按 notFoundOrConflict 区分。
func affected(res sql.Result, err error, onZero error) error {
	if err != nil {
		if isUnique(err) {
			return ErrExists
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return onZero
	}
	return nil
}

func isUnique(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "duplicate key")
}

func limitOf(p Page) int {
	if p.Limit <= 0 || p.Limit > 200 {
		return 50
	}
	return p.Limit
}

func jsonMap(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func parseMap(s string) map[string]string {
	m := map[string]string{}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

// —— 仓储入口 ——

func (d *DB) Users() UserRepo             { return users{d} }
func (d *DB) Sessions() SessionRepo       { return sessions{d} }
func (d *DB) Sources() SourceRepo         { return sources{d} }
func (d *DB) RuleSources() RuleSourceRepo { return ruleSources{d} }
func (d *DB) Snapshots() SnapshotRepo     { return snapshots{d} }
func (d *DB) Profiles() ProfileRepo       { return profiles{d} }
func (d *DB) Links() LinkRepo             { return links{d} }
func (d *DB) Renders() RenderRepo         { return renders{d} }
func (d *DB) Audit() AuditRepo            { return audits{d} }
func (d *DB) Settings() SettingsRepo      { return settings{d} }
func (d *DB) Invites() InviteRepo         { return invites{d} }

// —— 用户 ——

type users struct{ d *DB }

const userCols = "id, username, password_hash, role, disabled, invited, lang, timezone, created_at, updated_at, version"

func scanUser(r interface{ Scan(...any) error }) (*User, error) {
	var u User
	var dis, inv, c, up int64
	if err := r.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &dis, &inv, &u.Lang, &u.Timezone, &c, &up, &u.Version); err != nil {
		return nil, notFound(err)
	}
	u.Disabled, u.Invited, u.CreatedAt, u.UpdatedAt = dis == 1, inv == 1, fromMS(c), fromMS(up)
	return &u, nil
}

// NormalizeUsername 是用户名的规范形式（唯一性按它判断）。
func NormalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (r users) Create(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = NewID()
	}
	now := time.Now()
	u.CreatedAt, u.UpdatedAt, u.Version = now, now, 1
	_, err := r.d.exec(ctx, "INSERT INTO users (id, username, username_norm, password_hash, role, disabled, invited, lang, timezone, created_at, updated_at, version) VALUES (?,?,?,?,?,?,?,?,?,?,?,1)",
		u.ID, u.Username, NormalizeUsername(u.Username), u.PasswordHash, u.Role, b2i(u.Disabled), b2i(u.Invited), u.Lang, u.Timezone, ms(now), ms(now))
	if err != nil && isUnique(err) {
		return ErrExists
	}
	return err
}

func (r users) ByID(ctx context.Context, id string) (*User, error) {
	return scanUser(r.d.row(ctx, "SELECT "+userCols+" FROM users WHERE id = ?", id))
}

func (r users) ByUsername(ctx context.Context, name string) (*User, error) {
	return scanUser(r.d.row(ctx, "SELECT "+userCols+" FROM users WHERE username_norm = ?", NormalizeUsername(name)))
}

func (r users) Update(ctx context.Context, u *User) error {
	now := time.Now()
	res, err := r.d.exec(ctx, "UPDATE users SET password_hash=?, role=?, disabled=?, lang=?, timezone=?, updated_at=?, version=version+1 WHERE id=? AND version=?",
		u.PasswordHash, u.Role, b2i(u.Disabled), u.Lang, u.Timezone, ms(now), u.ID, u.Version)
	if err := affected(res, err, ErrConflict); err != nil {
		return err
	}
	u.Version++
	u.UpdatedAt = now
	return nil
}

func (r users) Prefs(ctx context.Context, id string) ([]byte, error) {
	var s string
	if err := r.d.row(ctx, "SELECT prefs FROM users WHERE id=?", id).Scan(&s); err != nil {
		return nil, notFound(err)
	}
	return []byte(s), nil
}

func (r users) SetPrefs(ctx context.Context, id string, prefs []byte) error {
	res, err := r.d.exec(ctx, "UPDATE users SET prefs=? WHERE id=?", string(prefs), id)
	return affected(res, err, ErrNotFound)
}

func (r users) SetInvited(ctx context.Context, id string, invited bool) error {
	res, err := r.d.exec(ctx, "UPDATE users SET invited=? WHERE id=?", b2i(invited), id)
	return affected(res, err, ErrNotFound)
}

func (r users) Count(ctx context.Context) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

func (r users) List(ctx context.Context, p Page) ([]User, error) {
	rows, err := r.d.query(ctx, "SELECT "+userCols+" FROM users WHERE id > ? ORDER BY id LIMIT ?", p.After, limitOf(p))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = "" // 管理用：只含元数据
		out = append(out, *u)
	}
	return out, rows.Err()
}

// —— 会话 ——

type sessions struct{ d *DB }

func (r sessions) Create(ctx context.Context, s *Session) error {
	_, err := r.d.exec(ctx, "INSERT INTO sessions (id_hash, user_id, csrf, created_at, last_seen, expires_at, ua, ip_prefix) VALUES (?,?,?,?,?,?,?,?)",
		s.IDHash, s.UserID, s.CSRF, ms(s.CreatedAt), ms(s.LastSeen), ms(s.ExpiresAt), s.UA, s.IPPrefix)
	return err
}

func scanSession(r interface{ Scan(...any) error }) (*Session, error) {
	var s Session
	var c, l, e int64
	if err := r.Scan(&s.IDHash, &s.UserID, &s.CSRF, &c, &l, &e, &s.UA, &s.IPPrefix); err != nil {
		return nil, notFound(err)
	}
	s.CreatedAt, s.LastSeen, s.ExpiresAt = fromMS(c), fromMS(l), fromMS(e)
	return &s, nil
}

const sessionCols = "id_hash, user_id, csrf, created_at, last_seen, expires_at, ua, ip_prefix"

func (r sessions) ByHash(ctx context.Context, h string) (*Session, error) {
	return scanSession(r.d.row(ctx, "SELECT "+sessionCols+" FROM sessions WHERE id_hash = ?", h))
}

func (r sessions) Touch(ctx context.Context, h string, at time.Time) error {
	_, err := r.d.exec(ctx, "UPDATE sessions SET last_seen=? WHERE id_hash=?", ms(at), h)
	return err
}

func (r sessions) Delete(ctx context.Context, userID, h string) error {
	res, err := r.d.exec(ctx, "DELETE FROM sessions WHERE user_id=? AND id_hash=?", userID, h)
	return affected(res, err, ErrNotFound)
}

func (r sessions) DeleteAll(ctx context.Context, userID string) error {
	_, err := r.d.exec(ctx, "DELETE FROM sessions WHERE user_id=?", userID)
	return err
}

func (r sessions) List(ctx context.Context, userID string) ([]Session, error) {
	rows, err := r.d.query(ctx, "SELECT "+sessionCols+" FROM sessions WHERE user_id=? ORDER BY last_seen DESC", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// —— 节点源 ——

type sources struct{ d *DB }

const sourceCols = "id, owner_id, name, kind, url_enc, content_enc, headers_enc, ua, interval_s, processing, created_at, updated_at, version"

func (r sources) scan(row interface{ Scan(...any) error }) (*Source, error) {
	var s Source
	var urlEnc, contentEnc, headersEnc []byte
	var proc string
	var c, u int64
	if err := row.Scan(&s.ID, &s.OwnerID, &s.Name, &s.Kind, &urlEnc, &contentEnc, &headersEnc, &s.UA, &s.Interval, &proc, &c, &u, &s.Version); err != nil {
		return nil, notFound(err)
	}
	var err error
	if s.URL, err = r.d.openStr(urlEnc, "sources", "url", s.ID); err != nil {
		return nil, err
	}
	if s.Content, err = r.d.open(contentEnc, "sources", "content", s.ID); err != nil {
		return nil, err
	}
	h, err := r.d.openStr(headersEnc, "sources", "headers", s.ID)
	if err != nil {
		return nil, err
	}
	s.Headers = parseMap(h)
	s.Processing, s.CreatedAt, s.UpdatedAt = []byte(proc), fromMS(c), fromMS(u)
	return &s, nil
}

func (r sources) sealAll(s *Source) (u, c, h []byte, err error) {
	if u, err = r.d.sealStr(s.URL, "sources", "url", s.ID); err != nil {
		return
	}
	if c, err = r.d.seal(s.Content, "sources", "content", s.ID); err != nil {
		return
	}
	if len(s.Headers) > 0 {
		h, err = r.d.sealStr(jsonMap(s.Headers), "sources", "headers", s.ID)
	}
	return
}

func procOrEmpty(b []byte) string {
	if len(b) == 0 {
		return "{}"
	}
	return string(b)
}

func (r sources) Create(ctx context.Context, owner string, s *Source) error {
	if s.ID == "" {
		s.ID = NewID()
	}
	s.OwnerID = owner
	now := time.Now()
	s.CreatedAt, s.UpdatedAt, s.Version = now, now, 1
	u, c, h, err := r.sealAll(s)
	if err != nil {
		return err
	}
	_, err = r.d.exec(ctx, "INSERT INTO sources ("+sourceCols+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,1)",
		s.ID, owner, s.Name, s.Kind, u, c, h, s.UA, s.Interval, procOrEmpty(s.Processing), ms(now), ms(now))
	return err
}

func (r sources) Get(ctx context.Context, owner, id string) (*Source, error) {
	return r.scan(r.d.row(ctx, "SELECT "+sourceCols+" FROM sources WHERE owner_id=? AND id=?", owner, id))
}

func (r sources) List(ctx context.Context, owner string, p Page) ([]Source, error) {
	rows, err := r.d.query(ctx, "SELECT "+sourceCols+" FROM sources WHERE owner_id=? AND id > ? ORDER BY id LIMIT ?", owner, p.After, limitOf(p))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Source
	for rows.Next() {
		s, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r sources) Update(ctx context.Context, owner string, s *Source) error {
	u, c, h, err := r.sealAll(s)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := r.d.exec(ctx, "UPDATE sources SET name=?, kind=?, url_enc=?, content_enc=?, headers_enc=?, ua=?, interval_s=?, processing=?, updated_at=?, version=version+1 WHERE owner_id=? AND id=? AND version=?",
		s.Name, s.Kind, u, c, h, s.UA, s.Interval, procOrEmpty(s.Processing), ms(now), owner, s.ID, s.Version)
	if err := affected(res, err, r.missing(ctx, owner, s.ID)); err != nil {
		return err
	}
	s.Version++
	s.UpdatedAt = now
	return nil
}

// missing 区分「不存在」与「版本冲突」。
func (r sources) missing(ctx context.Context, owner, id string) error {
	var x string
	if err := r.d.row(ctx, "SELECT id FROM sources WHERE owner_id=? AND id=?", owner, id).Scan(&x); err != nil {
		return ErrNotFound
	}
	return ErrConflict
}

func (r sources) Delete(ctx context.Context, owner, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM sources WHERE owner_id=? AND id=?", owner, id)
	if err := affected(res, err, ErrNotFound); err != nil {
		return err
	}
	return r.d.Snapshots().Delete(ctx, owner, "src/"+id)
}

func (r sources) Due(ctx context.Context, now time.Time, limit int) ([]Source, error) {
	// 到期判断在调度器中完成（需要快照的最近尝试时间）；这里返回全部订阅。
	rows, err := r.d.query(ctx, "SELECT "+sourceCols+" FROM sources WHERE kind = 'subscription' ORDER BY id LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Source
	for rows.Next() {
		s, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// —— 规则源 ——

type ruleSources struct{ d *DB }

const ruleCols = "id, owner_id, name, kind, preset, groups_url, rulesets_url, ini_url, groups_text, rulesets_text, ini_text, interval_s, created_at, updated_at, version"

func scanRule(row interface{ Scan(...any) error }) (*RuleSource, error) {
	var r RuleSource
	var c, u int64
	if err := row.Scan(&r.ID, &r.OwnerID, &r.Name, &r.Kind, &r.Preset, &r.GroupsURL, &r.RulesetsURL, &r.INIURL, &r.GroupsText, &r.RulesetsText, &r.INIText, &r.Interval, &c, &u, &r.Version); err != nil {
		return nil, notFound(err)
	}
	r.CreatedAt, r.UpdatedAt = fromMS(c), fromMS(u)
	return &r, nil
}

func (r ruleSources) Create(ctx context.Context, owner string, x *RuleSource) error {
	if x.ID == "" {
		x.ID = NewID()
	}
	x.OwnerID = owner
	now := time.Now()
	x.CreatedAt, x.UpdatedAt, x.Version = now, now, 1
	_, err := r.d.exec(ctx, "INSERT INTO rule_sources ("+ruleCols+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,1)",
		x.ID, owner, x.Name, x.Kind, x.Preset, x.GroupsURL, x.RulesetsURL, x.INIURL, x.GroupsText, x.RulesetsText, x.INIText, x.Interval, ms(now), ms(now))
	return err
}

func (r ruleSources) Get(ctx context.Context, owner, id string) (*RuleSource, error) {
	return scanRule(r.d.row(ctx, "SELECT "+ruleCols+" FROM rule_sources WHERE owner_id=? AND id=?", owner, id))
}

func (r ruleSources) list(ctx context.Context, q string, args ...any) ([]RuleSource, error) {
	rows, err := r.d.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []RuleSource
	for rows.Next() {
		x, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

func (r ruleSources) List(ctx context.Context, owner string, p Page) ([]RuleSource, error) {
	return r.list(ctx, "SELECT "+ruleCols+" FROM rule_sources WHERE owner_id=? AND id > ? ORDER BY id LIMIT ?", owner, p.After, limitOf(p))
}

func (r ruleSources) All(ctx context.Context, limit int) ([]RuleSource, error) {
	return r.list(ctx, "SELECT "+ruleCols+" FROM rule_sources ORDER BY id LIMIT ?", limit)
}

func (r ruleSources) Update(ctx context.Context, owner string, x *RuleSource) error {
	now := time.Now()
	res, err := r.d.exec(ctx, "UPDATE rule_sources SET name=?, kind=?, preset=?, groups_url=?, rulesets_url=?, ini_url=?, groups_text=?, rulesets_text=?, ini_text=?, interval_s=?, updated_at=?, version=version+1 WHERE owner_id=? AND id=? AND version=?",
		x.Name, x.Kind, x.Preset, x.GroupsURL, x.RulesetsURL, x.INIURL, x.GroupsText, x.RulesetsText, x.INIText, x.Interval, ms(now), owner, x.ID, x.Version)
	zero := ErrConflict
	if _, gerr := r.Get(ctx, owner, x.ID); gerr != nil {
		zero = ErrNotFound
	}
	if err := affected(res, err, zero); err != nil {
		return err
	}
	x.Version++
	x.UpdatedAt = now
	return nil
}

func (r ruleSources) Delete(ctx context.Context, owner, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM rule_sources WHERE owner_id=? AND id=?", owner, id)
	if err := affected(res, err, ErrNotFound); err != nil {
		return err
	}
	return r.d.Snapshots().Delete(ctx, owner, "rs/"+id+"/")
}

// —— 快照 ——

type snapshots struct{ d *DB }

func (r snapshots) Get(ctx context.Context, owner, key string) (*Snapshot, error) {
	var s Snapshot
	var enc []byte
	var headers string
	var f, la, fs int64
	err := r.d.row(ctx, "SELECT key, owner_id, content_enc, hash, headers, fetched_at, via, last_error, last_attempt, failing_since FROM snapshots WHERE owner_id=? AND key=?", owner, key).
		Scan(&s.Key, &s.OwnerID, &enc, &s.Hash, &headers, &f, &s.Via, &s.LastError, &la, &fs)
	if err != nil {
		return nil, notFound(err)
	}
	if s.Content, err = r.d.open(enc, "snapshots", "content", key); err != nil {
		return nil, err
	}
	s.Headers, s.FetchedAt, s.LastAttempt, s.FailingSince = parseMap(headers), fromMS(f), fromMS(la), fromMS(fs)
	return &s, nil
}

func (r snapshots) Put(ctx context.Context, owner string, s *Snapshot) error {
	enc, err := r.d.seal(s.Content, "snapshots", "content", s.Key)
	if err != nil {
		return err
	}
	res, err := r.d.exec(ctx, "UPDATE snapshots SET content_enc=?, hash=?, headers=?, fetched_at=?, via=?, last_error=?, last_attempt=?, failing_since=? WHERE owner_id=? AND key=?",
		enc, s.Hash, jsonMap(s.Headers), ms(s.FetchedAt), s.Via, s.LastError, ms(s.LastAttempt), ms(s.FailingSince), owner, s.Key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.d.exec(ctx, "INSERT INTO snapshots (key, owner_id, content_enc, hash, headers, fetched_at, via, last_error, last_attempt, failing_since) VALUES (?,?,?,?,?,?,?,?,?,?)",
		s.Key, owner, enc, s.Hash, jsonMap(s.Headers), ms(s.FetchedAt), s.Via, s.LastError, ms(s.LastAttempt), ms(s.FailingSince))
	return err
}

func (r snapshots) Delete(ctx context.Context, owner, prefix string) error {
	_, err := r.d.exec(ctx, "DELETE FROM snapshots WHERE owner_id=? AND (key=? OR key LIKE ?)", owner, prefix, strings.NewReplacer("%", "", "_", "").Replace(prefix)+"%")
	return err
}

// —— 方案 ——

type profiles struct{ d *DB }

func (r profiles) scan(row interface{ Scan(...any) error }) (*Profile, error) {
	var p Profile
	var enc []byte
	var c, u int64
	if err := row.Scan(&p.ID, &p.OwnerID, &p.Name, &enc, &c, &u, &p.Version); err != nil {
		return nil, notFound(err)
	}
	var err error
	if p.Doc, err = r.d.open(enc, "profiles", "doc", p.ID); err != nil {
		return nil, err
	}
	p.CreatedAt, p.UpdatedAt = fromMS(c), fromMS(u)
	return &p, nil
}

const profileCols = "id, owner_id, name, doc_enc, created_at, updated_at, version"

func (r profiles) addRevision(ctx context.Context, owner string, p *Profile, note string) error {
	id := NewID()
	enc, err := r.d.seal(p.Doc, "revisions", "doc", id)
	if err != nil {
		return err
	}
	_, err = r.d.exec(ctx, "INSERT INTO revisions (id, profile_id, owner_id, n, doc_enc, note, created_at) VALUES (?,?,?,?,?,?,?)",
		id, p.ID, owner, p.Version, enc, note, ms(time.Now()))
	return err
}

func (r profiles) Create(ctx context.Context, owner string, p *Profile, note string) error {
	if p.ID == "" {
		p.ID = NewID()
	}
	p.OwnerID = owner
	now := time.Now()
	p.CreatedAt, p.UpdatedAt, p.Version = now, now, 1
	return r.d.Tx(ctx, func(s Store) error {
		d := s.(*DB)
		enc, err := d.seal(p.Doc, "profiles", "doc", p.ID)
		if err != nil {
			return err
		}
		if _, err := d.exec(ctx, "INSERT INTO profiles ("+profileCols+") VALUES (?,?,?,?,?,?,1)", p.ID, owner, p.Name, enc, ms(now), ms(now)); err != nil {
			return err
		}
		return profiles{d}.addRevision(ctx, owner, p, note)
	})
}

func (r profiles) Get(ctx context.Context, owner, id string) (*Profile, error) {
	return r.scan(r.d.row(ctx, "SELECT "+profileCols+" FROM profiles WHERE owner_id=? AND id=?", owner, id))
}

func (r profiles) list(ctx context.Context, q string, args ...any) ([]Profile, error) {
	rows, err := r.d.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Profile
	for rows.Next() {
		p, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r profiles) List(ctx context.Context, owner string, p Page) ([]Profile, error) {
	return r.list(ctx, "SELECT "+profileCols+" FROM profiles WHERE owner_id=? AND id > ? ORDER BY id LIMIT ?", owner, p.After, limitOf(p))
}

func (r profiles) All(ctx context.Context, limit int) ([]Profile, error) {
	return r.list(ctx, "SELECT "+profileCols+" FROM profiles ORDER BY id LIMIT ?", limit)
}

func (r profiles) Update(ctx context.Context, owner string, p *Profile, note string) error {
	return r.d.Tx(ctx, func(s Store) error {
		d := s.(*DB)
		if err := (profiles{d}).bump(ctx, owner, p); err != nil {
			return err
		}
		return profiles{d}.addRevision(ctx, owner, p, note)
	})
}

// bump 写入新文档并把版本加一（乐观锁：Version 不一致时返回 ErrConflict）。须在事务中调用。
func (r profiles) bump(ctx context.Context, owner string, p *Profile) error {
	enc, err := r.d.seal(p.Doc, "profiles", "doc", p.ID)
	if err != nil {
		return err
	}
	now := time.Now()
	res, err := r.d.exec(ctx, "UPDATE profiles SET name=?, doc_enc=?, updated_at=?, version=version+1 WHERE owner_id=? AND id=? AND version=?", p.Name, enc, ms(now), owner, p.ID, p.Version)
	zero := ErrConflict
	if _, gerr := r.Get(ctx, owner, p.ID); gerr != nil {
		zero = ErrNotFound
	}
	if err := affected(res, err, zero); err != nil {
		return err
	}
	p.Version++
	p.UpdatedAt = now
	return nil
}

// AutosaveNote 是自动保存版本的备注；界面据此显示为「自动保存」。
const AutosaveNote = "autosave"

func (r profiles) Autosave(ctx context.Context, owner string, p *Profile, window time.Duration) error {
	return r.d.Tx(ctx, func(s Store) error {
		d := s.(*DB)
		prev := p.Version
		if err := (profiles{d}).bump(ctx, owner, p); err != nil {
			return err
		}
		now := p.UpdatedAt
		// 上一个版本是窗口内的自动保存：改写它（编号随之前进，保持与方案版本一致）
		var id, note string
		var created int64
		err := d.row(ctx, "SELECT id, note, created_at FROM revisions WHERE profile_id=? AND n=?", p.ID, prev).Scan(&id, &note, &created)
		if err == nil && note == AutosaveNote && now.Sub(fromMS(created)) < window {
			renc, err := d.seal(p.Doc, "revisions", "doc", id)
			if err != nil {
				return err
			}
			_, err = d.exec(ctx, "UPDATE revisions SET n=?, doc_enc=?, created_at=? WHERE id=?", p.Version, renc, ms(now), id)
			return err
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return profiles{d}.addRevision(ctx, owner, p, AutosaveNote)
	})
}

func (r profiles) Delete(ctx context.Context, owner, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM profiles WHERE owner_id=? AND id=?", owner, id)
	return affected(res, err, ErrNotFound)
}

func (r profiles) scanRev(row interface{ Scan(...any) error }) (*Revision, error) {
	var v Revision
	var enc []byte
	var c int64
	if err := row.Scan(&v.ID, &v.ProfileID, &v.OwnerID, &v.N, &enc, &v.Note, &c); err != nil {
		return nil, notFound(err)
	}
	var err error
	if v.Doc, err = r.d.open(enc, "revisions", "doc", v.ID); err != nil {
		return nil, err
	}
	v.CreatedAt = fromMS(c)
	return &v, nil
}

func (r profiles) Revisions(ctx context.Context, owner, profileID string, p Page) ([]Revision, error) {
	after := 1 << 30
	if p.After != "" {
		if n, err := strconv.Atoi(p.After); err == nil {
			after = n
		}
	}
	rows, err := r.d.query(ctx, "SELECT id, profile_id, owner_id, n, doc_enc, note, created_at FROM revisions WHERE owner_id=? AND profile_id=? AND n < ? ORDER BY n DESC LIMIT ?", owner, profileID, after, limitOf(p))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Revision
	for rows.Next() {
		v, err := r.scanRev(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r profiles) Revision(ctx context.Context, owner, profileID string, n int) (*Revision, error) {
	return r.scanRev(r.d.row(ctx, "SELECT id, profile_id, owner_id, n, doc_enc, note, created_at FROM revisions WHERE owner_id=? AND profile_id=? AND n=?", owner, profileID, n))
}

// —— 订阅链接 ——

type links struct{ d *DB }

const linkCols = "id, owner_id, profile_id, token_enc, token_hash, note, created_at, last_access, last_ua, version"

func (r links) scan(row interface{ Scan(...any) error }) (*Link, error) {
	var l Link
	var enc []byte
	var c, a int64
	if err := row.Scan(&l.ID, &l.OwnerID, &l.ProfileID, &enc, &l.TokenHash, &l.Note, &c, &a, &l.LastUA, &l.Version); err != nil {
		return nil, notFound(err)
	}
	var err error
	if l.Token, err = r.d.openStr(enc, "links", "token", l.ID); err != nil {
		return nil, err
	}
	l.CreatedAt, l.LastAccess = fromMS(c), fromMS(a)
	return &l, nil
}

func (r links) Create(ctx context.Context, owner string, l *Link) error {
	if l.ID == "" {
		l.ID = NewID()
	}
	l.OwnerID, l.CreatedAt, l.Version = owner, time.Now(), 1
	enc, err := r.d.sealStr(l.Token, "links", "token", l.ID)
	if err != nil {
		return err
	}
	_, err = r.d.exec(ctx, "INSERT INTO links ("+linkCols+") VALUES (?,?,?,?,?,?,?,0,'',1)", l.ID, owner, l.ProfileID, enc, l.TokenHash, l.Note, ms(l.CreatedAt))
	return err
}

func (r links) Get(ctx context.Context, owner, id string) (*Link, error) {
	return r.scan(r.d.row(ctx, "SELECT "+linkCols+" FROM links WHERE owner_id=? AND id=?", owner, id))
}

func (r links) List(ctx context.Context, owner, profileID string) ([]Link, error) {
	rows, err := r.d.query(ctx, "SELECT "+linkCols+" FROM links WHERE owner_id=? AND profile_id=? ORDER BY id", owner, profileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Link
	for rows.Next() {
		l, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (r links) Update(ctx context.Context, owner string, l *Link) error {
	enc, err := r.d.sealStr(l.Token, "links", "token", l.ID)
	if err != nil {
		return err
	}
	res, err := r.d.exec(ctx, "UPDATE links SET token_enc=?, token_hash=?, note=?, version=version+1 WHERE owner_id=? AND id=? AND version=?", enc, l.TokenHash, l.Note, owner, l.ID, l.Version)
	zero := ErrConflict
	if _, gerr := r.Get(ctx, owner, l.ID); gerr != nil {
		zero = ErrNotFound
	}
	if err := affected(res, err, zero); err != nil {
		return err
	}
	l.Version++
	return nil
}

func (r links) Delete(ctx context.Context, owner, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM links WHERE owner_id=? AND id=?", owner, id)
	return affected(res, err, ErrNotFound)
}

func (r links) ByTokenHash(ctx context.Context, hash string) (*Link, error) {
	return r.scan(r.d.row(ctx, "SELECT "+linkCols+" FROM links WHERE token_hash=?", hash))
}

func (r links) Accessed(ctx context.Context, id string, at time.Time, ua string) error {
	if len(ua) > 200 {
		ua = ua[:200]
	}
	_, err := r.d.exec(ctx, "UPDATE links SET last_access=?, last_ua=? WHERE id=?", ms(at), ua, id)
	return err
}

// —— 渲染 ——

type renders struct{ d *DB }

func (r renders) Get(ctx context.Context, owner, profileID string) (*Render, error) {
	var x Render
	var enc []byte
	var hdr string
	var gat, lat int64
	err := r.d.row(ctx, "SELECT profile_id, owner_id, good_key, good_yaml_enc, good_etag, good_at, good_headers, last_key, last_status, last_error, last_diags, last_at FROM renders WHERE owner_id=? AND profile_id=?", owner, profileID).
		Scan(&x.ProfileID, &x.OwnerID, &x.GoodKey, &enc, &x.GoodETag, &gat, &hdr, &x.LastKey, &x.LastStatus, &x.LastError, &x.LastDiags, &lat)
	if err != nil {
		return nil, notFound(err)
	}
	if x.GoodYAML, err = r.d.open(enc, "renders", "good_yaml", profileID); err != nil {
		return nil, err
	}
	x.GoodAt, x.LastAt, x.GoodHeaders = fromMS(gat), fromMS(lat), parseMap(hdr)
	return &x, nil
}

func (r renders) Put(ctx context.Context, owner string, x *Render) error {
	enc, err := r.d.seal(x.GoodYAML, "renders", "good_yaml", x.ProfileID)
	if err != nil {
		return err
	}
	diags := string(x.LastDiags)
	if diags == "" {
		diags = "[]"
	}
	res, err := r.d.exec(ctx, "UPDATE renders SET good_key=?, good_yaml_enc=?, good_etag=?, good_at=?, good_headers=?, last_key=?, last_status=?, last_error=?, last_diags=?, last_at=? WHERE owner_id=? AND profile_id=?",
		x.GoodKey, enc, x.GoodETag, ms(x.GoodAt), jsonMap(x.GoodHeaders), x.LastKey, x.LastStatus, x.LastError, diags, ms(x.LastAt), owner, x.ProfileID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.d.exec(ctx, "INSERT INTO renders (profile_id, owner_id, good_key, good_yaml_enc, good_etag, good_at, good_headers, last_key, last_status, last_error, last_diags, last_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)",
		x.ProfileID, owner, x.GoodKey, enc, x.GoodETag, ms(x.GoodAt), jsonMap(x.GoodHeaders), x.LastKey, x.LastStatus, x.LastError, diags, ms(x.LastAt))
	return err
}

// —— 审计与设置 ——

type audits struct{ d *DB }

func (r audits) Add(ctx context.Context, e *AuditEntry) error {
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	detail := string(e.Detail)
	if detail == "" {
		detail = "{}"
	}
	_, err := r.d.exec(ctx, "INSERT INTO audit (id, owner_id, at, action, detail) VALUES (?,?,?,?,?)", e.ID, e.OwnerID, ms(e.At), e.Action, detail)
	return err
}

func (r audits) List(ctx context.Context, owner string, p Page) ([]AuditEntry, error) {
	after := "ffffffff"
	if p.After != "" {
		after = p.After
	}
	return r.scan(r.d.query(ctx, "SELECT id, owner_id, at, action, detail FROM audit WHERE owner_id=? AND id < ? ORDER BY id DESC LIMIT ?", owner, after, limitOf(p)))
}

func (r audits) All(ctx context.Context, p Page) ([]AuditEntry, error) {
	after := "ffffffff"
	if p.After != "" {
		after = p.After
	}
	return r.scan(r.d.query(ctx, "SELECT id, owner_id, at, action, detail FROM audit WHERE id < ? ORDER BY id DESC LIMIT ?", after, limitOf(p)))
}

func (r audits) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.d.exec(ctx, "DELETE FROM audit WHERE at < ?", ms(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r audits) scan(rows *sql.Rows, err error) ([]AuditEntry, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var at int64
		var detail string
		if err := rows.Scan(&e.ID, &e.OwnerID, &at, &e.Action, &detail); err != nil {
			return nil, err
		}
		e.At, e.Detail = fromMS(at), []byte(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

type settings struct{ d *DB }

func (r settings) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := r.d.row(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&v)
	return v, notFound(err)
}

func (r settings) Set(ctx context.Context, key, value string) error {
	res, err := r.d.exec(ctx, "UPDATE settings SET value=? WHERE key=?", value, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.d.exec(ctx, "INSERT INTO settings (key, value) VALUES (?,?)", key, value)
	return err
}

// —— 邀请 ——

type invites struct{ d *DB }

func (r invites) Create(ctx context.Context, inv *Invite) error {
	if inv.ID == "" {
		inv.ID = NewID()
	}
	inv.CreatedAt = time.Now()
	_, err := r.d.exec(ctx, "INSERT INTO invites (id, token_hash, created_by, note, created_at, expires_at, used_at, used_by) VALUES (?,?,?,?,?,?,0,'')",
		inv.ID, inv.TokenHash, inv.CreatedBy, inv.Note, ms(inv.CreatedAt), ms(inv.ExpiresAt))
	return err
}

func (r invites) List(ctx context.Context, p Page) ([]Invite, error) {
	after := "ffffffff"
	if p.After != "" {
		after = p.After
	}
	rows, err := r.d.query(ctx, "SELECT id, token_hash, created_by, note, created_at, expires_at, used_at, used_by FROM invites WHERE id < ? ORDER BY id DESC LIMIT ?", after, limitOf(p))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Invite
	for rows.Next() {
		var v Invite
		var c, e, u int64
		if err := rows.Scan(&v.ID, &v.TokenHash, &v.CreatedBy, &v.Note, &c, &e, &u, &v.UsedBy); err != nil {
			return nil, err
		}
		v.CreatedAt, v.ExpiresAt, v.UsedAt = fromMS(c), fromMS(e), fromMS(u)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r invites) Delete(ctx context.Context, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM invites WHERE id=?", id)
	return affected(res, err, ErrNotFound)
}

func (r invites) Use(ctx context.Context, tokenHash, userID string, now time.Time) (*Invite, error) {
	n := ms(now)
	res, err := r.d.exec(ctx, "UPDATE invites SET used_at=?, used_by=? WHERE token_hash=? AND used_at=0 AND (expires_at=0 OR expires_at>?)", n, userID, tokenHash, n)
	if err := affected(res, err, ErrNotFound); err != nil {
		return nil, err
	}
	var v Invite
	var c, e, u int64
	err = r.d.row(ctx, "SELECT id, token_hash, created_by, note, created_at, expires_at, used_at, used_by FROM invites WHERE token_hash=?", tokenHash).
		Scan(&v.ID, &v.TokenHash, &v.CreatedBy, &v.Note, &c, &e, &u, &v.UsedBy)
	if err != nil {
		return nil, notFound(err)
	}
	v.CreatedAt, v.ExpiresAt, v.UsedAt = fromMS(c), fromMS(e), fromMS(u)
	return &v, nil
}
