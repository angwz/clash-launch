-- 订阅链接访问限制（需求 R42）：JSON {"clients": [...], "countries": [...], "perDay": n}；空串表示不限制。
ALTER TABLE links ADD COLUMN limits TEXT NOT NULL DEFAULT '';
