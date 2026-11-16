# 0007 · 分流视图、追踪与 routing 段

- 状态：已采纳并实施（2026-11-15）
- 范围：配置编辑器的「分流」视图（讲解图层、分流地图、流向追踪、常用网站体检、期望）与方案文档的 `routing` 段
- 实施规范：[design/routing.md](../design/routing.md)；交互原型：[design/routing-prototype.html](../design/routing-prototype.html)
- 依据：mihomo v1.19.33 源码（只读参考 `.cache/ref/mihomo`）、domainrouter_v2@2c812f9 的 `groups.toml` 与 `docs/guide/subconverter.md`；维护者意见见 [feedback](../development/feedback.md) #81–#84

## 决定

| # | 决定 | 理由 |
|---|---|---|
| 1 | **追踪在服务器上做，判断直接调用内核的规则集策略对象** | mihomo 的控制接口只有 `GET /rules`，没有「测一个请求命中哪条」。项目已以 Go 模块依赖 `github.com/metacubex/mihomo v1.19.33`，许可同为 GPL-3.0，调用内核代码可以保证与客户端逐条一致 |
| 2 | **不使用内核的 `RULE-SET` 规则对象**，而是自己持有每个规则集的策略对象（`provider.NewDomainStrategy`、`NewIPCidrStrategy`、`NewClassicalStrategy`，以及它们导出的 `Insert`、`FinishInsert`、`Match`、`FromMrs`），对 `RULE-SET` 行自行调用 | `rules/provider/rule_set.go` 的 `getProvider` 经全局的 `tunnel.RuleProviders()` 查找规则集，多用户的服务器不能依赖这个全局状态。其余规则类型用 `rules.ParseRule` 得到的对象直接匹配 |
| 3 | **规则文件的加载逐行移植内核的 `rulesParse`**（text、yaml）与 `rulesMrsParse` 的文件头读取（MRS），出处写在代码注释里 | 这两个函数未导出；移植部分约百行，行为由对照测试锁定（MRS 用内核导出的 `ConvertToMrs` 在测试中生成） |
| 4 | **不替用户解析 DNS**。`RuleMatchHelper.ResolveIP` 回调只记录「从这一条起需要 IP」，不发起查询 | 运行时出站连接只允许用户配置的订阅与规则源（[0003](0003-direct-fetch-and-invites.md)）。追踪结果给出分支，用户可以改为直接输入 IP |
| 5 | **服务器下载规则内容**（provider 模式下原本只有客户端下载），沿用现有的 SSRF 防护、普通用户的站点白名单与大小上限，只缓存在内存里，按用户限频；部署者可用 `CLASH_LAUNCH_TRACE=off` 关闭 | 追踪、体检与粒子样例离不开规则内容；内容是公开规则列表，不落盘因此不涉及迁移与数据搬迁 |
| 6 | **`routing` 段沿用规则源的模型**：`groups` 与 groups.toml 的 `custom_groups` 同一组键，`rulesets` 与 rulesets.toml 同一组键并新增 `bundle`；引擎在有 `routing` 时用它代替规则源展开，之后的 `Expand`、`BuildRules`、`PruneEmpty` 不变 | 不引入第二套语义，校验、选择器、空组处理、golden 全部复用 |
| 7 | **第一次在地图上点「应用」时才开始自己管理**（写入 `routing` 段），之前一直跟随上游；追踪与讲解不触发 | 维护者采纳推荐。只看不改的用户不失去自动跟随上游的好处 |
| 8 | **上游更新只提示、逐条采纳**，按开始管理时保存的 `base` 做三方比对；不自动合并 | 维护者采纳推荐。自动合并会在用户不知情时改变分流 |
| 9 | **合集**：同一来源中同名的 domain、ipcidr、classical 三件套（domainrouter_v2 的发布约定）自动识别；其他相邻且走向相同的规则集只提示合并。合集输出为连续的多条 `RULE-SET`，ipcidr 成员加 `no-resolve`，不编译为 `OR` 逻辑规则 | 维护者采纳推荐。逻辑规则会让客户端看不出命中的是哪个成员；无 `no-resolve` 的 IP 规则排在中间会让后续域名都先做 DNS 解析 |
| 10 | **按默认路径分层**：每个组的第一个成员构成默认路径；入口列放没有组引用的组与被规则直接使用且不是任何组默认成员的组，其余按「沿默认成员到节点的步数」从右往左排；候选成员只在聚焦或打开「显示全部候选」时画 | 维护者的 groups.toml 有 23 个组、74 条引用，全部引用的最长链 7 层，默认路径只有 4 层；7 层来自备用成员之间的交叉引用 |
| 11 | **智能视图是确定的规则与阈值**（见 routing.md 第 9 节），不是模型推断；界面用一行「智能视图」说明用了哪些画法，每一项都能在偏好里改回 | 同一份配置永远得到同一张图，可以写单元测试，用户能理解与纠正 |
| 12 | **粒子是示意，不是流量**；每个粒子携带规则集真实内容中的一个样例，路线按服务器的匹配结果计算。默认显示全部粒子，可切换为「聚焦」或关闭；系统「减少动态效果」时关闭 | 服务器看不到用户的真实流量，粒子多少不能暗示流量大小 |
| 13 | **流向色**独立于状态色：未分类、经代理、直连、拦截四个 token，浅色 `#a9a9b3 #7a73d4 #10a2a8 #cf7a3c`，深色 `#5f5f6a #8580e2 #23a3a6 #c98042` | 用调色板校验脚本检查过亮度区间、饱和度下限、色盲模拟色差（≥ 9.8）与对比度（≥ 3:1）；原先的绿色直连、红色拦截在红绿色弱模拟下色差只有 5.8 |
| 14 | **不引入画布库**（Svelte Flow、React Flow 等）。轨道用 SVG 几何属性，粒子用 Canvas | 这些库用元素的 `style` 属性定位，本站 CSP 为 `style-src 'self'`（[0004](0004-web-frontend.md)），引入就要放宽 |
| 15 | **组类型只提供官方四种**：`select`、`url-test`、`fallback`、`load-balance`。`relay` 已被内核移除（`adapter/outboundgroup/parser.go` 报错并提示改用 `dialer-proxy`），由链式代理承担；第三方分支的 Smart 组不提供 | 只按官方源码实现语义 |
| 16 | **组详情给出「适合 / 不适合」与参数提醒**，只提示不修改。提醒依据：url-test 只比较最近一次测速（`LastDelayForTestUrl` 取历史最后一条）、`tolerance` 默认 0、`lazy` 默认开启、默认间隔 300 秒、HTTP 测速地址可能被劫持（`adapter/adapter.go` 的日志提示） | 维护者反映自动选择体验不佳；源码给出了可以说清的原因 |
| 17 | **原始编辑优先**：图按最终配置画；来自原始编辑的规则或组标「原始编辑」且只读；原始编辑整段替换 `rules`、`proxy-groups` 或 `rule-providers` 时，地图只读并说明原因 | 原始编辑是用户写下的最终意图，可视化编辑不能悄悄失效 |
| 18 | **交给客户端拉取的订阅**（组内 `use` provider）在图上显示为 provider 出口及其筛选条件；追踪止于「由客户端在其中选择」 | 服务器看不到 provider 中的节点（开放注册用户一律如此，见反馈 #71），不猜 |

