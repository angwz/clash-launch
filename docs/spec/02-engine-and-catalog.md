# Clash, Launch! 实施规范 · 引擎与字段目录

版本：1.0 · 日期：2026-11-09 · 第 7～11 节

[返回规范入口](index.md) · 上一册：[01 · 基础与输入](01-foundations-and-input.md)

## 7. 生成管线

```text
         ┌──────────── 输入快照（不可变，按内容哈希）────────────┐
拉取 ─▶ 解析节点 ─▶ 整理 ─▶ 展开策略组 ─▶ 生成规则 ─▶ 表单层 ─▶ 原始编辑层 ─▶ 目标适配 ─▶ 渲染 ─▶ 格式自检 ─▶ 内核检查
 §5.4     §5.2–5.3   §5.5     §6.2         §6.3        §9.1      §9.2          §10        §11.1   §11.2       §11.3
```

### 7.1 不变量

- **确定性**：相同的输入快照与方案版本，渲染结果逐字节一致。管线中不得读取当前时间（文件头时间取自输入快照中最新的时间戳）、随机数或 map 的迭代顺序。
- **纯函数核心**：从「解析节点」到「渲染」都不联网、不读写数据库，输入与输出都是值。网络只在 `internal/source`，持久化只在 `internal/store`。
- **诊断不中断**：除致命错误（YAML 语法错误、格式自检或内核检查失败）外，各步骤都把问题写进诊断列表，然后继续执行，保证一次运行看到全部问题。
- **最后一次成功版本**：渲染或检查失败时，订阅端点继续提供上一次成功的结果，并在方案页与诊断中标出失败原因。客户端永远不会拿到一份坏掉的配置。

### 7.2 诊断

```go
type Diagnostic struct {
    Code     string   // 稳定的机器可读代码，如 "node.unsupported_scheme"
    Severity Severity // error | warning | info
    Path     string   // 定位：配置路径（dns.nameserver[2]）或输入位置（source:3:15）
    Args     map[string]string // 消息参数；消息文本由 Code 对应的 i18n 键生成
}
```

- 诊断代码集中登记在一处，每个代码都有 i18n 消息与文档锚点；代码一旦发布不改名。
- 诊断中不得出现凭据、令牌或订阅 URL 的完整值；路径与参数在生成时脱敏。

### 7.3 缓存

渲染结果以 `hash(方案版本, 输入快照哈希列表, 目录版本, 引擎版本)` 为键缓存。订阅请求命中缓存时不重新渲染；任何输入变化都会重新渲染并重新检查。缓存经接口访问，默认在进程内存与数据库中，以便日后多实例部署时替换（第 17 节）。

## 8. 字段目录

字段目录是整个产品的骨架：表单、补全、校验、目标适配、渲染时的键顺序都从目录生成，前后端都不硬编码字段。

### 8.1 位置与结构

```text
catalog/mihomo/
├─ pack.yaml              格式包元数据：名称、支持的内核版本区间、文档根地址
├─ general.yaml           顶层通用字段（端口、mode、log-level、external-controller……）
├─ dns.yaml · tun.yaml · sniffer.yaml · ntp.yaml · tunnels.yaml · experimental.yaml
├─ listeners/             入站，每种类型一个文件
├─ proxies/               出站，每种协议一个文件，共享的 tls、transport、dialer-proxy 片段单独成文件
├─ proxy-groups.yaml · proxy-providers.yaml · rule-providers.yaml · rules.yaml · sub-rules.yaml
└─ presets/               目标客户端预设（第 10 节）
```

分节与 mihomo 官方文档的配置目录一一对应（2026-11-09：general、dns、sniff、inbound/tun/listeners、proxies、proxy-providers、proxy-groups、rules、rule-providers、sub-rule、tunnels、ntp、experimental）。

### 8.2 字段定义

**目标是覆盖官方文档与源码中的全部字段，包括实验性字段**，并让每个字段都有足够的信息，让用户不看官方文档也能做决定。

```yaml
- key: enhanced-mode
  type: enum
  values:
    - { value: fake-ip,    i18n: catalog.dns.enhanced-mode.fake-ip }
    - { value: redir-host, i18n: catalog.dns.enhanced-mode.redir-host }
  default: redir-host            # 内核默认值；表单中显示，但不输出
  importance: recommended        # core | recommended | optional | expert
  stability: stable              # stable | experimental | deprecated
  since: "1.10.0"                # 首次出现的内核版本（不确定时留空并标 unverified）
  removed: null                  # 移除的版本
  examples:                      # 可直接套用的示例，点击即填入
    - { value: fake-ip, i18n: catalog.dns.enhanced-mode.example.fakeip }
  related: [dns.fake-ip-range, dns.fake-ip-filter]
  doc: https://wiki.metacubex.one/config/dns/#enhanced-mode
  sensitive: false               # 敏感字段：加密存储、界面打码、日志脱敏
  ui: { widget: segmented }
  i18n: catalog.dns.enhanced-mode            # 标题、一句话摘要、详细说明的消息键
  targets: { stash: unsupported }            # 与预设默认不同时才写
```

