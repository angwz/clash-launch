# 进度

> 项目的唯一状态记录。区分「已实现」与「已验证」，过期内容直接覆盖，不保留流水账。

## 现在

- **阶段**：M1、M2、M5 完成；M3 除 Docker 实测外完成；M4 第一阶段完成。核心路径（反馈 #14–#16）已完成，见「核心路径验收」；反馈 #17 的三项修正已完成（首次注册免令牌与简单口令、默认值显示、取消接管提示）。
- **反馈 #21–#25（2026-11-13）**：规则源可粘贴文本（并修复粘贴文本被地址快照覆盖）；节点源只分订阅与手动节点，源列表显示位置序号、GROUPID 按需显示；输出排版向官方示例看齐（节点默认多行、条目间空行）；链式代理（[chains](../design/chains.md)）。验证：`go test ./...`、`-tags kernel`（含新的 chains golden）、前端 check / 41 项单元 / build / budget 53.4 KiB / e2e 14 通过 6 跳过。本机未装 staticcheck、golangci-lint，由 CI 检查。
- **反馈 #26–#31（2026-11-14）**：管理页版本号改为 git describe 形式并显示提交与构建时间；预览按场景停靠或浮层显示并高亮当前字段；开关默认态改为文字；界面用词统一（[0004](../decisions/0004-web-frontend.md) #25）；字段「说明与建议」与 `guide` 建议值（7 个分区）；侧栏「帮助」含 7 篇教程与字段百科。验证：`go test ./...`、前端 check / 60 项单元 / build / budget 54.0 KiB / e2e 16 通过 6 跳过（含帮助页与预览新行为）。image 工作流的 git describe 已在 GitHub 上验证。
- **反馈 #32–#46 第一批（2026-11-14）**：配置导航去掉与主菜单重复的生成分节，配置名移到分节导航顶端，历史与直接编辑 YAML 收进末端「高级」；字段卡片「说明与建议」与「恢复默认」分处两端；预览开关移到顶栏；横向滚动条的两个根因（隐藏浮层带位移、表内绝对定位标签逃出滚动容器）已修，e2e 覆盖主区；细滚动条；侧栏 GitHub 入口；节点写法的全局默认值与锁定策略（迁移 0004 `users.invited`，`profile.mode_locked`）。验证：`go test ./...`、前端 check / 61 项单元 / build / budget 54.4 KiB / e2e 17 通过 5 跳过。第二批：直接编辑防误删测试；自动保存与撤销（连续自动保存合并为一个历史版本，`TestAutosaveCoalesces`，e2e 17 通过）。菜单顺序按用户保存（`TestPrefs`、e2e 18 通过）。WireGuard 配置文件导入（单元测试；转换结果经本机 mihomo v1.19.33 -t 通过）。空策略组：方案选项 `output.empty-groups`，默认移除（`TestPruneEmpty`、新 golden 经 mihomo -t）。状态 emoji 采用 Fluent Emoji Flat；前端 68 项单元、e2e 18 通过、budget 56.8 KiB。反馈 #32–#46 全部完成。CI 新增 `postgres` 任务：存储与 API 测试在 PostgreSQL 17 上通过（含迁移 0004、0005），发现并修正了两处测试中直接写的 SQLite 占位符。
- **反馈 #47–#52（2026-11-14）**：预览按钮回到编辑区右上角（滚动区内的粘性条）；「更多」默认收起并收纳分区说明（链到站内百科）；字段说明区域可点击展开（账号偏好可关）；配置模板（迁移 0006，加密存储，默认模板唯一）与新建对话框中直接粘贴节点。验证：`go test ./...`（含 `TestTemplates` 与全路由隔离测试）、前端 72 项单元、e2e 19 通过、budget 57.2 KiB。
- **反馈 #53–#57（2026-11-14）**：编辑器保存冲突与「检查中」卡住的两个根因已修；「常见做法」；「说明与建议」点击范围三选一；全站左缘与缩进梳理；新增「设置」页。验证：`go test ./...`、前端 74 项单元、e2e 20 通过。
- **反馈 #58–#65（2026-11-14）**：关闭的分节整段不下发（依据 mihomo 源码逐个确认，预览显示为注释）；空配置可渲染、新建配置可设嵌套字段；模板可更新与改名；侧栏底部与配置菜单重做；「密码」；自动内核检查；默认名称。验证：`go test ./...`（含 `TestDisabledSections`、`TestEmptyConfig`）、关闭分节的配置经 mihomo -t、前端 78 项单元、e2e 20 通过。
- **反馈 #66–#68（2026-11-14）**：统计（迁移 0007）：打开网站、页面、登录、订阅请求按天永久累计；订阅请求逐条记录与按 IP 汇总；令牌无效的来源；设置项热度每周重算并用于排序（总开关始终第一）。验证：`go test ./...`（`TestStats`、`TestStatsFlow`、全路由隔离与契约）、前端 82 项单元、e2e 21 通过、budget 58.5 KiB。
- **反馈 #69（2026-11-14）**：正式版文案梳理：去掉开发期的实现细节与内部术语（源、快照、字段目录、配置文档、生成器），检查结果统一用界面上的说法；用户名规则的三处说明与实际校验（1–32 个字符、不含空格）对齐；帮助按自动保存与自动检查更新；删除已不会出现的设置链接错误。验证：`go test ./...`、前端 check / 82 项单元 / build / budget 58.2 KiB / e2e 21 通过。PostgreSQL 上未重新验证迁移 0006、0007（推送后由 CI 的 `postgres` 任务覆盖）。
- **反馈 #70（2026-11-15）**：「直接编辑 YAML」改为在与预览一致的完整配置上修改，服务端换算为只含差异的原始编辑层（`engine.Absorb`、`layer.Diff`，接口 `/absorb`），订阅与规则更新后修改继续生效。验证：`TestAbsorb*`（往返逐字节一致、更新后保留修改、未启用分节不误删、无效输入）、`go test ./...`、前端 82 项单元、e2e 21 通过（含在完整 YAML 上新增一行只记下这一行）。
- **反馈 #71（2026-11-15）**：为开放注册做准备：默认写法为「交给客户端」时，开放注册用户的订阅只由客户端拉取，服务器不访问（调度跳过，手动拉取返回 409 `source.client_fetch`）；开放注册用户使用更小的默认配额（`CLASH_LAUNCH_OPEN_QUOTA_*`）；预览与内核检查按用户限速（每分钟 240 / 30 次，管理员不限），前端在检查被限速时只做预览；UUIDv7 改为进程内严格递增，修正同一毫秒内订阅请求记录顺序不稳定。验证：`TestNodeModePolicy`（策略、拒绝拉取、配额、限速）、`TestQuotaAndAdmin`、`TestNewIDMonotonic`、`go test ./...`、前端 82 项单元、e2e 21 通过。
- **反馈 #72（2026-11-15）**：普通用户的分流规则地址只能来自公共代码托管站（`source.Hosts`，主机名精确匹配、只接受 https、重定向逐跳检查；保存时校验，拉取时再校验；`CLASH_LAUNCH_RULE_HOSTS` 可替换）；配置链接旁「导出」（`/s/<令牌>?download=1`，附文件名、记为 export 请求）；用户时区（`PUT /account/timezone`，界面时间与未指定时区的配置文件头按它显示；可输入任意部分、不区分大小写自动补全）。验证：`TestHostsAllows`、`TestFetchHostsRedirect`、`TestRuleHostsExportTimezone`、`go test ./...`、前端 82 项单元、e2e 22 通过（含时区补全与导出下载）。
- **反馈 #73（2026-11-15）**：分流规则可拖动排序（预设与自己的规则源，指针与键盘，`prefs.rules` 保存在服务端；向导与编辑器同序），预设默认顺序改为 `rank`（ACL4SSR 推荐并居首，domainrouter_v2 移到最后）；时区列表常用在前；页面自动刷新（`lib/live.ts`：可见时轮询、切回即刷新、操作后稍后刷新）用于配置链接、节点、全站统计。数据可迁移性：真实数据演练发现 4219e64 曾修改已发布的迁移 0001（多出 `sources.allow_direct`），`db copy` 改为按列名对应、登记废弃列、版本不一致拒绝；新增 `TestMigrationsFrozen`（`migrations/SUMS`）与 `TestUpgradePreservesData`；规则写入 [data](../guide/data.md) 与 CLAUDE.md。验证：`go test ./...`、前端 83 项单元、e2e 23 通过；本机真实库在线备份 → `db copy` → 新库启动就绪。
- **反馈 #74（2026-11-15）**：订阅分析页（请求记录时用本机地理数据记录国家与运营商，迁移 0008；按用户时区的每日与星期 × 小时、地区、运营商、客户端、IP 明细与拉取间隔、新出现的 IP）；短链接 `/u/<短码>`（不重定向、计入原链接与短链接、停用与过期、猜错限速）；配置链接与短链接的「导入」菜单（Clash Verge Rev、Mihomo Party、FlClash、Clash Meta for Android 与导出文件）；站内消息（emoji、附件为节点或订阅、自动加入收件人节点页、不接收与屏蔽、加密保存、未读角标）。验证：`go test ./...`（含 `TestAnalyze`、`TestSocialRepos`、`TestShortLinksAnalyticsMessages`、`TestLookup` 与覆盖新路由的隔离测试）、前端 86 项单元、e2e 25 通过；截图检查了三个新页面。
- **反馈 #75（2026-11-15）**：审计日志按页码分页；消息入口移到右上角图标，收件人按用户名前缀联想并标出是否接收；交流过的人永久保留（联系人表，迁移 0009），× 删除放入回收站保存一年，可恢复，对方来信自动恢复；节点「分享」（副本）与「授权」（只读软链接，生成配置时用授权者的快照并一律写入配置）；短链接授权为好友单独生成新短链接，订阅分析按好友统计（`FriendUsage`，可只看某位好友）；删除对话收回双方全部授权；短链接改为根路径（保留路径表与前端路由对照测试），随机短码 5 位起；分析中本机与内网地址显示「本地网络」，查不到的留空。验证：`go test ./...`（含 `TestGrantsContactsRootShort`、`TestReservedCoversFrontendRoutes`）、前端 88 项单元、e2e 27 通过（含授权与回收站、审计分页）；截图检查了联想、授权消息、回收站与授权节点页。
- **反馈 #76（2026-11-15）**：节点页每一行可直接分享或授权（`ShareDialog`，收件人联想抽成 `UserPicker`）；配置模板可分享（附件 template，对方得到副本）；一键导入在点击时上报（`POST /api/v1/imports`，记为 import 并带客户端名称）；注册时安全问题可选并逐个出现（0–3 个，找回答对 min(2, n) 个，诱饵补足 3 个）；管理页用户按页码分页并可搜索，全站来源先显示 20 个；「服务器直接拉取、出口 IP」只给管理员看，其余用户可见的文字去掉服务器视角；立即拉取后自动刷新并告知结果，节点详情每 30 秒刷新；节点概况（可用节点数、协议分布、订阅流量与到期时间）。验证：`go test ./...`（含 `TestOptionalQuestionsTemplatesImportsPaging`、`TestParseUserinfo`）、前端 88 项单元、e2e 28 通过；截图检查了节点页与注册页。
- **待维护者**：① 在 OpenClash 中导入订阅链接实测（服务端检查已通过）。② 本机 Docker 守护进程未运行。启动 Docker Desktop（或提供远程 Docker 并同意在其上构建）后，补做镜像构建与 compose 运行验证。
- **远端**：公开仓库 https://github.com/angwz/clash-launch（GPL-3.0，2026-11-10 按维护者要求创建）。GitHub Actions：`ci`（Linux 上的 Go 与前端测试、真实 mihomo 内核测试）与 `image`（amd64、arm64 镜像发布到 `ghcr.io/angwz/clash-launch`，匿名可拉取）。

