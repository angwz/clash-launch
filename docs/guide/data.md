# 数据：备份、升级与搬迁

本项目承诺：**任何时候都能把数据完整地带走**。这包括备份、升级到新版本、在数据库之间搬迁，数据都不会丢。本文先写怎么操作，再写开发时必须遵守的规则，以及这些承诺由哪些测试保证。

## 数据在哪里

| 内容 | 位置 | 说明 |
| --- | --- | --- |
| 数据库 | `<数据目录>/clash-launch.db`（SQLite，默认）或 `CLASH_LAUNCH_DATABASE_URL` 指向的 PostgreSQL | 全部业务数据 |
| 主密钥 | `<数据目录>/master.key`（或 `CLASH_LAUNCH_MASTER_KEY*`） | 数据库中的订阅地址、节点、配置、链接令牌都用它加密。**丢失主密钥，数据库就无法解密** |
| 升级前的自动备份 | `<数据目录>/backups/before-migrate-*.db` | 每次升级执行迁移前自动生成（SQLite） |

## 操作

| 想做的事 | 命令 | 注意 |
| --- | --- | --- |
| 个人备份（用户自己） | 账号页「备份与还原」 | 只含该用户的数据，用备份口令加密，可在任何一台本程序的站点上还原；不需要主密钥。见 [0005](../decisions/0005-account-security.md#修订2026-11-21需求-r21个人备份与还原) |
| 在线备份 | `clash-launch backup <文件>` | SQLite 用 `VACUUM INTO` 生成一致快照，服务不用停。PostgreSQL 用 `pg_dump`。**同时备份主密钥** |
| 升级到新版本 | 换新镜像后启动即可 | 启动时自动执行尚未执行的迁移，执行前先自动备份。也可以单独执行 `clash-launch db migrate` |
| 回到旧版本 | 恢复升级前的自动备份 | 迁移只向前，不提供降级。数据库被新版本升级过后，旧版本程序会拒绝启动，不会在不认识的表结构上写坏数据 |
| 搬到另一个数据库 | `clash-launch db copy -from <DSN> -to <DSN>` | 见下文 |
| 轮换主密钥 | `clash-launch keys rotate` | 追加新密钥并重新加密全部数据；旧密钥确认备份后再手动删除 |

### 搬迁（SQLite → PostgreSQL、换机器、换文件）

```sh
# 1. 先备份（在线进行）
clash-launch backup /data/backups/before-copy.db
# 2. 复制到目标库：目标库会自动建表，必须为空
clash-launch db copy -from sqlite:///data/backups/before-copy.db -to postgres://user:pass@db/clash
# 3. 把 CLASH_LAUNCH_DATABASE_URL 改为目标库，重启
```

`db copy` 的行为：

- 两边必须处于**同一迁移版本**。源库较旧时，先用本版本程序对它执行 `db migrate`（会先自动备份）。版本不一致时直接拒绝，不会做半套复制。
- 目标库在一个事务中逐表写入，任何一张表失败都会整体回滚。完成后逐表比较行数与内容校验和，并打印每张表的结果。
- **按列名对应复制。** 源库多出来的列，只有登记在 `store.RetiredColumns` 里的废弃列可以跳过，跳过的列会逐一打印出来；其他对不上的列一律报错，不会悄悄丢数据。
- 密文原样复制，所以两边必须使用同一把主密钥。

## 开发规则：怎样改表结构

每条规则都有测试把关，违反时 `go test ./internal/store/` 会失败。

1. **迁移只向前，已发布的迁移文件不得修改。** 改表结构一律**新增**一个编号递增的迁移，例如 `0008_xxx.sql`；不能回头编辑已有的文件。`migrations/SUMS` 登记了每个迁移文件的 SHA-256，由 `TestMigrationsFrozen` 检查：
   - 新增迁移时，把测试打印出的那一行追加到 `SUMS`；
   - 改了已有文件，测试直接失败。
2. **两种数据库同时写。** `migrations/sqlite/` 与 `migrations/postgres/` 的版本号必须一一对应，建出的表与列一致，由 `TestMigrationsEquivalent` 检查。表示方式要两边通用（[0002](../decisions/0002-storage-and-service.md) #3）：
   - 时间存 UTC 毫秒整数；
   - 布尔存 0/1；
   - JSON 存文本；
   - 主键用应用生成的 UUIDv7。
3. **新表加入 `store.Tables`**，按依赖顺序排好；`db copy` 只复制这里列出的表。`TestMigrationsEquivalent` 会检查表的数量与它一致。
4. **优先做只增不改的变更**，即新增表、新增带默认值的列。确实需要转换数据时，转换要写在同一个迁移的 SQL 里，并让已有数据得到正确的值，例如 `0004` 根据邀请记录回填 `users.invited`。
5. **不再使用的列不删**：程序停止读写即可。如果某一列只存在于部分旧库中，就登记到 `store.RetiredColumns` 并写明原因，`db copy` 遇到它会明确跳过。

## 由哪些测试保证

| 承诺 | 测试 |
| --- | --- |
| 两种数据库的表结构一致；`Tables` 覆盖全部表 | `TestMigrationsEquivalent` |
| 已发布的迁移没有被修改 | `TestMigrationsFrozen` |
| 旧版本建的、已有数据的库能升级到最新版：自动备份、数据迁移结果正确、加密数据可读 | `TestUpgradePreservesData` |
| 升级后的库能完整复制到新库；版本不一致或有未登记的多余列时拒绝复制 | `TestUpgradePreservesData`、`TestKeyringRotateAndBackupCopy` |
| 旧版本程序拒绝打开被新版本升级过的库 | `TestMigrateRefusesNewerSchema` |
| 存储与接口在 PostgreSQL 上同样通过 | CI 的 `postgres` 任务 |

## 历史遗留

提交 4219e64 曾直接修改迁移 0001，删掉了 `sources.allow_direct` 列（出站代理随 [0003](../decisions/0003-direct-fetch-and-invites.md) 取消），因此在那之前建的数据库多这一列。程序读写都写明列名，不受影响；这一列已登记为废弃列，`db copy` 会跳过并打印说明。上面第 1 条规则就是为了不再出现这种情况。

**2026-11-15 的演练**：用维护者本机的真实数据库（SQLite）依次执行了在线备份和 `db copy` 到一个新 SQLite 库。
- 23 张表的行数与校验和全部一致，`sources.allow_direct` 按预期被跳过。
- 用复制出的库启动服务，就绪检查正常。

这次演练没有覆盖 PostgreSQL，因为本机没有实例。PostgreSQL 上的验证由 CI 完成。
