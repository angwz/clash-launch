-- 统计（反馈 #66–#68）。只按天汇总计数，永久保留：总数 = 全部天数之和。
--   scope 取值：pv（页面浏览，key = 页面类型）、uv（每日访问的不同账号数）、login（登录）、
--   sub（订阅请求，key = 链接 ID）、sub_all（全站订阅请求）、sub_fail（失败的订阅请求）、
--   sub_client（key = 客户端名称）、field_change / field_expand（key = 字段路径）。
--   day 为 UTC 日期 YYYYMMDD。不记录用户 ID：只有「某天、某项、多少次」。
CREATE TABLE stat_counts (
  scope TEXT    NOT NULL,
  key   TEXT    NOT NULL,
  day   INTEGER NOT NULL,
  count INTEGER NOT NULL,
  PRIMARY KEY (scope, key, day)
) STRICT;

-- 订阅请求逐条记录（反馈 #67）：配置链接所有者在「配置链接」页查看，用于安全检查。
-- 不记录令牌本身。kind：config（/s/ 配置）| provider（/p/ 托管的节点文件）。
CREATE TABLE sub_requests (
  id       TEXT    NOT NULL PRIMARY KEY,
  link_id  TEXT    NOT NULL,
  owner_id TEXT    NOT NULL,
  at       INTEGER NOT NULL,
  ip       TEXT    NOT NULL,
  ua       TEXT    NOT NULL DEFAULT '',
  client   TEXT    NOT NULL DEFAULT '',
  status   INTEGER NOT NULL,
  bytes    INTEGER NOT NULL DEFAULT 0,
  kind     TEXT    NOT NULL DEFAULT 'config'
) STRICT;
CREATE INDEX sub_requests_link ON sub_requests(link_id, at);
CREATE INDEX sub_requests_owner ON sub_requests(owner_id, at);

-- 按链接与 IP 汇总（永久保留）。link_id 为空表示令牌无效的请求（只给管理员看，用于发现试探）。
CREATE TABLE sub_ips (
  link_id  TEXT    NOT NULL,
  ip       TEXT    NOT NULL,
  owner_id TEXT    NOT NULL DEFAULT '',
  first_at INTEGER NOT NULL,
  last_at  INTEGER NOT NULL,
  ok       INTEGER NOT NULL DEFAULT 0,
  fail     INTEGER NOT NULL DEFAULT 0,
  client   TEXT    NOT NULL DEFAULT '',
  PRIMARY KEY (link_id, ip)
) STRICT;
