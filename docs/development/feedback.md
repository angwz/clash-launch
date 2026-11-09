# 维护者意见

维护者对项目提出的每条意见及其处理结果，按时间排列，供维护者回顾、供后续接手的代理了解哪些方向已经讨论过。

- 每条意见用一句话概括其要求与动机，不摘录原话。
- 新意见追加在末尾，编号连续；处理有进展时只更新「状态」与「落点」。
- 立项前确认的设计决定见[规范第 2 节](../spec/01-foundations-and-input.md#2-已确认的决定)；已形成正式决定的，细节以 [decisions/](../decisions/) 为准。

**状态** ✅ 已完成 · 🔄 进行中 · 🕓 已规划 · ➖ 不采纳

## 开始实施（2026-11-09）

| # | 意见 | 状态 | 落点 |
| ---: | --- | :---: | --- |
| 1 | 按 M1→M6 编码与验证；M7 的远端操作（建仓库、推送、发布镜像、配置密钥）暂不执行 | 🔄 | [progress](progress.md) |
| 2 | 实施可能在另一台电脑上进行：不假设绝对路径，先检查工具、按固定版本获取只读参考资料 | ✅ | [setup](setup.md) |
| 3 | 以 domainrouter_v2 的文档体系、groups.toml 校验与工程约定为范本 | 🔄 | [CLAUDE.md](../../CLAUDE.md)、本目录 |
| 4 | YAML 格式绝对正确并每次自检，字段顺序遵循官方示例，渲染结果确定 | ✅ | [rendering](../design/rendering.md)、[catalog](../design/catalog.md) |
| 5 | 两层编辑（表单 → 原始编辑，即后备隐藏能源）与合并标签语义严格落实 | ✅ | [merge](../design/merge.md) |
| 6 | 有序源与订阅编号、地区识别（线路词剔除、手动覆盖、装饰 emoji）、现代选择器与外部配置 ini | ✅ | [nodes](../design/nodes.md)、[rules](../design/rules.md) |
| 7 | 不静默丢弃：所有跳过、删除与无法转换的内容都进入诊断 | 🔄 | `internal/diag` |
| 8 | 凭据加密、SSRF 防护、出站隐私与可迁移的存储层 | ✅ | [network](../design/network.md)、[0002](../decisions/0002-storage-and-service.md) |
| 9 | 待环境核验的条目先查官方文档或源码再实现，结论写进决定 | 🔄 | [0001](../decisions/0001-deviations-from-spec-1.0.md#核验结论待环境核验项) |
| 10 | 运行时不调用任何第三方服务；出站连接只经过配置的出口代理（默认 WARP） | 🔄 | 拉取层：[network](../design/network.md)；WARP 边车已编写、未运行验证 |
| 11 | 每个里程碑闭环：实现、测试、整理、同步文档与进度、提交；未运行的验证不写成通过 | 🔄 | [progress](progress.md) |
