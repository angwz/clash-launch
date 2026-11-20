# 进度

> 项目的唯一状态记录。区分「已实现」与「已验证」，过期内容直接覆盖，不保留流水账。

## 现在

### 下次从这里开始（2026-11-20）

- **状态**：第 2、3 批已完成（R47–R53：订阅随源的修改立即更新、保存后返回列表、节点源列表直接删除、显示上次时间、节点内容即时预览、不被浏览器自动填充、恢复默认可撤销），未推送。R15、R16 暂不处理（mihomo 通用链接已满足需求）；R18 暂缓到有可用的 Docker 环境时。
- **下一步**：① 等维护者确认 R54 中英文间距与三处配色的样例（截图在本次对话中给出），确认后实施并删去 `a11y.spec.ts` 中的豁免；② R14 需要 PostgreSQL 实测，本机没有，等维护者决定是否在本机安装便携版；③ 继续批 8 的后半（R22 正在输入、在线状态、多标签页共用连接）与批 9 R11 在线指示灯。
- **工作方式**：维护者按 token 额度安排，每批开新对话；汇报用大白话讲功能变化与验证结果。收到「开始」前只登记需求、不编码。新意见照常追加到 [feedback](feedback.md)，完成的需求移入 backlog 的「已完成」。
- **本机环境要点**（Windows + Git Bash）：
  - 工具链不在系统 PATH：`export PATH="$PWD/.cache/toolchain/go/bin:$PWD/.cache/toolchain/node-v26.13.0-win-x64:$PATH"`；`gofmt`、`goimports`、`staticcheck`、`golangci-lint` 分别在 `.cache/toolchain/go/bin` 与 `.cache/gopath/bin`。
  - 没有 Python，脚本化编辑用 `node -e` 或 `awk`；`sed` 的多行插入容易出错，复杂改动用编辑工具。
  - mihomo 与地理数据在 `.cache/tools/`（`go run ./tools/toolsync` 按 `tools.lock.json` 下载校验，已含 `ip2region_v4.xdb`）。
  - e2e 共用一个后端、串行运行，注册有「每 IP 每小时 10 次」的限制：新测试不要再新注册账号，改用管理员或已有用户；也不要改管理员的用户名与密码。完整 e2e 约 3.5 分钟，当前基线 40 通过 22 跳过。
  - `golangci-lint` 全仓 0 issues（O4 已清零），新问题都应修掉。
- **新迁移**：下一个编号是 0013；两种数据库各一个同号文件，加入 `migrations/SUMS`（`TestMigrationsFrozen` 会打印要追加的行），新表加入 `store.Tables`。

### 各批次记录

以下按时间记录每批的实现与验证；当前状态以上一节为准。

