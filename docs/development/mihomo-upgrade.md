# 跟进 mihomo 新版本

本项目按某一个 mihomo 版本编写字段目录、校验语义并做内核检查（当前 **v1.19.33**）。mihomo 发布新版本后，按本文从头到尾执行一遍。本文面向维护者和任何 AI 代理，步骤可以逐条照做，每一步都写明要验证什么。

> 原则不变：只按官方文档与源码实现语义，查不到就记为限制，不猜。参考源码只读，放在 `.cache/ref/`。

## 版本固定在哪里

升级时这些位置必须一起改，漏改任何一处，测试都会失败或给出错误结果：

| 位置 | 内容 | 怎么改 |
|---|---|---|
| `go.mod` | `github.com/metacubex/mihomo`（分享链接解析、内核检查用到其源码） | `go get github.com/metacubex/mihomo@vX`，见第 4 步的依赖约束 |
| `tools.lock.json` | mihomo 可执行文件各平台的下载地址与 SHA-256 | 从 Release 页面取地址，下载后自行计算 SHA-256，不抄第三方的值 |
| `internal/kernel/check.go` | `Version()` 返回的版本号，以及「不联网」保证的源码核验说明 | 改版本号，按第 5 步重新核验 |
| `internal/kernel/useragents.go` | 订阅请求的 User-Agent 列表，抄自 `common/convert/util.go` 的 `userAgents` | 与新版源码逐项对照后手工同步，保留文件头 |
| `catalog/mihomo/pack.yaml` | `core-max`、`core-date`（官方发布日期）、`reference`、顶层 `order` | 第 3 步 |
| `catalog/mihomo/*.yaml` | 字段目录 | 第 2 步 |
| `testdata/catalog/example-order.json` | 官方示例的键顺序 | 第 3 步自动生成 |
| `.cache/ref/mihomo` | 只读参考源码 | 第 1 步 |

## 步骤

### 1. 取得新旧两版源码

```sh
git clone --depth 1 --branch vX https://github.com/MetaCubeX/mihomo .cache/ref/mihomo-vX
# 旧版保留在 .cache/ref/mihomo（当前 v1.19.33）
```

### 2. 找出字段变化并更新目录

```sh
go run ./tools/catalog-extract scan -src .cache/ref/mihomo     -version v1.19.33 -o .cache/old.json
go run ./tools/catalog-extract scan -src .cache/ref/mihomo-vX  -version vX       -o .cache/new.json
go run ./tools/catalog-extract diff .cache/old.json .cache/new.json        # 新增、移除、类型变化；有差异时退出码为 3
go run ./tools/catalog-extract coverage -o .cache/cov.json .cache/new.json # 每条路径在目录中的状态，摘要写到标准错误
```

按 diff 逐条处理，字段格式见 [catalog](../design/catalog.md)：

- **新增字段**：写入对应分区文件。类型、默认值、枚举取自源码；`doc` 指向官方文档；`zh` 与 `en` 都要写；加 `since: "x.y.z"`，界面会对更旧的内核给出提示。
- **移除或改名**：不要直接删除，标 `stability: deprecated`，有替代字段的写 `replacement`。
- **类型或默认值变化**：按源码改，并检查 `guide` 中的建议值是否仍然成立（`TestGuideSuggestPassForm` 会校验建议值能否通过表单）。
- **有意不收录**：登记到 `tools/catalog-extract/coverage.go` 的 `skipped`，并写明原因。

完成标准：coverage 中不再有 **missing**。

### 3. 更新键顺序与目录版本

```sh
go run ./tools/catalog-extract order -src .cache/ref/mihomo-vX > testdata/catalog/example-order.json
```

然后修改 `pack.yaml`：
- `order` 中补上新出现的顶层键，测试会检查目录顺序是官方示例顺序的保序扩展；
- 更新 `core-max`、`core-date`、`reference`。

### 4. 更新依赖与工具

- **依赖**：`go get github.com/metacubex/mihomo@vX && go mod tidy`。检查 `go get` 的输出中没有 `upgraded golang.org/x/…`；如有，按 [0001](../decisions/0001-deviations-from-spec-1.0.md) #17 处理。
- **工具**：更新 `tools.lock.json`，然后运行 `go run ./tools/toolsync`。它会校验 SHA-256，校验失败说明地址或哈希不对，不要跳过。
- **User-Agent**：同步 `internal/kernel/useragents.go`。

