# 与 subconverter 的差异

本项目兼容 subconverter 的 `rulesets.toml`、`groups.toml` 与外部配置 ini 的语义，基准是 [angwz/subconverter@a33d49e](https://github.com/angwz/subconverter)（与 domainrouter_v2 的验证基准相同）。本文逐条列出与它不同的地方，以及每一条是**有意的改进**还是**待修复的缺陷**。

## 证据层级

| 对比项 | 方法 | 结果（2026-11-09） |
| --- | --- | --- |
| `rules` | `TestParityRules`：按 fork 源码（`templates.cpp` 原生 provider 路径）实现参考模型，与本项目逐条比较 | domainrouter_v2 release 的 `rulesets.toml`（66 条）、`rulesets-mrs.toml`（66 条）以及 ACL4SSR 的 `Online`、`Online_Full`：**逐条一致** |
| `proxy-groups` 成员 | `TestParityGroups`：按 fork 的 `groupGenerate` 实现参考模型，在合成节点上比较 domainrouter_v2 的 23 个组 | 一致（空组的回退项不同，见下表 #6） |
| `rule-providers` | 源码阅读 | 差异见下表 #1、#2 |

两个参考模型都来自源码阅读，**没有运行实际部署的 subconverter**；这与 domainrouter_v2 的验证方式相同。实际运行的对比留待维护者提供 subconverter 实例后补做。

## 差异

| # | 项目 | subconverter | 本项目 | 性质 | 依据 |
| ---: | --- | --- | --- | --- | --- |
| 1 | rule-provider 的 `path` | `./providers/rule-provider_<名>.yaml` | `./ruleset/<名>.<扩展名>` | 有意 | 规范 §6.3；扩展名与内容一致，便于在客户端目录中辨认 |
| 2 | rule-provider 的 `format` | 不写（mihomo 一律按 YAML 读取） | 按扩展名写出 `yaml`、`text`、`mrs`；可用扩展键 `format` 显式指定 | 有意 | mihomo 不按扩展名判断格式；不写 `format` 时 `.mrs` 与 `.list` 无法加载（domainrouter_v2 决定 0003） |
| 3 | provider 重名判断 | 区分大小写 | 不区分大小写，冲突时追加 `_2` | 有意 | 规范 §6.3；避免在不区分大小写的文件系统上缓存文件互相覆盖 |
| 4 | 内联逻辑规则的组名位置 | 追加后套用 `^(.*?,.*?)(,.*)(,.*)$ → $1$3$2`，`AND,((A),(B))` 这类规则会被插进括号内 | 按 mihomo 规则语法：组名放在顶层条件之后、`no-resolve`/`src` 之前；括号内的逗号不参与拆分 | 有意 | 规范 §6.3；对 `GEOIP,CN,no-resolve` 等常见写法两者结果相同（差异测试覆盖） |
| 5 | 单条与批量节点的编号 | 每个输入链接都占一个 GroupID | 只有订阅类型的源参与编号 0、1、2……；单条、批量、文件不编号但保留位置 | 有意 | 已确认的决定 #7、规范 §5.7 |
| 6 | 正则未命中任何节点的组 | 写入 `DIRECT` | 默认移除未被规则使用的空组，被规则使用的指向 `MATCH` 目标；可选保持 `COMPATIBLE`（见 [rules](rules.md)） | 有意 | 规范 §6.2、反馈 #45；`DIRECT` 会让本应走代理的流量直连；mihomo 的空组语义就是 `empty-fallback`，默认 `COMPATIBLE`（`adapter/outboundgroup/parser.go`） |
| 7 | `[]名称` 重复出现 | 不去重 | 同一成员只出现一次，位置取首次出现处 | 有意 | 规范 §6.2「只出现一次」 |
| 8 | 组的其他键 | Clash 输出只写 `strategy`、`lazy`、`url`、`interval`、`tolerance`、`disable-udp` | 目录中的 mihomo 组字段（如 `icon`、`hidden`、`default-selected`、`empty-fallback`）原样透传；目录外的键透传并警告 | 有意 | 规范 §6.2；原则 1 |
| 9 | `timeout` | 默认 5（秒），Clash 输出中不写 | 忽略并警告：mihomo 组的 `timeout` 单位是毫秒，原样透传会变成 5 毫秒 | 有意 | `binding.h`、`subexport.cpp`；mihomo `GroupCommonOption.TestTimeout`；需要时在原始编辑中设置 |
| 10 | `evaluate-before-use`、`persistent` | Surge 专用 | 忽略并警告 | 有意 | 不是 mihomo 字段 |
| 11 | `relay`、`ssid` 类型 | 合法 | 报错（error）：mihomo 已移除 `relay`（改用 `dialer-proxy`），从未支持 `ssid`；组按原样输出，由内核检查拒绝 | 有意 | `adapter/outboundgroup/parser.go`（v1.19.33） |
| 12 | `!!INSERT=` | 匹配 insert_url 中的节点（编号取负数后比较） | 本项目没有 insert 源，匹配不到任何节点，给出警告 | 有意 | 有序源取代了 insert_url |
| 13 | `!!TYPE=` | 只认识 SS、SSR、VMESS、TROJAN、SNELL、HTTP、HTTPS、SOCKS5、WIREGUARD、VLESS、HYSTERIA、HYSTERIA2 | 同上；其余 mihomo 协议按 `type` 的大写形式参与比较（如 TUIC、ANYTLS） | 扩展 | fork 对未知类型直接返回不匹配 |
| 14 | `!!GROUP=` | 匹配链接上 `tag:名称,` 指定的组名 | 匹配源的名称 | 有意 | 规范 §6.2；本项目以源为单位组织节点 |
| 15 | `script:` | 授权后运行 JavaScript | 报错，不支持 | 有意 | 运行时不执行用户脚本 |
| 16 | 正则引擎 | PCRE2（`regFind` 为搜索，`(?i)` 前缀表示忽略大小写） | regexp2（与 mihomo `filter` 一致），默认选项、搜索语义，超时 100 ms | 有意 | 规范 §6.2；PCRE 独有的语法（如 `\K`、占有量词）在 regexp2 中不可用时报错 |
| 17 | `[custom_groups.select]` 表 | 忽略（`from_toml` 只读取已知键） | 现代选择器，见 [rules](rules.md) | 扩展 | 规范 §6.4；`binding.h` 不读取未知键 |
| 18 | rulesets 的 `format` 键 | 忽略 | 显式指定格式（Loyalsoldier 的 `.txt` 是 YAML payload） | 扩展 | 同上 |
| 19 | ini 的 `emoji=正则,emoji` | 给匹配的节点加 emoji | 国旗 emoji 转为源级地区规则（优先于内置识别）；其他 emoji 不支持并说明 | 有意 | 本项目的国旗由地区属性生成（规范 §5.8） |
| 20 | ini 的基础配置类键（`clash_rule_base` 等） | 指向基础配置 | 不支持并说明 | 有意 | 本项目的基础配置是表单层与原始编辑层 |
| 21 | 节点改名去重 | 同名追加 ` 2`、` 3` | 相同（跨源） | 一致 | `processRemark` |
| 22 | 节点名中的 `=` | 替换为 `-`（为 Surge 兼容） | 原样保留 | 有意 | 原则 1；Clash 没有这个限制 |

## 与 subconverter 一致、但值得注意的行为

- **`!!GROUPID=` 范围语法**完全照搬 `matchRange`（`TestMatchRange` 逐条锁定），包括它的怪异之处：`!n` 会覆盖前面各项的结论而不是取「或」；`!-3` 中的负号被丢弃；`n-` 表示 ≤ n，`n+` 表示 ≥ n。
- **`clash-ipcidr` 的引用追加 `,no-resolve`**；`[]FINAL` 等同 `[]MATCH`；provider 名取文件名去掉扩展名。
- **用户正则按原样执行**：domainrouter_v2 的「🎇 香港高速」用 `(港|HK|…)` 匹配，会把「广港 IEPL 新加坡」这类节点也选进来——与 subconverter 相同。需要按落地地区精确选择时，改用现代选择器 `select.region`，它使用剔除了线路词的地区识别结果。
