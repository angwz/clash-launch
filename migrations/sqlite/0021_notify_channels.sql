-- 提醒渠道（需求 R31，决定 0008）：Webhook、Telegram、Bark、ntfy。设置整体加密存储在 config_enc；
-- 测试消息送达后 enabled 才为 1，修改设置回到未验证。last_at、last_error 是最近一次发送的结果。
CREATE TABLE notify_channels (
  id          TEXT    NOT NULL PRIMARY KEY,
  owner_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind        TEXT    NOT NULL,
  name        TEXT    NOT NULL,
  config_enc  BLOB,
  enabled     INTEGER NOT NULL DEFAULT 0,
  verified_at INTEGER NOT NULL DEFAULT 0,
  last_at     INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT    NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL
) STRICT;
CREATE INDEX notify_channels_owner ON notify_channels(owner_id, created_at);
