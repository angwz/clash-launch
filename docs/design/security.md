# 安全：威胁模型与测试清单

> 本文是 M5 的验收依据（第 14.7 节）：先说明保护什么、防谁、假设什么，再逐项列出措施与验证它的测试。
> 参数与取舍见 [0002](../decisions/0002-storage-and-service.md)、[0003](../decisions/0003-direct-fetch-and-invites.md)、[0005](../decisions/0005-account-security.md)。

## 资产

| 资产 | 位置 | 泄露的后果 |
| --- | --- | --- |
| 节点凭据（订阅地址、节点密码、UUID） | 源的 URL 与内容、快照、渲染结果——全部加密存储 | 他人使用或耗尽用户的代理服务 |
| 订阅链接令牌 | `links`（加密 + SHA-256 查找） | 他人持续获取用户的完整配置 |
| 账号凭据：口令、安全问题答案、TOTP 密钥、恢复码、通行密钥 | argon2id / 加密 / SHA-256 / 公钥 | 账号被接管 |
| 用户的 SMTP 口令 | `user_smtp`（加密） | 用户邮箱被滥用 |
| 主密钥 | `<数据目录>/master.key` 或环境变量 | 以上所有加密数据 |
| 服务器出口 IP | 拉取订阅时对外可见 | 绕过反向代理与 CDN 的隐藏（[0003](../decisions/0003-direct-fetch-and-invites.md)） |

## 攻击者与边界

| 攻击者 | 能力 | 设计上的回应 |
| --- | --- | --- |
| 互联网上的匿名者 | 访问所有公开接口；批量请求 | 注册默认关闭；注册与找回需要工作量证明；限速与退避；不泄露用户名是否存在 |
| 已注册的普通用户 | 合法登录，构造任意请求与 YAML，填写任意地址 | 所有者隔离（他人的资源一律 404）；SSRF 防护；资源上限与配额；YAML 与正则的复杂度上限 |
| 恶意网站（浏览器中的跨站攻击） | 诱导已登录用户的浏览器发请求 | Cookie `SameSite=Lax` + 同步 CSRF 令牌 + `Origin` 检查；`frame-ancestors 'none'` |
| 被注入的内容（节点名、规则、YAML） | 让界面执行脚本 | 只以文本渲染；严格 CSP（无内联脚本与样式，原始编辑器在 Shadow DOM 中） |
| 订阅提供方或 SMTP 服务器 | 返回恶意内容，看到服务器 IP | 内容只解析不执行；体积、超时、重定向上限；出口 IP 可见是已接受的风险 |
| 拿到数据库副本的人 | 读取数据库文件或备份 | 凭据列加密，密文绑定「表.列:行」；没有主密钥无法解密 |

**信任与假设**：管理员可信（但看不到他人的内容）；服务器主机与主密钥未被攻破；对外部署在 HTTPS 反向代理之后。**不在范围内**：管理员恶意、主机被入侵、浏览器或操作系统被入侵、对 mihomo 内核本身的攻击。

## 默认值

| 项 | 默认 | 位置 |
| --- | --- | --- |
| 注册 | 关闭，凭一次性邀请 | [0003](../decisions/0003-direct-fetch-and-invites.md) |
| 口令 | ≥ 5 个字符且至少 2 类字符（小写、大写、数字、特殊字符）；少于 8 个字符时必须含特殊字符；拒绝常见口令表（9128 条，长度 ≥ 10，更短的常见口令靠登录退避兜底）；argon2id m=19 MiB、t=2、p=1 | `internal/auth` |
| 工作量证明 | 20 位（约 1 秒） | `CLASH_LAUNCH_POW_BITS` |
| 登录退避 | 按账号 3 次后 1 s 起翻倍，最长 15 分钟；按 IP 10 次后 | `internal/service/account.go` |
| 找回锁定 | 按账号 5 次、按 IP 10 次后锁定 15 分钟 | 同上 |
| 会话 | 空闲 7 天、绝对 30 天；`__Host-` 前缀、`HttpOnly`、`Secure`、`SameSite=Lax` | `internal/api/session.go` |
| 订阅端点 | 每 IP + 令牌每分钟 30 次 | `internal/api/subscribe.go` |
| 拉取 | 10 MiB、30 s、5 次重定向；最短间隔 30 分钟；手动拉取每人每小时 30 次 | `internal/source`、配额 |
| 配额 | 50 个源、20 个方案、50 条链接、100 MiB；管理员不受限 | `CLASH_LAUNCH_QUOTA_*` |
| 审计保留 | 180 天 | `CLASH_LAUNCH_AUDIT_DAYS` |
| 内核检查 | 临时目录、断开下载（geox 指向回环）、限时、`GOMEMLIMIT` | `internal/kernel` |