## 里程碑

| 阶段 | 状态 | 验收结果 |
| --- | :---: | --- |
| M1 引擎 | ✅ | 见下表 |
| M2 输入 | ✅ | 见下表 |
| M3 服务 | 🔄 | Docker 未验证，其余见下表 |
| M4 Web | 🔄 | 第一阶段见下表；视觉由实施者决定（反馈 #13） |
| M5 多用户与安全 | ✅ | 见下表；安全测试清单见 [security](../design/security.md) |
| M6 目标与目录 | 🔄 | OpenClash 预设与目录补全完成，其他客户端预设未开始 |
| M7 发布 | 🔄 | 仓库、CI 与镜像发布已上线；首个版本 v1.0.0 |

## 核心路径验收（反馈 #15）

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| OpenClash 预设有依据 | ✅ | [0006](../decisions/0006-openclash-preset.md)：参考 OpenClash@e553e82 的 `yml_change.sh` 等；内核为 mihomo Alpha，与 v1.19.33 只差 `tun.dns-mode` |
| 几乎全部字段可在表单配置 | ✅ | `catalog-extract coverage`：v1.19.33 scan 的 2075 条路径中 2070 条类型化收录、3 条源码本身为 any、2 条登记为源码未读取；透传与缺失均为 0 |
| 登录 → 订阅 → 规则源 → OpenClash → 订阅链接，服务端取回并通过 `mihomo -t` 与实际加载 | ✅ | 自动化：`TestKernelOpenClashFlow`（`-tags kernel`，规则源为 domainrouter_v2 的真实 toml）。真机：本机 `clash-launch serve`，订阅为本地提供的虚构节点，规则源为 domainrouter_v2 预设从 GitHub 实际拉取；取回 21 KB（4 节点、23 组、59 个规则集、66 条规则），`mihomo -t` 通过，实际启动后 mixed-port 监听，运行日志 0 error 0 warning |
| 在 OpenClash 中导入 | ➖ | 由维护者验证 |

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
| 镜像构建与运行 | 🔄 | 镜像在 GitHub Actions 中构建成功（amd64、arm64），清单可匿名拉取；容器实际运行尚未验证（本机 Docker 未运行），待维护者在 VPS 上首次部署 |
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
| 字段目录：加载、片段、校验、键序、导出 | `catalog/mihomo/`、`internal/catalog` | ✓ | 单元测试；文档锚点对照 wiki 快照；顶层、dns、tun、sniffer、28 种节点协议、20 种 listeners、rule-provider 的顺序对照官方示例；scan 覆盖率 100%（见核心路径验收） |
| 安全 YAML 解析 | `internal/yamlx` | ✓ | 体积、节点数、别名展开、重复键经合并与渲染测试间接覆盖 |
| 表单层校验与原始编辑合并 | `internal/layer` | ✓ | 表驱动、fuzz |
| 目标适配（版本门槛、不支持、弃用） | `internal/target` | ✓ | 端到端（`global-client-fingerprint` 移除警告）；预设 `mihomo`、`openclash`（[0006](../decisions/0006-openclash-preset.md)） |
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
| 镜像与部署 | `Dockerfile`、`compose.yaml`、`.github/workflows/` | ✓ | CI 构建并发布多架构镜像；容器运行待首次部署验证 |
| Web 前端 | `web/`（Svelte 5） | ✓ | Vitest 33 项（方案文档、差异、高亮、消息完整性、样式约束）；svelte-check 0 错误 0 警告；Playwright 见 M4 验收 |
| 命令行 | `cmd/clash-launch`、`internal/workspace` | ✓ | 手动运行 `render -check -inputs`、`catalog export`、`serve`、`healthcheck` |
| 工具 | `tools/catalog-extract`（scan、order、diff、coverage）、`tools/toolsync` | ✓ | 单元测试；toolsync 实际下载并校验 SHA-256 |

