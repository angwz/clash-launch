# 0001 · 相对实施规范 1.0 的调整

- 状态：已采纳（2026-11-09，实施开始时）；随里程碑推进追加条目，编号连续
- 范围：覆盖 [实施规范 1.0](../spec/index.md) 中与下列内容冲突的条款；未提及的条款仍然有效

## 背景

规范把若干外部事实标为「待环境核验」，并给出了部分实现细节的默认做法。实施前对照 mihomo v1.19.33 源码、官方示例配置与 wiki，以及 angwz/subconverter@a33d49e 源码逐项核对，下表记录与规范不同的选择及其依据。核验结论本身见文末。

## 决定

| # | 主题 | 决定 | 替代的规范条款 |
| ---: | --- | --- | --- |
| 1 | 工具链 | Go 1.27.3（`go.mod`：`go 1.27.0`，`toolchain go1.27.3`）；Node.js 26.13.0 LTS。开发机缺少这两个工具时，使用官方便携包放在被忽略的 `.cache/toolchain/` 中，并校验官方 SHA-256，不改动系统环境 | §17、§18（待核验 #11） |
| 2 | YAML 发射 | 渲染使用自研的受限发射器，遍历 yaml.v3 的 Node 树输出；**解析**与格式自检仍用 `go.yaml.in/yaml/v3` | §11.1「使用 yaml.v3 的 Node API 序列化」 |
| 3 | 顶层顺序 | 末尾分节按官方示例排列：`proxies → proxy-groups → proxy-providers → rule-providers → rules → sub-rules → listeners` | §11.1 中 providers 在 groups 之前的概述 |
| 4 | 节点头部 | 所有协议的前四个键统一为 `name, type, server, port`；其余键按该协议在官方示例中的顺序 | §8.2「以示例配置为准」的一处例外 |
| 5 | 包划分 | 诊断放在 `internal/diag`，另设 `internal/yamlx`（安全解析与节点工具）、`internal/profile`（方案文档）、`internal/version` | §19.1（诊断原属 `engine`） |
| 6 | 目录文本 | 字段说明直接写在目录数据中：`zh`/`en` 为 `[标题, 摘要]`，`detail` 为详细说明；导出时生成 `catalog.<字段 ID>.title/summary/detail` 消息键 | §8.2 示例中的 `i18n:` 键 |
| 7 | 版本未知 | 字段不写 `since` 即表示「版本未知」，不再逐个标注 `unverified: true` | §8.2 |
| 8 | 表单与生成分节 | 表单层不能设置 `proxies`、`proxy-groups`、`proxy-providers`、`rule-providers`、`rules`；它们由生成器产出，修改请用原始编辑的合并标签 | §9.1（补充） |
| 9 | 多行字符串 | 字面块只用 `|` 与 `|-`；以多个换行结尾、只含换行或行首为制表符的字符串改用双引号 | §11.1（补充） |
| 10 | 文件头时间 | 没有输入快照时间时省略 `UPDATED`；CLI 默认取方案文件的修改时间，可用 `-updated` 指定 | §11.1（补充） |
| 11 | 内核检查的内存上限 | 通过子进程的 `GOMEMLIMIT`（Go 运行时软上限，默认 256 MiB）限制；不使用网络命名空间 | §11.3「内存上限」 |

## 取舍

