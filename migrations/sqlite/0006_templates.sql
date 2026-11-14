-- 配置模板（反馈 #51）：一份配置文档（节点与写法、分流规则、链式代理、内核设置、直接编辑等，不含名称）
-- 存为带名称与说明的模板；新建配置时可从模板开始。文档与配置一样加密存储（可能含直接编辑中的凭据）。
-- is_default 每个用户最多一个，由应用在事务中维护。
CREATE TABLE templates (
  id         TEXT    NOT NULL PRIMARY KEY,
  owner_id   TEXT    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  note       TEXT    NOT NULL DEFAULT '',
  doc_enc    BLOB    NOT NULL,
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
) STRICT;
CREATE INDEX templates_owner ON templates(owner_id);
