# 代码地图：改什么，去哪里

> 给维护者与后续代理的索引：按「要改的东西」找到文件、相关测试与需要同步的文档。
> 只列入口与不变量；语义细节在 `design/`，取舍在 `decisions/`。新增包或移动职责时同步更新本文件。

## 一条请求的路径

```text
浏览器 ─ web/src/pages/*  ─ lib/api/client.ts ─┐
                                               ▼
              internal/api（路由、鉴权、CSRF、限流、DTO）
                                               ▼
              internal/service（编排：读存储 → 引擎 → 内核检查 → 写结果）
              ├─ internal/engine（纯函数管线：build → merge → target → render）
              │   ├─ catalog · layer · node · ruleset · target · render · profile
              ├─ internal/store（唯一的持久化）
              ├─ internal/source（唯一的出站网络）
              └─ internal/kernel（唯一的进程调用：mihomo -t）
```

依赖方向由 `TestDependencyDirection`（internal/engine）锁定：引擎不依赖 store、source、api。

## 按任务查找

### 配置生成

| 要改的东西 | 位置 | 测试 | 同步文档 |
| --- | --- | --- | --- |
| 字段（新增、说明、默认值、顺序、重要程度） | `catalog/mihomo/*.yaml`；顶层顺序在 `pack.yaml` 的 `order`，界面分节在 `nav` | `internal/catalog`（锚点、顺序、分节覆盖） | [catalog](../design/catalog.md) |
| 跟进新的 mihomo 版本、检查目录覆盖率 | `tools/catalog-extract`（scan / order / diff / coverage；有意不收录的路径登记在 `coverage.go` 的 `skipped`），`tools.lock.json` | `tools/catalog-extract` | [catalog](../design/catalog.md)、[setup](setup.md) |
| 表单层校验、合并标签 | `internal/layer/form.go`、`merge.go` | `merge_test.go`、`FuzzMerge` | [merge](../design/merge.md) |
| 输出格式（引号、空行、文件头） | `internal/render/render.go`；自检 `selfcheck.go` | golden + `FuzzRender`（`-update` 后人工审阅） | [rendering](../design/rendering.md)；改格式先给样例 |
| 目标客户端预设与适配 | `catalog/mihomo/presets/`、`internal/target` | `internal/engine`、`TestKernelOpenClashFlow`（`-tags kernel`） | [catalog](../design/catalog.md)、依据写进决定（如 [0006](../decisions/0006-openclash-preset.md)） |
| 节点解析、整理、地区识别 | `internal/node`（`node.go`、`process.go`、`region.go`、`singbox.go`）；地区表 `catalog/regions/` | `node_test.go`、`testdata/regions/tricky.yaml`、`FuzzParse` | [nodes](../design/nodes.md) |
| 链式代理（dialer-proxy、循环检查） | `internal/engine/chain.go`；方案字段 `internal/profile`（`Chain`）；界面 `web/src/components/editor/ChainsPanel.svelte` | `chain_test.go`、`testdata/workspace/chains.profile.yaml`（`-tags kernel`） | [chains](../design/chains.md) |
| 规则源、GROUPID、选择器、ini | `internal/ruleset`；有序源与编号在 `internal/engine/build.go` | `ruleset_test.go`、工作区 golden | [rules](../design/rules.md)、[parity](../design/subconverter-parity.md) |
| 诊断代码与中英文消息 | `internal/diag/codes.go`（唯一登记处） | 引用处的测试 | 前端按代码显示，无需改前端 |
| 内核检查 | `internal/kernel/check.go` | `-tags kernel` | [architecture](../design/architecture.md) |

### 服务与数据

