# 分流视图、追踪与 routing 段

> 状态：设计定稿（2026-11-15），实施中：第 0 步（`internal/match`）、第 1 步（讲解图层、规则集说明目录）已完成，第 2–4 步未开始。按第 13 节的步骤逐步实施，每一步单独提交并更新 [progress](../development/progress.md) 与 [codemap](../development/codemap.md)。
> 取舍与理由见 [0007](../decisions/0007-routing-visualization.md)，本文只写「做什么、怎么做、怎么验收」。
> 交互原型 [routing-prototype.html](routing-prototype.html) 是布局、粒子与追踪动画的参考实现（用浏览器直接打开）。它用了内联样式与示例数据，**不能照搬进站内**：站内 CSP 不允许 `style` 属性，数据来自接口。

## 1. 目标与非目标

**目标**：在配置编辑器里新增「分流」分节，提供：

1. **讲解图层**：五步讲清一次请求怎样经规则、策略组到出口（只读）。
2. **分流地图**：请求 → 规则 → 策略组（按默认路径分层）→ 出口 → 目的地；粒子按真实匹配结果流动；拖动修改走向、顺序、合集、组成员，改动先进草稿，应用后写入 `routing` 段。
3. **流向追踪**：输入域名或 IP（可选应用名、UDP），逐步给出命中依据；有草稿时同时给出应用前后的差别。
4. **常用网站体检**与**期望**：批量追踪常见网站；把追踪结果固定为期望，应用或上游更新时复核。
5. **上游同步**：开始自己管理后，上游变化逐条提示、逐条采纳。

**非目标**：真实流量统计；替用户解析 DNS；Smart 等第三方分支的组类型；手机上的拖动编辑（手机用列表模式）。

## 2. 术语

| 术语 | 含义 |
|---|---|
| 默认成员 | 组的第一个成员（select 未手动选择时用它，fallback 优先用它） |
| 默认路径 | 从规则的目标出发，每层取默认成员，直到节点、`DIRECT`、`REJECT` 或 provider |
| 候选成员 | 组的其余成员 |
| 入口列 | 地图上策略组的第一列，见第 8.2 节 |
| 合集 | 走向相同、在优先级中占一个位置的一组规则集，输出为连续的多条 `RULE-SET`（[0007](../decisions/0007-routing-visualization.md) #9） |
| 折叠段 | 智能视图把相邻且走向相同的规则显示为一行（只是显示，不改配置） |
| 自己管理 | 方案文档里有 `routing` 段；没有时叫「跟随上游」 |

## 3. 总体结构

```text
浏览器（分流分节）
  RoutingView ── POST /routing/graph   ──► service.RoutingGraph  ─► engine.Render（最终配置）
            ├── POST /routing/trace   ──► service.Trace         ─► match.Trace（内核策略对象）
            ├── POST /routing/health  ──► service.Health        ─┘        ▲
            ├── POST /routing/fork    ──► service.ForkRouting           规则内容缓存（内存）
            └── POST /routing/upstream──► service.Upstream              ▲
                                                                   internal/source（SSRF、白名单）
应用草稿：前端把 routing 写进方案文档（lib/profile-doc.ts）→ 现有的自动保存、预览与内核检查
```

依赖方向不变：`internal/match` 只依赖 mihomo 模块与 `internal/diag`，由 `internal/service` 调用；`internal/engine` 不依赖它（`TestDependencyDirection` 不需要改）。**不改数据库**：`routing` 在方案文档里，规则内容只缓存在内存，偏好沿用现有的偏好 JSON。

## 4. 数据模型

### 4.1 方案文档的 `routing` 段

```yaml
routing:
  from:                                   # 开始自己管理时的上游（只用于上游同步）
    rules: <规则源 ref>                    # 与 profile.rules.ref 相同
    digest: sha256:<hex>                  # 上游 groups + rulesets 规范化 JSON 的哈希
    at: 2026-11-15T08:00:00Z
  ini-node-ops:                           # 上游为 ini 时，原样保留影响节点的行；toml 上游省略
    - rename=…
    - emoji=…
    - include_remarks=…
  groups:                                 # 键与 groups.toml 的 [[custom_groups]] 完全相同
    - name: 🛫 香港专线
      type: fallback
      rule: ["[]🎇 香港高速", "[]🧊 新加坡专线"]
      url: https://www.gstatic.com/generate_204
      interval: 97
      lazy: false
  rulesets:                               # 键与 rulesets.toml 的 [[rulesets]] 相同，另加 bundle
    - { group: DIRECT, ruleset: "https://…/clash-domain/Direct.mrs", type: clash-domain, interval: 21600, bundle: Direct }
    - { group: DIRECT, ruleset: "https://…/clash-ipcidr/Direct-ipcidr.mrs", type: clash-ipcidr, interval: 21600, bundle: Direct }
    - { group: DIRECT, ruleset: "[]GEOIP,CN" }
    - { group: 🐳 漏网の鱼, ruleset: "[]FINAL" }
  base:                                   # 开始管理时上游的规范化副本，三方比对的基准；不手改
    groups: [ … ]
    rulesets: [ … ]
  checks:                                 # 期望：追踪 host 时，路径上必须出现 via（组名或出口名）
    - { host: chatgpt.com, via: 🗨️ ChatGPT }
```

规则：

