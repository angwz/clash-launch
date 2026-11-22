-- 站内公告（需求 R32）：管理员发布的横幅（维护通知、新功能）。starts_at 起显示，ends_at 为 0 表示一直显示到删除。
-- level：info 一般提示 | warn 需要注意（如维护停机）。用户关闭后记在 announcement_dismissals，换设备也不再出现。
CREATE TABLE announcements (
  id         TEXT    NOT NULL PRIMARY KEY,
  text       TEXT    NOT NULL,
  level      TEXT    NOT NULL DEFAULT 'info',
  starts_at  INTEGER NOT NULL,
  ends_at    INTEGER NOT NULL DEFAULT 0,
  created_by TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;
CREATE INDEX announcements_starts ON announcements(starts_at);
CREATE TABLE announcement_dismissals (
  user_id         TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  announcement_id TEXT    NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
  at              INTEGER NOT NULL,
  PRIMARY KEY (user_id, announcement_id)
) STRICT;