- **阶段**：M1、M2、M5 完成；M3 除 Docker 实测外完成；M4 第一阶段完成。核心路径（反馈 #14–#16）已完成，见「核心路径验收」；反馈 #17 的三项修正已完成（首次注册免令牌与简单口令、默认值显示、取消接管提示）。
- **反馈 #21–#25（2026-11-13）**：规则源可粘贴文本（并修复粘贴文本被地址快照覆盖）；节点源只分订阅与手动节点，源列表显示位置序号、GROUPID 按需显示；输出排版向官方示例看齐（节点默认多行、条目间空行）；链式代理（[chains](../design/chains.md)）。验证：`go test ./...`、`-tags kernel`（含新的 chains golden）、前端 check / 41 项单元 / build / budget 53.4 KiB / e2e 14 通过 6 跳过。本机未装 staticcheck、golangci-lint，由 CI 检查。
- **反馈 #26–#31（2026-11-14）**：管理页版本号改为 git describe 形式并显示提交与构建时间；预览按场景停靠或浮层显示并高亮当前字段；开关默认态改为文字；界面用词统一（[0004](../decisions/0004-web-frontend.md) #25）；字段「说明与建议」与 `guide` 建议值（7 个分区）；侧栏「帮助」含 7 篇教程与字段百科。验证：`go test ./...`、前端 check / 60 项单元 / build / budget 54.0 KiB / e2e 16 通过 6 跳过（含帮助页与预览新行为）。image 工作流的 git describe 已在 GitHub 上验证。
- **反馈 #32–#46 第一批（2026-11-14）**：配置导航去掉与主菜单重复的生成分节，配置名移到分节导航顶端，历史与直接编辑 YAML 收进末端「高级」；字段卡片「说明与建议」与「恢复默认」分处两端；预览开关移到顶栏；横向滚动条的两个根因（隐藏浮层带位移、表内绝对定位标签逃出滚动容器）已修，e2e 覆盖主区；细滚动条；侧栏 GitHub 入口；节点写法的全局默认值与锁定策略（迁移 0004 `users.invited`，`profile.mode_locked`）。验证：`go test ./...`、前端 check / 61 项单元 / build / budget 54.4 KiB / e2e 17 通过 5 跳过。第二批：直接编辑防误删测试；自动保存与撤销（连续自动保存合并为一个历史版本，`TestAutosaveCoalesces`，e2e 17 通过）。菜单顺序按用户保存（`TestPrefs`、e2e 18 通过）。WireGuard 配置文件导入（单元测试；转换结果经本机 mihomo v1.19.33 -t 通过）。空策略组：方案选项 `output.empty-groups`，默认移除（`TestPruneEmpty`、新 golden 经 mihomo -t）。状态 emoji 采用 Fluent Emoji Flat；前端 68 项单元、e2e 18 通过、budget 56.8 KiB。反馈 #32–#46 全部完成。CI 新增 `postgres` 任务：存储与 API 测试在 PostgreSQL 17 上通过（含迁移 0004、0005），发现并修正了两处测试中直接写的 SQLite 占位符。
- **反馈 #47–#52（2026-11-14）**：预览按钮回到编辑区右上角（滚动区内的粘性条）；「更多」默认收起并收纳分区说明（链到站内百科）；字段说明区域可点击展开（账号偏好可关）；配置模板（迁移 0006，加密存储，默认模板唯一）与新建对话框中直接粘贴节点。验证：`go test ./...`（含 `TestTemplates` 与全路由隔离测试）、前端 72 项单元、e2e 19 通过、budget 57.2 KiB。
- **反馈 #53–#57（2026-11-14）**：编辑器保存冲突与「检查中」卡住的两个根因已修；「常见做法」；「说明与建议」点击范围三选一；全站左缘与缩进梳理；新增「设置」页。验证：`go test ./...`、前端 74 项单元、e2e 20 通过。
- **反馈 #58–#65（2026-11-14）**：关闭的分节整段不下发（依据 mihomo 源码逐个确认，预览显示为注释）；空配置可渲染、新建配置可设嵌套字段；模板可更新与改名；侧栏底部与配置菜单重做；「密码」；自动内核检查；默认名称。验证：`go test ./...`（含 `TestDisabledSections`、`TestEmptyConfig`）、关闭分节的配置经 mihomo -t、前端 78 项单元、e2e 20 通过。
- **反馈 #66–#68（2026-11-14）**：统计（迁移 0007）：打开网站、页面、登录、订阅请求按天永久累计；订阅请求逐条记录与按 IP 汇总；令牌无效的来源；设置项热度每周重算并用于排序（总开关始终第一）。验证：`go test ./...`（`TestStats`、`TestStatsFlow`、全路由隔离与契约）、前端 82 项单元、e2e 21 通过、budget 58.5 KiB。
- **反馈 #69（2026-11-14）**：正式版文案梳理：去掉开发期的实现细节与内部术语（源、快照、字段目录、配置文档、生成器），检查结果统一用界面上的说法；用户名规则的三处说明与实际校验（1–32 个字符、不含空格）对齐；帮助按自动保存与自动检查更新；删除已不会出现的设置链接错误。验证：`go test ./...`、前端 check / 82 项单元 / build / budget 58.2 KiB / e2e 21 通过。迁移 0006、0007 在 PostgreSQL 17 上的验证见下文 2026-11-16 的 CI 记录。
- **反馈 #70（2026-11-15）**：「直接编辑 YAML」改为在与预览一致的完整配置上修改，服务端换算为只含差异的原始编辑层（`engine.Absorb`、`layer.Diff`，接口 `/absorb`），订阅与规则更新后修改继续生效。验证：`TestAbsorb*`（往返逐字节一致、更新后保留修改、未启用分节不误删、无效输入）、`go test ./...`、前端 82 项单元、e2e 21 通过（含在完整 YAML 上新增一行只记下这一行）。
- **反馈 #71（2026-11-15）**：为开放注册做准备：默认写法为「交给客户端」时，开放注册用户的订阅只由客户端拉取，服务器不访问（调度跳过，手动拉取返回 409 `source.client_fetch`）；开放注册用户使用更小的默认配额（`CLASH_LAUNCH_OPEN_QUOTA_*`）；预览与内核检查按用户限速（每分钟 240 / 30 次，管理员不限），前端在检查被限速时只做预览；UUIDv7 改为进程内严格递增，修正同一毫秒内订阅请求记录顺序不稳定。验证：`TestNodeModePolicy`（策略、拒绝拉取、配额、限速）、`TestQuotaAndAdmin`、`TestNewIDMonotonic`、`go test ./...`、前端 82 项单元、e2e 21 通过。
- **反馈 #72（2026-11-15）**：普通用户的分流规则地址只能来自公共代码托管站（`source.Hosts`，主机名精确匹配、只接受 https、重定向逐跳检查；保存时校验，拉取时再校验；`CLASH_LAUNCH_RULE_HOSTS` 可替换）；配置链接旁「导出」（`/s/<令牌>?download=1`，附文件名、记为 export 请求）；用户时区（`PUT /account/timezone`，界面时间与未指定时区的配置文件头按它显示；可输入任意部分、不区分大小写自动补全）。验证：`TestHostsAllows`、`TestFetchHostsRedirect`、`TestRuleHostsExportTimezone`、`go test ./...`、前端 82 项单元、e2e 22 通过（含时区补全与导出下载）。
- **反馈 #73（2026-11-15）**：分流规则可拖动排序（预设与自己的规则源，指针与键盘，`prefs.rules` 保存在服务端；向导与编辑器同序），预设默认顺序改为 `rank`（ACL4SSR 推荐并居首，domainrouter_v2 移到最后）；时区列表常用在前；页面自动刷新（`lib/live.ts`：可见时轮询、切回即刷新、操作后稍后刷新）用于配置链接、节点、全站统计。数据可迁移性：真实数据演练发现 4219e64 曾修改已发布的迁移 0001（多出 `sources.allow_direct`），`db copy` 改为按列名对应、登记废弃列、版本不一致拒绝；新增 `TestMigrationsFrozen`（`migrations/SUMS`）与 `TestUpgradePreservesData`；规则写入 [data](../guide/data.md) 与 CLAUDE.md。验证：`go test ./...`、前端 83 项单元、e2e 23 通过；本机真实库在线备份 → `db copy` → 新库启动就绪。
- **反馈 #74（2026-11-15）**：订阅分析页（请求记录时用本机地理数据记录国家与运营商，迁移 0008；按用户时区的每日与星期 × 小时、地区、运营商、客户端、IP 明细与拉取间隔、新出现的 IP）；短链接 `/u/<短码>`（不重定向、计入原链接与短链接、停用与过期、猜错限速）；配置链接与短链接的「导入」菜单（Clash Verge Rev、Mihomo Party、FlClash、Clash Meta for Android 与导出文件）；站内消息（emoji、附件为节点或订阅、自动加入收件人节点页、不接收与屏蔽、加密保存、未读角标）。验证：`go test ./...`（含 `TestAnalyze`、`TestSocialRepos`、`TestShortLinksAnalyticsMessages`、`TestLookup` 与覆盖新路由的隔离测试）、前端 86 项单元、e2e 25 通过；截图检查了三个新页面。
- **反馈 #75（2026-11-15）**：审计日志按页码分页；消息入口移到右上角图标，收件人按用户名前缀联想并标出是否接收；交流过的人永久保留（联系人表，迁移 0009），× 删除放入回收站保存一年，可恢复，对方来信自动恢复；节点「分享」（副本）与「授权」（只读软链接，生成配置时用授权者的快照并一律写入配置）；短链接授权为好友单独生成新短链接，订阅分析按好友统计（`FriendUsage`，可只看某位好友）；删除对话收回双方全部授权；短链接改为根路径（保留路径表与前端路由对照测试），随机短码 5 位起；分析中本机与内网地址显示「本地网络」，查不到的留空。验证：`go test ./...`（含 `TestGrantsContactsRootShort`、`TestReservedCoversFrontendRoutes`）、前端 88 项单元、e2e 27 通过（含授权与回收站、审计分页）；截图检查了联想、授权消息、回收站与授权节点页。
- **反馈 #76（2026-11-15）**：节点页每一行可直接分享或授权（`ShareDialog`，收件人联想抽成 `UserPicker`）；配置模板可分享（附件 template，对方得到副本）；一键导入在点击时上报（`POST /api/v1/imports`，记为 import 并带客户端名称）；注册时安全问题可选并逐个出现（0–3 个，找回答对 min(2, n) 个，诱饵补足 3 个）；管理页用户按页码分页并可搜索，全站来源先显示 20 个；「服务器直接拉取、出口 IP」只给管理员看，其余用户可见的文字去掉服务器视角；立即拉取后自动刷新并告知结果，节点详情每 30 秒刷新；节点概况（可用节点数、协议分布、订阅流量与到期时间）。验证：`go test ./...`（含 `TestOptionalQuestionsTemplatesImportsPaging`、`TestParseUserinfo`）、前端 88 项单元、e2e 28 通过；截图检查了节点页与注册页。
- **反馈 #77（2026-11-15）**：节点相关的剩余明文改为加密保存（迁移 0010：快照响应头、渲染响应头、检查结果、失败原因；旧明文在启动时加密并清除）；密钥轮换补上遗漏的 `messages.body_enc`，并以测试保证每个 `*_enc` 列都参与轮换；`TestNoPlaintextAtRest` 逐表逐列扫描确认没有明文；帮助新增「隐私与安全」，节点页加一句说明，如实写明运营者在技术上可以解密。验证：`go test ./...`、前端 88 项单元、e2e 29 通过（三次全量运行中有一次出现过一个 toHaveText 断言失败，未能复现）。
- **反馈 #78（2026-11-15）**：短链接卡片默认折叠，只显示短码、状态、指向、请求次数与上次请求时间（`StatsRepo.LastByShort`），趋势、二维码与管理操作在「详情」里；节点页每行的编辑改为带文字的「编辑」按钮。验证：`go test ./internal/api/ ./internal/store/`、前端 88 项单元、e2e 29 通过；截图检查了两个页面。
- **反馈 #79（2026-11-15）**：消息改为悬浮窗（`components/chat/ChatPanel.svelte`，状态在 `lib/chat.svelte.ts`），首次打开时按需加载；宽度 `clamp(320px, 62vw, 880px)`、高度随视口变化，容器查询按窗口自身宽度在两栏与单栏间切换，640px 以下铺满屏幕；`/messages` 与 `/messages/{name}` 重定向为打开悬浮窗。验证：`npm run check`、构建、首屏预算 65.2 KiB、e2e 30 通过（新增悬浮窗尺寸与 Esc 关闭）；截图检查了 900×640 与 1440×900。
- **反馈 #80（2026-11-15）**：消息悬浮窗点击窗口外关闭（`pointerdown` + `composedPath()`，避免窗口内被立即移除的元素如联想选项被误判为窗口外）。验证：`npm run check`、构建、预算 65.2 KiB、e2e 30 通过（新增窗口外点击关闭、窗口内点击不关闭、图标开关）。
- **反馈 #81–#84（2026-11-15，方案阶段）**：分流可视化定稿，未写功能代码。规范 [routing](../design/routing.md)，取舍 [0007](../decisions/0007-routing-visualization.md)，交互原型 [routing-prototype.html](../design/routing-prototype.html)（本机无头浏览器检查：无控制台错误；四种示例结构的分列、规则折叠、追踪自动展开、清空按钮、组提醒、粒子模式均按预期）。依据：mihomo v1.19.33 源码（`RULE-SET` 经全局 tunnel 查找，故改用策略对象；url-test 只比较最近一次测速等）、维护者 groups.toml 的实测（23 组、74 条引用、全部引用 7 层、默认路径 4 层）、流向色的调色板校验。
- **分流可视化第 0 步（2026-11-15）**：`internal/match` 匹配引擎。domain、ipcidr 用内核策略对象，text、yaml 的逐行解析与 MRS 文件头移植自内核；普通规则与逻辑规则经内核解析，子规则递归拦截；含无法判断成分（逻辑规则内的 RULE-SET、SUB-RULE、连接信息、未填的应用或端口、缺地理数据）的规则按三值判断；不解析 DNS，记录 `NeedsIPAt` 与 `IPCandidates`；命中依据用单条目的内核策略对象定位。地理数据：内核的 GEOIP/GEOSITE/IP-ASN 构造函数会联网下载且可能 `log.Fatalln`，改用内核导出的无全局状态加载器与匹配器，结论与基准写入 [0007](../decisions/0007-routing-visualization.md) 的「实测」。验证：`go test ./...`、`go vet ./...`、`staticcheck`、`golangci-lint`（`internal/match` 0 issues）；`TestParseParity`（对照内核 `rulesParse`）、`TestGeoMatchesKernel`（本机地理数据上与内核规则对象逐个一致）；基准 10 万条编译 345 ms、一次最坏追踪 16 µs。尚未接入 service 与接口。
- **分流可视化第 1 步（2026-11-15）**：讲解图层（`components/routing/ExplainLayer.svelte`）：节点与分流页「分流规则」标题旁的「?」打开，五步讲清一次请求经规则、策略组到出口；请求被规则认出之前不着色；窄屏时图在自身容器内滚动并跟随当前一步；方向键切换，Esc 关闭；按需加载。流向色 `--flow-*` 进入 token。规则集说明目录 `catalog/rulesets/descriptions.yaml`：domainrouter_v2 与 ACL4SSR 两家，177 个预设地址中 132 个有说明（第 2 步起经接口显示）。顺带修正 `ChatPanel.svelte` 一处四值 padding（`styles.test.ts` 自 #79 起失败）。验证：`go test ./...`（含 `TestRulesetDescriptions`）、前端 check / 90 项单元 / build / 首屏 66.0 KiB（此前 65.2，新增文案进入语言包）/ e2e 32 通过 18 跳过（新增「讲解图层」桌面与手机）；第一次完整运行时「分流规则：拖动手柄」失败一次，单独运行与再次完整运行均通过，未能复现；截图检查了桌面浅色、深色与手机。
- **分流可视化第 2 步（2026-11-15）**：配置编辑器新增「分流」分节（按需加载）。服务端：`/routing/graph|trace|health`，图按最终配置给出规则、组、出口、来源与组提醒；规则内容在内存中缓存（按字节 LRU、同一地址合并拉取、失败沿用旧内容并标记，复用 SSRF 防护与普通用户白名单），未缓存的在后台预取；样例的命中行由服务器追踪得到；每用户每分钟 60 次追踪；`CLASH_LAUNCH_TRACE`、`CLASH_LAUNCH_TRACE_CACHE_MB`。前端：按默认路径分列的地图、SVG 连线与粒子、智能视图（折叠、紧凑、出口归并，偏好可改）、窄屏列表、流向追踪（逐层依据、需要 IP 时的分支、清空）、常用网站体检、命令面板「追踪」。修正了实现中发现的两处：粒子场景在宽度抖动时被反复重建、手动追踪后被旧目标覆盖。验证：`go test ./...`、`go vet`、`staticcheck`；前端 check / 122 项单元 / build / 首屏 68.6 KiB（此前 66.0，增量为语言包）/ e2e 34 通过 18 跳过（新增「分流」桌面与手机）；用 domainrouter_v2 的真实发布文件（规则内容从 GitHub 实际拉取）截图检查了浅色、深色、追踪、体检与窄屏。`golangci-lint` 的 6 个问题在未改动的文件中，此前已存在。
- **分流可视化第 3 步（2026-11-15）**：分流可以在地图上编辑。方案文档新增 `routing` 段（键与 groups.toml、rulesets.toml 相同，另有 `bundle`），存在时引擎用它代替规则源；`POST /routing/fork` 生成开始自己管理时的 routing 段（domainrouter_v2 三件套自动识别为合集，`base` 留作之后的上游比对），追踪与体检接受草稿并给出应用前后的结果。前端：拖动规则改走向、移动、合并为合集，拖动组或节点加入成员，循环被拒绝；详情面板可改走向、上下移动、拆出合集、改组类型与参数（带适用场景与参数提醒）、设为首选与移除成员；改动先进草稿栏，「应用」写入 routing 段后照常自动保存、检查与撤销。验证：`go test ./...`（含 `TestRoutingRoundTrip`、`TestDetectBundles`、`TestForkINI`、`TestRoutingForkGolden`：domainrouter 开始自己管理后输出与跟随上游逐字节相同，文件头的名称与更新时间除外；`TestRoutingForkOwnDraft`）、`-tags kernel` 对新 golden 通过 mihomo -t；前端 check / 135 项单元（含 `draft.test.ts`）/ build / 首屏 70.5 KiB（增量为语言包）/ e2e 35 通过 19 跳过（新增「分流编辑」：合并与拆出、拒绝循环、改走向、改组类型、应用后预览、撤销）。截图 `web/test-results/shot-routing-edit.png`。
- **分流可视化第 4 步（2026-11-15）**：上游同步与期望，分流可视化全部完成（反馈 #81–#84）。`POST /routing/upstream` 按开始管理时的 `base` 做三方比对（组名、规则集地址对应，逐字段；移动按最长递增子序列；与用户自己的改动冲突时标出），分流页提示「规则源有 N 处更新」，对话框里逐条采纳或忽略，确认后进入草稿；追踪结果可「固定为期望」，期望在分流页列出是否满足（`routing.check_failed`、`routing.check_unknown`、`routing.upstream_changed`）。帮助新增「分流」教程（中英文），[rules](../design/rules.md) 新增 routing 段一节。顺带修正通用对话框的标题 id 写死、同页多个对话框共用一个名字的缺陷。验证：`go test ./...`（含 `TestRoutingUpstreamChecks`）、`staticcheck`、`-tags kernel`；前端 check / 138 项单元 / build / 首屏 71.1 KiB / e2e 36 通过 20 跳过（新增「规则源更新后提示并能采纳；从追踪固定期望」）。
- **反馈 #88（2026-11-16）**：分流页拖动后应用报「yamlx: 文档无效」的根因是后端的 go-yaml 不接受流式列表中未加引号、含 `?` 的普通标量（domainrouter_v2 的地区组正则 `(?=…)`），JS 的 yaml 库却不会加引号；写入方案文档时流式集合里含指示符的字符串一律加引号（routing 与链式代理）。「应用」改为先打开对比与检查（`ApplyDialog`：应用前后完整配置的逐行差异，未变的长段折叠；对应用后的配置做 mihomo -t，渲染失败或未通过时不能应用）。追踪后地图滚到可见；工具栏「重置视图」；下方追踪与详情两栏改为两等分、最小宽度 0，区域较窄时上下排列；详情里「默认」标签不再被拉成整行。「清空后追踪框与详情栏重叠」在 1440 宽与真实数据上未能复现。验证：用真实 domainrouter_v2 发布文件复现并确认修复（前端写出的七种编辑结果均能被后端解析），`go test ./...`、前端 139 项单元、e2e 36 通过 20 跳过（「分流编辑」加入含 `(?i)` 的正则组并经过对比对话框应用；「分流」覆盖重置视图与追踪后地图可见）。
- **推送与 CI（2026-11-16）**：本地积压的 35 个提交（反馈 #65–#89 与分流可视化）推送到 GitHub。CI 的 `postgres` 任务发现 `TestEncryptLegacy` 在测试里直接写了 SQLite 的 `?` 占位符，改用按数据库换算的 `d.exec` 后再次推送：ci 的 go、web、container、postgres 四项全部通过（存储与 API 测试在 PostgreSQL 17 上运行，覆盖迁移 0001–0010），image 发布 amd64、arm64 镜像。
- **需求清单（2026-11-16）**：生产部署后提出的需求逐项核实现状、评估工作量，连同此前的可选方向整理进 [backlog](backlog.md)（反馈 #90、#91）；R7 与 R11 附方案草案。维护者下达指令前只登记、不编码。
- **需求第 1 批（2026-11-16）**：R1 配置可以删除（列表卡片、编辑器「更多」与命令面板，对话框列出会一起删除的内容）；R2 删除节点源、配置、配置链接或短链接时先收回授权、重新生成对方的配置，并在对话中给对方发系统通知（`kind = notice`）；R6 账号记录每页 10 条。验证：`go test ./...`（新增 `TestDeleteRevokesGrantsAndNotifies`）、`go vet`、`staticcheck`、`golangci-lint`（service 与 api 0 issues）；前端 check / 139 项单元 / build / 首屏 71.6 KiB / e2e 38 通过 20 跳过（新增「删除配置」桌面与手机）。系统通知的样式未截图检查。
- **需求第 2 批（2026-11-16）**：R9 对外地址可写多个域名，按请求的 Host 选用并兼作白名单（链接、Cookie 的 Secure、HSTS、Origin 校验随之按域名决定；通行密钥只在主地址）；R8 登录设备定时退出（迁移 0011 新增 `sessions.logout_at`，只能早于自然到期，可取消，到点即失效并记审计；被定时的设备每页顶部提示）；R7 登录地点（同一迁移新增 `sessions.location`；ip2region IPv4 库进入 `tools.lock.json` 与镜像，约 11 MB；国家库与 ASN 库补充，IPv6 只有国家与运营商）。新增依赖 `lionsoul2014/ip2region/binding/golang`，未抬高 golang.org/x。验证：`go test ./...`（新增 `TestParsePublicURLs`、`TestPublicURLPerHost`、`TestScheduleLogout`、`TestPlaceRoundTrip`、`TestPlace`；本机数据上 114.114.114.114 → 中国 · 江苏省 · 南京市）、`go vet`、`staticcheck`；golangci-lint 在改动的包中只剩存储层 5 个旧的 nolintlint；前端 check / 140 项单元 / build / 首屏 72.0 KiB / e2e 39 通过 21 跳过（新增「定时退出」）。登录地点在本机只能以回环地址登录，界面上的显示未用真实公网地址验证。
- **需求第 3 批（2026-11-16）**：R4 改用户名（需要当前密码，180 天一次，旧名保留 180 天）；R5 昵称（可含空格、不唯一，搜索按用户名前缀或昵称中的任一段，消息与联想中显示「昵称 @用户名」）；R12 人人可邀请（名额与注册时长门槛、收回邀请权、可用多次、作废、二维码；注册页显示邀请人；注册开放时也记下邀请关系；邀请人收到系统通知并互为联系人；只有管理员的邀请赋予受邀权限，决定 [0003](../decisions/0003-direct-fetch-and-invites.md) 已补修订）。迁移 0012。顺带修正：表格外层滚动框没有定位，表头里绝对定位的读屏文字会跑出滚动框、在手机上撑宽管理页。验证：`go test ./...`（新增 `TestNicknameAndRename`、`TestInvitesForEveryone`）、`go vet`、`staticcheck`；前端 check / 140 项单元 / build / 首屏 72.8 KiB / e2e 40 通过 22 跳过（新增「昵称与邀请」）。改用户名在 e2e 中未覆盖（会影响共用的管理员账号），由 API 测试覆盖。
- **实施顺序第 2 批（2026-11-20）**：R47 保存规则源、节点源后立即重新生成用到它的配置（含被授权方），订阅链接随之更新；地址变了时丢弃旧地址的快照并立即拉取（此前改了地址而未拉取时，新地址下会继续下发旧地址的内容）；`rerenderUsing` 改为翻页、不再只取前 200 个配置；规则源缓存键带长度前缀并计入名称。R48 规则源与节点源保存后返回列表；R50 节点源列表直接删除；R53 订阅统计与配置链接页显示上次请求，首次放在悬停提示或副标题。验证：`go test ./...`（新增 `TestSubscriptionFollowsSourceEdits`，已确认在旧代码上失败；`TestRuleURLChangeRefetches`）、`go test -tags kernel`、`go vet`、`staticcheck`、golangci-lint（service、api 0 issues）；前端 check / 140 项单元 / build / 首屏 72.8 KiB / e2e 41 通过 23 跳过（新增「节点源：保存后返回列表…」）。维护者报告的现象未在生产站点上复现，以测试复现为准。
- **实施顺序第 3 批（2026-11-20）**：R49 节点源详情的节点表格始终反映表单当前状态（新接口 `…/nodes/preview`，不写存储，与保存后同一解析函数；只采用最后一次请求的结果），内容框随内容增高；R51 配置的敏感字段用 `autocomplete="new-password"` 并加密码管理扩展的忽略标记；R52 恢复默认后原位显示「已恢复默认 · 撤销」，YAML 字段的占位文字显示默认值或「由内核决定」。验证：`go test ./...`（新增 `TestSourceNodesPreview`，契约与隔离测试覆盖新路由）、`go vet`、golangci-lint（api 0 issues）；前端 check / 140 项单元 / build / 首屏 72.9 KiB / e2e 43 通过 25 跳过（新增两项）。R51 只验证了属性，真实 Chrome 中已保存密码时的表现需在维护者浏览器上确认。
- **实施顺序第 4 批（进行中，2026-11-20）**：O4 golangci-lint 全仓清零。R30 接入 axe（`@axe-core/playwright` 4.14.0 与 `axe-core` 4.14.0，固定为发布一个月以上的版本）：17 个页面的亮色与暗色按 WCAG 2.1 A/AA 检查；修复预览标签页 tablist 内混入普通按钮、窄屏搜索按钮在文字隐藏后没有名称（改为只在视觉上隐藏）、管理页可滚动表格不能用键盘聚焦；YAML 预览行号改为伪元素（外观不变）。剩余三处配色待确认、暂列豁免。验证：前端 check / 140 项单元 / build / 首屏 72.9 KiB / e2e 44 通过 26 跳过（含新的无障碍检查）；`go test ./...`、golangci-lint 0 issues。
- **实施顺序第 8 批第一步（2026-11-20）**：R22 实时通道。新包 `internal/events`（按用户分发、发布不阻塞、落后时发 resync）；`GET /api/v1/events` 升级为 WebSocket（`coder/websocket` v1.8.12，原为 mihomo 的间接依赖，直接使用后 go.mod 只把它移到直接依赖，golang.org/x 不变）：会话 Cookie 认证、握手校验 Origin、每用户最多 16 条、升级前清除 http.Server 的读写超时、每 30 秒心跳并重新核对会话。服务层在配置重新生成、节点源与规则源拉取或保存、消息、授权变化处发布，订阅被拉取时发布 sub；已读回执只在确有未读时发布，避免两边互相触发。前端 `realtime.svelte.ts` 负责连接、1–30 秒退避重连与重连后补读，`live()` 接受事件类型，8 处定时刷新改为事件驱动、轮询兜底。修正过程中发现并修复：探测请求的链接为空时取 `l.OwnerID` 会崩溃（被 recoverer 接住，表现为统计少记一条）。验证：`go test ./...`（新增 `TestHubRoutesPerUser`、`TestHubLimitAndLag`、`TestHubConcurrent`、`TestEventsStream`：真实 WebSocket 收到 hello、sub、source、render，跨站 403、未登录 401）、`-tags kernel`、golangci-lint 0 issues；前端 check / 142 项单元（新增 `realtime.test.ts`）/ build / 首屏 73.3 KiB / e2e 45 通过 27 跳过（新增「实时通道」：订阅被拉取后 5 秒内页面更新，定时刷新为 30 秒）。Nginx 的升级配置写入 reverse-proxy，未在真实 Nginx 上验证。
- **下一步**：见本节开头「下次从这里开始」。
- **待维护者**：① 在 OpenClash 中导入订阅链接实测（服务端检查已通过）。② 本机 Docker 守护进程未运行。启动 Docker Desktop（或提供远程 Docker 并同意在其上构建）后，补做镜像构建与 compose 运行验证。
- **远端**：仓库 https://github.com/angwz/clash-launch（GPL-3.0，2026-11-10 按维护者要求创建；2026-11-16 按维护者要求曾短暂转为私有，同日恢复公开）。GitHub Actions：`ci`（Linux 上的 Go 与前端测试、真实 mihomo 内核测试）与 `image`（amd64、arm64 镜像发布到 `ghcr.io/angwz/clash-launch`，匿名可拉取）。

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
| 存储、迁移、加密、备份、db copy | `internal/store`、`migrations/` | ✓ | 单元测试（SQLite）：所有者隔离、乐观锁、AAD 绑定、轮换后旧密文可解密；PostgreSQL 17：存储与 API 测试由 CI 的 `postgres` 任务运行（迁移 0001–0010，2026-11-16 通过）；`db copy` 只测了 SQLite → SQLite |
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