- `routing` 存在时，引擎**不读取**规则源的 groups、rulesets、ini（`profile.rules.ref` 仍保留，供上游同步）。
- `bundle` 是本项目的扩展键：同一 `bundle` 的条目必须**相邻**且 `group` 相同，否则诊断 `routing.bundle_split`（警告，按原顺序输出）。
- 合集内 `clash-ipcidr` 条目的引用已由 `ruleset.BuildRules` 自动加 `no-resolve`（[rules](rules.md) 的 rulesets 表），无需额外处理。
- `checks[].via` 按名字匹配路径上的组或出口；名字不存在时诊断 `routing.check_unknown`。

### 4.2 Go 类型与解析

`internal/profile/profile.go`：

```go
// Routing 是方案自己管理的分流（0007）。Groups、Rulesets、Base 保持原始映射，由 ruleset 包校验与转换。
type Routing struct {
	From       RoutingFrom    `yaml:"from"`
	ININodeOps []string       `yaml:"ini-node-ops,omitempty"`
	Groups     []map[string]any `yaml:"groups"`
	Rulesets   []map[string]any `yaml:"rulesets"`
	Base       *RoutingBase   `yaml:"base,omitempty"`
	Checks     []RoutingCheck `yaml:"checks,omitempty"`
}
type RoutingFrom struct{ Rules, Digest string; At time.Time } // yaml: rules、digest、at
type RoutingBase struct{ Groups, Rulesets []map[string]any }
type RoutingCheck struct{ Host, Via string }
```

- `knownKeys` 与 `Parse` 的键循环加入 `routing`；`Profile` 加字段 `Routing *Routing`。
- `Profile.Marshal` 当前没有调用者，且漏写 `chains`。本功能不依赖它（文档由前端编辑）；第 3 步顺手删除它，或补全 `chains`、`routing` 并加往返测试，二选一，不要留半成品。

`internal/ruleset`：

- 把 `ParseRulesets` 循环体抽成 `rulesetFromMap(raw map[string]any, where string, out *diag.List) (Ruleset, bool)`，`Ruleset` 加字段 `Bundle string`。
- 新增 `func FromRouting(groups, rulesets []map[string]any, out *diag.List) *Source`：逐条调用 `groupFromMap`、`rulesetFromMap`，`Where` 写 `routing.groups#3`、`routing.rulesets#5`。**数字类型**：TOML 解出 `int64`，YAML 解出 `int`；在两个 `fromMap` 入口统一转换为 `int64` 再沿用现有判断（`groupFromMap` 第 117 行等处只认 `int64`）。
- 从 `ParseINI` 抽出 `ParseNodeOps(lines []string, file string, out *diag.List) *node.Processing`，ini 解析与 `routing.ini-node-ops` 共用。
- 新增 `func ToRouting(src *Source) (groups, rulesets *yaml.Node)`：把展开前的模型写回 YAML 序列，供开始管理与 `base` 使用。**不要用 `map` 序列化**（会按键名排序）：组的键顺序固定为 `name、type、rule、select、use、url、interval、tolerance、lazy、strategy`，其余键按键名追加；规则集为 `group、ruleset、type、interval、format、bundle`。往返测试：`FromRouting(ToRouting(x))` 与 `x` 等价，且相同输入逐字节相同。
- `/routing/fork` 返回完整的 `routing` 段文本：`from`（规则源 ref、digest、当前时间）、`ini-node-ops`（ini 上游时）、`groups`、`rulesets`（合集按第 4.1 节自动识别并写入 `bundle`），以及与之相同的 `base`。合集识别：相邻、`group` 相同、文件名分别为 `X.*`、`X-ipcidr.*`、`X-classic.*` 且路径分别含 `clash-domain/`、`clash-ipcidr/`、`clash-classic/` 的条目记为合集 `X`。

`internal/engine/build.go`：`BuildInput` 加 `Routing *profile.Routing`。`Build` 中：

```go
if in.Routing != nil {
	rs = ruleset.FromRouting(in.Routing.Groups, in.Routing.Rulesets, &out)
	rs.NodeOps = ruleset.ParseNodeOps(in.Routing.ININodeOps, "routing.ini-node-ops", &out)
	rs.Origin = "routing"
} else if in.Rules != nil { … 现有逻辑 … }
```

`service.assemble` 把 `p.Routing` 传入；`Routing != nil` 时**不加载**规则源快照（缓存键因此不含规则源哈希，含 routing 本身，因为它在方案文档里）。

### 4.3 诊断代码（`internal/diag/codes.go` 登记中英文）

| 代码 | 级别 | 触发 |
|---|---|---|
| `routing.bundle_split` | 警告 | 同一 bundle 的条目不相邻或 group 不同 |
| `routing.check_unknown` | 警告 | 期望里的 via 不是任何组或出口 |
| `routing.check_failed` | 警告 | 期望不满足（第 4 步） |
| `routing.upstream_changed` | 信息 | 上游的 digest 与 `from.digest` 不同（第 4 步） |
| `routing.raw_override` | 信息 | 原始编辑改动了 `rules`、`proxy-groups` 或 `rule-providers`，地图部分只读 |

## 5. 匹配：`internal/match`

### 5.1 规则集的编译

