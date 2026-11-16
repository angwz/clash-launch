-- 登录设备（需求 R7、R8）。
-- logout_at：定时退出的时间（毫秒），0 表示未设定。单独保存而不改写 expires_at：取消定时只需清零，
--   原来的到期时间不受影响；生效时间取两者中较早的那个。
-- location：登录时由本地地理库得出的所在地（如「中国 · 上海 · 中国移动」），与 ip_prefix 一样只用于展示。
ALTER TABLE sessions ADD COLUMN logout_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN location TEXT NOT NULL DEFAULT '';