## 测试清单

每一项都由自动化测试覆盖；「层级」区分单元测试、API 测试（真实 HTTP 服务器与 SQLite）与端到端测试（真实二进制 + Edge）。

### 账号与认证

| # | 要求 | 测试 | 层级 |
| ---: | --- | --- | --- |
| A1 | 口令至少 5 位且至少 2 类字符，少于 8 位时须含特殊字符；拒绝常见口令（不区分大小写） | `TestCheckPassword` | 单元 |
| A2 | 用户名不存在与口令错误的响应相同，耗时相近（DummyVerify） | `TestAccountSecurity`（找回）；登录走同一路径 | API |
| A3 | 登录失败按账号退避；退避期内即使口令正确也拒绝 | `TestAccountSecurity` | API |
| A4 | 注册默认关闭；邀请一次性、可过期、可撤销 | `TestRegistrationAndInvites`、`TestInvites`、e2e 注册 | API / e2e |
| A5 | 注册需要 3 个互不相同的安全问题与有效的工作量证明；伪造与重放的证明被拒绝 | `TestAccountSecurity`、`TestPoW` | 单元 / API |
| A6 | 找回答对 2 个即可，答案规范化（大小写、全半角、空白）；成功后全部会话失效 | `TestAccountSecurity`、`TestNormalizeAnswer`、e2e 找回 | 单元 / API / e2e |
| A7 | 找回不泄露用户名：不存在的用户返回稳定的诱饵问题 | `TestAccountSecurity`、`TestDecoyQuestions` | 单元 / API |
| A8 | 找回连续失败 5 次后锁定 15 分钟（即使之后答案正确） | `TestAccountSecurity` | API |
| A9 | TOTP 符合 RFC 6238；同一时间步不能重放；恢复码一次性 | `TestTOTPVectors`、`TestVerifyTOTP`、`TestTOTPFlow`、e2e 两步验证 | 单元 / API / e2e |
| A10 | 启用两步验证后口令正确不建立会话；第二步凭证不可伪造、会过期 | `TestTOTPFlow`、`TestTickets` | 单元 / API |
| A11 | 通行密钥：挑战、来源、RP ID、类型、UP/UV、签名、计数逐项检查；挑战一次性 | `TestWebAuthn`、`TestParseCOSEKeyRejects`、`TestPasskeys` | 单元 / API |
| A12 | 他人的通行密钥不能用作第二步；删除需要口令 | `TestPasskeys` | API |
| A13 | 通行密钥在真实浏览器中可注册与登录 | e2e 通行密钥（Chromium 虚拟认证器） | e2e |
| A14 | 修改口令、安全问题、关闭两步验证需要当前口令；可注销其他设备 | `TestAccountSecurity`、`TestTOTPFlow` | API |
| A15 | 会话列表只含本人的会话；注销后立即失效 | `TestAccountSecurity` | API |
| A16 | 安全事件产生提醒（口令、安全问题等）；提醒文字中英齐全 | `TestSMTPSettingsAndAlerts`、`TestAlertTexts` | 单元 / API |

### 授权与隔离

