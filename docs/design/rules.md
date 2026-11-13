# 规则源与策略组

[`internal/ruleset`](../../internal/ruleset/) 把三种规则源——`rulesets.toml` + `groups.toml`、subconverter 外部配置 ini（如 ACL4SSR）、规则文件仓库的预设——转换为同一个模型，再展开为 `proxy-groups`、`rule-providers` 与 `rules`。与 subconverter 的逐条差异见 [subconverter-parity](subconverter-parity.md)。

规则源的每个文件（groups、rulesets、ini）可以给地址、粘贴文本或两者：粘贴文本非空时以它为准；为空且有地址时用最近一次拉取的快照；地址清空后旧快照不再生效。

## 有序源与订阅编号

方案中的源是有序列表，顺序决定节点在 `proxies` 与每个策略组中的先后，也决定订阅编号：只有订阅类型的源参与编号，按出现顺序为 0、1、2……，供 `!!GROUPID=` 使用；单条、批量与文件不编号，但节点出现在它们所在的位置，仍参与普通正则匹配。停用的源不参与编号，后面的订阅编号依次前移。使用 `!!GROUPID=` 或选择器 `source = ["#N"]` 的组会附带一条信息诊断，列出当前的编号（`0=机场 A, 1=机场 B`）；界面只在出现这条诊断时才在订阅旁显示 GROUPID，平时左侧数字只表示位置。

界面只呈现两类源：订阅（地址，定时拉取）与手动节点（粘贴分享链接、Clash YAML 或 sing-box JSON，按内容自动识别）。新建的手动节点存为 `batch`；1.0 的 `single`、`file` 照常可用，引擎对三者的处理相同。

## groups.toml

校验移植 domainrouter_v2@2c812f9 `groups.py` 的全部检查：`name`、`type` 必填；type ∈ `select`/`url-test`/`load-balance`/`fallback`/`relay`/`ssid`；需要测速的类型必须有 `url` 与整数 `interval`；至少有 `rule` 或 `use`，`rule` 为字符串数组；组名不含逗号、换行或首尾空白；组名不重复（重名时以后出现的定义为准，与 groups.py 一致）；`[]名称` 引用的组或内置策略（`DIRECT`、`REJECT`、`REJECT-DROP`、`PASS`、`COMPATIBLE`）必须存在；组之间无循环引用。另外，`relay` 与 `ssid` 报 error：mihomo 不支持。

`rule` 条目：

| 条目 | 写入配置（inline） | 交给客户端（provider） |
| --- | --- | --- |
| `[]名称` | 写入 `proxies` | 写入 `proxies` |
| 正则 | 在服务器上按名称匹配 | 改写为 `filter`（多个以反引号连接），`use` 全部 provider 源 |
| `!!GROUPID=范围[!!正则]` | 编号在范围内的订阅中的节点 | `use` 编号在范围内的 provider；正则并入 `filter`（对组内全部 provider 生效，给出警告） |
| `!!GROUP=正则[!!正则]` | 源名称匹配的节点 | `use` 名称匹配的 provider |
| `!!TYPE=`、`!!PORT=`、`!!SERVER=` | 按 fork 语义匹配 | 无法表达，警告 |
| `!!INSERT=` | 不匹配（没有 insert 源），警告 | 同左 |
| `script:` | 不支持，报错 | 同左 |
| `@select` | 现代选择器的结果放在这里 | — |

条目顺序即组内顺序；同一成员只出现一次，位置取首次出现处。没有成员也没有 `use` 的组写入 `empty-fallback`（默认 `COMPATIBLE`）并警告。其余键按目录透传，`timeout`、`evaluate-before-use`、`persistent` 忽略并说明原因；`use` 引用的 provider 不存在时警告（原始编辑中仍可补上，最终由内核检查判定）。

## 现代选择器

在 `[[custom_groups]]` 中加一个 `select` 表，按节点的属性选择节点；旧写法照常可用，两者可以并存：

