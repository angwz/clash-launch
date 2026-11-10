package store

import (
	"context"
	"errors"
	"time"
)

// 账号安全相关的仓储（第 14.1、14.7 节）。这些数据都属于某个用户本人；
// 方法以用户 ID 为第一个参数（在 ctx 之后），与其他仓储的所有者约定一致。
// 例外：Passkeys().ByCredential 供免密码登录查找凭据（登录前还不知道用户）。

// Question 是一个已保存的安全问题；Question 为 b:<编号> 或 c:<文字>，AnswerHash 为 argon2id。
type Question struct {
	Idx        int
	Question   string
	AnswerHash string
}

// TOTP 是两步验证的密钥；Enabled 为 false 时是尚未确认的待启用密钥。
type TOTP struct {
	UserID    string
	Secret    string // base32，加密存储
	Enabled   bool
	LastStep  int64
	CreatedAt time.Time
}

// Passkey 是一个 WebAuthn 凭据。
type Passkey struct {
	ID           string
	UserID       string
	CredentialID string // base64url
	PublicKey    []byte // COSE_Key
	SignCount    uint32
	Name         string
	CreatedAt    time.Time
	LastUsed     time.Time
}

// SMTP 是用户本人的提醒邮件设置。
type SMTP struct {
	UserID     string
	Host       string
	Port       int
	Security   string // tls | starttls
	Username   string
	Password   string // 加密存储
	From       string
	To         string
	Enabled    bool
	VerifiedAt time.Time
}

// Quota 是用户的配额；-1 表示使用全局默认值。MinInterval 单位为秒。
type Quota struct {
	UserID      string `json:"-"`
	Sources     int    `json:"sources"`
	Profiles    int    `json:"profiles"`
	Links       int    `json:"links"`
	MinInterval int    `json:"minInterval"`
	Storage     int64  `json:"storage"`
}

// Usage 是用户当前的用量。Bytes 为加密后保存的源内容、快照、方案文档与版本的总字节数。
type Usage struct {
	Sources  int   `json:"sources"`
	Profiles int   `json:"profiles"`
	Links    int   `json:"links"`
	Bytes    int64 `json:"bytes"`
}

// QuestionRepo 管理安全问题。
type QuestionRepo interface {
	// Set 整体替换用户的安全问题。
	Set(ctx context.Context, userID string, qs []Question) error
	List(ctx context.Context, userID string) ([]Question, error)
}

// TOTPRepo 管理两步验证密钥与恢复码。
type TOTPRepo interface {
	Get(ctx context.Context, userID string) (*TOTP, error)
	Put(ctx context.Context, t *TOTP) error
	Delete(ctx context.Context, userID string) error
	// Advance 原子地把 last_step 推进到 step；step 不大于已用的时间步时返回 ErrConflict（重放）。
	Advance(ctx context.Context, userID string, step int64) error
	// ReplaceCodes 用新的恢复码（SHA-256）替换全部旧码。
	ReplaceCodes(ctx context.Context, userID string, hashes []string) error
	// UseCode 原子地标记一个未用的恢复码；不存在或已用时返回 ErrNotFound。
	UseCode(ctx context.Context, userID, hash string, now time.Time) error
	RemainingCodes(ctx context.Context, userID string) (int, error)
}

// PasskeyRepo 管理通行密钥。
type PasskeyRepo interface {
	Create(ctx context.Context, k *Passkey) error
	List(ctx context.Context, userID string) ([]Passkey, error)
	ByCredential(ctx context.Context, credentialID string) (*Passkey, error)
	// Used 更新签名计数与最近使用时间。
	Used(ctx context.Context, id string, signCount uint32, now time.Time) error
	Rename(ctx context.Context, userID, id, name string) error
	Delete(ctx context.Context, userID, id string) error
}

// SMTPRepo 管理提醒邮件设置。
type SMTPRepo interface {
	Get(ctx context.Context, userID string) (*SMTP, error)
	Put(ctx context.Context, s *SMTP) error
	Delete(ctx context.Context, userID string) error
}