| # | 要求 | 测试 | 层级 |
| ---: | --- | --- | --- |
| B1 | 用户 A 的每个资源 ID，用户 B 访问时都得到 404（按路由表逐条检查） | `TestOwnerIsolationAllRoutes` | API |
| B2 | 仓储层查询都带所有者；密文绑定行与列，搬到别处无法解密 | `TestOwnerIsolationAndCrypto` | 单元 |
| B3 | 普通用户访问管理接口得到 404 | `TestRegistrationAndInvites`、`TestQuotaAndAdmin` | API |
| B4 | 管理员看不到他人的内容（用户列表只含元数据与用量） | `TestQuotaAndAdmin` | API |
| B5 | 停用账号立即注销其会话、不能登录；管理员不能停用自己 | `TestQuotaAndAdmin` | API |
| B6 | 配额：数量、拉取间隔、存储 | `TestQuotaAndAdmin` | API |

### Web 安全

| # | 要求 | 测试 | 层级 |
| ---: | --- | --- | --- |
| C1 | 写操作需要 CSRF 令牌与同源 `Origin` | `TestEndToEnd` | API |
| C2 | 安全响应头：CSP、`nosniff`、`no-referrer`、HTTPS 下 HSTS | `TestEndToEnd` | API |
| C3 | 所有页面在严格 CSP 下没有违规与页面错误 | e2e「各页面没有错误与 CSP 违规」 | e2e |
| C4 | 乐观锁：缺少 If-Match 为 400，过期为 412 | `TestEndToEnd` | API |
| C5 | 未知的 API 路径返回 404，不回退到页面；点文件不提供 | `TestStaticHandler` | API |
| C6 | 契约：每条路由都在 OpenAPI 中；数组字段不为 null | `TestContract`、`TestEndToEnd` | API |

### 不受信任的输入与出站

| # | 要求 | 测试 | 层级 |
| ---: | --- | --- | --- |
| D1 | SSRF：拒绝回环、私网、链路本地、CGNAT、保留、文档地址与非标准写法 | `TestCheckAddr`、`TestCheckHost` | 单元 |
| D2 | 连接时检查实际地址（DNS rebinding）、重定向到内网被拒绝 | `TestControlBlocksRebinding`、`TestDirectFetchAndLimits` | 单元 |
| D3 | 响应体上限、非 2xx | `TestDirectFetchAndLimits` | 单元 |
| D4 | SMTP：只允许 25/465/587；拒绝换行注入与带显示名的地址；不支持 STARTTLS 时不发送口令；内网地址被拦下 | `TestMailer`、`TestMailerRequiresTLS`、`TestSMTPSettingsAndAlerts` | 单元 / API |
| D5 | YAML 炸弹：体积、别名展开后的节点数、重复键 | `TestParseLimits` | 单元 |
| D6 | 正则灾难性回溯：`regexp2` 匹配超时 100 ms，超时记为诊断并按未命中处理 | `TestRegexTimeout` | 单元 |
| D7 | 订阅端点限流、尚无结果时 503 | `TestSubscriptionRateLimitAnd503` | API |
| D8 | 节点表格中的地址打码 | `TestEndToEnd` | API |

### 存储与密钥

| # | 要求 | 测试 | 层级 |
| ---: | --- | --- | --- |
| E1 | 凭据列 AES-256-GCM 加密；轮换后旧密文可解密；新增的加密列（TOTP、SMTP）参与轮换 | `TestOwnerIsolationAndCrypto`、`TestKeyringRotateAndBackupCopy`、`TestSecurityRepos` | 单元 |
| E2 | 恢复码与邀请令牌只存哈希；邀请一次性的原子性 | `TestSecurityRepos`、`TestInvites` | 单元 |

## 已知限制

- **服务器出口 IP 可见**：订阅提供方与 SMTP 服务器能看到；能添加订阅的用户也能借此得知（[0003](../decisions/0003-direct-fetch-and-invites.md)）。
- **单实例**：退避、工作量证明与通行密钥挑战的「已使用」记录在进程内；重启后清空，多实例部署需要共享实现（[0002](../decisions/0002-storage-and-service.md) #6）。
- **自定义安全问题**会让找回接口暴露账号存在。
- **新设备判断**只看 IP 前缀与浏览器族，不做地理位置识别。
- 通行密钥只在软件认证器与 Chromium 虚拟认证器上验证；真实设备未测试。手机上的工作量证明耗时未实测。