- **自研发射器（#2）**：yaml.v3 的 `is_printable` 不认 4 字节 UTF-8，emoji 与国旗会被强制写成 `"\U0001F1F8"` 形式的转义（本地探针实测）；它也不给 `yes`、`0123` 这类在 YAML 1.1 解析器中有歧义的字符串加引号，且无法在分节之间留空行。这三点都直接违反「格式第一」。发射器只输出块映射、块序列、流式集合与普通、单引号、双引号、字面块四种标量写法，不输出锚点与别名。正确性由两层保证：每次渲染后用 yaml.v3 重新解析并逐节点比较；fuzz 测试（任意 UTF-8 字符串出现在值、键、流式节点与列表中）在约 88 万次执行中无失败。期间 fuzz 找出并修正了五类边界：只含换行的字符串、U+FFFE 等非字符、U+2028/U+2029、流式上下文中 libyaml 额外拒绝的普通标量、超过 1024 字符的隐式键。
- **顶层顺序（#3）**：§8.2 规定示例与文档不一致时以示例为准；§11.1 的概述把 providers 写在 groups 之前，与示例冲突，按 §8.2 处理。
- **节点头部（#4）**：官方示例中只有 tuic 的样例把 `type` 写在 `port` 之后，属于个别写法；统一头部更易读，也便于逐行比较节点。
- **诊断独立成包（#5）**：节点、规则、合并、渲染各包都要产出诊断，而 `engine` 依赖这些包；诊断放在 `engine` 会形成循环依赖。
- **目录文本内联（#6）**：字段定义与说明放在一处，编写与审阅更直接；导出物仍按规范提供 i18n 源消息，前端与机器翻译流程不受影响。
- **表单不碰生成分节（#8）**：避免同一分节有两个来源而产生歧义；原始编辑的 `!patch-by:name`、`!prepend` 等已覆盖修改需求。

## 核验结论（待环境核验项）

| 规范 | 结论 | 依据 |
| --- | --- | --- |
| #1 直接 import mihomo | 可以。mihomo 的 `go.mod` 只有一条 `replace`（`google.golang.org/protobuf` → `metacubex/protobuf-go`，对应 golang/protobuf#1704），本项目 `go.mod` 复制同一条以保持行为一致；`adapter.ParseProxy`、`common/convert.ConvertsV2Ray` 编译与调用正常 | `go.mod`；本地探针 |
| #2 `mihomo -t` 是否联网 | `-t` 只执行 `config.Parse`，不调用 provider 的 `Initial()`，proxy-providers 与 rule-providers 不下载；`GEOIP`、`GEOSITE`、`IP-ASN` 在解析时调用 `geodata.Init*`，文件缺失才下载。做法：临时目录中预置 `tools.lock.json` 固定的地理数据，并把检查副本的 `geox-url` 改为回环上的不可达地址。覆盖范围见 [kernel 包注释](../../internal/kernel/check.go) | `main.go`、`hub/executor`、`component/geodata/init.go`（v1.19.33） |
| #3 配置结构体位置 | `config.RawConfig`（`yaml` tag）、`adapter.ParseProxy` 分派的 `outbound.*Option`（`proxy`）、`outboundgroup.ParseProxyGroup`（`group`）、`adapter/provider` 与 `rules/provider` 的 schema（`provider`）、`listener.ParseListener`（`inbound`）。`catalog-extract scan` 对 v1.19.33 提取 2075 个字段，对 v1.19.32 的差异为 `tun.dns-mode` 与 `listeners.tun.dns-mode` 两项新增 | `tools/catalog-extract` |
| #7（部分）策略组类型 | mihomo 只接受 `select`、`url-test`、`fallback`、`load-balance`；`relay` 已移除（提示改用 `dialer-proxy`），`ssid` 不支持。组内既无 `proxies` 也无 `use` 时报错；`empty-fallback` 默认 `COMPATIBLE`，且不能是策略组 | `adapter/outboundgroup/parser.go` |
| #11 版本号 | Go 1.27.3（2026-11-09 的最新稳定版），Node.js 26.13.0 LTS（Lithium）；mihomo v1.19.33 仍是最新稳定版 | go.dev/dl、nodejs.org/dist、GitHub tags |
| 示例中的地理数据 | `geoip.metadb`、`GeoSite.dat`、`ASN.mmdb` 的 SHA-256 与 domainrouter_v2 已核验的锁一致；`GeoIP.dat` 为本项目新增，取同一提交，首次下载时记录哈希 | `tools.lock.json` |

## 验证方式

- 发射器与自检：`go test ./internal/render/`（含 golden）与 `go test -fuzz FuzzRender ./internal/render/`。
- 顺序：`go test ./internal/catalog/` 检查顶层、dns、tun、sniffer、各协议与 rule-provider 的键顺序是官方示例的保序扩展。
- 内核：`go test -tags kernel ./internal/kernel/` 用 mihomo v1.19.33 检查全部 golden 输出。
