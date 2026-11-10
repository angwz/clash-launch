# 进度

> 项目的唯一状态记录。区分「已实现」与「已验证」，过期内容直接覆盖，不保留流水账。

## 现在

- **阶段**：M1、M2、M5 完成；M3 除 Docker 实测外完成；M4 第一阶段完成。**开发暂停**（2026-11-10，维护者要求）：下一轮先做核心路径——登录与生成适用于 OpenClash 的配置，其余功能后续迭代。
- **待维护者**：① 确认下一轮的范围（见「下一步」）。② 本机 Docker 守护进程未运行。启动 Docker Desktop（或提供远程 Docker 并同意在其上构建）后，补做镜像构建与 compose 运行验证。
- **远端**：尚未创建仓库；按维护者要求，远端操作（M7）暂不执行。

## 里程碑

| 阶段 | 状态 | 验收结果 |
| --- | :---: | --- |
| M1 引擎 | ✅ | 见下表 |
| M2 输入 | ✅ | 见下表 |
| M3 服务 | 🔄 | Docker 未验证，其余见下表 |
| M4 Web | 🔄 | 第一阶段见下表；视觉由实施者决定（反馈 #13） |
| M5 多用户与安全 | ✅ | 见下表；安全测试清单见 [security](../design/security.md) |
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
| 本地 Docker 运行 | ❌ 未验证 | 本机 Docker 守护进程未运行；`Dockerfile`、`compose.yaml` 已编写，未构建。二进制以 `clash-launch serve` 在本机运行：迁移、主密钥生成、设置链接、healthz、readyz、healthcheck 正常 |
| 用 mihomo 内核实际加载订阅链接 | ✅ | `TestKernelLoadsSubscription`（`-tags kernel`）：服务端渲染并通过 `mihomo -t` → 从 `/s/<令牌>` 取回配置 → mihomo v1.19.33（Windows amd64）实际启动并在 mixed-port 上监听 |
| API 契约测试通过 | ✅ | `TestContract`：47 条路由与 `api/openapi.yaml` 逐条一致；`TestEndToEnd` 等覆盖登录、CSRF、If-Match（400/412）、订阅、304、重置、越权 404、限流 429、503、SSRF |
| 存储可迁移性的设计写进决定 | ✅ | [0002](../decisions/0002-storage-and-service.md)；`TestMigrationsEquivalent`；`db copy` 在 SQLite → SQLite 上测试，**PostgreSQL 冒烟测试未运行**（本机没有实例） |

## M4 验收（第一阶段）

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| Playwright 桌面与手机视口全部通过 | ✅ | `npm run e2e`：11 通过、3 按设计跳过（键盘用例只在桌面，横向滚动检查只在手机）；Windows 10 上的 Edge 157，后端为真实二进制 + 临时数据目录。覆盖登录、编辑与保存、命令面板与原始编辑、键盘排序、邀请注册与复用、深色、伪 RTL、手机无横向滚动、全部页面无 CSP 违规与页面错误 |
| 视觉经维护者确认 | ➖ | 维护者授权由实施者决定（反馈 #13） |
| 首屏 JavaScript ≤ 150 KiB（gzip） | ✅ | `npm run budget`：45.9 KiB（入口 37.2 + 登录页及依赖 2.3 + 语言包 6.5）；CodeMirror 约 136 KiB 按需加载 |
| 导入（Clash YAML、Verge Merge、Stash 覆写） | 🕓 | 未开始；Verge 合并文档的官方地址尚未找到 |
| 规则源编辑器（CodeMirror toml/ini）、策略组预览、选择器标签 | 🕓 | 规则源详情目前是纯文本框 |

