-- 头像（需求 R19）：users.avatar 是当前的头像——「p:<图形>-<颜色>」为预置头像，「u:<版本>」为上传的图片，空串表示尚未分配。
-- 上传的图片已在服务器裁成正方形、缩到 256 px 并重新编码（去掉 EXIF），存在 user_avatars；换回预置头像时删除。
ALTER TABLE users ADD COLUMN avatar TEXT NOT NULL DEFAULT '';
CREATE TABLE user_avatars (
  user_id    TEXT    NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  mime       TEXT    NOT NULL,
  data       BLOB    NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;
