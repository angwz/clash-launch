# Clash, Launch! 实施规范 · 交付与交接

版本：1.0 · 日期：2026-11-09 · 第 17～22 节

[返回规范入口](index.md) · 上一册：[03 · 服务与界面](03-service-and-web.md)

## 17. 技术栈

| 层 | 选择 | 理由 |
| --- | --- | --- |
| 后端语言 | Go，当前稳定版（写入 `go.mod` 的 `toolchain`，**待环境核验**版本号） | 可以直接复用 mihomo 的分享链接解析、节点校验与 YAML 行为；编译为单个静态二进制 |
| HTTP | 标准库 `net/http`（`ServeMux` 的方法与路径模式） | 不引入路由框架 |
| 日志 | 标准库 `log/slog`，JSON 输出 | 结构化、可脱敏 |
| YAML | `go.yaml.in/yaml/v3` | 与 mihomo 相同；Node API 能控制顺序、标签与注释 |
| TOML | `github.com/pelletier/go-toml/v2`（**默认**） | 维护活跃，错误信息带位置 |
| 正则 | `github.com/dlclark/regexp2` | 与 mihomo `filter` 语义一致；设置 `MatchTimeout` |
| 存储 | 默认 SQLite：`modernc.org/sqlite`（纯 Go，免 CGO），WAL 模式；可替换为 PostgreSQL（`pgx`），见 17.1 | 自托管零运维，单文件易于备份；免 CGO 才能干净地交叉编译 armv7 |
| 密码学 | `golang.org/x/crypto/argon2`；标准库 `crypto/aes`、`crypto/cipher` | — |
| 账号安全 | WebAuthn 与 TOTP 选用维护活跃的 Go 库；工作量证明验证自托管（**待环境核验**选型） | 不依赖第三方服务 |
| 邮件 | 维护活跃的 SMTP 库（标准库 `net/smtp` 已冻结，**待环境核验**选型），支持 STARTTLS 与隐式 TLS | 用户自填 SMTP 的异常登录提醒 |
| 内核 | mihomo，固定版本，SHA-256 记录在 `tools.lock.json`；Go 代码依赖与校验用的二进制使用同一版本 | 解析与校验行为一致 |
| 前端 | Svelte 5、TypeScript（strict）、Vite、CodeMirror 6 | 沿用 domainrouter_v2；CodeMirror 比 Monaco 轻，适合低带宽 |
| 打包 | 前端构建产物通过 `embed` 打进二进制 | 单文件部署；满足严格 CSP |

### 17.1 存储的可迁移性

默认使用 SQLite，但不能把它写死：用户多了以后，要能换成 PostgreSQL，而不必改业务代码。

- **仓储接口**：业务代码只依赖 `internal/store` 中按领域划分的仓储接口（用户、源、方案、链接、审计……），不出现 SQL；每种数据库各有一份实现。
- **可移植的 SQL**：表结构与查询只使用两种数据库共有的写法：主键用应用生成的 UUIDv7（文本），不依赖自增 ID；时间统一以 UTC 存储；布尔、JSON 字段的表示方式写进决定；不使用 SQLite 独有的语法与类型宽松特性（建表使用 `STRICT`）。
- **迁移脚本**按数据库分目录（`migrations/sqlite/`、`migrations/postgres/`），版本号一一对应，只向前；CI 检查两套迁移得到相同的逻辑结构。
- **数据搬迁**：提供 `clash-launch db copy --from <DSN> --to <DSN>`，按表流式复制并校验行数与校验和，用于从 SQLite 迁到 PostgreSQL。
- **单实例假设显式化**：后台拉取、缓存、限流计数都经接口访问。1.0 只支持单实例；以后多实例部署时，拉取任务改用数据库租约锁，缓存与限流换成共享实现，业务代码不变。
- 1.0 **必须**实现 SQLite；PostgreSQL 实现为**可选**，但接口、迁移目录结构与 `db copy` 命令的设计须在 M3 定下，并有一个最小的 PostgreSQL 冒烟测试（有条件时在 CI 中用服务容器运行）。

### 17.2 依赖原则

能用标准库就不引入依赖；每个新增依赖在决定或 `THIRD_PARTY_NOTICES.md` 中写明用途与许可；许可必须与 GPL-3.0-or-later 兼容。

## 18. 部署

### 18.1 镜像

