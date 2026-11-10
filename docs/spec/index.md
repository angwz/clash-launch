# Clash, Launch! 实施规范 1.0 · 交接入口

版本：1.0 · 日期：2026-11-09 · 适用环境：已创建、尚无实现的 `clash-launch` 工作区

本交接包只包含文档，尚未实现任何代码。它定义了 Clash, Launch! 的目标、语义、架构、安全要求、多语言流程与验收条件，供 Claude Code 在新会话中按里程碑实施。四份分册共同生效，本入口不能代替正文。

## 一句话

**一个由字段目录驱动的 YAML 生成器。** 第一个格式包是 Mihomo 系 Clash 配置：在 Web 里填入节点、订阅与规则源（domainrouter_v2 的 `rulesets.toml`、`groups.toml`，或 ACL4SSR 这类外部配置），选择目标客户端，按官方文档逐字段设置，得到一条长期有效的订阅链接；官方新增的字段在目录跟进之前，用原始编辑模式直接写入。

```text
节点 · 订阅 · 批量节点 ─┐
rulesets.toml ──────────┼─▶ 解析 ─▶ 展开 ─▶ 表单层 ─▶ 原始编辑层 ─▶ 目标适配 ─▶ 格式自检 ─▶ mihomo -t ─▶ /s/<token>
groups.toml ────────────┘                      ▲
                                    字段目录 catalog/mihomo（随内核版本）
```

## 规范文件与阅读顺序

| 顺序 | 文件 | 内容 |
| --- | --- | --- |
| 1 | [01 · 基础与输入](01-foundations-and-input.md) | 第 0～6 节：给实施者的说明（含新电脑与参考资料）、目标与范围、已确认的决定、命名、核心模型、节点输入、规则输入 |
| 2 | [02 · 引擎与字段目录](02-engine-and-catalog.md) | 第 7～11 节：生成管线、字段目录、两层编辑与合并语义、目标适配、渲染与检查 |
| 3 | [03 · 服务与界面](03-service-and-web.md) | 第 12～16 节：HTTP API、订阅输出、多用户与安全、多语言、Web 界面 |
| 4 | [04 · 交付与交接](04-delivery-and-handoff.md) | 第 17～22 节：技术栈、部署、工程质量与开源资料、里程碑、依据与待核验项、启动提示语 |
| 启动 | [start-prompt.md](start-prompt.md) | 交给实施会话的短提示语；只需粘贴这一份 |

四份分册都低于 50,000 字符。用文件读取工具逐份读完；单次读取被截断时分段继续，不能把截断当作读完。

快速定位：[核心模型](01-foundations-and-input.md#4-核心模型) · [节点输入](01-foundations-and-input.md#5-节点输入) · [规则输入](01-foundations-and-input.md#6-规则输入) · [管线](02-engine-and-catalog.md#7-生成管线) · [合并语义](02-engine-and-catalog.md#9-两层编辑与合并语义) · [安全](03-service-and-web.md#14-多用户与安全) · [多语言](03-service-and-web.md#15-多语言) · [里程碑](04-delivery-and-handoff.md#20-里程碑与验收)

## 本版确定的事

- **YAML 生成器，而非订阅转换器的翻版。** 引擎只认识「字段目录 + 生成器」，不认识 Clash；Mihomo 是第一个格式包。subconverter 的 `rulesets.toml`、`groups.toml` 与外部配置 ini 的语义被兼容，其余 subconverter 功能不复刻。
- **字段尽可能全。** 选定客户端后展示官方文档中的全部字段（含实验性字段），每个字段附详细说明、默认值、示例与重要程度。
- **两层编辑。** 表单（默认）→ 原始编辑。原始编辑就是「后备隐藏能源」：界面上没有显眼入口，只能从分节 `⋯` 菜单或命令面板进入，可以写入任意字段，包括目录里还没有的新字段。
- **格式第一。** 输出的 YAML 格式绝对正确、缩进规整、字段顺序遵循官方示例；每次渲染都做格式自检，再用 mihomo 官方的 `-t` 检查。
- **节点更聪明。** 地区自动识别（不会把「广港 新加坡」误判为香港），可手动指定国旗；除兼容 `!!GROUPID=` 外，另有按地区、源、协议、标签、倍率组合的现代选择器；支持 ACL4SSR 等主流第三方规则。
- **订阅有序、链接永久。** 订阅按顺序编号 0、1、2……，兼容 groups.toml 中的 `!!GROUPID=`；单条节点不编号但保留位置。订阅链接随时可查看、复制、重置。
- **Go 后端 + Svelte 前端。** 后端直接复用 mihomo 源码中的分享链接解析与节点校验；前端沿用 domainrouter_v2 的 Svelte 5 + Vite。产物是嵌入前端的单个静态二进制，以 Docker 镜像发布。
- **多用户、面向公网、注册从简。** 用户名 + 密码 + 安全问题；可选通行密钥、两步验证、个人 SMTP 异常登录提醒。默认 SQLite，存储层可迁移到 PostgreSQL。
- **运行时零外部依赖。** 1.0 提供简体中文与英文，结构上支持 RTL；更多语言以后由构建时的机器翻译补充。运行时不调用任何翻译 API、CDN 或字体服务。
- **编码与远端操作分开。** 实施者按 M1→M6 编码与验证；创建 GitHub 仓库、推送、发布镜像、配置密钥均属于远端操作，只在维护者明确要求时进行。

## 与 domainrouter_v2 的关系

| | domainrouter_v2 | Clash, Launch! |
| --- | --- | --- |
| 职责 | 规则：编译、去重、优化 rule-provider | 配置：把节点、规则、参数组装成完整 YAML |
| 输出 | rule-provider、`rulesets.toml`、`groups.toml` | 客户端直接导入的订阅链接 |
| 运行 | GitHub Actions 每日构建 | 自托管 Docker 服务 |
| 仓库 | [angwz/domainrouter_v2](https://github.com/angwz/domainrouter_v2) | `angwz/clash-launch`（尚未创建） |

Clash, Launch! 消费 domainrouter_v2 的发布物，但不依赖它：任何符合 subconverter 语义的两份 toml 或外部配置 ini 都能使用。工程方式（规范交接、活文档、决定记录、意见记录、进度记录）照搬 domainrouter_v2，见第 19 节。

## 版本与状态

1.0 是首个版本。所有验收条件都是待实施的要求；文中引用的外部事实（mihomo 版本、客户端列表、第三方语义）注明了查证日期，实施时须重新核对，标为「待环境核验」的条目汇总在[第 21 节](04-delivery-and-handoff.md#21-依据与待核验项)。
