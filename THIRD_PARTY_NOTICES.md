# 第三方来源与许可

Clash, Launch! 以 GPL-3.0-or-later 发布。下表列出编译进程序、随镜像分发或复制了内容的第三方组件；规则源预设只引用地址，不复制其内容，不在此列（见 [rules](docs/design/rules.md#预设)）。

## Go 依赖（直接）

| 模块 | 版本 | 许可 | 用途 |
| --- | --- | --- | --- |
| `github.com/metacubex/mihomo` | v1.19.33 | GPL-3.0 | 分享链接解析（`common/convert`）与节点校验（`adapter.ParseProxy`） |
| `go.yaml.in/yaml/v3` | v3.0.5 | MIT 与 Apache-2.0 | YAML 解析与格式自检 |
| `github.com/pelletier/go-toml/v2` | v2.4.3 | MIT | groups.toml、rulesets.toml 解析 |
| `github.com/dlclark/regexp2` | v1.12.0 | MIT | 与 mihomo `filter` 一致的正则 |
| `golang.org/x/crypto` | v0.33.0 | BSD-3-Clause | argon2id 口令哈希 |
| `modernc.org/sqlite` | v1.36.1 | BSD-3-Clause | SQLite 驱动（纯 Go） |
| `github.com/jackc/pgx/v5` | v5.7.2 | MIT | PostgreSQL 驱动 |

间接依赖由 mihomo 引入，版本以 `go.sum` 为准。

## 复制的内容

| 内容 | 来源 | 许可 | 位置 |
| --- | --- | --- | --- |
| 常见口令表（长度 ≥ 10 的条目） | danielmiessler/SecLists@a1a06ab `Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt` | MIT | `internal/auth/common_passwords.txt`（由 `tools/commonpw` 生成） |
| 浏览器 User-Agent 表 | MetaCubeX/mihomo@v1.19.33 `common/convert/util.go` | GPL-3.0 | `internal/kernel/useragents.go`（生成） |

## 前端（编译进 web/dist）

| 包 | 版本 | 许可 | 用途 |
| --- | --- | --- | --- |
| `svelte`（含其运行时依赖 `clsx`、`esm-env`） | 5.57.2 | MIT | 界面框架（编译后的运行时） |
| `yaml` | 2.9.1 | ISC | 方案文档的结构编辑 |
| `intl-messageformat` | 12.1.4 | BSD-3-Clause | ICU MessageFormat |
| `@formatjs/*`（`intl-messageformat` 的依赖） | 见 `web/package-lock.json` | MIT | 消息解析 |
| `@codemirror/*`、`@lezer/*`、`style-mod`、`w3c-keyname`、`crelt` | 见 `web/package-lock.json` | MIT | 原始编辑器 |
| `qrcode-generator` | 2.0.4 | MIT | 本地生成二维码 |

开发与测试依赖（Vite、Vitest、Playwright、svelte-check、openapi-typescript 等）不随产物分发，版本以 `web/package-lock.json` 为准。

## 随镜像分发（M3）

| 组件 | 版本 | 许可 | 用途 |
| --- | --- | --- | --- |
| mihomo 可执行文件 | v1.19.33 | GPL-3.0 | 内核检查（`mihomo -t`），SHA-256 见 `tools.lock.json` |
| 地理数据 | MetaCubeX/meta-rules-dat@d5574ea0ab33 | GPL-3.0；ASN 数据为 MaxMind GeoLite2，CC BY-SA 4.0 | 内核检查时预置，避免联网下载 |