```go
package match

// Set 是一个编译好的规则集。domain、ipcidr 用内核的策略对象判断；lines 只用于定位依据。
type Set struct {
	Name, Behavior, Format string // behavior: domain | ipcidr | classical；format: text | yaml | mrs | inline
	Count                  int    // 与内核 ruleCount 相同
	// 未导出：策略对象、条目原文（mrs 为空，依据改用 DumpMrs）、classical 条目、是否依赖 IP
}
type Env struct { Geo *Geo; GeodataMode bool } // 地理数据与最终配置的 geodata-mode

func Compile(name, behavior, format string, body []byte, env Env) (*Set, error)
func CompileInline(name, behavior string, payload []string, env Env) (*Set, error)
func OpenGeo(dir string) (*Geo, error)
```

- 策略对象：`provider.NewDomainStrategy()`、`provider.NewIPCidrStrategy()`，依次调用 `Reset`、逐条 `Insert`、`FinishInsert`。
- classical：按内核 `classicalStrategy.payloadToRule` 解析每条（拒绝 `MATCH`、`RULE-SET`、`SUB-RULE`，解析失败的跳过且不计数），由本包按原顺序逐条 `Match`。内核的 classical 匹配本来就是顺序遍历；自己遍历是为了对含无法判断成分的条目单独处理（第 5.2 节），布尔结果与内核相同。
- text、yaml：逐行移植内核 `rules/provider/provider.go` 的 `rulesParse`，注释写明出处与版本；`TestParseParity` 用内核自己的 `rulesParse`（经 `ConvertToMrs`）对照条数与结果。
- mrs：移植 `rulesMrsParse` 的文件头读取（zstd 解压、`MRS\x01` 魔数、行为字节、`count`、`extra`），然后调用策略对象的 `FromMrs`。classical 没有 MRS 形式。解压后超过 256 MiB 报错（防压缩炸弹）。
- 大小与条数上限：单文件 20 MiB（与拉取上限一致）、单个规则集 200 万条，超过返回错误，由调用方把该规则集标为 `no-content`。
- `Env` 只影响 classical 中的地理规则；service 缓存 classical 规则集时键要包含 `GeodataMode`。

### 5.2 一次追踪

```go
type Request struct {
	Host        string     // 小写、去掉协议、路径、端口与末尾的点
	IP          netip.Addr // 输入的是 IP 时有效
	Port        uint16     // 输入带端口时有效；0 表示未知。体检按 443 传入
	Process     string     // 可选
	ProcessPath string     // 输入的应用含路径分隔符时为完整路径，Process 取最后一段
	Network     string     // tcp（默认）| udp
}
func NewRequest(input, process, network string) (Request, error) // 规范化用户输入
func ParseLine(index int, raw string) Line                        // 按内核 ParseRulePayload 拆分
type Line struct { // 最终配置 rules 中的一行
	Index  int
	Raw    string // 原文，例如 RULE-SET,Direct,DIRECT,no-resolve
	Type, Payload, Target string
	Params []string
}
type Step struct {
	Index    int
	Result   string // miss | hit | needs-ip | unknown
	Evidence string // 命中的那一行，例如 "+.chatgpt.com"、"IP-CIDR,91.108.4.0/22"；定位不到时为空
	Reason   string // unknown 的原因代码：logic-ruleset | sub-rule | needs-process | needs-connection | no-content | geodata | parse
}
type Outcome struct {
	Steps        []Step // 从第 0 条到命中为止
	Hit          int    // 命中的行号；-1 表示没有命中（不应出现，最后一条应为 MATCH）
	NeedsIPAt    int    // 第一次需要 IP 的行号，-1 表示不需要
	IPCandidates []int  // 从 NeedsIPAt 起、Hit 之前可能因 IP 命中的行（含 no-resolve 的行：内核解析一次后 DstIP 对后续各行可见）
}
func Trace(lines []Line, sets map[string]*Set, req Request, env Env) Outcome
func TraceFrom(lines []Line, start int, sets map[string]*Set, req Request, env Env) Outcome
```

逐行处理（顺序与内核 `tunnel.match` 相同，整个追踪共用一个 `Metadata`）：

1. `RULE-SET`：在 `sets` 中查找（找不到 → `unknown/no-content`），按内核 `rule_set.go` 的语义：带 `no-resolve` 时把 `helper.ResolveIP` 置空；`src` 参数（按来源地址匹配）→ `unknown/needs-connection`。
2. 其他类型：按 `rules.ParseRule` 解析，但解析函数由本包提供并递归传给 `logic.NewAND/OR/NOT`，因此逻辑规则的子规则同样经过拦截：
   - 子规则中的 `RULE-SET` → `logic-ruleset`；`SUB-RULE` → `sub-rule`；
   - 取决于追踪不掌握的连接信息的类型（`SRC-*`、`IN-*`、`DSCP`、`UID`、`REMATCH-NAME`、带 `src` 的 IP 规则）→ `needs-connection`；`DST-PORT` 在请求没有端口时同样如此；
   - `PROCESS-NAME*` 在没填应用、`PROCESS-PATH*` 在没有路径时 → `needs-process`；
   - `GEOIP`、`GEOSITE`、`IP-ASN` 见下文；解析失败 → `parse`。
