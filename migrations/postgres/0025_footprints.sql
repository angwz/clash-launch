-- 足迹（需求 R10）：不会重复、日后回看有意义的事——第一次、累计跨过的阈值、新的地区与客户端、改名。
-- (user_id, kind, subject) 唯一，重复写入由 ON CONFLICT DO NOTHING 忽略，触发点直接写入即可。
-- user_id 为空串表示站点本身（上线日、用户数），只给管理员看；因此不设外键，注销时随 personalTables 删除。
-- backfilled 为 1 表示上线时由已有数据补记，时间是推算的。永久保存，不受审计保留期影响。
CREATE TABLE footprints (
  user_id    TEXT    NOT NULL,
  kind       TEXT    NOT NULL,
  subject    TEXT    NOT NULL DEFAULT '',
  at         BIGINT  NOT NULL,
  detail     TEXT    NOT NULL DEFAULT '{}',
  backfilled INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, kind, subject)
);
CREATE INDEX footprints_at ON footprints(user_id, at);
