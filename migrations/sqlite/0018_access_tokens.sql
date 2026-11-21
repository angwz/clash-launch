-- 个人访问令牌（需求 R25）：供脚本调用 API。令牌只以 SHA-256 哈希保存，明文只在创建时显示一次。
-- scope 为 read（只允许读取）或 write；expires_at 为 0 表示不过期；last_used_at 至多每分钟更新一次。
CREATE TABLE access_tokens (
  id           TEXT    NOT NULL PRIMARY KEY,
  owner_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT    NOT NULL,
  token_hash   TEXT    NOT NULL UNIQUE,
  scope        TEXT    NOT NULL,
  created_at   INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  last_ip      TEXT    NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX access_tokens_owner ON access_tokens(owner_id, created_at);
