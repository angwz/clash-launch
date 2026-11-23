# 0010 · 默认使用 PostgreSQL

- 状态：已采纳（2026-11-23，反馈 #115）
- 范围：部署与运行时的数据库选择。修订 [0002](0002-storage-and-service.md) 中「默认 SQLite」与 backlog R14 中「SQLite 保留为很小的私人部署选项」两处

## 背景

R14 已把生产部署的推荐改为 PostgreSQL，并提供了叠加文件 `compose.postgres.yaml`；但没有设置 `CLASH_LAUNCH_DATABASE_URL` 时，程序仍在数据目录里自动建 SQLite 库，`compose.yaml` 也默认用 SQLite。维护者要求项目整体迁到 PostgreSQL，默认不再使用 SQLite。

## 决定

| # | 内容 | 理由 |
|---|---|---|
| 1 | `compose.yaml` 默认包含 PostgreSQL 17，数据库只在 compose 内部网络中可达；密码从 `.env` 的 `POSTGRES_PASSWORD` 读取，未设置时 compose 拒绝启动。删除 `compose.postgres.yaml` | 照文档部署就是 PostgreSQL；密码不写进仓库，也没有可被猜到的默认值 |
| 2 | 没有设置 `CLASH_LAUNCH_DATABASE_URL` 时不再自动建 SQLite 库：打开数据库时报错，说明生产用 PostgreSQL、试用可以显式选 SQLite | 不让新部署在不知情时落到 SQLite |
| 3 | 例外：数据目录里已有旧版默认的 `clash-launch.db` 时继续使用，每次启动记一条警告，给出 `db copy` 的迁移命令 | 已有部署升级后不会突然无法启动，也不会悄悄换成一个空的新库 |
| 4 | 显式的 `sqlite:///…` 照常可用，启动时警告它只适合本机试用与开发 | 离线单元测试、本机开发与 e2e 仍需要无需外部服务的数据库 |
| 5 | 保留 SQLite 的实现与迁移：两种数据库的迁移继续同号维护，`TestMigrationsEquivalent` 等测试在两种数据库上运行 | 完全移除会让 `go test ./...` 离不开数据库服务，且 `db copy` 需要读取旧的 SQLite 库才能把数据迁出 |
| 6 | 访客试用（`/try`）的示例环境仍用系统临时目录中的独立 SQLite 库，退出时删除 | 它是一次性的沙箱，与正式数据隔离；用 PostgreSQL 需要额外的库与清理，没有收益 |
| 7 | e2e 默认用临时 SQLite；设置 `E2E_DATABASE_URL` 时改用 PostgreSQL，正式改动前在 PostgreSQL 上完整跑一遍 | 默认路径快且互不影响；PostgreSQL 的整条链路另有实测 |

## 影响

- 部署：新部署多一个 `.env`；已有 SQLite 部署按 [deploy](../guide/deploy.md)「从 SQLite 迁到 PostgreSQL」迁移一次，主密钥不变。
- PostgreSQL 不在迁移前自动备份（SQLite 的 `before-migrate-*.db` 不适用），升级前用 `pg_dump`，见 [data](../guide/data.md)。
- CI 的容器测试改为带 `.env` 运行默认的 compose 文件。
