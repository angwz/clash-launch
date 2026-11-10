-- Clash, Launch! 初始结构（PostgreSQL）。与 sqlite/0001_init.sql 逻辑结构相同。可移植约定见 docs/decisions/0002：
-- 主键为应用生成的 UUIDv7 文本；时间为 UTC 毫秒整数；布尔为 0/1 整数；JSON 为文本；
-- *_enc 列是 AES-256-GCM 密文（含密钥 ID），只能由应用解密。
CREATE TABLE users (
  id            TEXT    NOT NULL PRIMARY KEY,
  username      TEXT    NOT NULL,
  username_norm TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  role          TEXT    NOT NULL,
  disabled      BIGINT NOT NULL DEFAULT 0,
  lang          TEXT    NOT NULL DEFAULT '',
  timezone      TEXT    NOT NULL DEFAULT '',
  created_at    BIGINT NOT NULL,
  updated_at    BIGINT NOT NULL,
  version       BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE sessions (
  id_hash     TEXT    NOT NULL PRIMARY KEY,
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf        TEXT    NOT NULL,
  created_at  BIGINT NOT NULL,
  last_seen   BIGINT NOT NULL,
  expires_at  BIGINT NOT NULL,
  ua          TEXT    NOT NULL DEFAULT '',
  ip_prefix   TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE sources (
  id           TEXT    NOT NULL PRIMARY KEY,
  owner_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT    NOT NULL,
  kind         TEXT    NOT NULL,
  url_enc      BYTEA,
  content_enc  BYTEA,
  headers_enc  BYTEA,
  ua           TEXT    NOT NULL DEFAULT '',
  interval_s   BIGINT NOT NULL DEFAULT 0,
  processing   TEXT    NOT NULL DEFAULT '{}',
  created_at   BIGINT NOT NULL,
  updated_at   BIGINT NOT NULL,
  version      BIGINT NOT NULL DEFAULT 1
);
CREATE INDEX sources_owner ON sources(owner_id);

CREATE TABLE rule_sources (
  id            TEXT    NOT NULL PRIMARY KEY,
  owner_id      TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name          TEXT    NOT NULL,
  kind          TEXT    NOT NULL,
  preset        TEXT    NOT NULL DEFAULT '',
  groups_url    TEXT    NOT NULL DEFAULT '',
  rulesets_url  TEXT    NOT NULL DEFAULT '',
  ini_url       TEXT    NOT NULL DEFAULT '',
  groups_text   TEXT    NOT NULL DEFAULT '',
  rulesets_text TEXT    NOT NULL DEFAULT '',
  ini_text      TEXT    NOT NULL DEFAULT '',
  interval_s    BIGINT NOT NULL DEFAULT 0,
  created_at    BIGINT NOT NULL,
  updated_at    BIGINT NOT NULL,
  version       BIGINT NOT NULL DEFAULT 1
);
CREATE INDEX rule_sources_owner ON rule_sources(owner_id);

CREATE TABLE snapshots (
  key           TEXT    NOT NULL PRIMARY KEY,
  owner_id      TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content_enc   BYTEA,
  hash          TEXT    NOT NULL DEFAULT '',
  headers       TEXT    NOT NULL DEFAULT '{}',
  fetched_at    BIGINT NOT NULL DEFAULT 0,
  via           TEXT    NOT NULL DEFAULT '',
  last_error    TEXT    NOT NULL DEFAULT '',
  last_attempt  BIGINT NOT NULL DEFAULT 0,
  failing_since BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX snapshots_owner ON snapshots(owner_id);

CREATE TABLE profiles (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  doc_enc    BYTEA    NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  version    BIGINT NOT NULL DEFAULT 1
);
CREATE INDEX profiles_owner ON profiles(owner_id);

CREATE TABLE revisions (
  id         TEXT    NOT NULL PRIMARY KEY,
  profile_id TEXT    NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  n          BIGINT NOT NULL,
  doc_enc    BYTEA    NOT NULL,
  note       TEXT    NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  UNIQUE (profile_id, n)
);

CREATE TABLE links (
  id          TEXT    NOT NULL PRIMARY KEY,
  owner_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  profile_id  TEXT    NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  token_enc   BYTEA    NOT NULL,
  token_hash  TEXT    NOT NULL UNIQUE,
  note        TEXT    NOT NULL DEFAULT '',
  created_at  BIGINT NOT NULL,
  last_access BIGINT NOT NULL DEFAULT 0,
  last_ua     TEXT    NOT NULL DEFAULT '',
  version     BIGINT NOT NULL DEFAULT 1
);
CREATE INDEX links_profile ON links(profile_id);

CREATE TABLE renders (
  profile_id    TEXT    NOT NULL PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
  owner_id      TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  good_key      TEXT    NOT NULL DEFAULT '',
  good_yaml_enc BYTEA,
  good_etag     TEXT    NOT NULL DEFAULT '',
  good_at       BIGINT NOT NULL DEFAULT 0,
  good_headers  TEXT    NOT NULL DEFAULT '{}',
  last_key      TEXT    NOT NULL DEFAULT '',
  last_status   TEXT    NOT NULL DEFAULT '',
  last_error    TEXT    NOT NULL DEFAULT '',
  last_diags    TEXT    NOT NULL DEFAULT '[]',
  last_at       BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE audit (
  id       TEXT    NOT NULL PRIMARY KEY,
  owner_id TEXT    NOT NULL DEFAULT '',
  at       BIGINT NOT NULL,
  action   TEXT    NOT NULL,
  detail   TEXT    NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_owner ON audit(owner_id, at);

CREATE TABLE settings (
  key   TEXT NOT NULL PRIMARY KEY,
  value TEXT NOT NULL
);
