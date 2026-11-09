# sing-box 出站到 mihomo 节点的映射

内容识别发现顶层有 `outbounds` 数组时，按下表把 sing-box 出站转换为 mihomo 节点（[`internal/node/singbox.go`](../../internal/node/singbox.go)）。字段名对照 sing-box 官方文档 `configuration/outbound/*`、`shared/tls`、`shared/v2ray-transport`（2026-11-09 查证）。转换后的节点同样经过 mihomo `adapter.ParseProxy` 校验。

**不静默丢弃**：表中没有的出站类型、表中没有的字段（含 `tls` 的子字段）都逐个写入诊断（`node.singbox_unsupported`、`node.singbox_field`）。

## 类型

| sing-box | mihomo | 说明 |
| --- | --- | --- |
| `shadowsocks` | `ss` | — |
| `vmess` · `vless` · `trojan` | 同名 | — |
| `hysteria2` · `hysteria` · `tuic` · `anytls` · `http` | 同名 | — |
| `socks` | `socks5` | 只支持 `version` 为 5 或省略 |
| `direct` · `block` · `dns` | — | 不是代理节点，跳过并说明 |
| `selector` · `urltest` | — | 策略组由规则源重建，跳过并说明 |
| 其他（`wireguard` 端点、`ssh`、`tor`……） | — | 不转换，说明原因 |

## 通用字段

| sing-box | mihomo |
| --- | --- |
| `tag` | `name` |
| `server` · `server_port` | `server` · `port` |
| `network`（`tcp` 表示禁用 UDP） | `udp: false`；省略时 `udp: true`（`http` 除外） |
| `detour` | `dialer-proxy` |

## 协议字段

| 协议 | sing-box → mihomo |
| --- | --- |
| shadowsocks | `method` → `cipher`；`password`；`plugin` + `plugin_opts`：`obfs-local` → `obfs`（`obfs`/`obfs-host` → `mode`/`host`），`v2ray-plugin` → `v2ray-plugin`（`mode`、`host`、`path`、`tls`）；`udp_over_tcp`（布尔或对象）→ `udp-over-tcp`、`udp-over-tcp-version` |
| vmess | `uuid`；`security` → `cipher`（省略时 `auto`）；`alter_id` → `alterId`（省略时 0）；`global_padding`、`authenticated_length`、`packet_encoding` → 同名短横线形式 |
| vless | `uuid`；`flow`；`packet_encoding` → `packet-encoding` |
| trojan | `password` |
| hysteria2 | `server_ports`（`2080:3000`）→ `ports`（`2080-3000`，多段以逗号连接）；`hop_interval`；`up_mbps`/`down_mbps` → `up`/`down`（`N Mbps`）；`obfs.type`/`obfs.password` → `obfs`/`obfs-password`；`password` |
| hysteria | `server_ports` 同上；`up`/`down` 或 `up_mbps`/`down_mbps`；`obfs`；`auth`、`auth_str` → `auth`、`auth-str`；`recv_window_conn`、`recv_window`、`disable_mtu_discovery` |
| tuic | `uuid`、`password`；`congestion_control` → `congestion-controller`；`udp_relay_mode`、`udp_over_stream` |
| socks | `username`、`password` |
| http | `username`、`password`；`headers`（值为列表时取第一个） |
| anytls | `password`；`idle_session_check_interval`、`idle_session_timeout`（时长字符串 → 秒）；`min_idle_session`、`client_metadata` |

## TLS（`tls` 对象）

| sing-box | mihomo |
| --- | --- |
| `enabled` | vmess、vless、http 写 `tls`；其余协议本身即 TLS |
| `server_name` | vmess、vless 写 `servername`；其余写 `sni` |
| `insecure` | `skip-cert-verify` |
| `alpn` | `alpn` |
| `disable_sni` | `disable-sni`（tuic） |
| `utls.enabled` + `utls.fingerprint` | `client-fingerprint` |
| `reality.enabled` + `public_key`、`short_id` | `reality-opts.public-key`、`short-id` |
| `ech.enabled` + `config`（列表按行连接）、`query_server_name` | `ech-opts.enable`、`config`、`query-server-name` |

## 传输（`transport` 对象，vmess、vless、trojan）

| `type` | mihomo |
| --- | --- |
| `ws` | `network: ws`；`path`、`headers`、`max_early_data`、`early_data_header_name` → `ws-opts` |
| `httpupgrade` | `network: ws`；`ws-opts.v2ray-http-upgrade: true`，`host` → `headers.Host` |
| `grpc` | `network: grpc`；`service_name` → `grpc-opts.grpc-service-name` |
| `http` | 开启 TLS 时 `network: h2`（`host`、`path` → `h2-opts`）；否则 `network: http`（`method`、`path` → `http-opts`） |
| 其他（`quic`……） | 不转换，报告为未转换字段 |

## 已知限制

- `multiplex` 暂不转换为 `smux`（两者参数不完全对应），报告为未转换字段。
- 只读取 `outbounds`；新版 sing-box 中位于 `endpoints` 的 WireGuard 不读取。