## 已知限制

- 逻辑规则（`AND`、`OR`、`NOT`）内引用 `RULE-SET`、以及 `SUB-RULE`：内核实现经全局 tunnel 查找规则集，服务器上无法复现，追踪标为「无法判断」并继续向下比对。
- `PROCESS-NAME` 等依赖进程的规则：用户未填写时标为「取决于应用」。`DST-PORT` 在输入没带端口时、以及来源地址、入站、DSCP、UID 等连接信息类规则，标为「无法判断」。含这些成分的逻辑规则按三值判断，另一项已能决定结果时照常给出结论（[routing](../design/routing.md) 第 5.2 节）。
- `GEOIP`、`GEOSITE`、`IP-ASN`：结果取决于服务器上的地理数据版本（`tools.lock.json` 锁定），客户端自己下载的数据可能更新或更旧，个别地址的归属因此可能不同。未配置地理数据目录时标为「无法判断」。
- 只有路径（`path`）、没有地址的规则集：服务器拿不到内容，标为「无法判断」。
- select 当前选了哪个、url-test 测出哪个最快，只有客户端知道；图与追踪按「默认第一个」展示，url-test 注明「测速决定」。

## 实测

第 0 步（2026-11-15），本机 Windows amd64、AMD EPYC 4 核、Go 1.27.3、mihomo v1.19.33 模块。

| 项目 | 结果 |
|---|---|
| 进程内加载地理数据 | **可行，但不能用内核的规则构造函数**。`NewGEOIP`、`NewGEOSITE`、`NewIPASN` 在文件缺失或校验失败时联网下载，MMDB 打不开时 `log.Fatalln`，路径与 `geodata-mode` 是进程级全局变量。改用内核导出的无全局状态部分（`LoadSiteByBytes`、`LoadIPByBytes`、`router` 匹配器、`mmdb.ASNReader`），三种规则的 `Match` 移植。`TestGeoMatchesKernel` 在 `tools.lock.json` 锁定的数据上与内核规则对象逐个对照一致：GEOSITE（`cn`、`CN`、`!cn`，402 个样本）、GEOIP 的 MMDB 与 geodata 两种模式（`CN`、`us`、`!cn`，302 个样本）、IP-ASN |
| `BenchmarkCompileDomain100k` | 10 万条 domain（text）编译 345 ms，累计分配 55.6 MB、60.8 万次（常驻内存未单独测量） |
| `BenchmarkTrace` | 66 条规则（60 条 RULE-SET，各 2000 条）一路未命中到兜底：每次 16 µs，46 次分配 |

据此，追踪与体检的耗时由规则内容的拉取决定，匹配本身可以忽略；冷缓存时编译大规则集是主要开销，应在拉取后一次编译、按第 6 节缓存。
