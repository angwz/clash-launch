-- 订阅分析的覆盖索引（需求 R14c）：分析按时间正序流式读取某个用户的请求，只用到下列列，
-- 不读 UA 与字节数。整行都在索引里，读取时不回表（PostgreSQL 为仅索引扫描），也不受明细表行宽影响。
-- 前缀 (owner_id, at, id) 同时覆盖原 sub_requests_owner 的用途，所以删掉它。
CREATE INDEX sub_requests_analytics ON sub_requests(owner_id, at, id, status, ip, country, asn, org, client, kind, link_id, short_id);
DROP INDEX sub_requests_owner;