```text
stage 1  node:<LTS>        构建 web/ → web/dist
stage 2  golang:<stable>   go build（CGO_ENABLED=0，-trimpath，注入版本号）；下载并校验 mihomo
stage 3  gcr.io/distroless/static:nonroot
         /clash-launch · /opt/mihomo/mihomo · 以 uid 65532 运行
```

- 架构：`linux/amd64`、`linux/arm64`、`linux/arm/v7`；每种架构附带对应的 mihomo 二进制。
- 运行要求：只读根文件系统；唯一可写位置为数据卷 `/data`（SQLite、快照、主密钥）与 `tmpfs` 上的 `/tmp`（内核校验临时目录）。
- 健康检查：二进制自带 `clash-launch healthcheck` 子命令（distroless 中没有 curl）。
- 发布到 `ghcr.io/angwz/clash-launch`；发布时生成 SBOM 与构建来源证明，并用 cosign 无密钥签名（发布属于远端操作，见第 20 节）。

### 18.2 配置

环境变量为主，也可用 `CLASH_LAUNCH_CONFIG` 指向一个 TOML 文件；同名设置以环境变量为准。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `CLASH_LAUNCH_LISTEN` | `:8080` | 监听地址 |
| `CLASH_LAUNCH_DATA_DIR` | `/data` | 数据目录 |
| `CLASH_LAUNCH_DATABASE_URL` | `sqlite:///data/clash-launch.db` | 数据库地址；`postgres://…` 时使用 PostgreSQL（第 17.1 节） |
| `CLASH_LAUNCH_PUBLIC_URL` | — | 对外地址，用于生成订阅链接；未设置时根据请求推断，并在管理页提示 |
| `CLASH_LAUNCH_MASTER_KEY` / `_FILE` | — | 主密钥（第 14.4 节） |
| `CLASH_LAUNCH_TRUSTED_PROXIES` | — | 受信任的反向代理网段 |
| `CLASH_LAUNCH_EGRESS_PROXY` | compose 中指向 WARP 边车 | 出站代理（`socks5h://…` 或 `http://…`），隐藏服务器真实 IP（第 14.6 节） |
| `CLASH_LAUNCH_FETCH_ALLOWLIST` | — | SSRF 白名单（网段或主机名） |
| `CLASH_LAUNCH_LOG_LEVEL` | `info` | 日志级别 |

### 18.3 示例与运维

- `compose.yaml`：本服务加 WARP 出口边车（第 14.6 节）；命名卷、只读根文件系统、`tmpfs`、健康检查、`restart: unless-stopped`。另附不带边车的最小示例。
- `docs/guide/reverse-proxy.md`：Caddy 与 Nginx 的 HTTPS 示例；说明为何订阅链接必须走 HTTPS。
- 备份：`clash-launch backup <文件>` 在 SQLite 下使用在线备份 API 生成一致的快照（PostgreSQL 使用其自带的备份工具）；文档强调主密钥要单独备份。
- 升级：启动时自动执行迁移，执行前自动备份数据库到 `/data/backups/`；不支持降级，降级需要恢复备份。

## 19. 工程质量与开源资料

### 19.1 仓库结构

```text
clash-launch/
├─ cmd/clash-launch/          入口：serve、render、admin、keys、backup、db、healthcheck
├─ internal/
│  ├─ engine/                 管线编排、诊断、缓存键
│  ├─ catalog/                目录加载、导出（表单元数据、JSON Schema、i18n 源）
│  ├─ layer/                  两层合并：表单与原始编辑（合并标签）
│  ├─ node/                   内容识别、整理、地区识别与装饰 emoji、标签与倍率
│  ├─ kernel/                 mihomo 适配层：分享链接解析、节点校验、内核校验
│  ├─ ruleset/                rulesets.toml、groups.toml、外部配置 ini 的解析、校验、展开；匹配器与现代选择器
│  ├─ target/                 预设与适配
│  ├─ render/                 YAML 渲染与文件头
│  ├─ source/                 拉取、快照、SSRF 防护（唯一联网的包）
│  ├─ store/                  仓储接口与 SQLite/PostgreSQL 实现、迁移、加密（唯一持久化的包）
│  ├─ auth/                   账号、会话、CSRF、令牌、TOTP
│  └─ api/                    HTTP 处理、订阅端点、问题详情
├─ api/openapi.yaml           API 契约
├─ catalog/                   数据：mihomo/ 字段目录与客户端预设；regions/ 地区、线路词与装饰 emoji 表；rulesources/ 第三方规则预设
├─ tools/                     catalog-extract、i18n-sync、文档链接检查
├─ web/                       Svelte 前端
├─ testdata/                  测试数据：只用保留示例地址与假凭据
├─ docs/                      见 19.3
├─ Dockerfile · compose.yaml · tools.lock.json
└─ README.md · CLAUDE.md · CONTRIBUTING.md · SECURITY.md · CHANGELOG.md · THIRD_PARTY_NOTICES.md · LICENSE
```

