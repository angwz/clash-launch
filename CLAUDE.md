# Clash, Launch! · 代理工作约定

字段目录驱动的 YAML 生成器。第一个格式包是 Mihomo 系 Clash 配置：节点、订阅与规则源（subconverter 语义的 `rulesets.toml`、`groups.toml`、外部配置 ini）经表单层与原始编辑层（后备隐藏能源）合并，按目标客户端适配，渲染为格式正确、顺序遵循官方示例的 YAML，经 `mihomo -t` 检查后由长期有效的订阅链接提供。

## 先读什么

1. [docs/development/progress.md](docs/development/progress.md) — 当前进度、最近一次验证、下一步。唯一的状态记录。
2. [docs/development/feedback.md](docs/development/feedback.md) — 维护者的全部意见。**每收到一条新意见就追加一行**；只写中性总结，不引用原话。
3. [docs/decisions/](docs/decisions/) — 已采纳的决定，与其他文档冲突时以决定为准。
4. [docs/spec/](docs/spec/index.md) — 实施规范 1.0（实施完成后归档到 `docs/history/spec-1.0/`，只读）。
5. [docs/design/architecture.md](docs/design/architecture.md) — 管线、包职责与依赖方向、验证层级。

其余文档从 [docs/README.md](docs/README.md) 进入。进度记录可能过期：恢复工作时以实际代码、`git log`、`git status` 与重新运行的检查为准。

## 硬性边界

- 远端操作（创建仓库、推送、发布镜像、创建 Release、配置 Actions secret、接入 Weblate）只在维护者明确要求时进行；仓库元数据先经维护者确认。
- 参考资料只读，放在 `.cache/ref/`：domainrouter_v2@2c812f9、mihomo@v1.19.33、angwz/subconverter@a33d49e、ACL4SSR（记录提交）。不运行其中的脚本，不修改，不推送。
- 只按官方文档或源码实现语义；查不到就列为限制，不猜。没有正式规范的输入格式不支持。
- 用户写下的内容是有意为之：节点名、组名、正则、手写规则、原始编辑中的值按原样保留，看似不规范时只报告。
- 不静默丢弃：被跳过、改写、删除或无法验证的内容都进入诊断（`internal/diag`）。
- 运行时不调用任何第三方服务（翻译、CDN、字体、验证码）；出站连接只有用户配置的订阅与规则源，服务器直连并经 SSRF 防护（[0003](docs/decisions/0003-direct-fetch-and-invites.md)）。
- 测试数据只用 `example.com`、`192.0.2.0/24`、`2001:db8::/32` 与明显虚构的凭据。

## 呈现

- 维护者对生成文件、文档与界面的审美要求很高。改动输出格式（文件头、排序、引号、空行）或视觉风格时，先在真实数据上给出样例或截图，确认后再改 golden。
- 文档同时面向人与代理：结构清晰、措辞克制、信息密度高；同一事实只在一处详述，其他地方链接。可以自由重组文档，改动后保证链接有效。

## 常用命令

本机工具链在 `.cache/toolchain/`（见 [setup](docs/development/setup.md)）。

```sh
go run ./tools/toolsync                       # 按 tools.lock.json 下载并校验 mihomo 与地理数据
go test ./...                                 # 单元、golden、属性测试（离线）
go test -tags kernel ./internal/kernel/ ./internal/api/   # 真实 mihomo：-t 检查全部 golden；实际加载订阅链接
go test -run X -fuzz FuzzRender ./internal/render/   # fuzz（另有 FuzzMerge 等）
go test ./internal/render/ ./internal/engine/ -update  # 更新 golden（必须人工审阅差异）
gofmt -l . && go vet ./... && staticcheck ./... && golangci-lint run
go run ./cmd/clash-launch render -check -mihomo .cache/tools/mihomo.exe -geodata .cache/tools/geodata testdata/profiles/m1-layers.yaml
CLASH_LAUNCH_DATA_DIR=.cache/servedata CLASH_LAUNCH_LISTEN=127.0.0.1:18080 go run ./cmd/clash-launch serve   # 本机运行服务
go run ./tools/catalog-extract scan -src .cache/ref/mihomo -version v1.19.33 -o fields.json
go run ./tools/catalog-extract order -src .cache/ref/mihomo > testdata/catalog/example-order.json
```

## 代码约定

- 依赖方向：`api → service → (engine, store, source, kernel)`；`engine → (catalog, layer, node, ruleset, target, render)`，不依赖 `store`、`source`、`api`（`TestDependencyDirection` 锁定）。网络只在 `internal/source`，持久化只在 `internal/store`，进程调用只在 `internal/kernel`。
- 新增依赖前先确认它不会把 `golang.org/x/*` 升到 mihomo `go.mod` 之上（`go get` 的输出中没有 upgraded golang.org/x/…），见 [0001](docs/decisions/0001-deviations-from-spec-1.0.md) #17。
- 管线核心是纯函数：不读当前时间、随机数，不依赖 map 迭代顺序；相同输入逐字节相同。
- 代码注释用简体中文，解释原因、不变量与复杂度，不逐行复述。
- 先写简单直接的实现，出现真实重复再抽象。不吞错误，错误用 `%w` 包装并带上下文；不静默降级。
- 每个可验证的小目标：实现 → 测试 → 整理（删除废弃分支与临时补丁）→ 同步文档与 progress → 提交。提交信息用中文，带 `feat:`/`fix:`/`docs:`/`chore:`/`ci:` 前缀。
- 未运行的验证不写成通过；源码阅读、单元测试、内核检查、真实客户端导入是不同层级的证据，分别记录。性能数字只写实测值。
- Go 源码中不要写反斜杠紧跟 u 的转义字面量，改用数值（`0xFEFF`）或拼接；编辑工具链会提前解码这类序列。
- 仓库统一 LF（`.gitattributes`）。