3. **三值判断**：含无法判断成分的规则，把这些成分分别当作「不命中」与「命中」各算一次，两次相同即为确定结果，不同才记 `unknown`。例如常见的 `AND,((DST-PORT,443),(NETWORK,UDP)),REJECT` 对 TCP 请求确定不命中，对未给端口的 UDP 请求无法判断。classical 规则集逐条如此判断：有确定命中的条目即命中，否则只要有无法判断的条目就记 `unknown`。
4. `helper.ResolveIP`：回调里只记录 `NeedsIPAt`（首次），**不解析**。之后依赖 IP 的行因没有 IP 而不命中，结果记 `needs-ip` 并收进 `IPCandidates`，追踪继续到兜底。因缺少地理数据被替换的 GEOIP、IP-ASN（无 `no-resolve`）同样视为触发了解析。
5. 依据定位：domain、ipcidr 规则集逐条用**单条目的内核策略对象**判断（先按文本粗筛），语义与整体匹配完全相同；MRS 用 `DumpMrs` 遍历（ipcidr 得到的是合并后的网段）；classical 给出命中的条目原文；普通规则给出类型与载荷。定位失败只留空，不影响结果。
6. 命中之后，内核还会因目标不存在、`PASS`、`REMATCH` 或节点不支持 UDP 跳过该行（`tunnel.go`）。这取决于策略组与节点，由 service 判断后用 `TraceFrom` 从下一行继续（第 2 步）。

地理数据（第 0 步结论，[0007](../decisions/0007-routing-visualization.md) 的「实测」）：内核的 `NewGEOIP`、`NewGEOSITE`、`NewIPASN` 在文件缺失或校验失败时会联网下载，MMDB 打不开时 `log.Fatalln` 退出进程，路径与 `geodata-mode` 又是进程级全局变量，**不能调用**。本包用 `OpenGeo` 只读打开 `CLASH_LAUNCH_GEODATA` 目录（文件名按内核 `constant/path.go` 的规则查找），借助内核导出的无全局状态部分（加载器的 `LoadSiteByBytes`/`LoadIPByBytes`、`router` 的匹配器、`mmdb.ASNReader`）实现三种规则，`Match` 与 MMDB 的 `LookupCode` 逐行移植。`GEOIP` 按 `Env.GeodataMode` 选择 geoip.dat 或 MMDB。目录未配置或缺少对应文件时，这些行记 `unknown/geodata`（`GEOIP,lan` 不需要数据）。

### 5.3 测试

- `match_test.go`：domain、ipcidr 各三种格式，classical 两种格式（另有内联）；MRS 用 `provider.ConvertToMrs` 在测试中由 text 生成；`no-resolve` 与 `NeedsIPAt`、`IPCandidates`；三值判断；进程、端口、来源地址规则；依据定位；输入规范化；`TestParseParity` 对照内核的 `rulesParse`。只用 `example.com`、`192.0.2.0/24`、`2001:db8::/32`（另有 `example.net`、`example.org`、`198.51.100.0/24`，同为保留地址）。
- `TestGeoMatchesKernel`：在同一份地理数据上，把本包的 GEOSITE、GEOIP（两种模式）、IP-ASN 与内核自己的规则对象逐个比较；样本从数据本身取出。需要地理数据，没有时跳过；内核一侧在临时目录中运行，下载地址指向不可达的本机端口。
- 基准：`BenchmarkCompileDomain100k`、`BenchmarkTrace`，实测值见 [0007](../decisions/0007-routing-visualization.md) 的「实测」。

## 6. 规则内容的获取与缓存（`internal/service/routing_cache.go`）

- 键：规则集的 URL；值：`{body sha256, *match.Set, fetchedAt, size}`。
- 拉取：`s.Fetcher.Fetch(ctx, source.Request{URL: u, Hosts: s.RuleHostsFor(user)})`，复用 SSRF 防护与普通用户的站点白名单；`MaxBytes` 20 MiB。
- 过期：`min(规则集 interval, 6 小时)` 后重新拉取；拉取失败时继续用旧内容并在追踪结果里标「内容可能过期」。
- 容量：按内容字节做 LRU，默认 256 MiB，`CLASH_LAUNCH_TRACE_CACHE_MB` 可改；同一 URL 的并发拉取合并为一次（自写的等待表，或使用已在依赖图中的 `golang.org/x/sync/singleflight`，不得升级版本）。
- 开关：`CLASH_LAUNCH_TRACE=off` 时 trace、health 接口返回 404 `feature_disabled`，前端隐藏追踪框与体检，地图照常（粒子只在组与出口之间流动，见第 10 节）。
- 内联规则集（`payload`）直接 `CompileInline`；只有 `path` 的规则集记 `no-content`。
- 配置项写进 `cmd/clash-launch/serve.go` 与 [deploy](../guide/deploy.md)。

## 7. 接口（`internal/api/routes.go`、`api/openapi.yaml`，改完运行 `npm run gen:api`）

全部是 `POST`，需登录，按所有者隔离（在 `TestOwnerIsolationAllRoutes` 的 `ids` 中补上），`doc` 是编辑器当前的方案文档文本（与 `preview` 相同的约定）。

| 路径 | 请求 | 响应 | 限流 |
|---|---|---|---|
| `/api/v1/profiles/{id}/routing/graph` | `{doc}` | `GraphDTO` | `renderLimit` |
| `/api/v1/profiles/{id}/routing/trace` | `{doc, draft?, host, process?, network?}` | `{current: TraceDTO, draft?: TraceDTO}` | 新增 `traceLimit` 60/分钟/用户 |
| `/api/v1/profiles/{id}/routing/health` | `{doc, draft?}` | `{items: [{host, category, summary, exit}]}` | `traceLimit` 计 1 次 |
| `/api/v1/profiles/{id}/routing/fork` | `{doc}` | `{routing: string}`（YAML 片段） | `renderLimit` |
| `/api/v1/profiles/{id}/routing/upstream` | `{doc}` | `{changes: [UpstreamChange]}` | `renderLimit` |

