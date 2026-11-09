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
| `catalog/`（数据） | 字段目录 `mihomo/`、客户端预设 `mihomo/presets/`；以 `embed` 打包 |
| `internal/catalog` | 目录加载、片段展开与校验、按位置查找字段与键序、导出（表单元数据、JSON Schema、i18n 源消息） |
| `internal/diag` | 诊断类型与代码登记（每个代码有中英文消息；发布后不改名） |
| `internal/yamlx` | 带资源上限的安全解析（体积、节点数、别名展开、重复键），节点构造与查找 |
| `internal/profile` | 方案文档 `clash-launch/profile: v1` 的解析、校验与规范化导出 |
| `internal/layer` | 表单层校验（只接受目录字段）与原始编辑合并（合并标签），见 [merge](merge.md) |
| `internal/target` | 目标预设加载；版本门槛、目标不支持的字段、接管字段、弃用提示 |
| `internal/render` | 按目录排序、受限发射器、文件头、格式自检，见 [rendering](rendering.md) |
| `internal/kernel` | mihomo 适配层：分享链接解析与节点校验（Go 库）、`mihomo -t`（可执行文件） |
| `internal/engine` | 管线编排、缓存键 |
| `internal/version` | 版本号与项目地址 |
| `cmd/clash-launch` | 命令行：`render`、`catalog export`、`version` |
| `tools/catalog-extract` | 从 mihomo 源码与示例配置提取字段全集、键顺序与版本差异 |
| `tools/toolsync` | 按 `tools.lock.json` 下载并校验 mihomo 与地理数据 |

依赖方向：`cmd → engine → (catalog, layer, target, render, profile) → (diag, yamlx)`；`kernel` 由外壳调用。`engine` 不依赖 `store`、`source`、`api`、`net/http`，由 `TestDependencyDirection` 用 `go list -deps` 锁定。

## 验证

| 层级 | 命令 | 做什么 |
| --- | --- | --- |
| 单元与表驱动 | `go test ./...` | 合并语义逐条规则、表单校验、目录结构与文档锚点、键顺序与官方示例一致 |
| golden | 同上；`-update` 更新 | 渲染器（`testdata/render/`）与完整方案（`testdata/profiles/`）的输出逐字节锁定 |
| fuzz | `go test -fuzz FuzzRender ./internal/render/`、`FuzzMerge ./internal/layer/` | 任意输入不 panic；渲染输出必过格式自检；合并结果确定、不修改输入 |
| 格式自检 | 每次渲染 | 重新解析输出并与渲染前的树逐节点比较；检查 YAML 1.1 歧义值、重复键、制表符缩进、CR、结尾换行 |
| 内核 | `go test -tags kernel ./internal/kernel/` | mihomo v1.19.33 `-t` 检查全部 golden 输出；离线（见 [0001](../decisions/0001-deviations-from-spec-1.0.md#核验结论待环境核验项)） |