未验证：在 OpenClash 中实际导入（维护者验证）；容器实际运行（镜像已构建）；PostgreSQL 上的实际部署与 `db copy`（存储与 API 测试已在 CI 的 PostgreSQL 17 上通过）；与实际运行的 subconverter 对比；真实手机与硬件安全密钥上的通行密钥；真实邮件服务商；手机浏览器上工作量证明的耗时。

## 已知限制

- 目录：scan 得到的 2075 条路径全部收录（覆盖率见 [catalog](../design/catalog.md#覆盖率)）；scan 只覆盖 RawConfig、节点、策略组、provider 与 listeners，规则语法等非结构体内容不在其中。
- 目标预设有 `mihomo` 与 `openclash`；其余客户端预设在 M6 按官方文档或源码逐项核对后加入。
- `geoip.dat` 的哈希是首次下载时记录的，没有上游公布的校验值可对照。
- sing-box：`multiplex` 不转换为 `smux`；只读取 `outbounds`，不读取 `endpoints` 中的 WireGuard。
- groups.toml 中组的透传键按键名排序输出（go-toml 解码为 map 时丢失原顺序）；目录内的键由渲染器按目录顺序排列，不受影响。

## 下一步

待办需求、工作量、优先级与可进一步优化的地方统一记在 [backlog](backlog.md)。
