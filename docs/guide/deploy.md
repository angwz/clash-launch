# 部署

Clash, Launch! 是一个单容器服务：一个静态二进制、内嵌的前端、固定版本的 mihomo（用于 `mihomo -t`）与地理数据。默认用 SQLite，数据全部在 `/data`。

> **状态**：镜像与 compose 文件已编写，**尚未实际构建与运行**（见 [0002](../decisions/0002-storage-and-service.md#未验证)）。二进制本身已在本机以 `clash-launch serve` 运行验证。

## 用 compose 启动

```sh
docker compose up -d          # 本服务 + WARP 出口边车（compose.yaml）
docker compose logs clash-launch | grep setup
```

1. 修改 `compose.yaml` 中的 `CLASH_LAUNCH_PUBLIC_URL` 为你的对外地址（HTTPS）。
2. 首次启动时日志里有一条**一次性设置链接**（15 分钟有效），打开它创建首个管理员；也可以用命令行：

   ```sh
   docker compose exec -T clash-launch /clash-launch admin create -username admin <<< '一个至少 10 个字符的口令'
   ```

3. 把 nginx 或 Caddy 反向代理到 `127.0.0.1:8080`，见 [reverse-proxy](reverse-proxy.md)。

不需要出口边车时用 `compose.minimal.yaml`。

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
| `CLASH_LAUNCH_EGRESS_PROXY` | compose 中为 `socks5h://warp:1080` | 出站代理：`socks5h://`、`socks5://` 或 `http://` |
| `CLASH_LAUNCH_EGRESS_KIND` | 地址含 `warp` 时为 `warp` | 向用户说明订阅的获取方式：`warp` 或 `proxy` |
| `CLASH_LAUNCH_ALLOW_USER_DIRECT` | `false` | 没有出站代理时，是否允许普通用户的订阅由服务器直连拉取（暴露服务器 IP） |
| `CLASH_LAUNCH_FETCH_ALLOWLIST` | — | SSRF 白名单：网段、IP 或主机名 |
| `CLASH_LAUNCH_MIHOMO` · `_GEODATA` | `/opt/mihomo/mihomo` · `/opt/mihomo/geodata` | 内核检查用的 mihomo 与地理数据 |
| `CLASH_LAUNCH_LOG_LEVEL` | `info` | `debug`、`info`、`warn`、`error` |

## 出站隐私

服务器拉取订阅时，对方能看到服务器的出口 IP。默认的 compose 把所有出站连接交给 WARP 边车，对方看到的是 Cloudflare 的地址；出口不可用时**不会回退直连**，而是沿用上次成功的快照。细节见 [network](../design/network.md)。

## 备份与升级

```sh
docker compose exec clash-launch /clash-launch backup /data/backups/manual.db
```

- 备份数据库时**必须同时备份主密钥**（`/data/master.key`）：丢失密钥就无法解密。
- 升级时启动会自动执行迁移，执行前自动备份到 `/data/backups/`；不支持降级，降级需要恢复备份。
- 轮换主密钥：`/clash-launch keys rotate`（追加新密钥并重新加密全部数据）。
- 迁到 PostgreSQL：`/clash-launch db copy -from sqlite:///data/clash-launch.db -to postgres://…`，然后把 `CLASH_LAUNCH_DATABASE_URL` 改为 PostgreSQL。