```toml
[custom_groups.select]
region   = ["HK"]                   # 落地地区（剔除线路词后的识别结果，含手动指定）
source   = ["机场 A", "#1"]          # 源名称，或 #编号（GROUPID）
protocol = ["hysteria2", "vless"]
tags     = ["IEPL"]
name     = "(?i)premium"            # 名称正则（regexp2）
rate     = { max = 1.5 }            # 倍率范围（min、max）
exclude  = { region = ["CN"], name = "(?i)游戏|测试" }
sort     = "source"                 # source（默认）| name | rate
```

同一键内的多个值是「或」，不同键之间是「且」；`exclude` 中任一条件命中即排除。provider 模式下 `source` → `use`，`name` → `filter`，`exclude.name` → `exclude-filter`，`protocol` → `exclude-type`（取补集）；`region`、`tags`、`rate` 依赖服务器端的信息，给出警告。subconverter 读取 groups.toml 时忽略这张表（`binding.h` 只读取已知键）。

## rulesets.toml

| 输入 | 输出 |
| --- | --- |
| `clash-domain` · `clash-ipcidr` · `clash-classic` | `behavior: domain` · `ipcidr`（引用追加 `,no-resolve`）· `classical` |
| `surge-ruleset`（缺省类型） | `behavior: classical`、`format: text`；mihomo 逐行跳过它不认识的规则类型（`classicalStrategy.Insert`，已核验），整个 provider 不会因此加载失败 |
| `quantumultx` | 不支持，报错 |
| `.mrs` · `.txt`/`.list` · 其他 | `format: mrs`（classical 不能是 MRS）· `text` · `yaml`；扩展键 `format` 可显式指定 |
| `[]GEOIP,CN,no-resolve` 等内联 | 插入组名：`GEOIP,CN,<组>,no-resolve`；`[]FINAL` 等同 `[]MATCH` |

provider 名取文件名（去掉扩展名），不区分大小写地全局唯一，冲突时追加 `_2`；`path` 为 `./ruleset/<名>.<扩展名>`，`interval` 缺省 86400。规则顺序即 toml 顺序；原始编辑可以用 `rules: !prepend` / `!append` 在前后插入。

## 外部配置 ini

`[custom]` 节中：`ruleset=组,来源[,间隔]`（来源可带 `clash-domain:` 等前缀）；``custom_proxy_group=名称`类型`成员…[`测速地址`间隔,超时,容差]``，成员中的 `!!PROVIDER=a,b` 写入 `use`；`rename`、`emoji`（国旗）、`include_remarks`、`exclude_remarks`、`add_emoji`、`remove_old_emoji` 映射到节点整理；`enable_rule_generator=false` 时不生成规则；其余键逐个说明不支持。

## 预设

[`catalog/rulesources/`](../../catalog/rulesources/) 中每个预设注明官方仓库、许可、核对日期；本项目只引用地址，不复制、不再分发规则内容。

| 预设 | 形式 | 许可 | 说明 |
| --- | --- | --- | --- |
| domainrouter_v2（推荐） | toml 一对（YAML 与 MRS 两个变体） | 见仓库 | — |
| ACL4SSR | ini（Online、Online Full、Online Mini、Online Full Netflix、Online Full NoAuto） | CC-BY-SA-4.0 | — |
| Loyalsoldier/clash-rules | 规则文件 + 策略组模板 | GPL-3.0 | 按官方 README 的白名单模式；`.txt` 内容是 YAML payload，显式写 `format = "yaml"` |
| MetaCubeX/meta-rules-dat | 规则文件 + 模板 | GPL-3.0 | geosite/geoip 的 MRS |
| blackmatrix7/ios_rule_script | 规则文件 + 模板 | GPL-2.0 | 按应用划分的 classical YAML |
| SukkaW 规则集 | 规则文件 + 模板 | AGPL-3.0 | ruleset.skk.moe 的 Clash 文本列表 |

地址、文件格式与许可均在 2026-11-09 实际访问核对；`TestPresets` 检查每个内联模板都能无错误地解析与展开。