## M5 验收

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| 安全测试清单逐项通过并记录 | ✅ | [security](../design/security.md) 的 A1–E2 共 38 项，每项对应自动化测试（单元、API 或端到端）；全部通过 |
| 注册（安全问题、工作量证明）、找回 | ✅ | `TestAccountSecurity`、e2e 找回；工作量证明 20 位在桌面实测平均 0.74 s |
| 通行密钥、TOTP | ✅ | `TestWebAuthn`、`TestPasskeys`、`TestTOTPVectors`、`TestTOTPFlow`；e2e 用 Chromium 虚拟认证器走通注册与免密码登录 |
| SMTP 异常登录提醒 | ✅ | `TestMailer`（假 SMTP 服务器）、`TestSMTPSettingsAndAlerts`；**未对真实邮件服务商发送** |
| 会话管理、配额、审计、限流、管理页 | ✅ | `TestQuotaAndAdmin`、`TestOwnerIsolationAllRoutes`（按路由表逐条检查他人资源为 404） |
| 出站代理与拉取权限 | ➖ | 由 [0003](../decisions/0003-direct-fetch-and-invites.md) 取消 |

## 能力一览

| 能力 | 位置 | 已实现 | 已验证 |
| --- | --- | :---: | --- |
| 字段目录：加载、片段、校验、键序、导出 | `catalog/mihomo/`、`internal/catalog` | ✓ | 单元测试；文档锚点对照 wiki 快照；顶层、dns、tun、sniffer、17 种协议、rule-provider 的顺序对照官方示例 |
| 安全 YAML 解析 | `internal/yamlx` | ✓ | 体积、节点数、别名展开、重复键经合并与渲染测试间接覆盖 |
| 表单层校验与原始编辑合并 | `internal/layer` | ✓ | 表驱动、fuzz |
| 目标适配（版本门槛、不支持、接管、弃用） | `internal/target` | ✓ | 端到端（`global-client-fingerprint` 移除警告）；预设 `mihomo`、`openclash`（[0006](../decisions/0006-openclash-preset.md)），`TestPresetManagedPathsInCatalog` 锁定接管路径 |
| 渲染与格式自检 | `internal/render` | ✓ | 刁钻标量表、golden、fuzz |
| 内核检查（离线 `mihomo -t`） | `internal/kernel` | ✓ | mihomo v1.19.33 实测：合法配置通过，坏节点失败并定位到 `proxies[0]` |
| 方案文档 | `internal/profile` | ✓ | 经引擎测试覆盖 |
| 管线编排、缓存键 | `internal/engine` | ✓ | golden、确定性、依赖方向测试 |
| 节点输入与整理、地区识别 | `internal/node`、`catalog/regions/` | ✓ | 单元测试、刁钻名称表、fuzz |
| 规则源、匹配器、现代选择器、预设 | `internal/ruleset`、`catalog/rulesources/` | ✓ | 单元测试、差异测试、真实数据（domainrouter_v2、ACL4SSR） |
| 直连拉取、SSRF、快照 | `internal/source` | ✓ | httptest；`Dialer.Control` 拦截 DNS rebinding；未对公网真实订阅长期运行 |
| 生成阶段（有序源、编号、provider 模式） | `internal/engine/build.go` | ✓ | 工作区 golden（两份方案）、内核检查 |
| 存储、迁移、加密、备份、db copy | `internal/store`、`migrations/` | ✓ | 单元测试（SQLite）：所有者隔离、乐观锁、AAD 绑定、轮换后旧密文可解密；PostgreSQL 未测试 |
| 口令、令牌 | `internal/auth` | ✓ | 经 API 测试覆盖 |
| 注册开关、一次性邀请 | `internal/service/register.go`、`internal/api/register.go` | ✓ | `TestInvites`、`TestRegistrationAndInvites`；无前端，未在浏览器中走过 |
| 服务编排、调度、托管 provider | `internal/service` | ✓ | API 端到端测试；后台调度器只在本机冒烟运行中启动，未长时间运行 |
| HTTP API、订阅端点、安全响应头、限流 | `internal/api`、`api/openapi.yaml` | ✓ | 契约测试、端到端测试、内核加载测试 |
| 镜像与部署 | `Dockerfile`、`compose.yaml` | ✓ | **未构建、未运行** |
| Web 前端 | `web/`（Svelte 5） | ✓ | Vitest 33 项（方案文档、差异、高亮、消息完整性、样式约束）；svelte-check 0 错误 0 警告；Playwright 见 M4 验收 |
| 命令行 | `cmd/clash-launch`、`internal/workspace` | ✓ | 手动运行 `render -check -inputs`、`catalog export`、`serve`、`healthcheck` |
| 工具 | `tools/catalog-extract`、`tools/toolsync` | ✓ | 单元测试；toolsync 实际下载并校验 SHA-256 |