| 要改的东西 | 位置 | 测试 | 同步文档 |
| --- | --- | --- | --- |
| 新增 API | `internal/api/routes.go`（路由表）+ 处理器 + `api/openapi.yaml` | `TestContract` 逐条对照路由与契约；改契约后在 `web/` 运行 `npm run gen:api` | — |
| 错误到 HTTP 状态的映射 | `internal/api/server.go` 的 `fail` | `api_test.go` | 前端 `web/src/lib/errors.ts` 的映射表 |
| 表结构 | 新增 `migrations/{sqlite,postgres}/000N_*.sql`（只向前，两套同号） | `TestMigrationsEquivalent` | [0002](../decisions/0002-storage-and-service.md) |
| 仓储方法 | `internal/store/store.go`（接口）+ `sql.go`（实现）；每个方法以所有者为第一个参数 | `store_test.go` | — |
| 加密列 | `internal/store/crypto.go`；AAD 为「表.列:行 ID」 | `TestOwnerIsolationAndCrypto` | [0002](../decisions/0002-storage-and-service.md) |
| 拉取、SSRF | `internal/source/fetch.go`、`guard.go` | `source_test.go` | [network](../design/network.md) |
| 后台调度 | `internal/service/jobs.go` | API 端到端 | — |
| 注册、邀请 | `internal/service/register.go`、`internal/api/register.go` | `TestRegistrationAndInvites`、`TestInvites` | [0003](../decisions/0003-direct-fetch-and-invites.md) |
| 口令、令牌 | `internal/auth/auth.go` | 经 API 测试 | — |
| 工作量证明、安全问题规范化、退避、设备识别 | `internal/auth/pow.go`、`questions.go`、`backoff.go`、`device.go`（纯函数与进程内状态） | `internal/auth/auth_test.go` | [security](../design/security.md) |
| 登录、找回、修改口令与安全问题 | `internal/service/account.go`（规则）、`internal/api/account.go`（HTTP） | `TestAccountSecurity` | [security](../design/security.md) |
| 安全提醒的触发点 | `internal/service/alerts.go`（种类与 `notify`）；发送实现见 SMTP | — | [security](../design/security.md) |
| 两步验证、第二步凭证 | `internal/auth/totp.go`、`ticket.go`；`internal/service/mfa.go`；`internal/api/mfa.go` | `TestTOTPVectors`、`TestTOTPFlow` | [0005](../decisions/0005-account-security.md) |
| 通行密钥 | 验证 `internal/auth/webauthn.go`（规范步骤见注释）；流程 `internal/service/passkey.go`；HTTP `internal/api/passkey.go`；浏览器 `web/src/lib/webauthn.ts` | `TestWebAuthn`、`TestPasskeys`、e2e 虚拟认证器 | [0005](../decisions/0005-account-security.md) |
| 个人 SMTP 提醒 | 发送 `internal/source/smtp.go`；设置与提醒文字 `internal/service/smtp.go`、`alerts.go`；HTTP `internal/api/smtp.go` | `TestMailer`、`TestSMTPSettingsAndAlerts`、`TestAlertTexts` | [security](../design/security.md) |
| 配额、审计、管理 | `internal/service/quota.go`、`admin.go`；HTTP `internal/api/admin.go`；配额检查在各新建与修改接口的 `s.quota(...)` | `TestQuotaAndAdmin` | [security](../design/security.md) |
| 常见口令表 | `internal/auth/common_passwords.txt`（`go run ./tools/commonpw` 生成，不手改） | `TestCheckPassword` | — |
| 数据隔离 | 仓储方法以所有者为参数；新增带路径参数的路由时在 `TestOwnerIsolationAllRoutes` 的 `ids` 中补上资源 | `TestOwnerIsolationAllRoutes` | [security](../design/security.md) |
| 账号安全的表 | `migrations/*/0003_security.sql`、`internal/store/security.go` | `TestSecurityRepos` | — |
| 会话、登录、CSRF | `internal/api/session.go` | `api_test.go` | [0002](../decisions/0002-storage-and-service.md) |
| 配置项（环境变量） | `cmd/clash-launch/serve.go` | — | [deploy](../guide/deploy.md) |
| 静态前端的缓存与回退 | `internal/api/server.go` 的 `staticHandler` | `TestStaticHandler` | [0004](../decisions/0004-web-frontend.md) |

### 界面（`web/`）

