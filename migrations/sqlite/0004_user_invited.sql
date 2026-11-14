-- 记录账号是否经邀请注册（反馈 #46）：经邀请的普通用户可自选节点写法，开放注册的用户跟随管理员设定的默认值。
-- 记在用户上而不是从 invites 推导：管理员删除已用的邀请不应改变用户的权限。
ALTER TABLE users ADD COLUMN invited INTEGER NOT NULL DEFAULT 0;
UPDATE users SET invited = 1 WHERE id IN (SELECT used_by FROM invites WHERE used_at <> 0);
