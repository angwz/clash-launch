-- 还原前的自动留底（需求 R21）：整体覆盖前为当前状态做一份备份，保存 7 天，可以一键撤销这次还原。
-- 每个用户每种 kind 最多一份；data_enc 用服务器主密钥加密（与其他加密列一样参与密钥轮换）。
CREATE TABLE user_backups (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL,
  created_at BIGINT  NOT NULL,
  expires_at BIGINT  NOT NULL,
  data_enc   BYTEA,
  UNIQUE (owner_id, kind)
);