## 最近一次验证

2026-11-10 · Windows 10 x64 · Go 1.27.3 · Node 26.13.0 · Edge 157

```text
gofmt -l                            → 无输出
go vet ./...                        → 通过
staticcheck ./...                   → 通过
golangci-lint run ./...             → 0 issues
go test ./...                       → 15 个有测试的包全部通过
go test -tags kernel ./internal/kernel/ ./internal/api/ → 通过（mihomo v1.19.33，含实际加载订阅链接）
npm run check（svelte-check）       → 0 错误 0 警告
vitest                              → 37 通过
npm run budget                      → 首屏 51.4 KiB（预算 150 KiB）
playwright（Edge，桌面 + 手机）     → 14 通过、6 按设计跳过
```

未验证：Linux 上的测试（CI 尚未创建）；Docker 镜像构建与 compose 运行；PostgreSQL；与实际运行的 subconverter 对比；真实手机与硬件安全密钥上的通行密钥；真实邮件服务商；手机浏览器上工作量证明的耗时。

## 已知限制

- 目录：`listeners` 的逐类型字段，以及 xhttp 细项、shadow-tls/restls/jls、realm、AmneziaWG 等节点子选项暂以 `any` 透传（M6 补齐）；详细说明与示例覆盖率在 M6 前补齐。
- 目标预设有 `mihomo` 与 `openclash`；其余客户端预设在 M6 按官方文档或源码逐项核对后加入。
- `geoip.dat` 的哈希是首次下载时记录的，没有上游公布的校验值可对照。
- sing-box：`multiplex` 不转换为 `smux`；只读取 `outbounds`，不读取 `endpoints` 中的 WireGuard。
- groups.toml 中组的透传键按键名排序输出（go-toml 解码为 map 时丢失原顺序）；目录内的键由渲染器按目录顺序排列，不受影响。

## 下一步

开发暂停中，下一轮在新对话中进行。方向见反馈 #14、#15：只做核心路径，其余功能留待后续迭代。

**下一轮范围（按顺序）**

1. **OpenClash 预设**：对照 OpenClash 官方文档与源码（只读参考，记录提交），确认它使用的内核版本、接管或改写的字段（端口、DNS、TUN、external-controller、geo 等），加入 `catalog/mihomo/presets/openclash.yaml`；依据写进决定。
2. **字段目录补全**：几乎全部 Clash（mihomo）字段都要能在表单中配置。优先补齐目前以 `any` 透传的部分（`listeners` 逐类型字段、xhttp、shadow-tls/restls/jls、realm、AmneziaWG 等节点子选项），以 `catalog-extract scan` 的覆盖率为准，目录说明中英齐全。
3. **验收**：用真实数据走通「登录 → 添加订阅 → 选规则源 → 选 OpenClash → 订阅链接」，服务端取回订阅输出并通过 `mihomo -t`（含 `-tags kernel` 的实际加载）即算达标；在 OpenClash 中导入由维护者自行验证。

**不在下一轮范围**：通行密钥的进一步工作（已实现并有测试，按 #15 不再投入；如需精简可在之后删除 `internal/auth/webauthn.go`、`internal/service/passkey.go`、`internal/api/passkey.go` 与前端对应组件）、配置导入、规则源编辑器与策略组预览、其他客户端预设、Docker 与 PostgreSQL 实测、M7。