依赖方向：`api → engine → (catalog, layer, node, ruleset, target, render, kernel)`；`engine` 不依赖 `store`、`source`、`api`。用 `go list` 检查依赖方向的测试锁定这一点。

### 19.2 检查与测试

| 类别 | 要求 |
| --- | --- |
| 静态检查 | `gofmt`、`go vet`、`staticcheck`、`golangci-lint`（配置入库）；前端 `svelte-check --fail-on-warnings`、ESLint、Prettier |
| 单元测试 | 表驱动；合并语义、路径语法、整理步骤、toml 展开全覆盖 |
| fuzz | 分享链接解析、内容识别、原始编辑合并、GROUPID 范围解析：不得 panic，凭据不得进入错误信息 |
| golden | 渲染结果；`go test -update` 更新，更新必须人工审阅 |
| 内核 | `-tags kernel`：真实 mihomo `-t` |
| 差异 | 与 angwz/subconverter 的产物对比（第 6.6 节），包括 domainrouter_v2 的两份 toml 与一个 ACL4SSR 变体 |
| 端到端 | Playwright：向导、表单、源排序与编号、添加订阅时的出口说明、隐藏能源入口与原始编辑、订阅链接、伪 RTL、手机视口；后端使用真实二进制 |
| 安全 | 第 14 节的测试清单：越权（404）、CSRF、SSRF（含重定向与 DNS rebinding）、限流、上传、YAML 炸弹、正则超时 |
| 体积 | 首屏 JS 预算（第 15.4 节）；镜像体积写进 performance 文档 |

CI 工作流（外部 Action 一律固定到完整提交 SHA；权限最小化；来自 fork 的 PR 不获得密钥）：

- `ci.yml`：检查、测试、内核、端到端、构建多架构镜像（不推送）；Linux 与 Windows 都运行 Go 测试。
- `catalog-watch.yml`：每周对比 mihomo 最新 release 与目录，有差异时开 Issue（第 8.3 节）。
- `i18n-sync.yml`（以后再做）：源消息变化时运行机器翻译，以 PR 提交译文（第 15.2 节）。
- `release.yml`：只能手动触发；构建、签名、推送镜像，创建 GitHub Release。

### 19.3 文档

照搬 domainrouter_v2 的文档体系：

| 文件 | 作用 |
| --- | --- |
| `README.md` | 一屏讲清是什么、长什么样、怎么跑起来；产物样例；支持范围表 |
| `CLAUDE.md` | 给代理：先读什么、硬性边界、呈现要求、常用命令、代码约定 |
| `docs/README.md` | 文档地图：按「使用 / 设计 / 开发 / 历史 / 给 AI 代理」分组，每份文档一句「读完你会知道」 |
| `docs/guide/` | 部署、反向代理、节点源、规则源、原始编辑与覆写标签、目标预设、多语言贡献 |
| `docs/design/` | 架构、字段目录、合并语义、安全与威胁模型、subconverter 差异、性能（只写实测值） |
| `docs/decisions/` | 编号递增的决定；与其他文档冲突时以决定为准。实施开始时的第一份是 `0001`：相对本规范的调整 |
| `docs/development/progress.md` | 唯一的状态记录：区分「已实现」与「已验证」；过期内容直接覆盖 |
| `docs/development/feedback.md` | 维护者的每条意见：一句话中性总结、状态、落点；**不引用原话** |
| `docs/history/spec-1.0/` | 本规范在实施完成后原样归档，只读 |
| `CHANGELOG.md` | Keep a Changelog 格式 |
| `CONTRIBUTING.md` · `SECURITY.md` · `THIRD_PARTY_NOTICES.md` | 参与方式、私密漏洞报告、第三方来源与许可 |

文档要求（**必须**）：同时面向人与 AI 代理；结构清晰、措辞克制、信息密度高；同一事实只在一处详述，其他地方链接；文档中的命令、路径、配置键与实现一致；CI 检查文档内链接与锚点有效。

