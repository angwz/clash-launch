-- 账号安全（PostgreSQL；与 sqlite/0003_security.sql 逻辑结构相同）。说明见 SQLite 版本。

CREATE TABLE user_questions (
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idx         INTEGER NOT NULL,
  question    TEXT    NOT NULL,
  answer_hash TEXT    NOT NULL,
  PRIMARY KEY (user_id, idx)
);

CREATE TABLE user_totp (
  user_id    TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  secret_enc BYTEA   NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 0,
  last_step  BIGINT  NOT NULL DEFAULT 0,
  created_at BIGINT  NOT NULL
);

CREATE TABLE recovery_codes (
  user_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash TEXT    NOT NULL,
  used_at   BIGINT  NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, code_hash)
);

CREATE TABLE passkeys (
  id            TEXT    NOT NULL PRIMARY KEY,
  user_id       TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  credential_id TEXT    NOT NULL UNIQUE,
  public_key    BYTEA   NOT NULL,
  sign_count    BIGINT  NOT NULL DEFAULT 0,
  name          TEXT    NOT NULL DEFAULT '',
  created_at    BIGINT  NOT NULL,
  last_used     BIGINT  NOT NULL DEFAULT 0
);
CREATE INDEX passkeys_user ON passkeys(user_id);

CREATE TABLE user_smtp (
  user_id      TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  host         TEXT    NOT NULL,
  port         INTEGER NOT NULL,
  security     TEXT    NOT NULL,
  username     TEXT    NOT NULL DEFAULT '',
  password_enc BYTEA,
  from_addr    TEXT    NOT NULL,
  to_addr      TEXT    NOT NULL,
  enabled      INTEGER NOT NULL DEFAULT 0,
  verified_at  BIGINT  NOT NULL DEFAULT 0
);

CREATE TABLE user_quotas (
  user_id       TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  sources       INTEGER NOT NULL DEFAULT -1,
  profiles      INTEGER NOT NULL DEFAULT -1,
  links         INTEGER NOT NULL DEFAULT -1,
  min_interval  INTEGER NOT NULL DEFAULT -1,
  storage_bytes BIGINT  NOT NULL DEFAULT -1
);

CREATE TABLE known_devices (
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  fingerprint TEXT    NOT NULL,
  first_seen  BIGINT  NOT NULL,
  last_seen   BIGINT  NOT NULL,
  PRIMARY KEY (user_id, fingerprint)
);
