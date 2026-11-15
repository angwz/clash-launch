-- 联系人、回收站与授权（反馈 #75）。

-- 联系人：交流过的人永久留在会话列表里，按最近交流时间排序（每个用户各一行）。
-- deleted_at 非 0 表示对话已删除、在回收站中；保存一年后消息对本人永久隐藏并移除这一行。
-- 任何一方再发消息时恢复（deleted_at 归零）。
CREATE TABLE contacts (
  user_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  peer_id    TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  last_at    INTEGER NOT NULL,
  deleted_at INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, peer_id)
) STRICT;
CREATE INDEX contacts_user ON contacts(user_id, last_at);

-- 已有消息的双方补成联系人
INSERT INTO contacts (user_id, peer_id, last_at, deleted_at)
SELECT u, p, MAX(t), 0 FROM (
  SELECT sender_id AS u, recipient_id AS p, created_at AS t FROM messages
  UNION ALL
  SELECT recipient_id AS u, sender_id AS p, created_at AS t FROM messages
) x GROUP BY u, p;

-- 授权：把自己的节点源或短链接给另一个用户「只能使用」。
--   kind = source：target_id 是授权者的节点源；对方节点页出现只读的软链接，内容随授权者更新。
--   kind = short：target_id 是为对方单独生成的短链接（short_links.grantee_id = 对方）。
-- revoked_at 非 0 表示已收回；记录保留，用于统计与消息中的状态。
CREATE TABLE grants (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  grantee_id TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT    NOT NULL,
  target_id  TEXT    NOT NULL,
  created_at INTEGER NOT NULL,
  revoked_at INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE INDEX grants_grantee ON grants(grantee_id, revoked_at);
CREATE INDEX grants_owner ON grants(owner_id, grantee_id);
CREATE INDEX grants_target ON grants(target_id);

-- 为好友单独生成的短链接：grantee_id 是被授权的用户（空表示自己用的短链接）
ALTER TABLE short_links ADD COLUMN grantee_id TEXT NOT NULL DEFAULT '';
