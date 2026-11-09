# Clash, Launch! 实施规范 · 基础与输入

版本：1.0 · 日期：2026-11-09 · 第 0～6 节

本分册与入口列出的其他三份分册共同构成完整规范，须按顺序阅读。[返回规范入口](index.md)

## 0. 给实施者的说明

请先读完四份分册，再检查工作区当前状态，按[第 20 节](04-delivery-and-handoff.md#20-里程碑与验收)的里程碑实施。

约束词：

- **必须**：相应里程碑的验收条件，不得自行省略。
- **默认**：本规范选定的默认做法；改动须有明确理由，并记录为决定（`docs/decisions/`）。
- **可选**：不影响核心交付，不得成为核心路径的依赖。
- **待环境核验**：实施时须查官方文档、源码或实际运行确认的外部事实；核验结果写进决定或活文档，不放宽正确性要求。

工作原则（来自 domainrouter_v2 的实践，均为**必须**）：

1. **用户写下的内容是有意为之。** 节点名、组名、正则、手写规则、原始编辑中的值按原样保留；看似不规范时只报告，不改写。
2. **只按官方文档或源码实现语义。** 不确定时查 mihomo 官方文档（<https://wiki.metacubex.one/>）或其源码的固定版本；查不到就明确列为限制，不猜。没有正式规范的输入格式不支持。
3. **不静默丢弃。** 无法解析的节点、无法转换的字段、被目标删除的字段，都进入诊断清单并在界面显示原因。
4. **未运行的验证不写成通过。** 源码阅读、静态检查、单元测试、内核校验、真实客户端导入是不同层级的证据，分别记录。
5. **不为假想需求增加功能。** 扩展点可以留，实现只做本规范要求的部分。

规范中的目录、命令、配置键是计划，不是现状；实施过程中以活文档记录实际情况。

### 0.1 工作环境与参考资料

实施可能在另一台电脑上进行。以当前打开的 `clash-launch` 工作区为项目根，不依赖编写本规范时的用户名、盘符或绝对路径。

**需要的工具**：git、Go（当前稳定版）、Node.js（当前 LTS）；Docker 用于 M3 起的镜像验证，不可用时记录为未验证，不阻塞其他工作。本机没有 Docker 时，维护者可能提供远程 VPS 上的 Docker（经 `docker context` 或 `DOCKER_HOST=ssh://…`）；在远程主机上构建或运行容器之前**必须**先征得维护者同意，只做构建与测试，用完清理本项目创建的容器、镜像与卷，不触碰主机上的其他容器。工具缺失时先告诉维护者需要安装什么，不擅自修改系统环境。

**参考资料一律只读，按固定版本获取**，下载到 `.cache/`（已在 `.gitignore` 中，不入库、不打包）：

| 资料 | 来源 | 固定版本 | 用途 |
| --- | --- | --- | --- |
| domainrouter_v2 | <https://github.com/angwz/domainrouter_v2>（公开） | 提交 `2c812f9da217a12d16507c9af8d0b89a5f413e4c` | 文档体系与工程约定的范本；`groups.py` 的校验逻辑；`docs/guide/subconverter.md` |
| domainrouter_v2 发布物 | `release` 分支 | 跟随最新 | 真实的 `rulesets.toml`、`groups.toml`，用于 M2 测试 |
| mihomo | <https://github.com/MetaCubeX/mihomo> | 标签 `v1.19.33`（或实施时的最新稳定版，写进决定） | Go 依赖、`catalog-extract` 的输入、内核检查用的二进制 |
| angwz/subconverter | <https://github.com/angwz/subconverter> | 提交 `a33d49e` | groups/rulesets/ini 语义与匹配器的基准；差异测试 |
| ACL4SSR | <https://github.com/ACL4SSR/ACL4SSR> | 实施时的 master，记录提交 | 外部配置 ini 的样例 |

- 本机若已有 `../domainrouter_v2`，可以直接读取，但须确认其提交与上表一致或更新，并以 GitHub 上的固定提交为准。
- 不运行参考仓库中的脚本，不修改它们，不推送任何内容。
- GitHub 暂时不可达时，继续做不依赖这些资料的部分，并在 progress 中列出缺项；不得凭记忆臆造它们的内容。

## 1. 目标与范围

Clash, Launch! 是开源、自托管、多用户的 **YAML 配置生成器**。第一个格式包面向 Mihomo 系 Clash 客户端。

必须交付：

1. 字段目录驱动的生成引擎：表单与原始编辑（后备隐藏能源）两层编辑，结果确定、可复现；输出的 YAML 格式正确、缩进规整、字段顺序遵循官方示例。
2. 节点输入：单条分享链接、批量粘贴、订阅 URL、上传文件；后端解析并校验。
3. 规则输入：subconverter 语义的 `rulesets.toml`、`groups.toml` 与外部配置 ini（如 ACL4SSR），可用 URL 跟随更新，也可在线编辑；另有主流第三方规则的预设。
4. 目标客户端预设：Mihomo 通用、主流 GUI 客户端、OpenWrt 插件、Stash。
5. 长期有效的订阅链接：随时可查看、复制、重置，一个方案可有多条。
6. 现代、美观、适配手机、支持多语言（含 RTL）的 Web 界面。
7. Docker 镜像（多架构），单容器即可运行。

不在范围内：

| 不做 | 原因 |
| --- | --- |
| 规则编译、去重、优化 | domainrouter_v2 负责 |
| subconverter 的全部功能、非 Clash 输出 | 只兼容 groups.toml、rulesets.toml 与外部配置 ini 中规则和策略组的语义；其余功能不复刻 |
| 机场面板、计费、节点测速、代理转发 | 不是配置生成器的职责 |
| Quantumult X、Shadowrocket 等没有正式语法文档的格式 | 原则 2 |
| AI 功能 | 以后另行立项 |

## 2. 已确认的决定

| # | 事项 | 决定 |
| ---: | --- | --- |
| 1 | 定位 | YAML 生成器；引擎与格式解耦，Mihomo 是首个格式包 |
| 2 | 两层编辑 | 表单（默认）与原始编辑。**原始编辑就是「后备隐藏能源」**：界面上不放显眼入口，只能从分节 `⋯` 菜单或命令面板进入；在其中可以写入任意字段 |
| 3 | 字段展示 | 选定客户端后，按官方文档展示尽可能多的字段，包括实验性字段；每个字段附详细说明、默认值、示例与重要程度 |
| 4 | 输出质量 | 最重要的是 YAML 格式正确、缩进规整、字段顺序遵循官方示例，美观易读；能用 mihomo 官方的 `-t` 检查就检查，目标是客户端导入不报错 |
| 5 | 用户模型 | 多用户，面向公网部署（在 nginx 反向代理之后）；注册从简：用户名、密码与安全问题，找回时回答安全问题；可选个人 SMTP 邮箱用于异常登录提醒 |
| 6 | 目标 | 主流 Mihomo 系：Mihomo 通用、Clash Verge Rev 等 GUI、OpenClash 等 OpenWrt 插件、Stash |
| 7 | 节点 | 在 Web 中填入节点或订阅，后端解析；源是有序列表，订阅按顺序编号 0、1、2……，供 groups.toml 的 `!!GROUPID=` 筛选；单条与批量节点不编号，但保留所在位置 |
| 8 | 订阅链接 | 长链接即可，不刻意缩短；网页上随时可查看与复制，可一键重置 |
| 9 | 存储 | 默认 SQLite，但存储层可替换，日后用户多了可迁移到 PostgreSQL |
| 10 | 语言 | 后端 Go，前端 Svelte 5 + TypeScript；混合使用 |
| 11 | 多语言 | 1.0 做到简体中文与英文，并预留 RTL（波斯语）与更多语言的结构；机器翻译流水线以后再完善 |
| 12 | 许可 | GPL-3.0-or-later（引用 mihomo，GPL-3.0） |
| 13 | 工程方式 | 照搬 domainrouter_v2：规范交接、活文档、决定、意见记录、进度记录 |
| 14 | 地区与国旗 | 默认自动识别（剔除「广港」这类线路词，取落地地区），单个节点可手动指定并覆盖自动结果；认不出地区时用精选的装饰 emoji，不留空 |
| 15 | 节点选择 | 兼容 `!!GROUPID=` 等旧写法；另提供按地区、源、协议、标签、倍率组合的现代选择器 |
| 16 | 第三方规则 | 支持 ACL4SSR 这类 subconverter 外部配置 ini，以及主流规则文件仓库的预设 |
| 17 | 出站隐私 | 服务器的出站请求可经出站代理，避免向订阅提供者暴露真实 IP；未配置时普通用户只能用「交给客户端」模式 |

## 3. 命名

| 用途 | 约定 |
| --- | --- |
| 展示名 | `Clash, Launch!`（逗号与感叹号是名字的一部分） |
| 仓库、目录 | `clash-launch`；目标地址 `https://github.com/angwz/clash-launch`（尚未创建） |
| Go 模块 | `github.com/angwz/clash-launch` |
| 二进制、CLI、镜像 | `clash-launch`；镜像 `ghcr.io/angwz/clash-launch` |
| 环境变量前缀 | `CLASH_LAUNCH_` |
| 方案文档类型标识 | `clash-launch/profile: v1` |
| Go 包与文件 | 小写、短名，遵循 Effective Go；不使用下划线 |
| 前端文件 | Svelte 组件 PascalCase，其余 kebab-case |

外部协议名、字段名、组名、正则不得为了统一风格而改写大小写。

## 4. 核心模型

```text
用户 User
├─ 节点源 Source        单条 / 批量 / 订阅 URL / 上传文件
├─ 规则源 RuleSource    rulesets.toml + groups.toml
└─ 方案 Profile         目标 · 字段取值 · 原始编辑 · 有序的源列表 · 规则源
     ├─ 版本 Revision   每次保存生成一个，可对比与回滚
     └─ 订阅链接 Link   /s/<令牌> → 最新成功的渲染结果
```

### 4.1 方案是一份声明式文档

方案的权威形式是一份 YAML 文档，数据库只是存储。它必须可以导出、导入、放进 git 版本管理。凭据不出现在文档中，只以源的引用代替。

```yaml
clash-launch/profile: v1
name: 手机 · Stash
target: { pack: mihomo, preset: stash, core: "1.19.33" }
sources:                        # 有序列表：顺序即节点顺序；订阅按出现顺序编号（第 5.7 节）
  - ref: src_airport_a          # 订阅 → GROUPID 0；凭据存在源里，不在方案里
    mode: inline                # inline 写入配置 | provider 交给客户端
  - ref: src_my_vps             # 单条节点 → 不编号，但保留在这个位置
  - ref: src_airport_b          # 订阅 → GROUPID 1
rules:
  ref: rs_domainrouter          # 引用规则源
form:                           # 表单层：只记录用户改过的字段
  mixed-port: 7890
  dns: { enable: true, enhanced-mode: fake-ip }
raw: |                          # 原始编辑（后备隐藏能源）：原文保存，保留注释与标签
  rules: !prepend
    - DOMAIN,internal.example.com,DIRECT
  experimental:
    some-new-field: true        # 目录里还没有的新字段，照样输出
```

- 表单层只保存用户改过的字段；未设置的字段不输出，交给内核默认值。
- 原始编辑以原文保存，用户的注释、空行与标签逐字节保留。
- 导出时可选择「含凭据」（仅限本人，需二次确认）或「不含凭据」（用于分享模板）。

### 4.2 格式包

格式包 = 声明式字段目录（`catalog/<pack>/`）+ 可选的生成器（Go 代码，如节点解析、策略组展开）+ 目标预设（数据文件）。引擎只依赖格式包接口，不认识 Clash 的任何字段。1.0 只实现 `mihomo` 包（含 Stash 变体）；不预先实现其他格式。

## 5. 节点输入

### 5.1 源类型

| 类型 | 输入 | 存储 |
| --- | --- | --- |
| 单条 | 一条分享链接，或一段 Clash 节点 YAML | 加密的原文 |
| 批量 | 多行粘贴：分享链接、base64、Clash YAML 片段混合 | 加密的原文 |
| 订阅 | URL、可选 UA、拉取间隔、可选请求头 | 加密的 URL；拉取结果快照 |
| 文件 | 上传的 YAML、文本或 JSON | 加密的原文 |

### 5.2 内容识别

按以下顺序判断，第一个成功的为准；都失败时报告「无法识别」，并附带开头 200 字节的脱敏预览：

1. Clash/Mihomo YAML：顶层有 `proxies` 列表（完整配置或片段）。
2. sing-box JSON：顶层有 `outbounds` 数组。字段映射以 sing-box 官方文档为依据（**待环境核验**：逐协议的映射表写入 `docs/design/`）。
3. base64：整体解码后按行重新识别（标准与 URL-safe 两种字母表，容忍缺失的填充）。
4. 分享链接：每行一条，按 scheme 分派。

### 5.3 分享链接

**默认**复用 mihomo 源码 `common/convert.ConvertsV2Ray`，与内核行为一致。2026-11-09 查证，它处理的 scheme 为：

`hysteria` · `hysteria2`/`hy2`/`hysteria2+realm`/`hy2+realm` · `tuic` · `trojan` · `vless` · `vmess` · `ss` · `ssr` · `socks`/`socks5`/`socks5h`/`http`/`https` · `anytls` · `mierus`

- `wireguard` 不在其中，且没有正式的分享链接规范，因此**不支持** WireGuard 链接；WireGuard 节点通过 Clash YAML 输入。
- 每个解析出的节点**必须**再经 mihomo 的节点解析（`adapter.ParseProxy`）校验，失败者进入跳过清单，附原因与行号。
- 直接 import mihomo 模块可能受其 `go.mod` 中 `replace` 指令影响（`replace` 对下游不生效）。**待环境核验**；若无法直接引用，把调用隔离在 `internal/kernel` 适配层，固定 mihomo 版本并记录原因。
- 分享链接解析**必须**有 Go 原生 fuzz 测试：任意输入不得导致 panic，凭据不得出现在错误信息里。

### 5.4 订阅拉取

- 在后台按间隔拉取，**不**在客户端请求订阅时同步拉取。
- 默认 UA `clash.meta`，促使机场返回 Clash 格式；可按源修改。
- 保存响应头 `subscription-userinfo`、`profile-update-interval`、`content-disposition`，供订阅输出时选用。
- 失败（网络错误、非 2xx、内容无法识别、解析后为 0 个节点）时继续使用上次成功的快照，在界面上标出失败时间与原因；连续失败超过 7 天时在方案页顶部提示（默认值，可配置）。
- 网络安全约束见[第 14 节](03-service-and-web.md#14-多用户与安全)。

### 5.5 节点整理

按以下固定顺序执行，每一步都可在源上单独配置，且有预览：

1. **过滤**：包含正则、排除正则（`regexp2`，与 mihomo `filter` 语义一致）；另有一个**默认开启**的开关「去掉信息节点」，去掉机场放在节点列表里的「剩余流量」「到期时间」「官网地址」这类假节点（内置模式表，可关闭、可覆盖），被去掉的节点列入诊断。
2. **重命名**：有序的「正则 → 替换」列表。
3. **地区与国旗**：识别每个节点的落地地区，可选在名称前加国旗 emoji；手动指定优先于自动识别，见第 5.8 节。
4. **去重**：按「协议 + 服务器 + 端口 + 凭据 + 传输参数」判重，保留首个，并在诊断中列出被去掉的节点。
5. **排序**：保持原序（默认）或按名称排序。
6. **同名处理**：跨源同名时追加 ` 2`、` 3`……；源可配置名称前缀以避免冲突。

### 5.6 两种写法

用大白话说：

- **写入配置**（`inline`，默认）：服务器拉取订阅、解析出每个节点，把节点本身写进最终 YAML 的 `proxies`。groups.toml 中的正则在服务器上就匹配好了。客户端拿到的是完整配置，所有客户端都能用；节点更新要等客户端刷新这条订阅。
- **交给客户端**（`provider`）：最终 YAML 不写节点，只写一段 `proxy-providers`，让客户端自己去拉取节点；groups.toml 的正则改写成组的 `filter`。适合服务器访问不到机场、或希望客户端独立刷新节点的情况。单条和批量节点在此模式下由本服务托管为 provider 文件，同样受令牌保护。

`provider` 模式下，整理规则中只有过滤能等价地改写为 provider 的 `filter`/`exclude-filter`；重命名、国旗、去重无法等价表达时，**必须**在界面上提示哪些设置不会生效，不得静默忽略。provider 的 `override` 等字段的可用性以目录为准。

### 5.7 源的顺序与编号

方案中的源是一个**有序列表**，界面上可拖动排序。顺序决定两件事：

1. **节点顺序**：最终 `proxies` 与每个策略组中节点的先后，按源在列表中的位置排列，源内部保持各自的顺序。
2. **订阅编号（GROUPID）**：只有**订阅**类型的源参与编号，按列表中出现的先后依次为 `0`、`1`、`2`……，与 subconverter 中多个订阅地址的编号方式一致，供 groups.toml 的 `!!GROUPID=` 使用（第 6.2 节）。

单条、批量与文件类型的源**不编号**，`!!GROUPID=` 永远不会匹配它们；但它们在列表中的位置保留，节点就出现在那个位置。它们仍然参与普通正则的匹配。

```text
源列表（拖动排序）          编号        节点出现的位置
① 机场 A（订阅）            0          第 1 段
② 我的 VPS（单条）           —          第 2 段
③ 机场 B（订阅）            1          第 3 段
④ 朋友分享（批量）           —          第 4 段
```

- 编号随顺序自动变化：拖动订阅会改变它的编号，界面**必须**在拖动时即时显示新编号；groups.toml 中引用了受影响编号的组，在预览中给出提示。
- 每个源可以起一个名字，供 `!!GROUP=` 按名称匹配（第 6.2 节）。
- 订阅被停用时不参与编号，后面的订阅编号依次前移；界面同样给出提示。

### 5.8 地区识别与国旗

subconverter 用「第一个命中的正则」决定国旗，碰到「广港 IEPL 新加坡 01」这种名字，会因为「港」先命中而打上 🇭🇰，而这其实是走广港专线、落地新加坡的节点。本项目的识别结果是节点的一个**属性**（`region`，ISO 3166-1 两位代码），国旗只是它的一种显示方式；同一个属性还供现代选择器按地区选节点（第 6.4 节）。

**优先级**（高的覆盖低的）：

| 级别 | 来源 | 说明 |
| :---: | --- | --- |
| 1 | 单个节点的手动指定 | 在节点表格中点选地区；以节点指纹（协议 + 服务器 + 端口 + 凭据哈希）保存，机场改名后依然有效；指纹找不到时再按名称匹配 |
| 2 | 源级规则 | 用户为某个源写的「正则 → 地区」列表，按顺序第一个命中为准 |
| 3 | 内置识别 | 见下文；数据表随项目发布 |
| 4 | 未识别 | 不加国旗，改为一个「装饰 emoji」（见下文）；`region` 记为未知 |

**内置识别**不是单个正则，而是按以下步骤打分：

1. **名称中已有国旗 emoji**：直接采用，并且不再重复添加。
2. **剔除线路词**：先去掉描述中转线路的词，它们含有地区字，但说的不是落地地。数据表包括「城市简称 + 地区简称」的组合（广港、深港、沪港、莞港、沪日、京德……）以及 IPLC、IEPL、BGP、中转、专线等；表可覆盖、可扩充。
3. **识别中转写法**：出现 `→`、`->`、`=>`、`⇒`、`转` 等方向符号时，取符号之后的地区。
4. **匹配地区关键词**：中文国名与常见简称、主要城市、英文国名、ISO 两位与三位代码（按词边界匹配，避免 `AUS` 中的 `US`）、机场常用的写法；用排除表防误判，如「尼日利亚」不算日本、「印度尼西亚」不算印度。
5. **多个候选时**取名称中最靠后的一个（机场惯例是线路在前、落地在后），并在诊断中标为「推断」。

节点表格中每个节点显示识别结果与**理由**（例如「命中关键词 新加坡；已忽略线路词 广港」），点一下即可改为手动指定。

国旗的写法：`prefix`（**默认**，`🇸🇬 原名称`；名称已有国旗时不重复添加）/ `replace`（替换名称中已有的国旗）/ `off`。

**装饰 emoji**（开启国旗时，未识别的节点使用）：维护者重视观感，未识别的节点不能留空，要和带旗的节点一样整齐。

- 从一份**精选表**中挑选：风格统一、在主流系统上显示良好、不带地域或政治含义的符号，例如 🪐 🛰️ 🌙 ✨ 🔮 🧭 🌀 🍀 🎐 🪁 💎 🌊。表中**不得**包含国旗、地区旗、常被误认为地区的符号（如 🗽、🗼）以及含义负面的符号。精选表是数据文件，可覆盖。
- 「随机」但**稳定**：以节点指纹的哈希值决定挑哪一个，同一个节点每次刷新都得到同一个 emoji，保证渲染的确定性（第 7.1 节）。同一个源内，相邻节点尽量不重复。
- 用户可以给单个节点手动指定 emoji，也可以在设置中只保留自己喜欢的几个。

内置识别**必须**有一份刁钻名称的测试表（中转、混合语言、代码冲突、已有国旗、信息节点），每一条都写明期望结果；识别规则改动时，这份表就是回归测试。

## 6. 规则输入

### 6.1 规则源

一个规则源有两种形式，在引擎内部转换为同一个模型：

1. 一份 `rulesets.toml` 加一份 `groups.toml`（第 6.2、6.3 节）；
2. 一份 subconverter 外部配置 ini，例如 ACL4SSR（第 6.5 节）。

它们都可以是：

- **URL**：后台按间隔拉取，失败沿用上次成功的副本（同 5.4）；
- **在线编辑的文本**：保存为原文，带版本历史。

预设「domainrouter_v2 release」一键填入（其他预设见第 6.5 节）：

```text
https://raw.githubusercontent.com/angwz/domainrouter_v2/release/rulesets.toml
https://raw.githubusercontent.com/angwz/domainrouter_v2/release/groups.toml
```

以及 MRS 版 `rulesets-mrs.toml`。语义基准是 [angwz/subconverter](https://github.com/angwz/subconverter) 提交 `a33d49e`，与 domainrouter_v2 的验证基准相同。

### 6.2 groups.toml

结构为 `[[custom_groups]]` 数组。校验**必须**移植 domainrouter_v2 [`src/domainrouter_v2/groups.py`](https://github.com/angwz/domainrouter_v2/blob/2c812f9da217a12d16507c9af8d0b89a5f413e4c/src/domainrouter_v2/groups.py)（固定提交 `2c812f9`）的全部检查：

- `name`、`type` 必填；type ∈ `select`/`url-test`/`load-balance`/`fallback`/`relay`；
- `url-test`、`fallback`、`load-balance` 必须有 `url` 与 `interval`；
- 至少有 `rule` 或 `use` 之一，`rule` 为字符串数组；
- 组名不含逗号、换行或首尾空白；组名不重复；
- `[]名称` 引用的组或内置策略（`DIRECT`、`REJECT`、`REJECT-DROP`、`PASS`、`COMPATIBLE`）必须存在；组之间无循环引用。

`ssid` 类型在 subconverter 中合法，但 mihomo 没有对应的组类型，遇到时报错并说明原因（**待环境核验**：以 mihomo 当前文档为准）。

`rule` 条目语义：

| 条目 | 含义 | `inline` 模式 | `provider` 模式 |
| --- | --- | --- | --- |
| `[]名称` | 引用组或内置策略 | 写入 `proxies` | 写入 `proxies` |
| 其他字符串 | 匹配节点名的正则 | 在服务器上匹配，结果写入 `proxies` | 改写为 `filter`，并加上 `use` |
| `!!GROUPID=<范围>` 或 `!!GROUPID=<范围>!!<正则>` | 只匹配编号在范围内的订阅中的节点，可再用正则按名称筛选（第 5.7 节） | 在服务器上匹配 | 改为只 `use` 对应订阅的 provider，正则转为 `filter` |
| `!!GROUP=<名称或正则>` | 按源的名称匹配 | 同上 | 同上 |
| `!!INSERT=…`、`!!TYPE=`、`!!PORT=`、`!!SERVER=` 等其他匹配器 | subconverter 的其余匹配器 | 按 fork 源码的语义实现；无法等价实现的报错并说明 | 无法表达为 `filter` 的报错并说明 |

`!!GROUPID=` 的范围语法沿用 fork 中 `matchRange` 的实现（2026-11-09 读到的源码）：逗号分隔多项，任一项命中即可；`n` 精确匹配，`a-b` 闭区间，前缀 `!` 取反，另有 `n-`、`n+` 两种开区间写法。开区间的方向与 `!!INSERT=` 的取值（fork 中插入节点的编号取负数后比较）都**待环境核验**：实现前对照 fork 的 `subexport.cpp` 与编号赋值处，用测试锁定，不凭字面猜测。

- 正则使用 `dlclark/regexp2`（与 mihomo 一致），支持前瞻与后顾，例如 `(^(?=.*(日|JP))(?!.*尼日利亚).*$)`；Go 标准库 RE2 不支持，**不得**使用。
- 条目顺序即组内顺序；多个正则命中同一节点时只出现一次，位置取首次命中。
- 正则未命中任何节点的组：按目录中 `empty-fallback` 等字段的语义处理（自 mihomo v1.19.27）；目标内核不支持时，组内写入 `COMPATIBLE` 并给出警告（**待环境核验**）。

其他键：`url`、`interval`、`tolerance`、`timeout`、`lazy`、`strategy` 等映射到 mihomo 组的同名字段；groups.toml 中出现、但 subconverter 不认识的键，只要是目录中合法的 mihomo 组字段（如 `icon`、`hidden`、`default-selected`），就**原样透传**；目录中也没有的键，透传并给出「目录外字段」警告（原则 1）。

### 6.3 rulesets.toml

结构为 `[[rulesets]]` 数组，键：`group`、`ruleset`、`type`、`interval`。

| 输入 | 输出 |
| --- | --- |
| `type = "clash-domain"` | rule-provider `behavior: domain`；`RULE-SET,<名>,<组>` |
| `type = "clash-ipcidr"` | `behavior: ipcidr`；`RULE-SET,<名>,<组>,no-resolve`（与 fork 行为一致） |
| `type = "clash-classic"` | `behavior: classical` |
| `ruleset` 以 `.mrs` 结尾 | 另写 `format: mrs`；classical 不能是 MRS，遇到时报错 |
| 其他扩展名 | `format: yaml`；`.txt`/`.list` 写 `format: text` |
| `ruleset = "[]GEOIP,CN"` 等内联 | 原样写入 `GEOIP,CN,<组>`；参数（如 `no-resolve`）按原样保留 |
| `ruleset = "[]MATCH"` | `MATCH,<组>` |
| `type = "surge-ruleset"` | `behavior: classical`、`format: text`（**待环境核验**：Surge 列表与 mihomo classical text 的差异，列入限制） |
| `quantumultx` 等 | 报错：不支持 |

- rule-provider 名从文件名推导，全局唯一（大小写不敏感）；冲突时追加 `_2`，与 fork 一致。
- provider 的 `path` 默认 `./ruleset/<名>.<扩展名>`，`interval` 取自 toml，缺省为 86400。
- 规则顺序即 toml 顺序；原始编辑可在其前后插入规则（见[第 9 节](02-engine-and-catalog.md#9-两层编辑与合并语义)）。
- 内联规则含三个及以上逗号时，fork 的组名插入位置有特殊处理；本项目按 mihomo 规则语法把组名放在规则参数（`no-resolve`、`src`）之前，并以测试锁定（**待环境核验**：对照 fork 源码）。

### 6.4 现代节点选择器

`!!GROUPID=0!!(港|HK)` 把「哪个订阅」「什么地区」揉进一个字符串，难写、难读、难复用。本项目在 `[[custom_groups]]` 中增加一个可选的 `select` 表，按节点的**属性**选择节点；旧写法完全兼容，两者可以在同一个组里并存。

```toml
[[custom_groups]]
name = "🇭🇰 香港"
type = "url-test"
url = "https://www.gstatic.com/generate_204"
interval = 300
rule = ["[]DIRECT", "@select"]      # 旧写法照常可用；"@select" 标出选择结果放在哪里

[custom_groups.select]
region   = ["HK"]                   # 落地地区（第 5.8 节，含手动指定）
source   = ["机场 A", "#1"]          # 源名称，或 #编号（即 GROUPID）
protocol = ["hysteria2", "vless"]   # 协议类型
tags     = ["IEPL"]                 # 用户给源或节点打的标签
name     = "(?i)premium"            # 名称正则（regexp2）
rate     = { max = 1.5 }            # 倍率上限：从名称解析 x2、2倍、0.5× 等，可手动指定
exclude  = { region = ["CN"], name = "(?i)游戏|测试" }
sort     = "source"                 # source（默认，按源顺序）| name | rate
```

语义：

- 同一个键内的多个值是「或」，不同键之间是「且」；`exclude` 中任一条件命中即排除。
- 选择结果的位置：`rule` 中写 `"@select"` 的位置；没有写时追加在 `rule` 条目之后。
- 同一节点在组内只出现一次，位置取首次出现处。
- **标签**：在界面上给源或单个节点打标签（例如「家宽」「IEPL」「低倍率」），以节点指纹保存，与地区的手动指定一样不怕改名。
- **倍率**：内置识别名称中的常见写法，识别不到的视为 1；可手动指定。
- `provider` 模式下，`source` 改写为 `use`，`name`/`exclude.name` 改写为 `filter`/`exclude-filter`，`protocol` 改写为 `exclude-type`；`region`、`tags`、`rate` 依赖服务器端才知道的信息，无法等价表达时**必须**给出警告，并说明哪些条件在客户端不生效。
- groups.toml 中写了 `select` 时，subconverter 会忽略这张表（**待环境核验**），因此仍能被 subconverter 读取，只是不会生效；界面导出 groups.toml 时提示这一点。

界面上的**策略组编辑器**把这些条件做成可点选的标签，右侧实时显示命中的节点；保存时写回 TOML 原文，只改动用户改过的组。

### 6.5 第三方规则

除了 domainrouter_v2，还支持社区里使用广泛的规则。它们大致分两类：

**一类是成套的 subconverter 外部配置**，以 ACL4SSR 为代表。这类配置是一份 ini，`[custom]` 节中：

```ini
ruleset=🎯 全球直连,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/LocalAreaNetwork.list
ruleset=🎯 全球直连,[]GEOIP,CN
ruleset=🐟 漏网之鱼,[]FINAL
custom_proxy_group=🚀 节点选择`select`[]♻️ 自动选择`[]DIRECT`.*
custom_proxy_group=♻️ 自动选择`url-test`.*`http://www.gstatic.com/generate_204`300,,50
```

- `ruleset=组,来源`：与 rulesets.toml 的一项等价；`[]FINAL` 等同 `[]MATCH`。
- ``custom_proxy_group=名称`类型`成员…``：反引号分隔；成员语法与 groups.toml 的 `rule` 相同；url-test 等类型的最后两段是测速地址与 `间隔,超时,容差`（字段含义**待环境核验**：对照 fork 源码）。
- `enable_rule_generator`、`overwrite_original_rules` 按其含义处理；`rename`、`emoji`、`include_remarks`、`exclude_remarks` 映射到节点整理（第 5.5 节）；其余键不支持的，列出并说明。
- 界面提供「转换为 groups.toml / rulesets.toml」，方便用户从 ACL4SSR 迁移后自行维护。

**另一类是规则文件仓库**，只提供规则列表，不提供策略组。预设为它们附带一份建议的策略组模板，用户可以在此基础上修改。

**规则文件格式**：`.list` 是一行一条的经典规则（`DOMAIN-SUFFIX,example.com`），写成 rule-provider `behavior: classical`、`format: text`。列表中出现 mihomo 不支持的规则类型（例如 Surge 的 `USER-AGENT`、`URL-REGEX`）时，mihomo 是跳过该行还是让整个 provider 加载失败，**待环境核验**；如果是后者，就提供**可选**的「托管清洗」：由服务器拉取列表、去掉不支持的行，再以受令牌保护的地址 `/r/<令牌>/<名称>` 提供给客户端，并在诊断中列出去掉的行数。规则的深度去重与优化不在本项目范围内（domainrouter_v2 负责）。

**预设**（数据文件 `catalog/rulesources/`，每个预设注明官方仓库、许可、最近核对日期；具体的地址、变体与使用人数在实施时核对，**待环境核验**）：

| 预设 | 类型 | 说明 |
| --- | --- | --- |
| domainrouter_v2 | toml 一对 | 维护者自己的规则，**默认推荐** |
| ACL4SSR | ini | 多个变体（如 Online、Online_Full、Online_Mini），用户选择其一 |
| Loyalsoldier/clash-rules | 规则文件 | 官方 README 给出了 rule-providers 与 rules 的建议写法，预设按此生成 |
| MetaCubeX/meta-rules-dat | 规则文件 | mihomo 官方组织的 geosite/geoip 数据与 MRS 规则集 |
| blackmatrix7/ios_rule_script | 规则文件 | 按应用划分的大量规则列表 |
| SukkaW 规则集 | 规则文件 | 提供 Clash/mihomo 版本 |

本项目只引用这些规则的地址，不复制、不再分发其内容；「托管清洗」生成的副本只供本服务的用户使用，并在界面上标明来源与许可。

### 6.6 与 subconverter 的差异测试

**必须**用维护者的真实两份 toml（domainrouter_v2 release）以及一个 ACL4SSR 变体，与 angwz/subconverter 的产物逐项对比 `rules`、`proxy-groups`、`rule-providers`；任何差异都要在 `docs/design/subconverter-parity.md` 中逐条解释：是有意的改进（附依据）还是待修复的缺陷。
