-- 用户协议（需求 R40）：记录用户同意的协议版本与时间（毫秒）。0 表示尚未同意任何版本：
-- 上线前已有的普通用户下次登录时先阅读并同意，才能继续使用。协议文本与当前版本号保存在 settings。
ALTER TABLE users ADD COLUMN terms_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN terms_at BIGINT NOT NULL DEFAULT 0;
