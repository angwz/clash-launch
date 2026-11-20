-- 热点查询的索引（需求 R14d，依据 tools/dbbench 在千万行数据上的执行计划，见 docs/development/benchmarks.md）。

-- 会话：按「发送者、接收者、时间」有序，会话页的两个方向各走一次索引、按时间合并，不再整体排序。
CREATE INDEX messages_pair ON messages(sender_id, recipient_id, created_at, id);
-- 未读数：只索引未读且接收者未隐藏的消息；读过的消息不进入索引，索引随已读而缩小。
CREATE INDEX messages_unread ON messages(recipient_id, sender_id) WHERE read_at = 0 AND recipient_hidden = 0;
-- 短链接最近访问：只索引经短链接的请求（占少数），按短链接分组取最大时间可以直接读索引。
CREATE INDEX sub_requests_short ON sub_requests(owner_id, short_id, at) WHERE short_id <> '';
