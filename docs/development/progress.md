# 进度

> 项目的唯一状态记录。区分「已实现」与「已验证」，过期内容直接覆盖，不保留流水账。

## 现在

- **阶段**：M1（引擎）、M2（输入）完成；M3（服务）除 Docker 实测外完成；下一步 M4（Web）。
- **待维护者**：本机 Docker 守护进程未运行。启动 Docker Desktop（或提供远程 Docker 并同意在其上构建）后，补做镜像构建与 compose 运行验证。
- **远端**：尚未创建仓库；按维护者要求，远端操作（M7）暂不执行。

## 里程碑

| 阶段 | 状态 | 验收结果 |
| --- | :---: | --- |
| M1 引擎 | ✅ | 见下表 |
| M2 输入 | ✅ | 见下表 |
| M3 服务 | 🔄 | Docker 未验证，其余见下表 |
| M4 Web | 🕓 | — |
| M5 多用户与安全 | 🕓 | — |
| M6 目标与目录 | 🕓 | — |
| M7 发布 | ➖ | 由维护者决定何时进行 |

## M1 验收

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| 合并语义表驱动测试与 fuzz | ✅ | `TestMerge` 27 个用例；`FuzzMerge` 60 秒无失败 |
| golden 测试 | ✅ | `testdata/render/basic`、`testdata/profiles/m1-layers` |
| 格式自检在全部测试数据上通过 | ✅ | 每次渲染执行；`FuzzRender` 约 88 万次执行无失败（修复 5 类边界后） |
| 渲染结果通过 `mihomo -t` | ✅ | `go test -tags kernel ./internal/kernel/`：两份 golden 均通过 mihomo v1.19.33（Windows amd64） |
| 两次渲染逐字节一致 | ✅ | `TestProfilesGolden` 每份方案渲染两次比较 |
| `catalog-extract` 能对两个版本输出字段差异 | ✅ | v1.19.32 → v1.19.33：新增 `tun.dns-mode`、`listeners.tun.dns-mode`；单元测试覆盖 order 与 diff |
| CLI `clash-launch render <方案文件>` | ✅ | `-check` 端到端：示例方案通过 `mihomo -t` |

## M2 验收

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| 分享链接 fuzz | ✅ | `FuzzParse`（内容识别 + 分享链接 + sing-box + 校验）120 秒约 10 万次执行无失败；检查凭据不进入错误信息 |
| GROUPID 范围语法与 fork 行为一致 | ✅ | `TestMatchRange` 26 个用例逐条对照 subexport.cpp；`FuzzMatchRange` 20 秒无失败 |
| 地区识别的刁钻名称测试表全部通过 | ✅ | `testdata/regions/tricky.yaml` 55 条 |
| 用维护者真实的两份 toml 与合成节点生成配置并通过内核校验 | ✅ | `testdata/workspace/domainrouter`：domainrouter_v2 release（23 组、66 条 rulesets）+ 3 个有序源 → golden；`go test -tags kernel` 通过 mihomo v1.19.33 |
| 与 subconverter 的差异逐条写入 parity 文档 | ✅ | [subconverter-parity](../design/subconverter-parity.md) 22 条；rules 与组成员由源码参考模型逐条对比（domainrouter_v2 两份 rulesets、ACL4SSR Online 与 Online_Full），**未运行实际的 subconverter** |

## M3 验收

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| 本地 Docker 运行 | ❌ 未验证 | 本机 Docker 守护进程未运行；`Dockerfile`、`compose.yaml`、`compose.minimal.yaml`、`deploy/warp/` 已编写，未构建。二进制以 `clash-launch serve` 在本机运行：迁移、主密钥生成、设置链接、healthz、readyz、healthcheck 正常 |
| 用 mihomo 内核实际加载订阅链接 | ✅ | `TestKernelLoadsSubscription`（`-tags kernel`）：服务端渲染并通过 `mihomo -t` → 从 `/s/<令牌>` 取回配置 → mihomo v1.19.33（Windows amd64）实际启动并在 mixed-port 上监听 |
| API 契约测试通过 | ✅ | `TestContract`：41 条路由与 `api/openapi.yaml` 逐条一致；`TestEndToEnd` 等覆盖登录、CSRF、If-Match（400/412）、订阅、304、重置、越权 404、限流 429、503、SSRF |
| 存储可迁移性的设计写进决定 | ✅ | [0002](../decisions/0002-storage-and-service.md)；`TestMigrationsEquivalent`；`db copy` 在 SQLite → SQLite 上测试，**PostgreSQL 冒烟测试未运行**（本机没有实例） |

## 能力一览

