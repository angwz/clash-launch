# 部署

Clash, Launch! 是一个单容器服务：一个静态二进制、内嵌的前端、固定版本的 mihomo（用于 `mihomo -t`）与地理数据。默认用 SQLite，数据全部在 `/data`。

镜像由 GitHub Actions 构建并发布到 `ghcr.io/angwz/clash-launch`（amd64、arm64）：推送到 `main` 更新 `:latest`，推送 `v*` 标签发布版本号标签。

## 用 compose 启动

```sh
curl -fsSLO https://raw.githubusercontent.com/angwz/clash-launch/main/compose.yaml
docker compose up -d
```

1. 默认明文 HTTP、对所有 IP 开放 20230 端口，便于试用；风险与正式使用的做法见 README 的「安全」。
2. 首次启动后打开对外地址，会自动进入**首次注册**：第一个访问者成为管理员，之后这个入口关闭。部署到公网时请在启动后尽快完成这一步。也可以用命令行：

   ```sh
   docker compose exec -T clash-launch /clash-launch admin create -username admin <<< '至少 5 位、含 2 类字符的密码'
   ```

3. 正式使用：把 nginx 或 Caddy 反向代理到本机 20230（反代与服务在同一台机器时，可把 compose 的端口改为 `["127.0.0.1:20230:20230"]`，只让反代访问），设置 `CLASH_LAUNCH_PUBLIC_URL=https://你的域名`，见 [reverse-proxy](reverse-proxy.md)。