`draft` 是草稿的 `routing` YAML 文本：服务器用它替换 `doc` 中的 `routing` 再计算。`host` 校验：去空白后长度 1–253，域名字符或合法 IP；否则 400 `invalid_host`。

```ts
type GraphDTO = {
  state: "follow" | "own";            // 跟随上游 / 自己管理
  editable: boolean; readonlyReason?: "raw_override" | "no_rules";
  rules: RuleDTO[]; groups: GroupDTO[]; exits: ExitDTO[];
  upstreamChanges?: number;           // 自己管理且上游有变化时
  diagnostics: Diagnostic[];          // 与 preview 相同的结构
};
type RuleDTO = {
  index: number; raw: string; type: string; target: string;
  origin: "routing" | "rules" | "raw";              // raw 来自原始编辑，只读
  set?: { name: string; behavior: string; format: string; url?: string; bundle?: string;
          count?: number;                           // 内容已缓存时才有
          samples?: { host: string; hit: number }[] // 最多 8 个；hit 是服务器追踪该样例得到的命中行
        };
  flags: ("logic" | "sub-rule" | "process" | "no-resolve")[];
};
type GroupDTO = {
  name: string; type: "select" | "url-test" | "fallback" | "load-balance";
  members: { name: string; kind: "group" | "node" | "builtin" | "provider"; entry?: number }[]; // entry：来自 rule 列表的第几条；来自正则或选择器时省略
  use?: string[]; filter?: string;
  params: { url?: string; interval?: number; tolerance?: number; lazy?: boolean; strategy?: string };
  origin: "routing" | "rules" | "raw";
  lints: string[];                    // 提醒代码，见第 11.3 节
};
type ExitDTO = { name: string; kind: "node" | "builtin" | "provider";
  region?: string; source?: string; protocol?: string; via?: string;  // via：链式代理的前置
  filter?: string };                   // provider 的筛选条件
type TraceDTO = {
  host: string; ip?: string;
  steps: { index: number; result: "miss" | "hit" | "needs-ip" | "unknown"; evidence?: string; reason?: string }[];
  hit: number;
  chain: { group: string; type: string; pick: string; candidates: string[] }[];  // 沿默认路径
  exit: { kind: "node" | "builtin" | "provider" | "unknown"; name: string; region?: string; via?: string };
  needsIp?: { at: number; candidates: number[] };
  stale?: boolean;                     // 用了过期的规则内容
};
type UpstreamChange = { id: string; kind: "group" | "ruleset"; op: "add" | "remove" | "modify" | "move";
  key: string; field?: string; base?: unknown; upstream?: unknown; mine?: unknown; conflict: boolean };
```

图数据由 `service.RoutingGraph` 生成：调用与预览相同的渲染路径得到**最终配置**（表单层与原始编辑层合并之后），从中读取 `rules`、`proxy-groups`、`rule-providers`；用引擎生成阶段的结果（合并之前）判断每条的 `origin`（最终配置有而生成结果没有的记 `raw`）；节点的地区、来源、协议取自 `Result.Nodes`；链式代理取自 `Result.Chains`。

## 8. 布局（`web/src/lib/routing/layout.ts`，纯函数，可单测）

### 8.1 默认路径

`steps(g)`：沿 `members[0]` 走到出口的步数（出口为 0）。组成员不可能成环（编辑时拒绝，内核也拒绝）；保险起见遇到重复访问时返回已有值并加诊断。

### 8.2 分列

```text
defaultChild = { g.members[0] | g ∈ groups, members[0] 是组 }
hasParent    = 所有组的成员中出现过的组
ruleUsed     = 所有规则的目标
entry(g)     = g ∉ hasParent  或  (g ∈ ruleUsed 且 g ∉ defaultChild)
C            = max(steps(g))
col(g)       = entry(g) ? 1 : C − steps(g) + 1
```

性质：默认路径上的连线总是从左到右（父组的步数一定大于子组）。只当备份的组不会进入入口列。

### 8.3 列内顺序

按「最早被哪条规则或哪个组引用」：规则引用取规则序号；组引用取父组的键 + 成员序号 × 0.001，候选成员再加 0.5；未被引用的排在最后。

### 8.4 连线

- 规则 → 目标：每条一根三次贝塞尔（水平切线，控制点在中点）。折叠段内的规则共用一根。
- 组 → 成员：默认成员实线；url-test 的全部成员虚线（测速决定）；load-balance 的全部成员实线；其余候选成员**只在**该组被悬停或选中、或打开「显示全部候选」时画（点线）。
- 目标在同列或左侧的候选线：控制点改为 `(a.x+26, a.y)`、`(b.x−26, b.y)`，从列间空隙绕行。
- 入度 ≥ 4 的组：连线终点改为组左侧 18 px 处的汇聚点，再一段直线进入。

几何只写进 SVG 的 `d` 属性。网格列宽等动态样式用 CSSOM（`element.style.setProperty`），不写 `style` 属性。

## 9. 智能视图（`web/src/lib/routing/plan.ts`）