### 5. 重新核验依赖源码语义的代码

下面这些代码的正确性依赖 mihomo 的具体行为，代码注释里写明了源码出处。新版本必须逐条对照，确认行为没变：

```sh
grep -rn "v1\.19\.33" internal --include=*.go | grep -v _test
```

| 文件 | 依赖的行为 | 对照的源码 |
|---|---|---|
| `internal/engine/disabled.go` | 各分区 `enable: false` 时其余字段不生效；DNS 关闭时 TUN 仍读取 `fake-ip-range` | `hub/executor/executor.go`、`config/config.go` 的 `parseTun`、`parseDNS` |
| `internal/ruleset/prune.go` | 空组使用 `empty-fallback`，默认值为 `COMPATIBLE`（直连），且不能填策略组 | `adapter/outboundgroup/parser.go`、`groupbase.go` |
| `internal/engine/chain.go` | `dialer-proxy` 是唯一的链式写法 | `adapter/outbound`、`config` |
| `internal/node/wireguard.go` | wireguard 字段名，`ip` 接受前缀写法 | `adapter/outbound/wireguard.go` |
| `internal/node/*.go`、`internal/kernel/links.go` | 分享链接解析 | `common/convert/converter.go` |
| `internal/kernel/check.go` | `-t` 检查不联网 | 见文件内说明 |
| `internal/match`（[routing](../design/routing.md) 第 5 节，实施后生效） | 规则集策略对象的构造函数与导出方法（`Insert`、`FinishInsert`、`Match`、`FromMrs`、`DumpMrs`）；`rulesParse` 的逐行规则与 MRS 文件头；`RULE-SET` 的 `no-resolve`、`src` 语义；`RuleMatchHelper` 的回调字段；`rules.ParseRule` 的签名 | `rules/provider/*.go`、`rules/parser.go`、`constant/rule.go` |
| 组的提醒（routing 第 11.3 节） | url-test 只比较最近一次测速、`tolerance` 默认 0、`lazy` 默认开启、默认间隔 300 秒；HTTP 测速地址的警告 | `adapter/outboundgroup/urltest.go`、`parser.go`，`adapter/adapter.go` 的 `URLTest`、`LastDelayForTestUrl` |

行为变了就改代码和注释，并补测试；无法确认的写进限制，不要猜。

### 6. 全量验证

```sh
go run ./tools/toolsync
gofmt -l . && go vet ./...
go test ./...
go test -tags kernel ./internal/kernel/ ./internal/api/       # 用真实的新内核检查全部 golden，并实际加载订阅链接
go test ./internal/render/ ./internal/engine/ ./internal/workspace/ -update   # golden 有变化时才执行，差异必须逐行人工审阅
cd web && npm run check && npm test && npm run build && npm run budget && npm run e2e
```

golden 的差异只应来自新版本的真实变化，比如新的键顺序。每一处差异都要能说出原因。

### 7. 同步文档并提交

- 用 `grep -rn "1\.19\.33" README.md THIRD_PARTY_NOTICES.md docs/guide docs/design docs/development CLAUDE.md` 找出版本号，逐一更新。`docs/history/` 与 `docs/spec/` 是归档，不改。
- 更新 [progress](progress.md)，记录这次升级与验证结果。
- 提交信息示例：`feat: 跟进 mihomo vX：新增 N 个字段，……`。

## 完成标准

- [ ] coverage 中没有 missing；新增字段都有 `since`、中英文说明与文档链接
- [ ] 第 5 步表中的语义逐条核验，结论写进代码注释
- [ ] `go test ./...`、`-tags kernel`、前端检查与 e2e 全部通过
- [ ] golden 的差异已逐条审阅
- [ ] 版本号在第一张表与文档中已全部更新

## 跟进多个版本

一次只跨一个版本：从 v1.19.33 升到 v1.19.35，要经过 v1.19.34。每个版本单独提交，出问题时容易定位到具体是哪一版。