// QuotaRepo 管理配额与用量。
type QuotaRepo interface {
	// Get 返回用户的配额；没有单独设置时返回全部为 -1 的配额。
	Get(ctx context.Context, userID string) (*Quota, error)
	Put(ctx context.Context, q *Quota) error
	Usage(ctx context.Context, userID string) (*Usage, error)
}

// DeviceRepo 记录已知设备。
type DeviceRepo interface {
	// Seen 记录一次登录；设备第一次出现时返回 true。
	Seen(ctx context.Context, userID, fingerprint string, now time.Time) (bool, error)
}

func (d *DB) Questions() QuestionRepo { return questions{d} }
func (d *DB) TOTP() TOTPRepo          { return totps{d} }
func (d *DB) Passkeys() PasskeyRepo   { return passkeys{d} }
func (d *DB) SMTP() SMTPRepo          { return smtps{d} }
func (d *DB) Quotas() QuotaRepo       { return quotas{d} }
func (d *DB) Devices() DeviceRepo     { return devices{d} }

// —— 安全问题 ——

type questions struct{ d *DB }

func (r questions) Set(ctx context.Context, userID string, qs []Question) error {
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM user_questions WHERE user_id=?", userID); err != nil {
			return err
		}
		for _, q := range qs {
			if _, err := tx.exec(ctx, "INSERT INTO user_questions (user_id, idx, question, answer_hash) VALUES (?,?,?,?)", userID, q.Idx, q.Question, q.AnswerHash); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r questions) List(ctx context.Context, userID string) ([]Question, error) {
	rows, err := r.d.query(ctx, "SELECT idx, question, answer_hash FROM user_questions WHERE user_id=? ORDER BY idx", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Question
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.Idx, &q.Question, &q.AnswerHash); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// —— TOTP 与恢复码 ——

type totps struct{ d *DB }

func (r totps) Get(ctx context.Context, userID string) (*TOTP, error) {
	var t TOTP
	var enc []byte
	var en int
	var created int64
	err := r.d.row(ctx, "SELECT user_id, secret_enc, enabled, last_step, created_at FROM user_totp WHERE user_id=?", userID).Scan(&t.UserID, &enc, &en, &t.LastStep, &created)
	if err != nil {
		return nil, notFound(err)
	}
	if t.Secret, err = r.d.openStr(enc, "user_totp", "secret", userID); err != nil {
		return nil, err
	}
	t.Enabled, t.CreatedAt = en != 0, fromMS(created)
	return &t, nil
}

func (r totps) Put(ctx context.Context, t *TOTP) error {
	enc, err := r.d.sealStr(t.Secret, "user_totp", "secret", t.UserID)
	if err != nil {
		return err
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM user_totp WHERE user_id=?", t.UserID); err != nil {
			return err
		}
		_, err := tx.exec(ctx, "INSERT INTO user_totp (user_id, secret_enc, enabled, last_step, created_at) VALUES (?,?,?,?,?)", t.UserID, enc, b2i(t.Enabled), t.LastStep, ms(t.CreatedAt))
		return err
	})
}

func (r totps) Delete(ctx context.Context, userID string) error {
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM recovery_codes WHERE user_id=?", userID); err != nil {
			return err
		}
		_, err := tx.exec(ctx, "DELETE FROM user_totp WHERE user_id=?", userID)
		return err
	})
}

func (r totps) Advance(ctx context.Context, userID string, step int64) error {
	res, err := r.d.exec(ctx, "UPDATE user_totp SET last_step=? WHERE user_id=? AND last_step<?", step, userID, step)
	return affected(res, err, ErrConflict)
}

func (r totps) ReplaceCodes(ctx context.Context, userID string, hashes []string) error {
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM recovery_codes WHERE user_id=?", userID); err != nil {
			return err
		}
		for _, h := range hashes {
			if _, err := tx.exec(ctx, "INSERT INTO recovery_codes (user_id, code_hash, used_at) VALUES (?,?,0)", userID, h); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r totps) UseCode(ctx context.Context, userID, hash string, now time.Time) error {
	res, err := r.d.exec(ctx, "UPDATE recovery_codes SET used_at=? WHERE user_id=? AND code_hash=? AND used_at=0", ms(now), userID, hash)
	return affected(res, err, ErrNotFound)
}

