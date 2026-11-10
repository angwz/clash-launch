-- Clash, Launch! 初始结构（SQLite）。可移植约定见 docs/decisions/0002：
-- 主键为应用生成的 UUIDv7 文本；时间为 UTC 毫秒整数；布尔为 0/1 整数；JSON 为文本；
-- *_enc 列是 AES-256-GCM 密文（含密钥 ID），只能由应用解密。
CREATE TABLE users (
  id            TEXT    NOT NULL PRIMARY KEY,
  username      TEXT    NOT NULL,
  username_norm TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  role          TEXT    NOT NULL,
  disabled      INTEGER NOT NULL DEFAULT 0,
  lang          TEXT    NOT NULL DEFAULT '',
  timezone      TEXT    NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  version       INTEGER NOT NULL DEFAULT 1
) STRICT;

CREATE TABLE sessions (
  id_hash     TEXT    NOT NULL PRIMARY KEY,
  user_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf        TEXT    NOT NULL,
  created_at  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  ua          TEXT    NOT NULL DEFAULT '',
  ip_prefix   TEXT    NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE sources (
  id           TEXT    NOT NULL PRIMARY KEY,
  owner_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT    NOT NULL,
  kind         TEXT    NOT NULL,
  url_enc      BLOB,
  content_enc  BLOB,
  headers_enc  BLOB,
  ua           TEXT    NOT NULL DEFAULT '',
  interval_s   INTEGER NOT NULL DEFAULT 0,
  processing   TEXT    NOT NULL DEFAULT '{}',
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  version      INTEGER NOT NULL DEFAULT 1
) STRICT;
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
  interval_s    INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL,
  version       INTEGER NOT NULL DEFAULT 1
) STRICT;
CREATE INDEX rule_sources_owner ON rule_sources(owner_id);

CREATE TABLE snapshots (
  key           TEXT    NOT NULL PRIMARY KEY,
  owner_id      TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content_enc   BLOB,
  hash          TEXT    NOT NULL DEFAULT '',
  headers       TEXT    NOT NULL DEFAULT '{}',
  fetched_at    INTEGER NOT NULL DEFAULT 0,
  via           TEXT    NOT NULL DEFAULT '',
  last_error    TEXT    NOT NULL DEFAULT '',
  last_attempt  INTEGER NOT NULL DEFAULT 0,
  failing_since INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX snapshots_owner ON snapshots(owner_id);

CREATE TABLE profiles (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  doc_enc    BLOB    NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  version    INTEGER NOT NULL DEFAULT 1
) STRICT;
CREATE INDEX profiles_owner ON profiles(owner_id);

CREATE TABLE revisions (
  id         TEXT    NOT NULL PRIMARY KEY,
  profile_id TEXT    NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  n          INTEGER NOT NULL,
  doc_enc    BLOB    NOT NULL,
  note       TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (profile_id, n)
) STRICT;

CREATE TABLE links (
  id          TEXT    NOT NULL PRIMARY KEY,
  owner_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  profile_id  TEXT    NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
  token_enc   BLOB    NOT NULL,
  token_hash  TEXT    NOT NULL UNIQUE,
  note        TEXT    NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  last_access INTEGER NOT NULL DEFAULT 0,
  last_ua     TEXT    NOT NULL DEFAULT '',
  version     INTEGER NOT NULL DEFAULT 1
) STRICT;
CREATE INDEX links_profile ON links(profile_id);

CREATE TABLE renders (
  profile_id    TEXT    NOT NULL PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
  owner_id      TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  good_key      TEXT    NOT NULL DEFAULT '',
  good_yaml_enc BLOB,
  good_etag     TEXT    NOT NULL DEFAULT '',
  good_at       INTEGER NOT NULL DEFAULT 0,
  good_headers  TEXT    NOT NULL DEFAULT '{}',
  last_key      TEXT    NOT NULL DEFAULT '',
  last_status   TEXT    NOT NULL DEFAULT '',
  last_error    TEXT    NOT NULL DEFAULT '',
  last_diags    TEXT    NOT NULL DEFAULT '[]',
  last_at       INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE audit (
  id       TEXT    NOT NULL PRIMARY KEY,
  owner_id TEXT    NOT NULL DEFAULT '',
  at       INTEGER NOT NULL,
  action   TEXT    NOT NULL,
  detail   TEXT    NOT NULL DEFAULT '{}'
) STRICT;
CREATE INDEX audit_owner ON audit(owner_id, at);

CREATE TABLE settings (
  key   TEXT NOT NULL PRIMARY KEY,
  value TEXT NOT NULL
) STRICT;