`planView(graph, viewportWidth, prefs) → ViewPlan`，确定的规则与阈值，每条产生一个 `why` 文案键与参数，显示在工具栏下的「智能视图」一行；`prefs` 中的覆盖优先。

| # | 量 | 条件 | 结果 | 偏好覆盖 |
|---|---|---|---|---|
| 1 | 规则条数 | `> 30` | 相邻且目标相同的 `RULE-SET` 行折叠成段；追踪命中、或用户点开的段展开 | `routingView.rules = all` |
| 2 | 连续手写规则 | 连续 `≥ 3` 条非 RULE-SET 且非 GEOIP、MATCH，目标相同 | 折叠为「手写规则 ×N」 | 同上 |
| 3 | 组的列数 | `≥ 3` | 紧凑行：类型标签缩成一个字（选、速、备、均） | — |
| 4 | 列宽不够 | 最小宽度 > 容器宽度 | 卡片内横向滚动，规则列与出口列不缩 | — |
| 5 | 入度 | `≥ 4` | 汇聚点（8.4） | — |
| 6 | 未使用的组 | 不被规则、也不被组引用 | 放到组列底部「未使用」区，提醒 `group.unused` | — |
| 7 | 成员数 | `> 12` | 只画默认成员；详情中的成员列表可搜索 | — |
| 8 | 节点归并 | `≤ 8` 个节点 → 逐个；`≥ 80%` 有地区 → 按地区；否则 → 按来源 | 出口行 | `routingView.exits = node | region | source` |
| 9 | provider | 组有 `use` | 出口显示 provider 名与 `filter`，标「客户端拉取」 | — |
| 10 | 链式代理 | 节点有 `via` | 出口行加「经 X 中转」 | — |
| 11 | 原始编辑 | `origin = raw` 的行或 `readonlyReason` | 行标「原始编辑」且只读；整段替换时地图只读并在顶部说明 | — |
| 12 | 视口 | `< 760` → 列表模式；`760–1100` → 一律紧凑；更宽 → 按上面 | — | — |
| 13 | 粒子上限 | `clamp(规则数 × 3, 30, 70)`；规则 `> 30` 时标签概率减半 | — | `routingView.particles` |

单元测试用第 13 节的图数据夹具，断言每个夹具得到的 `ViewPlan`（含 `why`）。

## 10. 粒子（`web/src/lib/routing/particles.ts`，不依赖 Svelte 的类）

| 参数 | 值 |
|---|---|
| 颜色 | `--flow-neutral`（未命中规则前）、`--flow-proxy`、`--flow-direct`、`--flow-reject`；在 `tokens.css` 的浅色与深色两处定义（值见 0007 #13），`styles.test.ts` 同步检查 |
| 染色 | 进入命中的规则行后，0.35 秒内从未分类色线性过渡到流向色 |
| 光点 | 半径 1.8（追踪 3）；透明度 0.62（追踪 0.95）；拖尾 8 帧（追踪 14），线宽随位置递减，透明度 × 0.4；追踪粒子外圈 2.6 倍半径、透明度 0.14 |
| 速度 | 140–210 px/s 随机；路径按 4 px 间距预采样 |
| 生成 | 每 190 ms 一个，同屏上限见第 9 节 #13；「聚焦」模式且正在追踪时改为 560 ms |
| 样例 | 只用 `RuleDTO.set.samples`：从命中行 `hit` 的行出发，沿默认路径到出口；没有样例的规则不生成粒子；`CLASH_LAUNCH_TRACE=off` 时从组的入口出发，只演示组到出口 |
| 标签 | 左侧请求区与右侧目的地区显示样例的 host（概率 0.45），淡入淡出；追踪粒子在扫描轨道上也显示 |
| 光圈 | 粒子到达行时在行外沿画圆角框，600 ms 淡出；环境粒子只有 20% 触发，透明度 0.18 |
| 追踪动画 | 粒子先到规则列左侧 16 px 的轨道，逐条下行：未命中停 160 ms 并画灰色 ✗；命中停 340 ms，画 ✓ 与光圈，开始染色；需要 IP 时画 ? 并分成两路（A：IP 候选行的第一条；B：继续到兜底）；整条结束后 900 ms 重播 |
| 拦截 | 在「拦截」行左缘画 600 ms 的扩散圆环 |
| 模式 | `all`（默认，始终显示全部）、`focus`（悬停或追踪时其余粒子透明度 0.1）、`off`；`prefers-reduced-motion` 时固定为 `off` |
| 性能 | 设备像素比上限 2；`IntersectionObserver` 不可见或 `document.hidden` 时暂停；布局、草稿或追踪目标变化时清空重来，单纯切换选中不清空 |

## 11. 前端

### 11.1 文件

