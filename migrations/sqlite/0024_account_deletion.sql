-- 注销账户（需求 R20）：deletion_at 是计划删除的时间（7 天冷静期，期间登录即撤销），deleted_at 是已注销的时间。
-- 注销后用户行保留为墓碑：对方的对话、邀请关系仍然指向它；个人数据全部删除，用户名改为 deleted-…，原用户名按改名规则保留 180 天。
ALTER TABLE users ADD COLUMN deletion_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN deleted_at INTEGER NOT NULL DEFAULT 0;
