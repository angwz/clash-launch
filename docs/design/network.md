# 拉取与 SSRF 防护

[`internal/source`](../../internal/source/) 是唯一联网的包。它拉取订阅与规则源，把结果保存为快照；渲染只读快照，订阅请求从不触发拉取。完整的威胁模型在 M5 写入 `security.md`，本文是拉取层的契约。

## 出站连接

- 服务器**直接**连接目标，不经过出站代理（[0003](../decisions/0003-direct-fetch-and-invites.md)）。订阅提供方因此能看到服务器的出口 IP；谁能添加订阅由注册开关与邀请控制。
- 请求头只有 User-Agent（默认 `clash.meta`，可按源修改）与用户为该源设置的请求头；`Referer`、`Cookie`、`X-Forwarded-*`、`Forwarded`、`X-Real-IP` 即使被设置也不发送；不附带本项目的名称、版本与对外地址。
- 除用户配置的订阅与规则源外，运行时没有任何出站连接。

## SSRF 防护（第 14.5 节）

| 检查 | 时机 | 内容 |
| --- | --- | --- |
| URL | 请求前、每次重定向后 | 只允许 `http`/`https`；拒绝 `localhost`、`*.localhost`、`.local`、`.internal`、`.lan` 与单标签主机名；拒绝 `0x7f.1`、`2130706433` 这类非标准 IPv4 写法 |
| 字面 IP | 同上 | 回环、私网、链路本地、CGNAT、组播、未指定、保留、文档与基准测试网段（含常作 fake-ip 池的 `198.18.0.0/15`）；IPv4 映射、NAT64、6to4、Teredo 地址按其中的 IPv4 判断 |
| 实际连接的地址 | 每次连接，`net.Dialer.Control` | 与字面 IP 相同的规则——即使域名在检查之后被改为解析到内网（DNS rebinding），或重定向到解析为内网的域名，连接时仍会被拒绝 |

管理员可以用 `CLASH_LAUNCH_FETCH_ALLOWLIST`（逗号分隔的网段、IP 或主机名）放行特定目标。

资源上限：响应体默认 10 MiB，单次请求超时 30 s，重定向最多 5 次，响应头最多 64 KiB。错误信息经过分类与脱敏，不包含 URL 及其中的凭据。

## 快照（第 5.4 节）

- 成功：保存内容、SHA-256、需要的响应头（`subscription-userinfo`、`profile-update-interval`、`content-disposition`、`profile-web-page-url`）与拉取时间。
- 失败（网络错误、非 2xx、内容无法识别、解析后为 0 个节点）：沿用上次成功的内容，记录失败原因与连续失败的起点；连续失败超过 7 天（默认，可配置）时在方案页顶部提示。
- 快照时间是渲染时文件头 `UPDATED` 的来源，保证确定性。

## 测试

[`source_test.go`](../../internal/source/source_test.go)：各类被拒绝与被允许的地址和主机名（`TestCheckAddr`、`TestCheckHost`）；`Dialer.Control` 拦截 DNS rebinding（`TestControlBlocksRebinding`）；直连拉取与请求头、体积上限、重定向到内网、非 2xx、未加白名单时的回环地址（`TestDirectFetchAndLimits`）；快照状态迁移（`TestSnapshotUpdate`）。
