-- 按时间清理与按时间查询的索引（需求 R14c）。
-- 订阅请求明细设保留期后，每天按 at 删除过期行；审计日志按 at 清理。
-- 已有的 (owner_id, at)、(link_id, at) 索引以所有者或链接开头，无法用于「at 早于某时刻」的全表条件，
-- 没有这两个索引时清理要扫描整张表。
CREATE INDEX sub_requests_at ON sub_requests(at);
CREATE INDEX audit_at ON audit(at);