| 要改的东西 | 位置 | 测试 |
| --- | --- | --- |
| 颜色、间距、字号、主题 | `src/styles/tokens.css`（深色定义两处，`styles.test.ts` 检查一致） | `styles.test.ts` |
| 通用控件样式（按钮、输入框、表格） | `src/styles/base.css` | — |
| 界面文字 | `src/lib/i18n/messages/{zh-CN,en}.json`；动态前缀登记在 `i18n.test.ts` 的 `dynamic` | `i18n.test.ts`（缺失、多余、ICU 语法） |
| 字段说明文字 | 不在前端：来自目录（`catalog/`） | — |
| 页面与路由 | `src/lib/router.svelte.ts`（路由表）+ `src/App.svelte`（懒加载表）+ `src/pages/*` | `e2e/app.spec.ts` |
| 顶栏、侧栏、响应式外壳 | `src/components/AppShell.svelte`；顶栏的方案状态由 `lib/shell.svelte.ts` 传递 | e2e |
| 方案编辑器 | `pages/ProfileEditor.svelte`（三栏与定位）、`lib/editor.svelte.ts`（状态、预览、保存）、`components/editor/*` | e2e |
| 字段卡片与控件 | `components/editor/FieldCard.svelte`、`FieldControl.svelte`；「说明与建议」面板 `FieldGuide.svelte`（编辑器与百科共用），YAML 示例 `lib/yaml-snippet.ts` | e2e、`yaml-snippet.test.ts` |
| 字段建议（怎么选、推荐 / 建议值） | 目录 `catalog/mihomo/*.yaml` 的 `guide`（[catalog](../design/catalog.md)） | `TestGuideSuggestPassForm` |
| 帮助：教程与百科 | `pages/Help.svelte`；教程正文 `src/help/{zh-CN,en}/NN-主题.md`（序号决定顺序，两种语言同步），极简 Markdown `lib/md.ts` | `md.test.ts`、e2e「帮助」 |
| 预览高亮当前字段 | `lib/yaml-range.ts`（路径 → 行范围）、`components/editor/YamlView.svelte` | `yaml-range.test.ts`、e2e |
| 方案文档的读写 | `src/lib/profile-doc.ts`（只做结构操作） | `profile-doc.test.ts` |
| 原始编辑器 | `components/editor/RawEditor.svelte`、`lib/cm.ts`（Shadow DOM，原因见 0004 #11） | e2e |
| 命令面板 | `components/CommandPalette.svelte`、`lib/palette.svelte.ts`；页面用 `palette.register` 添加来源 | e2e |
| 图标 | `src/components/Icon.svelte`（自绘路径表） | — |
| 工作量证明（浏览器端） | `src/lib/pow.ts`、`pow.worker.ts`、`sha256.ts`（与后端 `auth.LeadingZeroBits` 一致） | `sha256.test.ts`、e2e |
| 安全问题表单与内置问题文字 | `src/components/QuestionFields.svelte`；文字在消息 `question.builtin.0–9`（数量与 `auth.BuiltinQuestions` 一致） | `i18n.test.ts` |
| 批量添加界面文字 | `scripts/merge-messages.mjs <补丁.json>`（按键排序写回两种语言） | `i18n.test.ts` |
| API 类型 | `src/lib/api/schema.d.ts` 由 `npm run gen:api` 生成，不手改 | `npm run check` |

## 不变量（改动时最容易破坏的）

- 管线纯函数：不读时间、随机数，不依赖 map 顺序；相同输入逐字节相同。
- 网络只在 `internal/source`，持久化只在 `internal/store`，进程调用只在 `internal/kernel`。
- 不静默丢弃：跳过、改写、删除都进 `internal/diag`。
- CSP 不放宽：前端不写内联脚本与 `style` 属性；需要动态样式时用 class 或 CSSOM。
- 数组字段在 JSON 中不为 `null`（契约测试检查预览响应）。
- Svelte 的 `$effect` 中启动异步加载时，用 `untrack` 包住：加载函数同步读取的状态会成为依赖，加载后写回它会再次触发 effect（`AuditLog.svelte` 曾因此无限请求）。
- Go 源码中不写反斜杠紧跟 u 的转义字面量（编辑工具会提前解码），用数值或拼接。

## 常见问题的入口

| 现象 | 先看 |
| --- | --- |
| 生成的 YAML 引号或顺序不对 | `internal/render/render.go`、目录的 `order` |
| 某个字段在界面上没有出现 | 目录中是否有该字段、`pack.yaml` 的 `nav` 是否包含其顶层键 |
| 诊断显示为代码而不是文字 | `internal/diag/codes.go` 是否登记 |
| 预览报错但保存成功（或相反） | `service.Render`（预览）与 `service.RenderProfile`（保存后渲染）是否同一路径 |
| 前端控制台出现 CSP 违规 | 内联样式或第三方库插入 `<style>`；参考 `lib/cm.ts` 的处理 |
| 订阅链接返回 503 | 该方案还没有成功的渲染结果：看 `renders` 表与方案页的诊断 |