| 能力 | 位置 | 已实现 | 已验证 |
| --- | --- | :---: | --- |
| 字段目录：加载、片段、校验、键序、导出 | `catalog/mihomo/`、`internal/catalog` | ✓ | 单元测试；文档锚点对照 wiki 快照；顶层、dns、tun、sniffer、17 种协议、rule-provider 的顺序对照官方示例 |
| 安全 YAML 解析 | `internal/yamlx` | ✓ | 体积、节点数、别名展开、重复键经合并与渲染测试间接覆盖 |
| 表单层校验与原始编辑合并 | `internal/layer` | ✓ | 表驱动、fuzz |
| 目标适配（版本门槛、不支持、接管、弃用） | `internal/target` | ✓ | 端到端（`global-client-fingerprint` 移除警告）；预设只有 `mihomo` |
| 渲染与格式自检 | `internal/render` | ✓ | 刁钻标量表、golden、fuzz |
| 内核检查（离线 `mihomo -t`） | `internal/kernel` | ✓ | mihomo v1.19.33 实测：合法配置通过，坏节点失败并定位到 `proxies[0]` |
| 方案文档 | `internal/profile` | ✓ | 经引擎测试覆盖 |
| 管线编排、缓存键 | `internal/engine` | ✓ | golden、确定性、依赖方向测试 |
| 节点输入与整理、地区识别 | `internal/node`、`catalog/regions/` | ✓ | 单元测试、刁钻名称表、fuzz |
| 规则源、匹配器、现代选择器、预设 | `internal/ruleset`、`catalog/rulesources/` | ✓ | 单元测试、差异测试、真实数据（domainrouter_v2、ACL4SSR） |
| 拉取、SSRF、出站代理、快照 | `internal/source` | ✓ | httptest 与最小 SOCKS5 服务器；**未在真实 WARP 出口上验证** |
| 生成阶段（有序源、编号、provider 模式） | `internal/engine/build.go` | ✓ | 工作区 golden（两份方案）、内核检查 |
| 存储、迁移、加密、备份、db copy | `internal/store`、`migrations/` | ✓ | 单元测试（SQLite）：所有者隔离、乐观锁、AAD 绑定、轮换后旧密文可解密；PostgreSQL 未测试 |
| 口令、令牌 | `internal/auth` | ✓ | 经 API 测试覆盖 |
| 服务编排、调度、托管 provider | `internal/service` | ✓ | API 端到端测试；后台调度器只在本机冒烟运行中启动，未长时间运行 |
| HTTP API、订阅端点、安全响应头、限流 | `internal/api`、`api/openapi.yaml` | ✓ | 契约测试、端到端测试、内核加载测试 |
| 镜像与部署 | `Dockerfile`、`compose*.yaml`、`deploy/warp/` | ✓ | **未构建、未运行** |
| 命令行 | `cmd/clash-launch`、`internal/workspace` | ✓ | 手动运行 `render -check -inputs`、`catalog export`、`serve`、`healthcheck` |
| 工具 | `tools/catalog-extract`、`tools/toolsync` | ✓ | 单元测试；toolsync 实际下载并校验 SHA-256 |

## 最近一次验证

2026-11-09 · Windows 10 x64 · Go 1.27.3

```text
gofmt -l .                         → 无输出
go vet ./...                       → 通过
staticcheck ./...                  → 通过（2026.2.2）
golangci-lint run ./...            → 0 issues（v2.15.0）
go test ./...                      → catalog、engine、layer、render、catalog-extract 通过
go test -tags kernel ./internal/kernel/   → 通过（mihomo v1.19.33）
go test -fuzz FuzzRender（180 s）  → 878,011 次执行无失败
go test -fuzz FuzzMerge（60 s）    → 无失败
go test -fuzz FuzzParse（120 s）   → 100,228 次执行无失败
go test -fuzz FuzzMatchRange（20 s）→ 无失败
go test -tags kernel ./internal/kernel/ ./internal/api/ → 通过（含实际加载订阅链接）
clash-launch serve（本机）          → 迁移、设置链接、healthz、readyz、healthcheck 正常
```

未验证：Linux 上的测试（CI 尚未创建）；Docker 镜像构建与 compose 运行（本机守护进程未运行）；WARP 边车；经真实 WARP 出口拉取；PostgreSQL；与实际运行的 subconverter 对比。

## 已知限制

- 目录：`listeners` 的逐类型字段，以及 xhttp 细项、shadow-tls/restls/jls、realm、AmneziaWG 等节点子选项暂以 `any` 透传（M6 补齐）；详细说明与示例覆盖率在 M6 前补齐。
- 目标预设只有 `mihomo`；其余预设在 M6 按客户端官方文档逐项核对后加入。
- `geoip.dat` 的哈希是首次下载时记录的，没有上游公布的校验值可对照。
- sing-box：`multiplex` 不转换为 `smux`；只读取 `outbounds`，不读取 `endpoints` 中的 WireGuard。
- groups.toml 中组的透传键按键名排序输出（go-toml 解码为 map 时丢失原顺序）；目录内的键由渲染器按目录顺序排列，不受影响。

## 下一步

1. M4：Svelte 5 前端——向导、方案编辑三栏、字段卡片、源排序与编号、命令面板、后备隐藏能源、节点源与规则源页、策略组预览、订阅链接、版本历史、导入；Playwright。
2. Docker 可用后补做 M3 的镜像与 compose 验证。