func (r totps) RemainingCodes(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.d.row(ctx, "SELECT COUNT(*) FROM recovery_codes WHERE user_id=? AND used_at=0", userID).Scan(&n)
	return n, err
}

// —— 通行密钥 ——

type passkeys struct{ d *DB }

const passkeyCols = "id, user_id, credential_id, public_key, sign_count, name, created_at, last_used" //nolint:gosec // 列名列表，不是凭据

func (r passkeys) Create(ctx context.Context, k *Passkey) error {
	if k.ID == "" {
		k.ID = NewID()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
	}
	_, err := r.d.exec(ctx, "INSERT INTO passkeys ("+passkeyCols+") VALUES (?,?,?,?,?,?,?,0)", k.ID, k.UserID, k.CredentialID, k.PublicKey, int64(k.SignCount), k.Name, ms(k.CreatedAt))
	if err != nil && isUnique(err) {
		return ErrExists
	}
	return err
}

func (r passkeys) scan(sc interface{ Scan(...any) error }) (*Passkey, error) {
	var k Passkey
	var cnt, created, used int64
	if err := sc.Scan(&k.ID, &k.UserID, &k.CredentialID, &k.PublicKey, &cnt, &k.Name, &created, &used); err != nil {
		return nil, notFound(err)
	}
	k.SignCount, k.CreatedAt, k.LastUsed = uint32(cnt), fromMS(created), fromMS(used) //nolint:gosec // 计数来自 uint32
	return &k, nil
}

