# 文档地图

按你想做的事找到对应的文档。每份文档只负责一个主题，同一事实只在一处详述，其余位置用链接指过去。

## 使用

| 文档 | 读完你会知道 |
| --- | --- |
| [guide/cli.md](guide/cli.md) | 怎样用命令行离线渲染一份方案文档，诊断怎么读 |

部署、反向代理、节点源、规则源、原始编辑与覆写标签、目标预设、多语言贡献的指南随 M2–M6 补齐。

## 设计

| 文档 | 读完你会知道 |
| --- | --- |
| [design/architecture.md](design/architecture.md) | 管线、包职责与依赖方向、验证层级 |
| [design/catalog.md](design/catalog.md) | 字段目录的结构、顺序与文档链接的来源、提取与跟进、覆盖率 |
| [design/merge.md](design/merge.md) | 表单层校验与原始编辑的合并标签，逐条对应测试 |
| [design/rendering.md](design/rendering.md) | 输出样式、引号规则、键顺序与格式自检 |
| [design/nodes.md](design/nodes.md) | 内容识别、分享链接与校验、整理步骤、地区识别与装饰 emoji |
| [design/rules.md](design/rules.md) | 有序源与订阅编号、groups.toml 与匹配器、现代选择器、rulesets、外部配置 ini、预设 |
| [design/network.md](design/network.md) | 拉取、SSRF 防护、出站代理与快照 |
| [design/subconverter-parity.md](design/subconverter-parity.md) | 与 subconverter 的逐条差异及其性质 |
| [design/singbox-mapping.md](design/singbox-mapping.md) | sing-box 出站到 mihomo 节点的字段映射 |
| [decisions/](decisions/) | 影响架构或兼容性的决定，按编号递增；与其他文档冲突时以决定为准 |

## 开发

| 文档 | 读完你会知道 |
| --- | --- |
| [development/progress.md](development/progress.md) | 当前进度、最近一次验证、下一步 |
| [development/feedback.md](development/feedback.md) | 维护者提出的每条意见及其处理结果 |
| [development/setup.md](development/setup.md) | 工具链、内核与地理数据、只读参考资料 |

## 规范

| 文档 | 内容 |
| --- | --- |
| [spec/](spec/index.md) | 实施规范 1.0：目标、语义、架构、安全、多语言与验收条件。实施完成后原样归档到 `history/spec-1.0/` |

## 给 AI 代理

先读仓库根目录的 [CLAUDE.md](../CLAUDE.md)，再按其中的顺序阅读。约定：

- **状态**以 `development/progress.md` 为准，**语义**以 `design/` 为准，**取舍**以 `decisions/` 为准。
- 维护者的新意见追加到 `development/feedback.md`，只写中性的总结，不引用原话。
- 文档中的命令、路径、配置键必须与代码一致；性能数字只写实测值。
