# 进度

> 项目的唯一状态记录。区分「已实现」与「已验证」，过期内容直接覆盖，不保留流水账。

## 现在

- **阶段**：M1（引擎）完成；下一步 M2（输入）。
- **远端**：尚未创建仓库；按维护者要求，远端操作（M7）暂不执行。

## 里程碑

| 阶段 | 状态 | 验收结果 |
| --- | :---: | --- |
| M1 引擎 | ✅ | 见下表 |
| M2 输入 | 🕓 | — |
| M3 服务 | 🕓 | — |
| M4 Web | 🕓 | — |
| M5 多用户与安全 | 🕓 | — |
| M6 目标与目录 | 🕓 | — |
| M7 发布 | ➖ | 由维护者决定何时进行 |

## M1 验收

| 验收条件 | 结果 | 证据 |
| --- | :---: | --- |
| 合并语义表驱动测试与 fuzz | ✅ | `TestMerge` 27 个用例；`FuzzMerge` 60 秒无失败 |
| golden 测试 | ✅ | `testdata/render/basic`、`testdata/profiles/m1-layers` |
| 格式自检在全部测试数据上通过 | ✅ | 每次渲染执行；`FuzzRender` 约 88 万次执行无失败（修复 5 类边界后） |
| 渲染结果通过 `mihomo -t` | ✅ | `go test -tags kernel ./internal/kernel/`：两份 golden 均通过 mihomo v1.19.33（Windows amd64） |
| 两次渲染逐字节一致 | ✅ | `TestProfilesGolden` 每份方案渲染两次比较 |
| `catalog-extract` 能对两个版本输出字段差异 | ✅ | v1.19.32 → v1.19.33：新增 `tun.dns-mode`、`listeners.tun.dns-mode`；单元测试覆盖 order 与 diff |
| CLI `clash-launch render <方案文件>` | ✅ | `-check` 端到端：示例方案通过 `mihomo -t` |

## 能力一览

| 能力 | 位置 | 已实现 | 已验证 |
| --- | --- | :---: | --- |
| 字段目录：加载、片段、校验、键序、导出 | `catalog/mihomo/`、`internal/catalog` | ✓ | 单元测试；文档锚点对照 wiki 快照；顶层、dns、tun、sniffer、17 种协议、rule-provider 的顺序对照官方示例 |
| 安全 YAML 解析 | `internal/yamlx` | ✓ | 体积、节点数、别名展开、重复键经合并与渲染测试间接覆盖 |
| 表单层校验与原始编辑合并 | `internal/layer` | ✓ | 表驱动、fuzz |
| 目标适配（版本门槛、不支持、接管、弃用） | `internal/target` | ✓ | 端到端（`global-client-fingerprint` 移除警告）；预设只有 `mihomo` |
| 渲染与格式自检 | `internal/render` | ✓ | 刁钻标量表、golden、fuzz |
| 内核检查（离线 `mihomo -t`） | `internal/kernel` | ✓ | mihomo v1.19.33 实测：合法配置通过，坏节点失败并定位到 `proxies[0]` |
| 方案文档 | `internal/profile` | ✓ | 经引擎测试覆盖 |
| 管线编排、缓存键 | `internal/engine` | ✓ | golden、确定性、依赖方向测试 |
| 命令行 | `cmd/clash-launch` | ✓ | 手动运行 `render -check`、`catalog export` |
| 工具 | `tools/catalog-extract`、`tools/toolsync` | ✓ | 单元测试；toolsync 实际下载并校验 SHA-256 |

## 最近一次验证

2026-11-09 · Windows 10 x64 · Go 1.27.3

```text
gofmt -l .                         → 无输出
go vet ./...                       → 通过
staticcheck ./...                  → 通过（2026.2.2）
golangci-lint run ./...            → 0 issues（v2.15.0）
go test ./...                      → catalog、engine、layer、render、catalog-extract 通过
go test -tags kernel ./internal/kernel/   → 通过（mihomo v1.19.33）
go test -fuzz FuzzRender（180 s）  → 878,011 次执行无失败
go test -fuzz FuzzMerge（60 s）    → 无失败
```

未验证：Linux 上的测试（CI 尚未创建）；Docker（本机守护进程未运行，M3 处理）。

## 已知限制

- 目录：`listeners` 的逐类型字段，以及 xhttp 细项、shadow-tls/restls/jls、realm、AmneziaWG 等节点子选项暂以 `any` 透传（M6 补齐）；详细说明与示例覆盖率在 M6 前补齐。
- 目标预设只有 `mihomo`；其余预设在 M6 按客户端官方文档逐项核对后加入。
- `geoip.dat` 的哈希是首次下载时记录的，没有上游公布的校验值可对照。

## 下一步

1. M2：内容识别、分享链接（`ConvertsV2Ray` 逐行调用 + `ParseProxy` 校验）、整理、有序源与订阅编号、地区识别、groups/rulesets 展开与匹配器、现代选择器、外部配置 ini、拉取与快照、SSRF 防护与出口代理。
2. 与 subconverter 的差异逐条写入 `docs/design/subconverter-parity.md`。