### 19.4 代码约定

- 代码注释用简体中文，解释原因、不变量与复杂度，不逐行复述代码。
- 先写简单直接的实现，出现真实重复再抽象。不吞错误；错误用 `%w` 包装并带上下文；不静默降级。
- 每个可验证的小目标：实现 → 测试 → 整理（删除废弃分支与临时补丁）→ 同步文档与 progress → 提交。
- 提交信息用中文，`feat:`/`fix:`/`docs:`/`chore:`/`ci:` 前缀，说明改了什么、为什么。
- 测试数据只用 `example.com`、`192.0.2.0/24`、`2001:db8::/32` 与明显虚构的凭据；仓库中不得出现任何真实节点、订阅或令牌。
- Windows 上开发时注意换行：仓库统一 LF（`.gitattributes` 已设置）。

## 20. 里程碑与验收

| 阶段 | 内容 | 验收（全部**必须**） |
| --- | --- | --- |
| **M1 引擎** | 工程骨架、CLAUDE.md、决定 0001；目录加载与导出（先覆盖 general、dns、tun、sniffer、proxy-groups、rule-providers、rules 与主要协议）；两层合并；渲染与格式自检；内核检查；CLI `clash-launch render <方案文件>` | 合并语义表驱动测试与 fuzz；golden 测试；格式自检在全部测试数据上通过；渲染结果通过 `mihomo -t`；两次渲染逐字节一致；`catalog-extract` 能对两个 mihomo 版本输出字段差异 |
| **M2 输入** | 内容识别、分享链接与节点校验、整理、有序源与订阅编号、地区识别与国旗、标签与倍率、groups/rulesets 展开（含 `!!GROUPID=` 等匹配器与现代选择器）、外部配置 ini 与规则预设、拉取与快照、SSRF 防护 | 分享链接 fuzz；GROUPID 范围语法与 fork 行为一致的测试；地区识别的刁钻名称测试表全部通过；用维护者真实的两份 toml（domainrouter_v2 release）与合成节点生成配置并通过内核校验；与 subconverter 的差异逐条写入 parity 文档 |
| **M3 服务** | 仓储接口、SQLite 与迁移、加密、单管理员账号、API 与 OpenAPI 契约、订阅端点、订阅链接、后台拉取、缓存；Dockerfile 与 compose | 本地 Docker 运行；用 mihomo 内核实际加载订阅链接；API 契约测试通过；存储可迁移性的设计写进决定 |
| **M4 Web** | 向导、方案编辑三栏、字段卡片（全部字段、重要程度、示例）、源排序与编号、命令面板、隐藏能源（原始编辑）、节点源与规则源页、策略组预览、订阅链接、版本历史、导入（Clash YAML、Verge Merge、Stash 覆写） | Playwright 桌面与手机视口全部通过；视觉经维护者确认 |
| **M5 多用户与安全** | 注册（安全问题、工作量证明）、找回、通行密钥、TOTP、SMTP 异常登录提醒、会话管理、出站代理与拉取权限、配额、审计、限流、管理页；威胁模型文档 | 安全测试清单逐项通过并记录 |
| **M6 目标与目录** | 全部预设；Stash 变体目录；目录补齐全部协议与分节（含实验性字段），说明、示例与重要程度齐全；`catalog-watch`；`zh-CN`、`en` 完整，伪 RTL 布局检查 | 各预设的依据链接齐全；目录覆盖率报告（相对 `catalog-extract` 的字段全集）写进 progress；伪 RTL 截图测试 |
| **M7 发布** | README、指南、CHANGELOG、多架构镜像、签名 | 由维护者决定何时创建远端、推送与发布 |

**远端操作**（创建 GitHub 仓库、推送、发布镜像、创建 Release、配置 Actions secret、接入 Weblate）只在维护者明确要求时进行；仓库描述、主题等元数据须先经维护者确认（domainrouter_v2 的惯例）。

每个里程碑结束时：progress 写明已实现与已验证的内容（附实际命令与结果），feedback 更新状态，提交。

## 21. 依据与待核验项

### 21.1 依据（2026-11-09 查证）

