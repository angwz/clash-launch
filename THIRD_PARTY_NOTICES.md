# 第三方来源与许可

Clash, Launch! 以 GPL-3.0-or-later 发布。下表列出编译进程序、随镜像分发或复制了内容的第三方组件；规则源预设只引用地址，不复制其内容，不在此列（见 [rules](docs/design/rules.md#预设)）。

## Go 依赖（直接）

| 模块 | 版本 | 许可 | 用途 |
| --- | --- | --- | --- |
| `github.com/metacubex/mihomo` | v1.19.33 | GPL-3.0 | 分享链接解析（`common/convert`）与节点校验（`adapter.ParseProxy`） |
| `go.yaml.in/yaml/v3` | v3.0.5 | MIT 与 Apache-2.0 | YAML 解析与格式自检 |
| `github.com/pelletier/go-toml/v2` | v2.4.3 | MIT | groups.toml、rulesets.toml 解析 |
| `github.com/dlclark/regexp2` | v1.12.0 | MIT | 与 mihomo `filter` 一致的正则 |
| `golang.org/x/net` | v0.35.0 | BSD-3-Clause | SOCKS5 出站代理（`proxy`） |
| `golang.org/x/crypto` | v0.33.0 | BSD-3-Clause | argon2id 口令哈希 |
| `modernc.org/sqlite` | v1.36.1 | BSD-3-Clause | SQLite 驱动（纯 Go） |
| `github.com/jackc/pgx/v5` | v5.7.2 | MIT | PostgreSQL 驱动 |

间接依赖由 mihomo 引入，版本以 `go.sum` 为准。

## 复制的内容

| 内容 | 来源 | 许可 | 位置 |
| --- | --- | --- | --- |
| 浏览器 User-Agent 表 | MetaCubeX/mihomo@v1.19.33 `common/convert/util.go` | GPL-3.0 | `internal/kernel/useragents.go`（生成） |

## 随镜像分发（M3）

| 组件 | 版本 | 许可 | 用途 |
| --- | --- | --- | --- |
| mihomo 可执行文件 | v1.19.33 | GPL-3.0 | 内核检查（`mihomo -t`），SHA-256 见 `tools.lock.json` |
| 地理数据 | MetaCubeX/meta-rules-dat@d5574ea0ab33 | GPL-3.0；ASN 数据为 MaxMind GeoLite2，CC BY-SA 4.0 | 内核检查时预置，避免联网下载 |
