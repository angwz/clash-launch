-- 站内消息的端到端加密（需求 R46，决定 0009）：每个用户一对 ECDH P-256 密钥。
-- public_key 是公钥（SPKI，Base64URL），任何登录用户可读；wrapped 是用登录密码派生的密钥包裹的私钥（JSON），
-- recovery 是用恢复密钥包裹的另一份（可为空）。重新生成时旧行保留、active 置 0：对方仍能解开以前的消息。
-- 注销账户时清空 wrapped 与 recovery，公钥保留（对话保留，R20）。
CREATE TABLE message_keys (
  user_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key_id     TEXT    NOT NULL,
  public_key TEXT    NOT NULL,
  wrapped    TEXT    NOT NULL DEFAULT '',
  recovery   TEXT    NOT NULL DEFAULT '',
  active     INTEGER NOT NULL DEFAULT 1,
  created_at BIGINT  NOT NULL,
  PRIMARY KEY (user_id, key_id)
);
