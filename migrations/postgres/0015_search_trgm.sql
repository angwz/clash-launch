-- 用户搜索走索引（需求 R14e）：「用户名前缀或昵称中任一段」用 LIKE '%…%'，普通 B 树索引用不上，
-- 用户多时整表扫描。pg_trgm 的 GIN 索引让三个字符以上的片段走索引；更短的片段仍会扫描，但结果集小、可以接受。
-- pg_trgm 是可信扩展（PostgreSQL 13 起），数据库所有者即可创建，不需要超级用户。
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX users_username_trgm ON users USING gin (username_norm gin_trgm_ops);
CREATE INDEX users_nickname_trgm ON users USING gin (nickname_norm gin_trgm_ops);