## 最近一次验证

2026-11-10 · Windows 10 x64 · Go 1.27.3 · Node 26.13.0 · Edge 157 · mihomo v1.19.33

```text
gofmt -l cmd internal tools        → 无输出
go vet ./...                        → 通过
staticcheck ./...                   → 通过
golangci-lint run ./...             → 0 issues
go test ./...                       → 全部通过
go test -tags kernel ./internal/kernel/ ./internal/api/ → 通过（含 TestKernelOpenClashFlow；首次注册改为免令牌后重跑）
catalog-extract coverage            → 2075 条：covered 2070、free 3、skipped 2、passthrough 0、missing 0
npm run check（svelte-check）       → 0 错误 0 警告
vitest                              → 37 通过
npm run build                       → 通过
playwright（Edge，桌面 + 手机）     → 14 通过、6 按设计跳过（反馈 #17 修改后重跑）
真机验收（clash-launch serve）      → 订阅取回 200，mihomo -t 通过，实际启动监听，日志 0 error
```

未验证：在 OpenClash 中实际导入（维护者验证）；容器实际运行（镜像已构建）；PostgreSQL；与实际运行的 subconverter 对比；真实手机与硬件安全密钥上的通行密钥；真实邮件服务商；手机浏览器上工作量证明的耗时。

