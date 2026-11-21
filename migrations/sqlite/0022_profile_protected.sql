-- 配置的删除保护（需求 R55）：为 1 时删除接口返回 409，界面上的删除入口禁用。开关不产生新版本。
ALTER TABLE profiles ADD COLUMN protected INTEGER NOT NULL DEFAULT 0;