| 文件 | 职责 |
|---|---|
| `pages/ProfileEditor.svelte` | 分节导航加「分流」（`section=routing`），按需加载 `RoutingView`；该分节不停靠预览 |
| `components/routing/RoutingView.svelte` | 取图数据、草稿状态、工具栏（示例外的三个开关：规则、粒子、候选）、智能视图说明、组合下面各组件 |
| `components/routing/RoutingMap.svelte` | 五列 DOM、SVG 轨道、Canvas 粒子层、悬停与拖动 |
| `components/routing/RoutingList.svelte` | 窄屏列表模式（无连线、无粒子；每个规则集一行，点开选择走向） |
| `components/routing/TracePanel.svelte` | 追踪框（悬停或聚焦且有内容时显示 ×，Esc 同样清空）、示例、逐步卡片、应用前后对比 |
| `components/routing/DetailPanel.svelte` | 规则、合集、组、出口的详情与无鼠标编辑；组类型分段按钮、参数、适合与不适合、提醒、成员、YAML 片段 |
| `components/routing/DraftBar.svelte` | 改动列表、取消、应用 |
| `components/routing/ExplainLayer.svelte` | 「?」讲解图层（五步） |
| `components/routing/HealthDialog.svelte`、`UpstreamDialog.svelte` | 体检表格；上游变化逐条采纳或忽略 |
| `lib/routing/layout.ts`、`plan.ts`、`particles.ts` | 第 8–10 节 |
| `lib/routing/draft.ts` | 草稿操作（纯函数，对 routing 映射操作）：改走向、移动、合并、拆出、改类型、改参数、加入成员、设为首选、移除成员；`changes(applied, draft)` 生成改动列表（用最长递增子序列判断「移动过」，一次交换只报一条）；`reaches` 循环检查 |
| `lib/profile-doc.ts` | 加 `getRouting(doc)`、`setRouting(doc, yamlText)`，只做结构操作 |
| `lib/prefs.svelte.ts`、`internal/api/prefs.go` | 偏好加 `routingView: {rules: "auto" | "all", particles: "all" | "focus" | "off", candidates: boolean, exits: "auto" | "node" | "region" | "source"}`，服务端校验取值 |
| `styles/tokens.css` | 四个 `--flow-*` token（两处深色定义） |
| `lib/i18n/messages/{zh-CN,en}.json` | `routing.*` 文案；动态前缀在 `i18n.test.ts` 登记 |
| `src/help/{zh-CN,en}/NN-分流.md` | 教程（第 4 步） |
| `components/editor/SourcesPanel.svelte` | 规则源选择旁加「?」打开讲解图层 |
| 命令面板 | `palette.register` 一条来源：输入域名后选「追踪」，跳到分流分节并开始追踪 |

### 11.2 编辑与草稿

- 草稿是 `routing` 映射的副本。**跟随上游时**第一次改动：前端先调 `/routing/fork` 取得 routing 作为「已应用」基线，再在其上修改；点「应用」才把 routing 写入文档。
- 应用：`setRouting(doc, yaml)` → 现有自动保存、预览、内核检查。检查失败时保留草稿并显示诊断，已有的撤销提示照常。
- 拖动规则到组或「直连」「拦截」：改 `group`；到规则行上沿或下沿（行高 28% 内）：移动；到行中间：合并（两者都是规则集且不是兜底；合并后成员按 domain → ipcidr → classical 排序，`bundle` 取目标合集名或目标规则集名，`group` 随目标）。兜底行不可移动，其后不可插入。
- 拖动出口或组到组：加入成员。组 → 追加 `"[]组名"`；单个节点 → 追加 `"[]节点名"`；地区出口 → 组已有 `select` 时在 `select.region` 追加，否则新建 `select = {region: [X]}` 并在 `rule` 末尾加 `"@select"`。会形成循环时拒绝（拖动预览变红：「会形成循环，mihomo 不接受」）。
- 「设为首选」只对来自 `rule` 列表单独一条的成员可用（`GroupDTO.members[].entry` 存在）；来自正则或选择器的成员显示「来自筛选条件，不能单独移动」。
- 改类型：参数按类型出现（`url-test`: url、interval、tolerance、lazy；`fallback`: url、interval、lazy；`load-balance`: url、interval、strategy），缺省值来自目录；切回原类型时保留原值。
- 原始编辑来源的行与组：不可拖动，详情只读并说明原因。

### 11.3 组的提醒（服务端计算，放在 `GroupDTO.lints`，文案在前端）

| 代码 | 条件 |
|---|---|
| `group.single_member` | 非 select 且展开后成员 < 2 |
| `group.tolerance_zero` | url-test 且 tolerance 为 0 或未设 |
| `group.lazy_stale` | url-test 且 lazy 为真或未设 |
| `group.latency_only` | url-test（始终提示：只比较最近一次延迟，不看带宽与丢包） |
| `group.round_robin` | load-balance 且 strategy 为 round-robin |
| `group.http_probe` | url-test、fallback、load-balance 的 url 以 `http://` 开头 |
| `group.unused` | 不被规则也不被组引用 |

「适合 / 不适合」文案按类型固定，见原型 `TYPE_INFO`。

## 12. 期望与上游同步（第 4 步）

- 期望：`routing.checks`。应用前、上游同步后，服务端对每条执行追踪，`via` 不在路径（组或出口名）上时产生 `routing.check_failed`。追踪卡片上有「固定为期望」按钮。
- 上游同步：`/routing/upstream` 计算 `base`（开始管理时）、`upstream`（规则源当前快照经 `ToRouting`）、`mine`（routing）的三方差异。组按 `name`、规则集按 `ruleset` 地址为键；字段级比较；顺序变化记 `move`。`mine` 与 `base` 相同而 `upstream` 不同 → 可直接采纳；三者两两不同 → `conflict`。采纳：把 `upstream` 的值写入 routing 对应位置并更新 `base`；忽略：只把 `base` 更新为 `upstream`。都在前端 `draft.ts` 中完成，再经「应用」写入。

## 13. 实施步骤

