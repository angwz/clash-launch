-- 站内提醒（需求 R23、R24）：节点源连续拉取失败、配置在后台重新生成时检查不通过、订阅链接被新的国家或客户端拉取。
-- dedup 让同一件事只提醒一次（例如同一段连续失败），重复插入时忽略；detail 是 JSON（名称、国家、客户端等）。
CREATE TABLE notifications (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL,
  dedup      TEXT    NOT NULL,
  detail     TEXT    NOT NULL DEFAULT '{}',
  created_at BIGINT NOT NULL,
  read_at    BIGINT NOT NULL DEFAULT 0,
  UNIQUE (owner_id, dedup)
);
CREATE INDEX notifications_owner ON notifications(owner_id, created_at);

-- 订阅链接见过的国家与客户端（需求 R23，永久保留）：第一次出现新的值时提醒。
-- 从按 IP 的汇总补齐已有的记录，上线后不会把老用户的常用地区当成新出现的。
CREATE TABLE link_seen (
  link_id  TEXT    NOT NULL,
  owner_id TEXT    NOT NULL,
  kind     TEXT    NOT NULL,
  value    TEXT    NOT NULL,
  first_at BIGINT NOT NULL,
  PRIMARY KEY (link_id, kind, value)
);
INSERT INTO link_seen (link_id, owner_id, kind, value, first_at)
  SELECT link_id, MIN(owner_id), 'country', country, MIN(first_at) FROM sub_ips WHERE link_id <> '' AND country <> '' GROUP BY link_id, country;
INSERT INTO link_seen (link_id, owner_id, kind, value, first_at)
  SELECT link_id, MIN(owner_id), 'client', client, MIN(first_at) FROM sub_ips WHERE link_id <> '' AND client <> '' GROUP BY link_id, client;
