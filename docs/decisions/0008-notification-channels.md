# 0008 · 提醒渠道：Webhook、Telegram、Bark、ntfy

- 状态：已采纳（2026-11-21，需求 R31）
- 范围：修订 [network](../design/network.md)「出站连接」中「除用户配置的订阅与规则源外，运行时没有任何出站连接」一句，以及项目约定中「运行时不调用任何第三方服务」的适用范围；补充第 14.1 节的提醒方式

## 背景

提醒（安全提醒与 R23、R24 的运行提醒）原先只经用户本人的 SMTP 发送。很多人更常看手机推送或聊天软件。加入这些渠道，服务器就要连接新的一类目标：用户填写的推送服务地址。邮件已经是先例，它的做法是：目标由用户本人填写，经同一套 SSRF 防护，只用于该用户自己的提醒。本决定把同样的规则扩展到 HTTP 推送。

## 决定

| # | 主题 | 决定 |
| ---: | --- | --- |
| 1 | 渠道 | 四种：`webhook`（通用 JSON）、`telegram`（Bot API `sendMessage`）、`bark`（`POST /push`）、`ntfy`（向根地址发布 JSON）。格式只按各自的官方文档实现（见「依据」）。每个用户最多 5 个渠道 |
| 2 | 出站边界 | 运行时的出站连接为：用户配置的订阅与规则源，以及用户本人配置的提醒目标（SMTP 服务器、推送地址）。服务器自己不依赖任何第三方服务；Bark 与 ntfy 的公共服务地址只是表单里的默认值，用户可以换成自建的 |
| 3 | SSRF 防护 | 与拉取相同的 `Guard`：保存时检查 URL 与主机名，每次连接在 `net.Dialer.Control` 中检查实际地址；`CLASH_LAUNCH_FETCH_ALLOWLIST` 同样生效（自建在内网的 ntfy、Bark 需要管理员放行）。不走代理 |
| 4 | 传输 | 只允许 `https`（地址或令牌在请求中，不以明文传输）；不跟随重定向，3xx 视为失败；单次请求 10 s 超时；响应体最多读 64 KiB 后丢弃；2xx 视为成功 |
| 5 | 请求内容 | 只有 `Content-Type: application/json`、`User-Agent: clash-launch`，以及渠道需要的认证头（ntfy 的 `Authorization: Bearer`、Webhook 的签名头）。正文是提醒的标题、正文与细节，与邮件一样不含对象 ID |
| 6 | Webhook 签名 | 设置了密钥时附带 `X-Timestamp`（Unix 秒）与 `X-Signature-256: sha256=<hex>`，后者是对「时间戳 + `.` + 请求体」的 HMAC-SHA256，接收方据此确认来源并拒绝重放 |
| 7 | Telegram | 主机固定为 `api.telegram.org`，用户只填 Bot 令牌与 chat ID，令牌与 ID 按格式校验后才拼进地址 |
| 8 | 保存与启用 | 设置整体加密存储（`notify_channels.config_enc`，参与密钥轮换）。接口不返回令牌、密钥与 Webhook 地址的路径，只返回「已设置」与主机名；编辑时留空表示保留原值。与邮件相同，**测试消息送达后才启用**，修改设置回到未验证状态；测试每个用户每小时最多 5 次 |
| 9 | 发送 | 每条提醒在后台发给邮件与全部已启用的渠道，彼此独立；失败只记日志，并把分类后的原因与时间记在渠道上，界面显示。错误信息不含地址与令牌 |

## 后果

- 用户填写的推送服务能看到服务器的出口 IP，与订阅提供方、SMTP 服务器相同。
- 推送内容会经过用户选择的服务（例如 Telegram、公共 ntfy 服务器）。提醒里有用户名、IP、设备等信息；表单说明会写明这一点，由用户决定是否使用。
- ntfy 单条消息上限 4096 字节、Telegram 4096 个字符：正文超过时截断并以「…」结尾。

## 依据

- Telegram Bot API：`https://api.telegram.org/bot<token>/sendMessage`，参数 `chat_id`、`text`（1–4096 个字符），成功时响应 `ok` 为真。
- Bark（bark-server `docs/API_V2.md`）：`POST /push`，JSON 字段 `device_key`、`title`、`body`、`group`。文档没有写明成功时的响应格式，因此只按 HTTP 状态码判断。
- ntfy（`docs.ntfy.sh/publish`「Publish as JSON」）：向根地址 POST，JSON 字段 `topic`、`title`、`message`、`tags`、`priority`；访问令牌经 `Authorization: Bearer` 发送；消息上限 4096 字节。

## 验证

- 单元测试：`internal/source` `TestPush`（四种格式的请求内容、签名、https 与重定向限制、内网地址被拒绝、错误不含令牌）；`internal/service` `TestChannels`（校验、加密存储、测试后启用、修改回到未验证、提醒发给已启用渠道并记录结果、数量上限）；`TestContract` 对照 [`openapi.yaml`](../../api/openapi.yaml)。
- e2e：在账号页添加渠道，测试时内网地址被拒绝并显示原因。
- 没有向真实的 Telegram、Bark、ntfy 服务发送过消息。