每一步：实现 → 测试 → 文档（codemap、progress、本文的「状态」行）→ 提交。界面改动先在真实数据上截图给维护者确认（CLAUDE.md「呈现」）。

### 第 0 步 · 匹配引擎验证

- `internal/match`（第 5 节）与测试；进程内加载地理数据的验证。
- 验收：`go test ./internal/match/` 通过；基准实测值写入 0007；地理数据结论写入 0007 的限制或删除该限制。
- [mihomo-upgrade](../development/mihomo-upgrade.md) 第 5 步的表格已登记 `internal/match` 依赖的内核接口（本次提交的文档改动已加）。

### 第 1 步 · 讲解图层与规则集说明

- `ExplainLayer`、SourcesPanel 的「?」入口。
- 目录 `catalog/rulesets/descriptions.yaml`：按 URL 正则匹配常见上游的规则集，给出中英文说明、仓库、许可与核对日期；`TestRulesetDescriptions` 检查正则可编译、每条至少匹配一个预设中的地址。
- 验收：e2e「讲解图层可打开、逐步切换、Esc 关闭」；截图确认。
- 完成情况（2026-11-15）：讲解图层用示意的规则与网址；原型中「已有配置时用自己的规则集与组名」依赖第 2 步的图数据，届时补上。说明目录收录了能在本地参考资料中核对的两家：domainrouter_v2（按 README 写整套规则集的组织方式）与 ACL4SSR（按 README 的「Clash规则碎片」表逐个写，14 条）；Loyalsoldier、blackmatrix7、MetaCubeX、SukkaW 本地没有其文档，未收录。

### 第 2 步 · 只读地图、追踪、粒子、体检

- 后端：`routing_cache.go`、`RoutingGraph`、`Trace`、`Health`、`/routing/graph|trace|health`、`traceLimit`、`CLASH_LAUNCH_TRACE`、`CLASH_LAUNCH_TRACE_CACHE_MB`。体检的网站列表放在 `catalog/routing/health.yaml`（约 30 个，按类别）。
- 前端：`RoutingView`、`RoutingMap`（只读）、`TracePanel`、`layout.ts`、`plan.ts`、`particles.ts`、`RoutingList`、`HealthDialog`、偏好 `routingView`、`--flow-*` token、命令面板入口。
- 夹具：`web/src/lib/routing/fixtures/` 下的 GraphDTO：`tiny`（1 层）、`domainrouter`（由 `testdata/rules/domainrouter` 渲染得到）、`acl4ssr`（由 `testdata/workspace/acl4ssr-style` 渲染得到）、`provider`（交给客户端）、`raw-override`、`deep8`（合成的 8 层）。由一个 Go 测试生成并校验与当前代码一致（`-update` 时重写）。
- 测试：`match` 已覆盖；`service` 追踪对 golden 方案逐条断言；`layout.test.ts`、`plan.test.ts`（每个夹具的列与 `why`）；e2e：分流分节渲染、追踪 `www.example.com` 的逐步卡片（e2e 不联网：用 rulesets 的内联规则 `[]DOMAIN-SUFFIX,example.com` 与内联 payload）、清空按钮、粒子模式切换、窄屏列表模式。
- 验收：以上全部通过；`npm run budget` 首屏不变（分流分节按需加载）；四种示例截图确认。

### 第 3 步 · 编辑与 routing

- `profile.Routing`、`ruleset.FromRouting`、`ToRouting`、`ParseNodeOps`、引擎接入、`/routing/fork`、诊断代码；`Profile.Marshal` 的处理（第 4.2 节）。
- 前端：拖动、草稿、`draft.ts`、`DetailPanel` 的编辑、`DraftBar`、合集合并与拆出、组类型与参数、提醒、循环检查。
- 测试：`ToRouting`/`FromRouting` 往返；带 routing 的 golden（domainrouter 开始管理后输出与跟随上游时**逐字节相同**）；`draft.test.ts`（每个操作与 `changes` 文案）；e2e：拖动改走向 → 应用 → 预览中 rules 变化 → 撤销；合并与拆出；改组类型后预览的 proxy-groups 变化；循环被拒绝。
- 验收：以上通过；`go test -tags kernel` 对带 routing 的 golden 通过。

### 第 4 步 · 上游同步、期望、文档

- `/routing/upstream`、`UpstreamDialog`、`checks` 与「固定为期望」、`routing.check_failed`、`routing.upstream_changed`。
- 帮助教程一篇（两种语言）；[rules](rules.md) 增加「routing 段」一节并链接本文；[guide/deploy](../guide/deploy.md) 补环境变量。
- 验收：三方比对的单元测试覆盖「可直接采纳、冲突、忽略、移动」；e2e：修改上游规则源快照后出现提示并能采纳。

## 14. 风险与对策

| 风险 | 对策 |
|---|---|
| 内核升级改变策略对象的方法或 `rulesParse` 行为 | mihomo-upgrade 第 5 步逐条核验；`match_test.go` 的对照用例会先失败 |
| 大规则集占内存 | 缓存按字节 LRU，上限可配；单文件 20 MiB、单集 200 万条 |
| 地理数据无法在进程内加载 | GEO 类规则标「无法判断」，地图与其他规则不受影响 |
| 节点很多（上千） | 按地区或来源归并；出口行数与节点数无关 |
| 原始编辑与地图编辑冲突 | 原始编辑优先，地图对应部分只读（0007 #17） |