4. 让别人注册：注册默认关闭。管理员可以在设置中打开注册，或者为每个人生成一次性邀请链接（可选有效期），见 [注册与邀请](#注册与邀请)。

### 更新

```sh
docker compose pull && docker compose up -d
```

不想手动执行时，可以在同一台机器上运行 [Watchtower](https://containrrr.dev/watchtower/) 定时检查新镜像；它会自动重启容器，适合个人使用，不适合需要控制升级时机的场合。

## 数据放在哪

数据（数据库与主密钥 `master.key`）在 `compose.yaml` 旁边的 `data` 文件夹，备份时整个文件夹打包即可。不需要手动改权限：镜像以 root 启动，程序先把 `/data` 交给运行用户，再放弃 root 继续运行（`cmd/clash-launch/privdrop_linux.go`）；宿主机上由 root 创建的目录、Docker 数据卷、NAS 的共享目录都能直接用。

| 情况 | 做法 |
| --- | --- |
| 群晖、威联通、Unraid 等要让文件属于宿主机上的某个用户 | 在 `environment` 中设置 `PUID`、`PGID`（如 `1026`、`100`）；默认是 `65532`。不接受 `0`：服务不以 root 运行 |
| 想用 Docker 数据卷 | 把 `./data:/data` 改为 `clash-launch-data:/data`，并在文件末尾加上 `volumes: {clash-launch-data: {}}` |
| 用 `--user`、rootless 等方式以普通用户启动 | 程序不做任何整理；目录不可写时退出，日志里给出针对当前用户的 `chown` 命令 |

### 从旧版 compose 升级

2026-11-21 之前的 `compose.yaml` 把数据放在 Docker 卷 `<目录名>_data` 中。换成新文件前，先把数据复制到 `./data`，否则新实例是空的（旧数据仍在原来的卷里，不会丢）：

```sh
docker compose down
docker run --rm -v "$(basename "$PWD")_data:/from" -v "$PWD/data:/to" busybox cp -a /from/. /to/
curl -fsSLO https://raw.githubusercontent.com/angwz/clash-launch/main/compose.yaml
docker compose up -d
```

继续用旧文件也可以：它以 `user: "0:0"` 启动并自带加固项，与新镜像兼容。

### 不用 compose

```sh
docker run -d --name clash-launch --restart unless-stopped -p 20230:20230 -v "$PWD/data:/data" ghcr.io/angwz/clash-launch:latest
```

### 安全加固

```sh
curl -fsSLO https://raw.githubusercontent.com/angwz/clash-launch/main/compose.hardened.yaml
docker compose -f compose.yaml -f compose.hardened.yaml up -d
```

叠加后根文件系统只读（只写 `/data` 与 `/tmp`）、去掉其余全部 Linux 能力、禁止提权。保留的 `CHOWN`、`DAC_OVERRIDE`、`SETUID`、`SETGID` 只用于启动时整理 `/data` 并降权，降权后全部失效。默认不开，因为部分 NAS 与 rootless 环境不支持去掉能力或只读根文件系统。

### 使用 PostgreSQL

```sh
curl -fsSLO https://raw.githubusercontent.com/angwz/clash-launch/main/compose.postgres.yaml
echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)" > .env
docker compose -f compose.yaml -f compose.postgres.yaml up -d
```

数据库文件在旁边的 `postgres` 文件夹，只在 compose 内部网络中可达，不对外开放端口；主密钥仍在 `data`，两者都要备份。备份用 `pg_dump`，见下文「备份与升级」。已有的 SQLite 数据要先用 `db copy` 搬过去，见 [data](data.md)。三个文件可以同时叠加：`-f compose.yaml -f compose.hardened.yaml -f compose.postgres.yaml`。

## 版本

| 镜像标签 | 何时更新 | 适合 |
| --- | --- | --- |
| `:latest` | 每次推送到 `main` | 想第一时间用上修复与新功能 |
| `:1.0.3` 这类完整版本号 | 维护者打 `v1.0.3` 标签时 | 想固定在某个版本，自己决定何时升级 |
| `:1.0` | 1.0.x 每次发布 | 只接收修补，不接收新功能 |

版本号为「主版本.次版本.修订号」：修补加修订号，新增功能加次版本，不兼容的变化（如需要手动处理数据）才加主版本，并在发布说明中写明。数据库迁移随新版本自动执行，见下文「备份与升级」。

## 配置

环境变量为主；`CLASH_LAUNCH_CONFIG` 可以指向一个 TOML 文件（键为变量名去掉前缀后的小写短横线形式，如 `public-url`），同名设置以环境变量为准。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `CLASH_LAUNCH_LISTEN` | `:20230` | 监听地址 |
| `CLASH_LAUNCH_DATA_DIR` | `/data` | 数据目录：数据库、主密钥、迁移前备份 |
| `CLASH_LAUNCH_DATABASE_URL` | `sqlite:///data/clash-launch.db` | `postgres://…` 时使用 PostgreSQL |
| `CLASH_LAUNCH_DB_MAX_CONNS` · `_DB_MAX_IDLE` | `20` · `5` | PostgreSQL 连接池：同时打开的连接上限与保持的空闲连接。上限加上备份、管理工具的连接，要低于数据库的 `max_connections`（默认 100） |
| `CLASH_LAUNCH_DB_IDLE_MINUTES` | `5` | 空闲连接多久后关闭 |
| `CLASH_LAUNCH_DB_STATEMENT_TIMEOUT` | `30` | 单条语句的最长执行时间（秒），防止慢查询长期占住连接；启动时的迁移不受限制。地址中写了 `statement_timeout` 时以地址为准 |
| `CLASH_LAUNCH_PUBLIC_URL` | — | 对外地址；生成的订阅链接与 Cookie 的 `Secure` 以它为准。可写多个，逗号分隔，第一个为主地址（见 [多个域名](reverse-proxy.md#多个域名)）。未设置时根据请求推断 |
| `CLASH_LAUNCH_MASTER_KEY` / `_FILE` | — | 主密钥；都没有时生成 `/data/master.key` |
| `CLASH_LAUNCH_TRUSTED_PROXIES` | — | 受信任的反向代理网段，逗号分隔；只有来自这些地址的请求才采信 `X-Forwarded-For`/`-Proto` |
| `CLASH_LAUNCH_FETCH_ALLOWLIST` | — | SSRF 白名单：网段、IP 或主机名 |
| `CLASH_LAUNCH_RULE_HOSTS` | 内置列表 | 普通用户的分流规则地址允许的站点，逗号分隔的主机名（精确匹配、只接受 https）；设置后替换内置列表（GitHub、jsDelivr、GitLab、Codeberg、Gitee 等，见 `internal/source/hosts.go`）。管理员不受限制 |
| `CLASH_LAUNCH_MIHOMO` · `_GEODATA` | `/opt/mihomo/mihomo` · `/opt/mihomo/geodata` | 内核检查用的 mihomo 与地理数据；地理数据同时用于订阅请求的归属地与分流追踪中的 GEOIP、GEOSITE、IP-ASN（只读加载，缺失时不下载） |
| `CLASH_LAUNCH_LOG_LEVEL` | `info` | `debug`、`info`、`warn`、`error` |
| `CLASH_LAUNCH_LOG_FORMAT` | `json` | `json`：一行一条，便于日志系统采集；`text`：便于直接阅读 |
| `CLASH_LAUNCH_METRICS_TOKEN` | 空 | 设置后 `/metrics` 凭 `Authorization: Bearer <令牌>` 访问，见下文「监控」 |
| `CLASH_LAUNCH_POW_BITS` | `20` | 注册与找回的工作量证明难度（前导零位数）；每加 1 位耗时翻倍，20 位在桌面浏览器上约 1 秒 |
| `CLASH_LAUNCH_QUOTA_SOURCES` · `_PROFILES` · `_LINKS` | `50` · `20` · `50` | 每个用户的节点源、方案、订阅链接数上限（管理员不受限制，可在管理页为单个用户调整） |
| `CLASH_LAUNCH_QUOTA_MIN_INTERVAL` | `30` | 订阅与规则源的最短拉取间隔（分钟）；手动拉取每人每小时最多 30 次 |
| `CLASH_LAUNCH_QUOTA_STORAGE_MB` | `100` | 每个用户保存的内容上限（源、快照、方案与版本，按加密后计） |
| `CLASH_LAUNCH_OPEN_QUOTA_*`（同上五项） | `10` · `5` · `10` · `60` · `20` | 开放注册（未经邀请）的用户使用的默认配额，比上面更小；管理员与受邀用户使用上面的值 |
| `CLASH_LAUNCH_AUDIT_DAYS` | `180` | 审计日志保留天数；`0` 表示不清理。管理页「数据保留」中的设置优先于它，立即生效 |
| `CLASH_LAUNCH_SUB_REQUEST_DAYS` | `400` | 订阅请求明细（每次拉取一行）的保留天数，每天按批清理；按天、按链接与按 IP 的汇总永久保留。低于 365 时，订阅分析选「一年」会提示前段不完整；`0` 表示不清理。管理页「数据保留」中的设置优先于它 |
| `CLASH_LAUNCH_TRACE` | `on` | 分流追踪与常用网站体检。开启时服务器会下载方案引用的规则内容（与拉取订阅相同的 SSRF 防护与站点白名单，只缓存在内存）；`off` 时追踪与体检不可用，分流地图照常 |
| `CLASH_LAUNCH_DEMO` | `on` | 访客试用（`/try`）：不登录就能在示例配置上试用编辑器与分流图。示例环境建在系统临时目录里的独立 SQLite 库中，与正式数据隔离，退出时删除；不联网、不做内核检查，按 IP 限流。`off` 时关闭，登录页不再显示入口 |
| `CLASH_LAUNCH_TRACE_CACHE_MB` | `256` | 规则内容的内存缓存上限，按内容字节淘汰最久未用的 |

## 注册与邀请

| 方式 | 做法 |
| --- | --- |
| 关闭注册（默认） | 只有持有邀请链接的人能注册。管理员生成 `<对外地址>/register#<令牌>`：有效期 1 小时到 90 天或不过期，可附备注；用过一次即作废，未使用的可以撤销 |
| 开放注册 | 任何能访问服务的人都可以注册普通账号；每个 IP 每小时最多 10 次 |

接口见 [`openapi.yaml`](../../api/openapi.yaml) 的 `admin` 标签；设计见 [0003](../decisions/0003-direct-fetch-and-invites.md)。

## 统计与请求记录

站点会记录订阅请求的 IP、User-Agent、时间，以及由本机地理数据查出的国家与运营商，供链接所有者在「配置链接」与「订阅分析」页做安全检查；站点访问与设置项热度只按天记次数，不记录是谁。站内消息加密保存。细节见 [stats](../design/stats.md) 与 [sharing](../design/sharing.md)。对外部署时，请在你的隐私说明中告知用户这一点。

### IPv6 地址的省市（可选）

镜像带的 ip2region 只有 IPv4 库，登录设备与订阅请求中的 IPv6 地址只显示国家与运营商。IPv6 库约 37 MB，按需自行加上：从 [lionsoul2014/ip2region](https://github.com/lionsoul2014/ip2region/tree/master/data) 下载 `ip2region_v6.xdb`，挂载进地理数据目录后重启：

```yaml
services:
  clash-launch:
    volumes:
      - ./ip2region_v6.xdb:/opt/mihomo/geodata/ip2region_v6.xdb:ro
```

启动时校验文件头：不是 IPv6 库时日志给出警告，只停用它，其余地理数据照常使用。只把约 512 KB 的索引读入内存，其余按需从文件读取。

## 监控

`/metrics` 以 Prometheus 文本格式提供运行指标：请求数与耗时（按路由模式）、配置生成耗时（含内核检查）、数据库语句耗时、订阅与规则源的拉取结果、后台任务的次数、失败与耗时、实时在线人数与 Go 运行时。指标不含用户数据，但会暴露各接口的使用情况，所以：

- 没有设置 `CLASH_LAUNCH_METRICS_TOKEN` 时，只接受**直接**来自本机或内网、且不带转发头的请求。经反向代理转来的请求即使代理在本机也会被拒绝（404），公网上看不到这个端点。
- 设置了令牌时只认令牌，可以经反向代理抓取：

  ```yaml
  # prometheus.yml
  scrape_configs:
    - job_name: clash-launch
      authorization: { credentials: <令牌> }
      static_configs: [{ targets: ["clash-launch.example.com"] }]
      scheme: https
  ```

管理页的「后台任务」用同一份数据显示每个任务最近一次的运行时间、耗时与失败原因。数据在内存中，服务重启后从零开始。

## 出站与服务器 IP

服务器直接拉取订阅与规则源，对方能看到服务器的出口 IP；能添加订阅的用户也可以借此得知它。需要隐藏时，在宿主机或网络层让整台服务器经代理出网。SSRF 防护与请求头约束见 [network](../design/network.md)。

## 备份与升级

```sh
docker compose exec clash-launch /clash-launch backup /data/backups/manual.db
```

- 备份数据库时**必须同时备份主密钥**（`/data/master.key`）：丢失密钥就无法解密。
- 升级时启动会自动执行迁移，执行前自动备份到 `/data/backups/`；不支持降级，降级需要恢复备份。数据库被更新的版本迁移过之后，旧版本程序会拒绝启动并提示原因（`ErrNewerSchema`），不会在不认识的表结构上运行。
- 轮换主密钥：`/clash-launch keys rotate`（追加新密钥并重新加密全部数据）。
- 迁到 PostgreSQL 或换机器：`db copy`，步骤与保证见 [data](data.md)。
- 使用 PostgreSQL 时，应用内的 `backup` 命令不可用（它只处理 SQLite），改用数据库自带的工具，并同样备份主密钥：

  ```sh
  pg_dump --format=custom --file=clash-launch-$(date +%F).dump "$CLASH_LAUNCH_DATABASE_URL"   # 备份
  pg_restore --clean --if-exists --dbname="$CLASH_LAUNCH_DATABASE_URL" clash-launch-2026-11-20.dump   # 还原到同版本
  ```

  PostgreSQL 上的迁移没有自动备份：升级前先执行一次 `pg_dump`。
