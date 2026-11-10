# 架构与验证

## 管线

```text
          联网与持久化（外壳）                        纯函数核心（engine，不联网、不读写数据库、不读时间）
 ┌──────────────────────────────┐   ┌──────────────────────────────────────────────────────────────────────┐
 source：拉取 ─▶ 快照（不可变）──────▶ 解析节点 ─▶ 整理 ─▶ 展开策略组 ─▶ 生成规则 ─▶ 表单层 ─▶ 原始编辑层 ─▶ 目标适配 ─▶ 渲染 ─▶ 格式自检
 store：方案文档、版本、链接       │     kernel·node    node      ruleset       ruleset     layer      layer        target     render    render
 └──────────────────────────────┘   └──────────────────────────────────────────────────────────────────────┘
                                                                                                              │
                                                                         kernel：mihomo -t（调用方在渲染之后执行）┘
```

- **快照**：订阅与规则源在后台按间隔拉取，成功后写入不可变快照（按内容哈希）；失败时沿用上一次成功的快照。渲染只读快照，从不在请求订阅时同步拉取。
- **核心**：从「解析节点」到「格式自检」都是值进值出。文件头时间取自输入快照中最新的时间戳；任何 map 都不按迭代顺序输出。相同输入两次渲染逐字节一致（`TestProfilesGolden` 渲染两次比较）。
- **诊断不中断**：除致命错误（原始编辑的 YAML 语法错误、格式自检失败、内核检查失败）外，每一步都把问题写进诊断并继续，一次运行看到全部问题。致命错误时订阅端点继续提供上一次成功的结果。

## 包

| 包 | 职责 |
| --- | --- |
| `catalog/`（数据） | 字段目录 `mihomo/`、客户端预设 `mihomo/presets/`、地区与装饰 emoji `regions/`、规则源预设 `rulesources/`；以 `embed` 打包 |
| `internal/catalog` | 目录加载、片段展开与校验、按位置查找字段与键序、导出（表单元数据、JSON Schema、i18n 源消息） |
| `internal/diag` | 诊断类型与代码登记（每个代码有中英文消息；发布后不改名） |
| `internal/yamlx` | 带资源上限的安全解析（体积、节点数、别名展开、重复键），节点构造与查找 |
| `internal/profile` | 方案文档 `clash-launch/profile: v1` 的解析、校验与规范化导出 |
| `internal/node` | 内容识别、分享链接与 YAML 的解析与校验、sing-box 映射、整理、地区识别与装饰 emoji、倍率与标签，见 [nodes](nodes.md) |
| `internal/ruleset` | groups.toml、rulesets.toml、外部配置 ini 的解析与校验；匹配器、现代选择器、策略组展开、规则与 rule-provider、预设，见 [rules](rules.md) |
| `internal/source` | 唯一联网的包：拉取、SSRF 防护、出站代理、快照，见 [network](network.md) |
| `internal/workspace` | 命令行的工作区文件（源与规则源的快照） |
| `internal/layer` | 表单层校验（只接受目录字段）与原始编辑合并（合并标签），见 [merge](merge.md) |
| `internal/target` | 目标预设加载；版本门槛、目标不支持的字段、弃用提示 |
| `internal/render` | 按目录排序、受限发射器、文件头、格式自检，见 [rendering](rendering.md) |
| `internal/kernel` | mihomo 适配层：分享链接解析与节点校验（Go 库）、`mihomo -t`（可执行文件） |
| `internal/engine` | 生成阶段（`Build`：有序源、订阅编号、provider 模式、策略组与规则）与管线编排（`Render`）、缓存键 |
| `internal/store` | 唯一持久化的包：仓储接口、SQLite/PostgreSQL 通用实现、迁移、加密、备份与 `db copy`，见 [0002](../decisions/0002-storage-and-service.md) |
| `internal/auth` | argon2id 口令哈希、口令策略、随机令牌与哈希、常数时间比较 |
| `internal/service` | 外壳编排：从存储组装引擎输入、渲染与内核检查、保存最近一次成功的结果、拉取与快照、后台调度、托管 provider |
| `internal/api` | HTTP：管理接口、订阅端点、会话与 CSRF、安全响应头、限流；契约见 [`api/openapi.yaml`](../../api/openapi.yaml) |
| `web` | 嵌入前端构建产物 |
| `internal/version` | 版本号与项目地址 |
| `cmd/clash-launch` | 命令行：`serve`、`render`、`catalog export`、`admin create`、`keys rotate`、`backup`、`db migrate`/`copy`、`healthcheck` |
| `tools/catalog-extract` | 从 mihomo 源码与示例配置提取字段全集、键顺序与版本差异 |
| `tools/toolsync` | 按 `tools.lock.json` 下载并校验 mihomo 与地理数据 |

依赖方向：`cmd → (workspace, engine) → (catalog, layer, node, ruleset, target, render, profile) → (kernel, diag, yamlx)`。`engine` 不依赖 `store`、`source`、`api`、`auth`，由 `TestDependencyDirection` 用 `go list -deps` 锁定；`kernel` 中调用可执行文件的 `Checker` 由外壳使用，分享链接解析与节点校验是纯函数。

## 验证

| 层级 | 命令 | 做什么 |
| --- | --- | --- |
| 单元与表驱动 | `go test ./...` | 合并语义逐条规则、表单校验、目录结构与文档锚点、键顺序与官方示例一致 |
| golden | 同上；`-update` 更新 | 渲染器（`testdata/render/`）、完整方案（`testdata/profiles/`）与端到端工作区（`testdata/workspace/`：有序源 + domainrouter_v2 真实规则源 / 外部配置 ini）的输出逐字节锁定 |
| 差异 | `TestParityRules`、`TestParityGroups` | 按 fork 源码实现的参考模型与本项目逐条比较，见 [subconverter-parity](subconverter-parity.md) |
| fuzz | `FuzzRender`、`FuzzMerge`、`FuzzParse`（node）、`FuzzMatchRange`（ruleset） | 任意输入不 panic；渲染输出必过格式自检；合并结果确定、不修改输入；凭据不进入错误信息 |
| 格式自检 | 每次渲染 | 重新解析输出并与渲染前的树逐节点比较；检查 YAML 1.1 歧义值、重复键、制表符缩进、CR、结尾换行 |
| 内核 | `go test -tags kernel ./internal/kernel/ ./internal/api/` | mihomo v1.19.33 `-t` 检查全部 golden 输出（离线，见 [0001](../decisions/0001-deviations-from-spec-1.0.md#核验结论待环境核验项)）；端到端：mihomo 实际加载订阅链接返回的配置并开始监听 |
| 契约与 API | `TestContract`、`TestEndToEnd` 等 | 路由表与 OpenAPI 逐条一致；登录、CSRF、If-Match、订阅、304、重置、越权 404、限流、503、SSRF |
