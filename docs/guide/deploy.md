# 部署

Clash, Launch! 是一个单容器服务：一个静态二进制、内嵌的前端、固定版本的 mihomo（用于 `mihomo -t`）与地理数据。默认用 SQLite，数据全部在 `/data`。

> **状态**：镜像与 compose 文件已编写，**尚未实际构建与运行**（见 [0002](../decisions/0002-storage-and-service.md#未验证)）。二进制本身已在本机以 `clash-launch serve` 运行验证。

## 用 compose 启动

```sh
docker compose up -d
docker compose logs clash-launch | grep setup
```

1. 修改 `compose.yaml` 中的 `CLASH_LAUNCH_PUBLIC_URL` 为你的对外地址（HTTPS）。
2. 首次启动后打开对外地址，会自动进入**首次注册**：第一个访问者成为管理员，之后这个入口关闭。部署到公网时请在启动后尽快完成这一步。也可以用命令行：

   ```sh
   docker compose exec -T clash-launch /clash-launch admin create -username admin <<< '至少 5 位、含 2 类字符的口令'
   ```

3. 把 nginx 或 Caddy 反向代理到 `127.0.0.1:8080`，见 [reverse-proxy](reverse-proxy.md)。

4. 让别人注册：注册默认关闭。管理员可以在设置中打开注册，或者为每个人生成一次性邀请链接（可选有效期），见 [注册与邀请](#注册与邀请)。

## 配置

环境变量为主；`CLASH_LAUNCH_CONFIG` 可以指向一个 TOML 文件（键为变量名去掉前缀后的小写短横线形式，如 `public-url`），同名设置以环境变量为准。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `CLASH_LAUNCH_LISTEN` | `:8080` | 监听地址 |
| `CLASH_LAUNCH_DATA_DIR` | `/data` | 数据目录：数据库、主密钥、迁移前备份 |
| `CLASH_LAUNCH_DATABASE_URL` | `sqlite:///data/clash-launch.db` | `postgres://…` 时使用 PostgreSQL |
| `CLASH_LAUNCH_PUBLIC_URL` | — | 对外地址；生成的订阅链接与 Cookie 的 `Secure` 以它为准。未设置时根据请求推断 |
| `CLASH_LAUNCH_MASTER_KEY` / `_FILE` | — | 主密钥；都没有时生成 `/data/master.key` |
| `CLASH_LAUNCH_TRUSTED_PROXIES` | — | 受信任的反向代理网段，逗号分隔；只有来自这些地址的请求才采信 `X-Forwarded-For`/`-Proto` |
| `CLASH_LAUNCH_FETCH_ALLOWLIST` | — | SSRF 白名单：网段、IP 或主机名 |
| `CLASH_LAUNCH_MIHOMO` · `_GEODATA` | `/opt/mihomo/mihomo` · `/opt/mihomo/geodata` | 内核检查用的 mihomo 与地理数据 |
| `CLASH_LAUNCH_LOG_LEVEL` | `info` | `debug`、`info`、`warn`、`error` |
| `CLASH_LAUNCH_POW_BITS` | `20` | 注册与找回的工作量证明难度（前导零位数）；每加 1 位耗时翻倍，20 位在桌面浏览器上约 1 秒 |
| `CLASH_LAUNCH_QUOTA_SOURCES` · `_PROFILES` · `_LINKS` | `50` · `20` · `50` | 每个用户的节点源、方案、订阅链接数上限（管理员不受限制，可在管理页为单个用户调整） |
| `CLASH_LAUNCH_QUOTA_MIN_INTERVAL` | `30` | 订阅与规则源的最短拉取间隔（分钟）；手动拉取每人每小时最多 30 次 |
| `CLASH_LAUNCH_QUOTA_STORAGE_MB` | `100` | 每个用户保存的内容上限（源、快照、方案与版本，按加密后计） |
| `CLASH_LAUNCH_AUDIT_DAYS` | `180` | 审计日志保留天数；`0` 表示不清理 |

## 注册与邀请

| 方式 | 做法 |
| --- | --- |
| 关闭注册（默认） | 只有持有邀请链接的人能注册。管理员生成 `<对外地址>/register#<令牌>`：有效期 1 小时到 90 天或不过期，可附备注；用过一次即作废，未使用的可以撤销 |
| 开放注册 | 任何能访问服务的人都可以注册普通账号；每个 IP 每小时最多 10 次 |

接口见 [`openapi.yaml`](../../api/openapi.yaml) 的 `admin` 标签；设计见 [0003](../decisions/0003-direct-fetch-and-invites.md)。

## 出站与服务器 IP

服务器直接拉取订阅与规则源，对方能看到服务器的出口 IP；能添加订阅的用户也可以借此得知它。需要隐藏时，在宿主机或网络层让整台服务器经代理出网。SSRF 防护与请求头约束见 [network](../design/network.md)。

## 备份与升级

```sh
docker compose exec clash-launch /clash-launch backup /data/backups/manual.db
```

- 备份数据库时**必须同时备份主密钥**（`/data/master.key`）：丢失密钥就无法解密。
- 升级时启动会自动执行迁移，执行前自动备份到 `/data/backups/`；不支持降级，降级需要恢复备份。
- 轮换主密钥：`/clash-launch keys rotate`（追加新密钥并重新加密全部数据）。
- 迁到 PostgreSQL：`/clash-launch db copy -from sqlite:///data/clash-launch.db -to postgres://…`，然后把 `CLASH_LAUNCH_DATABASE_URL` 改为 PostgreSQL。
