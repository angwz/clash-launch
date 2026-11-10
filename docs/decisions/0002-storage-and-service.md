# 0002 · 存储的可迁移性与服务层

- 状态：已采纳（2026-11-09，M3）
- 范围：第 12–14、17、18 节在 M3 中的具体选择；补充而非替代规范

## 存储（第 17.1 节）

| # | 主题 | 决定 |
| ---: | --- | --- |
| 1 | 仓储接口 | 业务代码只依赖 [`internal/store`](../../internal/store/store.go) 中按领域划分的接口；每个方法以所有者 ID 为第一个参数（在 ctx 之后），不提供绕过所有者的查询。跨所有者的 `Due`/`All` 只供调度器使用，管理用的 `Users().List` 只返回元数据 |
| 2 | 实现 | 一份通用 SQL 实现服务两种数据库，差异只在占位符（`?` 与 `$n`）与驱动；SQLite 用 `modernc.org/sqlite`（纯 Go，WAL、外键、`busy_timeout`，单连接串行写入），PostgreSQL 用 `jackc/pgx/v5` 的 `database/sql` 适配 |
| 3 | 可移植的表示 | 主键为应用生成的 UUIDv7 文本；时间为 UTC 毫秒整数（SQLite `INTEGER` / PostgreSQL `BIGINT`）；布尔为 0/1 整数；JSON 为文本；二进制为 `BLOB` / `BYTEA`；SQLite 建表使用 `STRICT` |
| 4 | 迁移 | `migrations/sqlite/` 与 `migrations/postgres/` 版本号一一对应、只向前；`TestMigrationsEquivalent` 检查两套脚本的表与列一致。启动时自动迁移，SQLite 下若已有数据，先 `VACUUM INTO` 备份到 `<数据目录>/backups/` |
| 5 | 数据搬迁 | `clash-launch db copy -from <DSN> -to <DSN>`：目标迁移后必须为空；按依赖顺序逐表流式复制，复制后逐表比较行数与内容校验和；密文原样复制（两边使用同一把主密钥） |
| 6 | 单实例假设 | 后台拉取、渲染锁、订阅限流都在进程内；多实例部署时，拉取任务改用数据库租约锁，限流换成共享实现，业务代码不变 |
| 7 | 依赖版本 | `modernc.org/sqlite` 固定在 v1.36.1：更新的版本会把 `golang.org/x/*` 升到高于 mihomo v1.19.33 `go.mod` 的版本（见 [0001](0001-deviations-from-spec-1.0.md) #17）；随 mihomo 升级时一并评估 |

## 加密（第 14.4 节）

- AES-256-GCM；密文格式「版本 | 密钥 ID 长度 | 密钥 ID | nonce | 密文与标签」；附加数据为「表.列:行 ID」，密文不能被搬到另一行或另一列使用（`TestOwnerIsolationAndCrypto` 验证）。
- 加密列：源的 URL、内容与请求头；快照内容；方案文档与各版本；链接令牌；渲染结果。链接另存令牌的 SHA-256 用于查找，查找后以常数时间比较原文。
- 主密钥来源：`CLASH_LAUNCH_MASTER_KEY`（base64 或 64 位十六进制）→ `CLASH_LAUNCH_MASTER_KEY_FILE` → `<数据目录>/master.key`；都不存在时生成 `master.key`（0600），日志醒目提示必须单独备份。密钥文件每行「密钥 ID:base64」，最后一行是当前密钥。
- `clash-launch keys rotate`：追加新密钥，并在一个事务中用它重新加密全部数据；旧密钥保留在文件中，确认备份后由管理员手动删除。

## 账号与会话（第 14.1–14.2 节，M3 部分）

- 口令：argon2id，m=19 MiB、t=2、p=1、32 字节输出、16 字节盐（OWASP Password Storage Cheat Sheet 的推荐配置之一），PHC 字符串格式，参数随哈希保存；用户不存在时执行一次同等开销的计算。最短 10 个字符；常见口令表 M5 补全。
- 首个管理员：`clash-launch admin create`（口令从标准输入读取），或首次启动时打印在日志中的一次性设置链接（15 分钟有效，用后作废）。不存在默认口令。
- 会话：32 字节随机令牌，只存 SHA-256；Cookie `HttpOnly`、`SameSite=Lax`、HTTPS 下 `Secure` 且名称带 `__Host-` 前缀；每次登录都是新的会话 ID；空闲 7 天、绝对 30 天过期（可配置）。
- CSRF：会话绑定的同步令牌（登录与 `/me` 返回，写请求放在 `X-CSRF-Token`），并检查 `Origin` 与对外地址一致。没有采用规范中「双提交 Cookie」的写法：会话已经在服务端，同步令牌更简单且同样有效。
- 注册开关与一次性邀请见 [0003](0003-direct-fetch-and-invites.md)。个人 API 令牌（Bearer）、找回、通行密钥、TOTP、SMTP 提醒、登录退避与配额在 M5。

## 订阅与托管地址（第 13 节）

- 订阅端点只读最近一次成功的渲染结果；尚无成功结果时返回 503 与一段 YAML 注释；`ETag` 按实际输出计算，支持 `If-None-Match`；按 IP 与令牌每分钟 30 次限流。
- 「交给客户端」模式下，单条、批量、文件源由本服务托管在 `/p/<令牌>/<源>`。一个方案的渲染结果由它的多条链接共享，因此渲染时在 URL 中写入占位符 `LINKTOKEN-PLACEHOLDER-…`，提供订阅时替换为请求者自己的链接令牌：两者都只含 base64url 字符，替换不改变 YAML 的结构与引号；格式自检针对的是带占位符的版本。

## 部署（第 18 节）

- 镜像：前端构建（`node:26.13.0-bookworm-slim`）→ Go 交叉编译（`golang:1.27.3-bookworm`，`CGO_ENABLED=0`）并用 `tools/toolsync` 下载、校验目标架构的 mihomo 与地理数据 → `gcr.io/distroless/static-debian12:nonroot`，以 uid 65532 运行，只读根文件系统，可写位置为 `/data` 与 `tmpfs` 的 `/tmp`。基础镜像目前按标签固定，发布前（M7）改为摘要。
- 单容器部署；原先的 WARP 边车已由 [0003](0003-direct-fetch-and-invites.md) 取消。

## 未验证

- **镜像构建与 compose 运行**：本机 Docker 守护进程未运行，未构建、未运行。
- **PostgreSQL**：驱动与迁移已就绪，`db copy` 在 SQLite → SQLite 上测试；本机没有 PostgreSQL 实例，冒烟测试未运行。
