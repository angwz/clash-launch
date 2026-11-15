-- 订阅分析、短链接与站内消息（反馈 #74）。

-- 请求的归属地：记录时用本机地理数据库查出（internal/geo），之后不再变化。
-- country 为 ISO 3166-1 两位代码；asn 形如 AS4134；org 为运营商名。short_id 非空表示经短链接访问。
ALTER TABLE sub_requests ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE sub_requests ADD COLUMN asn TEXT NOT NULL DEFAULT '';
ALTER TABLE sub_requests ADD COLUMN org TEXT NOT NULL DEFAULT '';
ALTER TABLE sub_requests ADD COLUMN short_id TEXT NOT NULL DEFAULT '';
ALTER TABLE sub_ips ADD COLUMN country TEXT NOT NULL DEFAULT '';
ALTER TABLE sub_ips ADD COLUMN org TEXT NOT NULL DEFAULT '';

-- 短链接：/u/<code> 直接提供所指配置链接的内容。code 为小写，全站唯一。
-- expires_at 为 0 表示不过期。删除配置链接时一并删除。
CREATE TABLE short_links (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  link_id    TEXT    NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  code       TEXT    NOT NULL UNIQUE,
  note       TEXT    NOT NULL DEFAULT '',
  disabled   INTEGER NOT NULL DEFAULT 0,
  expires_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  version    INTEGER NOT NULL DEFAULT 1
) STRICT;
CREATE INDEX short_links_owner ON short_links(owner_id, created_at);
CREATE INDEX short_links_link ON short_links(link_id);

-- 站内消息：正文与附件（节点或订阅）加密保存在 body_enc。
-- 收发双方各自「删除」只是隐藏；added_source 是收件人节点页中由附件加入的源 ID。
CREATE TABLE messages (
  id               TEXT    NOT NULL PRIMARY KEY,
  sender_id        TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_id     TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind             TEXT    NOT NULL DEFAULT 'text',
  body_enc         BLOB    NOT NULL,
  created_at       INTEGER NOT NULL,
  read_at          INTEGER NOT NULL DEFAULT 0,
  sender_hidden    INTEGER NOT NULL DEFAULT 0,
  recipient_hidden INTEGER NOT NULL DEFAULT 0,
  added_source     TEXT    NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX messages_recipient ON messages(recipient_id, created_at);
CREATE INDEX messages_sender ON messages(sender_id, created_at);
