-- 一次性邀请（PostgreSQL；与 sqlite/0002_invites.sql 逻辑结构相同。docs/decisions/0003）：注册关闭时，管理员生成邀请链接发给别人注册。
-- 只存令牌的 SHA-256；令牌原文只在创建时返回一次。expires_at 为 0 表示不过期；used_at 非 0 表示已用。
CREATE TABLE invites (
  id         TEXT    NOT NULL PRIMARY KEY,
  token_hash TEXT    NOT NULL UNIQUE,
  created_by TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  note       TEXT    NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  expires_at BIGINT NOT NULL DEFAULT 0,
  used_at    BIGINT NOT NULL DEFAULT 0,
  used_by    TEXT    NOT NULL DEFAULT ''
);
