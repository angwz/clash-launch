-- 账号安全（第 14.1、14.7 节，M5）：安全问题、TOTP 与恢复码、通行密钥、个人 SMTP 提醒、配额、已知设备。
-- 加密列（*_enc）登记在 internal/store/crypto.go 的 encrypted 中，参与密钥轮换。

-- 安全问题：question 为 b:<内置编号> 或 c:<自定义文字>；答案规范化后以 argon2id 存储。
CREATE TABLE user_questions (
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idx         INTEGER NOT NULL,
  question    TEXT    NOT NULL,
  answer_hash TEXT    NOT NULL,
  PRIMARY KEY (user_id, idx)
) STRICT;

-- TOTP：enabled 为 0 时是尚未确认的待启用密钥；last_step 防止同一时间步的验证码被重放。
CREATE TABLE user_totp (
  user_id    TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  secret_enc BLOB    NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 0,
  last_step  INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
) STRICT;

-- 一次性恢复码：只存 SHA-256；used_at 非 0 表示已用。
CREATE TABLE recovery_codes (
  user_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash TEXT    NOT NULL,
  used_at   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, code_hash)
) STRICT;

-- 通行密钥（WebAuthn）：public_key 为 COSE 编码；credential_id 为 base64url。
CREATE TABLE passkeys (
  id            TEXT    NOT NULL PRIMARY KEY,
  user_id       TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id TEXT    NOT NULL UNIQUE,
  public_key    BLOB    NOT NULL,
  sign_count    INTEGER NOT NULL DEFAULT 0,
  name          TEXT    NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  last_used     INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX passkeys_user ON passkeys(user_id);

-- 个人 SMTP 提醒：security 为 tls（465）或 starttls（25/587）；verified_at 为测试邮件确认的时间。
CREATE TABLE user_smtp (
  user_id      TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  host         TEXT    NOT NULL,
  port         INTEGER NOT NULL,
  security     TEXT    NOT NULL,
  username     TEXT    NOT NULL DEFAULT '',
  password_enc BLOB,
  from_addr    TEXT    NOT NULL,
  to_addr      TEXT    NOT NULL,
  enabled      INTEGER NOT NULL DEFAULT 0,
  verified_at  INTEGER NOT NULL DEFAULT 0
) STRICT;

-- 配额：-1 表示使用全局默认值。
CREATE TABLE user_quotas (
  user_id       TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  sources       INTEGER NOT NULL DEFAULT -1,
  profiles      INTEGER NOT NULL DEFAULT -1,
  links         INTEGER NOT NULL DEFAULT -1,
  min_interval  INTEGER NOT NULL DEFAULT -1,
  storage_bytes INTEGER NOT NULL DEFAULT -1
) STRICT;

-- 已知设备：fingerprint 为「IP 前缀 + 浏览器族」的哈希；首次出现时发送新设备登录提醒。
CREATE TABLE known_devices (
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  fingerprint TEXT    NOT NULL,
  first_seen  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL,
  PRIMARY KEY (user_id, fingerprint)
) STRICT;
