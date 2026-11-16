-- 用户名、昵称与邀请（需求 R4、R5、R12）。

-- 昵称：可含空格、不唯一；nickname_norm 是小写形式，用于搜索。
ALTER TABLE users ADD COLUMN nickname TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN nickname_norm TEXT NOT NULL DEFAULT '';
-- 最近一次改用户名的时间（毫秒），0 表示从未改过；两次修改至少间隔 180 天。
ALTER TABLE users ADD COLUMN username_changed_at INTEGER NOT NULL DEFAULT 0;
-- 邀请关系：邀请人与所用邀请。邀请人注销后保留其用户名快照（invited_by_name），关系不断。
ALTER TABLE users ADD COLUMN invited_by TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN invited_by_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN invite_id TEXT NOT NULL DEFAULT '';
-- 管理员可以收回某个人的邀请权。
ALTER TABLE users ADD COLUMN invite_banned INTEGER NOT NULL DEFAULT 0;

-- 用过的用户名：改名后旧名保留 180 天，期间别人不能注册或改用，防止冒充。
CREATE TABLE username_history (
  user_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  username   TEXT    NOT NULL,
  name_norm  TEXT    NOT NULL,
  changed_at INTEGER NOT NULL
) STRICT;
CREATE INDEX username_history_norm ON username_history(name_norm, changed_at);
CREATE INDEX username_history_user ON username_history(user_id, changed_at);

-- 邀请可用多次：max_uses 为上限（已有的一次性邀请为 1），uses 为已用次数；revoked_at 非 0 表示已作废。
-- used_at、used_by 保留为第一次使用的时间与用户。
ALTER TABLE invites ADD COLUMN max_uses INTEGER NOT NULL DEFAULT 1;
ALTER TABLE invites ADD COLUMN uses INTEGER NOT NULL DEFAULT 0;
ALTER TABLE invites ADD COLUMN revoked_at INTEGER NOT NULL DEFAULT 0;
UPDATE invites SET uses = 1 WHERE used_at <> 0;

-- 每次使用一行（多次邀请需要逐次记录）。
CREATE TABLE invite_uses (
  invite_id TEXT    NOT NULL,
  user_id   TEXT    NOT NULL,
  used_at   INTEGER NOT NULL,
  PRIMARY KEY (invite_id, user_id)
) STRICT;
CREATE INDEX invite_uses_user ON invite_uses(user_id);

-- 补齐已有的邀请关系
INSERT INTO invite_uses (invite_id, user_id, used_at) SELECT id, used_by, used_at FROM invites WHERE used_at <> 0 AND used_by <> '';
UPDATE users SET
  invite_id = (SELECT i.id FROM invites i WHERE i.used_by = users.id ORDER BY i.used_at LIMIT 1),
  invited_by = (SELECT i.created_by FROM invites i WHERE i.used_by = users.id ORDER BY i.used_at LIMIT 1),
  invited_by_name = COALESCE((SELECT u.username FROM invites i JOIN users u ON u.id = i.created_by WHERE i.used_by = users.id ORDER BY i.used_at LIMIT 1), '')
WHERE id IN (SELECT used_by FROM invites WHERE used_at <> 0);
