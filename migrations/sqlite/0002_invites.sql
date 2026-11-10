-- 一次性邀请（docs/decisions/0003）：注册关闭时，管理员生成邀请链接发给别人注册。
-- 只存令牌的 SHA-256；令牌原文只在创建时返回一次。expires_at 为 0 表示不过期；used_at 非 0 表示已用。
CREATE TABLE invites (
  id         TEXT    NOT NULL PRIMARY KEY,
  token_hash TEXT    NOT NULL UNIQUE,
  created_by TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  note       TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL DEFAULT 0,
  used_at    INTEGER NOT NULL DEFAULT 0,
  used_by    TEXT    NOT NULL DEFAULT ''
) STRICT;