| 事实 | 来源 |
| --- | --- |
| mihomo 最新稳定版 v1.19.33（2026-10-21）；近期版本持续新增字段（`dns-mode`、`hash-key`、`default-selected`、`empty-fallback` 等），v1.19.27 移除 `global-client-fingerprint` | <https://github.com/MetaCubeX/mihomo/releases> |
| 配置文档目录 | <https://wiki.metacubex.one/config/> |
| mihomo 使用 `go.yaml.in/yaml/v3`、`dlclark/regexp2` | mihomo `go.mod`（Meta 分支） |
| `common/convert.ConvertsV2Ray` 支持的 scheme，不含 wireguard | mihomo `common/convert/converter.go`（Meta 分支） |
| 主流客户端与维护状态 | <https://wiki.metacubex.one/startup/client/client/> |
| Stash 覆写语义：`#!replace`、`#!append`，列表默认插入到开头 | <https://stash.wiki/configuration/override> |
| Clash Verge Rev Merge 的 prepend/append 键 | 用户指南镜像，需以官方文档核对 |
| Google 翻译 2022 年退出中国大陆 | 2022-10 多家媒体报道（如 SCMP、CNBC） |
| Google Cloud 在伊朗等受制裁地区不可用 | Google Workspace 帮助中心、Google Cloud 社区 |
| ACL4SSR 外部配置的语法：`[custom]`、`ruleset=`、反引号分隔的 `custom_proxy_group=` | ACL4SSR 仓库 `Clash/config/ACL4SSR_Online_Full.ini` |
| subconverter 匹配器：`!!GROUPID=`/`!!INSERT=` 共用范围语法（`matchRange`），可接 `!!正则` | angwz/subconverter `src/generator/config/subexport.cpp` 的 `applyMatcher` |
| groups.toml 校验与 subconverter 行为基准 | domainrouter_v2@2c812f9 的 `src/domainrouter_v2/groups.py`、`docs/guide/subconverter.md`；angwz/subconverter@a33d49e |

### 21.2 待环境核验

1. 直接 import mihomo 模块是否受其 `replace` 指令影响；不行时的适配层方案（§5.3）。
2. `mihomo -t` 是否会下载 geodata 与 rule-provider；无网络检查的覆盖边界（§11.3）。
3. 配置结构体在 mihomo 源码中的位置，`catalog-extract` 的扫描范围（§8.3）。
4. 各预设的 `defaults` 与 `managed`，特别是 OpenClash、nikki、ShellCrash 接管哪些字段（§10.2）。
5. Stash 的字段全集与协议支持表（§10.3）。
6. Clash Verge Rev 当前版本的 Merge 语义（§9.2）。
7. groups.toml 中 `ssid` 类型、`empty-fallback` 与 `COMPATIBLE` 的处理（§6.2）；内联规则含多个逗号时的组名位置（§6.3）；Surge 列表作为 mihomo classical text 的差异（§6.3）。
8. sing-box outbounds 到 mihomo 节点的逐协议映射（§5.2）。
9. meta-json-schema 的许可（§8.3）。
10. Weblate 免费托管的条件；Google Cloud Translation 的计费、配额与术语表功能（§15.2，以后再做）。
11. Go 稳定版与 Node LTS 的具体版本号（§17、§18）。
12. subconverter fork 的 `!!GROUPID=` 开区间方向、`!!INSERT=` 的编号规则、订阅编号的赋值方式，以及 `!!GROUP=`、`!!TYPE=` 等其余匹配器的语义（§6.2、§5.7）。
13. WebAuthn 的 Go 实现库选型；工作量证明验证的方案选型（自托管、无第三方请求）（§14.1）。
14. 外部配置 ini 中 `custom_proxy_group` 末段数字的含义与其余键的语义；subconverter 是否忽略 groups.toml 中的未知表 `select`（§6.4、§6.5）。
15. mihomo 加载 classical text 列表时遇到不支持的规则类型的行为（§6.5）；各第三方规则预设的地址、变体、许可与使用人数（§6.5）。
16. WARP 边车：官方客户端在容器中的代理模式是否可行、免费版的条款与限制、镜像选型；Go HTTP 客户端与 SMTP 库经 SOCKS5 出口的实现方式（§14.6）。

核验结果写进对应的决定或设计文档，并在 progress 中勾掉。

## 22. 启动提示语



全文见 [start-prompt.md](start-prompt.md)；它的第一句是：

> 请读取工作区 `docs/spec/index.md`（Clash, Launch! 实施规范 1.0 入口），再逐份完整读取它列出的四份分册；被截断时分段继续。