func (r passkeys) List(ctx context.Context, userID string) ([]Passkey, error) {
	rows, err := r.d.query(ctx, "SELECT "+passkeyCols+" FROM passkeys WHERE user_id=? ORDER BY id", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Passkey
	for rows.Next() {
		k, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

func (r passkeys) ByCredential(ctx context.Context, credentialID string) (*Passkey, error) {
	return r.scan(r.d.row(ctx, "SELECT "+passkeyCols+" FROM passkeys WHERE credential_id=?", credentialID))
}

func (r passkeys) Used(ctx context.Context, id string, signCount uint32, now time.Time) error {
	res, err := r.d.exec(ctx, "UPDATE passkeys SET sign_count=?, last_used=? WHERE id=?", int64(signCount), ms(now), id)
	return affected(res, err, ErrNotFound)
}

func (r passkeys) Rename(ctx context.Context, userID, id, name string) error {
	res, err := r.d.exec(ctx, "UPDATE passkeys SET name=? WHERE user_id=? AND id=?", name, userID, id)
	return affected(res, err, ErrNotFound)
}

func (r passkeys) Delete(ctx context.Context, userID, id string) error {
	res, err := r.d.exec(ctx, "DELETE FROM passkeys WHERE user_id=? AND id=?", userID, id)
	return affected(res, err, ErrNotFound)
}

// —— SMTP ——

type smtps struct{ d *DB }

func (r smtps) Get(ctx context.Context, userID string) (*SMTP, error) {
	var s SMTP
	var enc []byte
	var en int
	var verified int64
	err := r.d.row(ctx, "SELECT user_id, host, port, security, username, password_enc, from_addr, to_addr, enabled, verified_at FROM user_smtp WHERE user_id=?", userID).
		Scan(&s.UserID, &s.Host, &s.Port, &s.Security, &s.Username, &enc, &s.From, &s.To, &en, &verified)
	if err != nil {
		return nil, notFound(err)
	}
	if s.Password, err = r.d.openStr(enc, "user_smtp", "password", userID); err != nil {
		return nil, err
	}
	s.Enabled, s.VerifiedAt = en != 0, fromMS(verified)
	return &s, nil
}

func (r smtps) Put(ctx context.Context, s *SMTP) error {
	enc, err := r.d.sealStr(s.Password, "user_smtp", "password", s.UserID)
	if err != nil {
		return err
	}
	return r.d.Tx(ctx, func(st Store) error {
		tx := st.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM user_smtp WHERE user_id=?", s.UserID); err != nil {
			return err
		}
		_, err := tx.exec(ctx, "INSERT INTO user_smtp (user_id, host, port, security, username, password_enc, from_addr, to_addr, enabled, verified_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
			s.UserID, s.Host, s.Port, s.Security, s.Username, enc, s.From, s.To, b2i(s.Enabled), ms(s.VerifiedAt))
		return err
	})
}

func (r smtps) Delete(ctx context.Context, userID string) error {
	_, err := r.d.exec(ctx, "DELETE FROM user_smtp WHERE user_id=?", userID)
	return err
}

// —— 配额与用量 ——

type quotas struct{ d *DB }

func (r quotas) Get(ctx context.Context, userID string) (*Quota, error) {
	q := Quota{UserID: userID, Sources: -1, Profiles: -1, Links: -1, MinInterval: -1, Storage: -1}
	err := r.d.row(ctx, "SELECT sources, profiles, links, min_interval, storage_bytes FROM user_quotas WHERE user_id=?", userID).
		Scan(&q.Sources, &q.Profiles, &q.Links, &q.MinInterval, &q.Storage)
	if err != nil && !errors.Is(notFound(err), ErrNotFound) {
		return nil, err
	}
	return &q, nil
}

func (r quotas) Put(ctx context.Context, q *Quota) error {
	return r.d.Tx(ctx, func(s Store) error {
		tx := s.(*DB)
		if _, err := tx.exec(ctx, "DELETE FROM user_quotas WHERE user_id=?", q.UserID); err != nil {
			return err
		}
		_, err := tx.exec(ctx, "INSERT INTO user_quotas (user_id, sources, profiles, links, min_interval, storage_bytes) VALUES (?,?,?,?,?,?)", q.UserID, q.Sources, q.Profiles, q.Links, q.MinInterval, q.Storage)
		return err
	})
}

func (r quotas) Usage(ctx context.Context, userID string) (*Usage, error) {
	var u Usage
	err := r.d.row(ctx, `SELECT
		(SELECT COUNT(*) FROM sources WHERE owner_id=?),
		(SELECT COUNT(*) FROM profiles WHERE owner_id=?),
		(SELECT COUNT(*) FROM links WHERE owner_id=?),
		(SELECT COALESCE(SUM(LENGTH(content_enc)),0) FROM sources WHERE owner_id=?) +
		(SELECT COALESCE(SUM(LENGTH(content_enc)),0) FROM snapshots WHERE owner_id=?) +
		(SELECT COALESCE(SUM(LENGTH(doc_enc)),0) FROM profiles WHERE owner_id=?) +
		(SELECT COALESCE(SUM(LENGTH(doc_enc)),0) FROM revisions WHERE owner_id=?)`,
		userID, userID, userID, userID, userID, userID, userID).Scan(&u.Sources, &u.Profiles, &u.Links, &u.Bytes)
	return &u, err
}

// —— 已知设备 ——

type devices struct{ d *DB }

func (r devices) Seen(ctx context.Context, userID, fingerprint string, now time.Time) (bool, error) {
	res, err := r.d.exec(ctx, "UPDATE known_devices SET last_seen=? WHERE user_id=? AND fingerprint=?", ms(now), userID, fingerprint)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return false, nil
	}
	_, err = r.d.exec(ctx, "INSERT INTO known_devices (user_id, fingerprint, first_seen, last_seen) VALUES (?,?,?,?)", userID, fingerprint, ms(now), ms(now))
	if err != nil && isUnique(err) {
		return false, nil // 并发的同一次登录已经插入
	}
	return err == nil, err
}

// KeyStatus 返回当前主密钥 ID 与密钥环中的密钥数。
func (d *DB) KeyStatus() (string, int) { return d.keys.Current(), len(d.keys.IDs()) }