## 已知限制

- 目录：scan 得到的 2075 条路径全部收录（覆盖率见 [catalog](../design/catalog.md#覆盖率)）；scan 只覆盖 RawConfig、节点、策略组、provider 与 listeners，规则语法等非结构体内容不在其中。
- 目标预设有 `mihomo` 与 `openclash`；其余客户端预设在 M6 按官方文档或源码逐项核对后加入。
- `geoip.dat` 的哈希是首次下载时记录的，没有上游公布的校验值可对照。
- sing-box：`multiplex` 不转换为 `smux`；只读取 `outbounds`，不读取 `endpoints` 中的 WireGuard。
- groups.toml 中组的透传键按键名排序输出（go-toml 解码为 map 时丢失原顺序）；目录内的键由渲染器按目录顺序排列，不受影响。

## 下一步

本轮核心路径已完成，等待维护者在 OpenClash 中导入实测并决定下一轮范围。可选方向（未排序）：

- 维护者导入后反馈的问题（优先）。
- 其他客户端预设（Clash Verge Rev、Stash 等），按官方文档或源码逐项核对。
- 配置导入、规则源编辑器与策略组预览（M4 剩余）。
- Docker 与 PostgreSQL 实测（需要环境）。
- 通行密钥按 #15 不再投入；如需精简，可删除 `internal/auth/webauthn.go`、`internal/service/passkey.go`、`internal/api/passkey.go` 与前端对应组件。
