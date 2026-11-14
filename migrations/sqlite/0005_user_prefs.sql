-- 用户界面偏好（反馈 #37）：菜单与分节的自定义顺序等，JSON 对象，由 API 校验结构。
ALTER TABLE users ADD COLUMN prefs TEXT NOT NULL DEFAULT '{}';
