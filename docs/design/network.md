# 拉取、SSRF 防护与出站隐私

[`internal/source`](../../internal/source/) 是唯一联网的包。它拉取订阅与规则源，把结果保存为快照；渲染只读快照，订阅请求从不触发拉取。完整的威胁模型在 M5 写入 `security.md`，本文是拉取层的契约。

## 出站代理（第 14.6 节）

- 所有出站连接走 `CLASH_LAUNCH_EGRESS_PROXY` 配置的出口，支持 `socks5h://`、`socks5://` 与 `http://`。两种 SOCKS5 写法都由**代理**解析域名（主机名原样交给代理），服务器自身不做 DNS 查询。
- **出口不可用时不回退直连**：拉取失败（`ErrEgressDown`），沿用上次成功的快照。
- 没有配置出口时，只有被允许直连的源（仅管理员可设）才会被拉取，其余返回 `ErrNoEgress`；这一策略由服务层决定，拉取层只执行。
- 请求头只有 User-Agent（默认 `clash.meta`，可按源修改）与用户为该源设置的请求头；`Referer`、`Cookie`、`X-Forwarded-*`、`Forwarded`、`X-Real-IP` 即使被设置也不发送；不附带本项目的名称、版本与对外地址。

## SSRF 防护（第 14.5 节）

| 检查 | 时机 | 内容 |
| --- | --- | --- |
| URL | 请求前、每次重定向后 | 只允许 `http`/`https`；拒绝 `localhost`、`*.localhost`、`.local`、`.internal`、`.lan` 与单标签主机名；拒绝 `0x7f.1`、`2130706433` 这类非标准 IPv4 写法 |
| 字面 IP | 同上 | 回环、私网、链路本地、CGNAT、组播、未指定、保留、文档与基准测试网段（含常作 fake-ip 池的 `198.18.0.0/15`）；IPv4 映射、NAT64、6to4、Teredo 地址按其中的 IPv4 判断 |
| 实际连接的地址 | 直连时，`net.Dialer.Control` | 与字面 IP 相同的规则——即使域名在检查之后被改为解析到内网（DNS rebinding），连接时仍会被拒绝 |

使用代理出口时，服务器无法在连接前得知域名解析结果，只能检查字面 IP 与主机名；私网地址相对于出口代理而言已不属于本机网络，风险随之转移到出口一侧。管理员可以用 `CLASH_LAUNCH_FETCH_ALLOWLIST`（逗号分隔的网段、IP 或主机名）放行特定目标。

资源上限：响应体默认 10 MiB，单次请求超时 30 s，重定向最多 5 次，响应头最多 64 KiB。错误信息经过分类与脱敏，不包含 URL 及其中的凭据。

## 快照（第 5.4 节）

- 成功：保存内容、SHA-256、需要的响应头（`subscription-userinfo`、`profile-update-interval`、`content-disposition`、`profile-web-page-url`）与拉取时间。
- 失败（网络错误、非 2xx、内容无法识别、解析后为 0 个节点）：沿用上次成功的内容，记录失败原因与连续失败的起点；连续失败超过 7 天（默认，可配置）时在方案页顶部提示。
- 快照时间是渲染时文件头 `UPDATED` 的来源，保证确定性。

## 测试

[`source_test.go`](../../internal/source/source_test.go)：各类被拒绝与被允许的地址和主机名、`Dialer.Control`、直连拉取与请求头、体积上限、重定向到内网、非 2xx、未加白名单时的回环地址、经最小 SOCKS5 服务器的出口（检查域名原样交给代理）、出口不可用时不回退、快照状态迁移。