每个字段的说明分三层：**标题**（几个字）、**一句话摘要**（卡片上直接显示）、**详细说明**（展开后显示：作用、取值含义、与其他字段的关系、常见坑、何时需要修改）。说明依据官方文档与源码，用自己的话写，不整段复制。

**重要程度**（决定表单中的排列与默认是否展开）：

| 级别 | 含义 | 示例 |
| --- | --- | --- |
| `core` | 几乎每份配置都要考虑 | `mixed-port`、`mode`、`dns.enable` |
| `recommended` | 多数用户应当设置 | `dns.enhanced-mode`、`geodata-mode` |
| `optional` | 特定场景才需要 | `routing-mark`、`interface-name` |
| `expert` | 了解内部机制才建议改 | `tcp-concurrent`、`keep-alive-interval` |

**稳定性**：`experimental` 字段（包括 `experimental:` 分节下的全部字段，以及官方文档标注为实验性的字段）正常展示，带「实验」徽章与一句风险说明；`deprecated` 字段展示替代字段；已移除的字段只在所选内核版本仍支持时出现。

类型系统：`string`、`int`、`bool`、`enum`、`duration`（带单位的字符串或整数，按字段文档区分）、`list<T>`、`map<string,T>`、`object`（嵌套字段）、`oneOf`（如 `proxies` 按 `type` 区分协议）、`any`（只允许出现在目录无法精确描述的位置，并附原因）。

- 目录中的顺序就是渲染时的键顺序，**必须**与官方示例配置（mihomo 仓库 `docs/config.yaml`）和文档的介绍顺序一致；二者不一致时以示例配置为准，并在决定中记录。
- 每个字段**必须**有 `doc` 链接、`importance` 与摘要；详细说明与示例在 M6 前补齐，覆盖率写进 progress。
- `since`/`removed` 的取值以 mihomo 的 git 历史为准，由提取工具给出初值、人工核对；无法确认时标 `unverified: true`，界面显示为「版本未知」，而不是猜一个值。

### 8.3 提取与跟进

- `tools/catalog-extract`：用 Go 的 `go/ast` 读取 mihomo 固定版本源码中配置结构体的 `yaml` tag（`config/`、`adapter/outbound/`、`listener/`、`dns/` 等，**待环境核验**具体位置），输出字段全集与类型初稿；同时解析官方示例配置中的注释，作为说明与默认值的参考。人工补充说明、文档链接与界面提示。
- 提取工具对两个内核版本运行即可得到差异：新增、移除、类型变化。CI **必须**每周对比 mihomo 最新 release 与目录，有差异时自动开 Issue，附字段清单、所在源码位置与相关提交；在目录跟进之前，用户可以用原始编辑先用上新字段。
- 社区的 meta-json-schema 只作交叉核对；许可确认之前不复制其内容（**待环境核验**）。

### 8.4 导出

同一份目录导出：

1. 前端表单元数据（JSON，按分节懒加载）；
2. JSON Schema（draft 2020-12），供原始编辑器补全与校验，也供用户在本地编辑器里使用；
3. i18n 源消息（字段标题、摘要、详细说明、示例说明，进入第 15 节的翻译流程）。

导出物由构建生成，不手工编辑；CI 检查导出物与目录一致。

## 9. 两层编辑与合并语义

```text
表单层 ──合并──▶ 原始编辑层（后备隐藏能源）──▶ 目标适配 ──▶ 渲染
（目录内字段，      （任意 YAML，可写目录外字段，
  生成器产出的部分）   带合并标签）
```

原始编辑优先于表单。两层的结果都要经过目标适配、格式自检与内核检查。

### 9.1 表单层

- 输入来源：节点与规则生成的部分（`proxies`、`proxy-groups`、`rule-providers`、`rules`）加上用户在表单中设置的字段。
- 只输出用户设置过的字段，以及生成器产出的部分；其余字段交给内核默认值。
- 用户把字段设回默认值时，记为「显式设置」，照样输出；「恢复默认」操作才会删除该字段。

### 9.2 原始编辑层（后备隐藏能源）

最后的兜底手段：一段 YAML，合并在表单结果之上，**可以写任何字段**，包括目录里还没有的新字段，原样输出。以原文保存，注释与空行逐字节保留。

**入口不显眼**（界面细节见[第 16.4 节](03-service-and-web.md#164-后备隐藏能源)）：界面上没有按钮或标签页，只能从分节标题的 `⋯` 菜单或命令面板进入。从分节菜单进入时，编辑器自动定位到该分节的路径（不存在则插入空的骨架）。首次使用时显示一次说明。

合并规则：

| 情况 | 默认行为 |
| --- | --- |
| 映射对映射 | 递归合并：同名键继续合并，新键追加 |
| 标量 | 替换 |
| 列表 | 替换 |
| 值为 `null` | 设为 null（**不是**删除；删除用 `!delete`） |

用 YAML 标签改变行为。标签只能出现在两个位置：映射的值上，以及 `!patch-by` 列表的元素上。

| 标签 | 适用 | 行为 |
| --- | --- | --- |
| `!replace` | 映射、列表 | 整体替换，不递归 |
| `!append` | 列表 | 追加到原列表末尾 |
| `!prepend` | 列表 | 插入到原列表开头 |
| `!delete` | 任意 | 删除该键（值被忽略，写 `!delete ~`） |
| `!patch-by:<键>` | 元素为映射的列表 | 按指定键（如 `name`）找到原列表中的元素并递归合并，找不到则追加；给元素加 `!delete` 标签则删除该元素 |

```yaml
dns:
  nameserver: !replace
    - https://dns.example.com/dns-query
rules: !prepend
  - DOMAIN,intranet.example.com,DIRECT
proxy-groups: !patch-by:name
  - name: 🐳 漏网の鱼
    type: fallback
    url: https://www.gstatic.com/generate_204
    interval: 300
  - !delete { name: 📱 TikTok }   # 删除整个元素
experimental:
  some-new-field: true            # 目录外字段：原样输出
```

- 写成 `!patch-by:name` 而不是 `!patch-by name`：标签后的空格会把 `name` 变成节点内容，导致 YAML 无效。
- 未知标签是错误；合并语义**必须**有表驱动测试与属性测试（对同一输入结果必须确定）。
- 引用不存在的组或节点时，只给警告；最终由内核检查判定。
- 合并后的键顺序：原有键保持原位，新键按目录顺序插入；目录外的键排在所在映射的末尾，按出现顺序排列。
- 来自原始编辑的字段在表单与 YAML 预览中带「⚡」标记，便于识别。
- **回收**：目录更新后，若原始编辑中的某个字段已成为目录字段，界面提示「可以转为表单字段」，由用户确认后迁移。

**导入兼容**（导入时一次性转换为上述标签，原文另存备查）：

- Clash Verge Rev Merge：`prepend-rules`、`append-rules`、`prepend-proxies`、`append-proxies`、`prepend-proxy-groups`、`append-proxy-groups` → 对应列表的 `!prepend`/`!append`；其余键按深度合并（**待环境核验**：以当前版本官方文档为准）。
- Stash 覆写（`.stoverride`）：`#!replace` → `!replace`，`#!append` → `!append`；Stash 列表的默认行为是插入到开头，转换时显式写成 `!prepend`。`name`、`desc` 只用于展示，不写入配置。

## 10. 目标适配

### 10.1 目标的组成

目标 = 格式包 + 预设 + 内核版本。预设是 `catalog/mihomo/presets/` 下的数据文件，不写进代码：

```yaml
id: openclash
label: i18n:preset.openclash
platform: openwrt
core: mihomo                   # mihomo | stash
docs: https://github.com/vernesong/OpenClash/wiki
defaults:                      # 新建方案时的表单初值
  external-controller: 0.0.0.0:9090
managed:                       # 由客户端或插件接管的字段：照常展示，带「由客户端接管」提示
  - dns
  - tun
  - mixed-port
notes: i18n:preset.openclash.notes
```

### 10.2 第一版预设

依据 mihomo 官方客户端列表（<https://wiki.metacubex.one/startup/client/client/>，2026-11-09 查证，列表中多数标注为维护中）：

| 预设 | 平台 | 说明 |
| --- | --- | --- |
| `mihomo` | 通用 | 基准，不接管任何字段 |
| `clash-verge-rev` | Windows、macOS、Linux | — |
| `sparkle` | Windows、macOS、Linux | — |
| `flclash` | 桌面、Android | — |
| `clash-nyanpasu` | 桌面 | — |
| `clashmi` | 全平台 | — |
| `openclash`、`nikki`、`shellcrash` | OpenWrt、路由器 | 插件会接管部分字段 |
| `stash` | iOS、macOS、tvOS | 独立的字段子集与扩展 |

每个预设的 `defaults` 与 `managed` 都是**待环境核验**：实施时按该客户端的官方文档或源码逐项确认，并在预设文件中注明依据链接；查不到依据的项不写。

### 10.3 适配步骤

选定目标后，表单展示该目标支持的**全部**字段（第 8.2 节）。渲染前：

1. **内核版本门槛**：字段 `since` 高于所选内核版本、或 `removed` 不高于所选版本时，给出警告；不自动删除（用户可能在用更新的内核）。
2. **目标不支持的字段**：目录中标为 `unsupported` 的表单字段被删除，并在诊断中逐条列出。原始编辑中写入的字段不受此步骤影响（用户明确要求输出）。
3. **接管字段**：`managed` 中的字段照常输出，同时提示用户这些字段会被客户端覆盖。
4. **Stash**：使用 `stash` 变体目录；mihomo 独有的协议或字段删除并报告，Stash 独有字段只在 Stash 目标下出现。Stash 文档：<https://stash.wiki/>（**待环境核验**：字段全集与协议支持表）。

## 11. 渲染与检查

优先级（从高到低）：**格式绝对正确 → 字段顺序与排版美观 → 客户端导入不报错 → 每个字段都有效**。无效字段宁可舍弃，也不能输出一份格式错误的文件。

### 11.1 渲染

- 使用 `go.yaml.in/yaml/v3` 的 Node API（与 mihomo 相同的 YAML 库），由程序构造节点树再序列化，**不做字符串拼接**；缩进固定 2 个空格，不出现制表符，统一 LF 换行，文件以单个换行结尾。
- **顺序**：顶层分节按官方示例配置的顺序排列（通用 → 入站 → DNS 与嗅探 → TUN → 其他分节），`proxies`、`proxy-providers`、`proxy-groups`、`rule-providers`、`rules` 依次排在最后；分节内部按目录顺序。每个节点、组、provider 内部也按目录顺序排列键（`name`、`type` 在前，其后按文档顺序）。
- **排版**：顶层分节之间空一行；字符串只在必要时加引号，必须加引号时用单引号；含 emoji 与非 ASCII 字符的名称原样输出，不转义。
- `proxies` 中每个节点以流式映射（单行）输出，便于浏览；其余部分使用块格式。这是**默认**做法，可按方案切换为块格式。
- 文件头注释（每行以 `# ` 开头，键名大写，与 domainrouter_v2 风格一致）：

  ```yaml
  # NAME: 手机 · Stash
  # TARGET: stash · mihomo 1.19.33
  # GENERATOR: Clash, Launch! 1.0.0 · https://github.com/angwz/clash-launch
  # UPDATED: 2026-11-09 06:08:31 (UTC+8)
  ```

  时区取方案设置（默认为用户设置的时区）；时间取自输入快照（见 7.1），保证确定性。
- golden 测试**必须**锁定渲染结果；改动渲染格式时，先在真实数据上生成样例，交维护者确认后再改测试。

### 11.2 格式自检（必须，每次渲染都执行）

1. 把渲染出的文本重新解析为节点树，与渲染前的树逐节点比较：类型、键、值、顺序全部一致才算通过。这一步保证缩进、引号、转义没有出错。
2. 检查 YAML 层面的风险：重复键、`yes`/`no`/`on`/`off` 等可能被旧解析器当作布尔值的裸字符串（必须加引号）、以 `0` 开头可能被当作八进制的数字串、超长行。
3. 自检失败属于引擎缺陷：不发布，写入错误日志，测试中视为失败。

### 11.3 内核检查（尽力而为）

mihomo 自带官方的配置检查：`mihomo -t`（只检查配置，不启动代理）。

- 镜像内置固定版本的 mihomo 可执行文件（SHA-256 记录在 `tools.lock.json`，与 domainrouter_v2 的做法相同），对每次渲染结果执行 `mihomo -t -d <临时目录> -f <文件>`。
- 检查**不得**联网：geodata 与 rule-provider 是否会在 `-t` 时下载，**待环境核验**；如果会下载，就在临时目录中放置空的占位文件或关闭自动下载，并在文档中说明这一层检查覆盖到哪里为止。
- 资源限制：超时（默认 10 s）、内存上限；并发检查数受信号量限制。
- 不通过时：把内核输出映射为诊断，尽量定位到配置路径；订阅继续提供上一次成功的结果。若出错的是原始编辑中的字段，诊断上提供「舍弃这个字段」的快捷操作。
- Stash 没有可用的内核，只做目录级校验与格式自检；界面上**不得**显示为「已通过内核检查」。
- 内核检查的单元测试带 `kernel` 构建标签，可单独运行；CI 必须运行。
