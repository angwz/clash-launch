# 0006 · OpenClash 预设

- 状态：已采纳（2026-11-09）
- 范围：`catalog/mihomo/presets/openclash.yaml` 的内核版本判断、接管字段与刻意不做的事
- 参考：`vernesong/OpenClash@e553e82`（master，luci-app-openclash 0.48.002），只读放在 `.cache/ref/openclash/`；内核版本信息取自同仓库 `core` 分支 `88a4504`

## 内核版本

| 事实 | 来源 |
| --- | --- |
| OpenClash 只下载 Meta 内核，地址为 `core` 分支的 `<release_branch>/meta/clash-<架构>.tar.gz`；`release_branch` 默认 `master` | `openclash_core.sh`、`etc/config/openclash` |
| `core/master/core_version` 为 `alpha-gbc76c13`（另有 Smart 内核 `alpha-smart-gdaf3ad0`）：即 mihomo **Alpha** 分支的构建，不是带版本号的发布 | `core` 分支 |
| mihomo 提交 `bc76c13`（2026-10-11）是 `v1.19.33` 的祖先，落后 6 个提交；其间唯一的字段变化是 `tun.dns-mode` | GitHub compare `bc76c13...v1.19.33` |

**决定**：预设不固定内核版本，新建方案沿用格式包的最高版本（v1.19.33），内核检查用同一个 mihomo。理由：Alpha 构建持续前移，固定一个快照版本很快就会过时；唯一差异的 `tun.dns-mode` 所在的 `tun` 整段由 OpenClash 重写，对输出没有影响。Smart 内核是第三方分支，不在支持范围内。

## 接管字段

依据是 `yml_change.sh`：OpenClash 每次启动内核前读取订阅配置，按 LuCI 设置改写后再交给内核。预设的 `managed` 只列**无条件改写**，或**按默认开启的设置改写**的字段；这些字段照常输出，界面标注「接管」，渲染时给出 `target.managed_field` 提示。

| 类别 | 字段 | OpenClash 的行为 |
| --- | --- | --- |
| 入站 | `port` `socks-port` `redir-port` `tproxy-port` `mixed-port` `allow-lan` `bind-address` | 写入 LuCI 端口（默认 7890/7891/7892/7895/7893），`allow-lan: true`，`bind-address: "*"` |
| 控制面 | `external-controller` `secret` `external-ui` `external-ui-name` `external-ui-url` | `0.0.0.0:<cn_port>`（默认 9090）与 LuCI 中的密钥、面板 |
| 运行 | `mode` `log-level` `ipv6` | 按 LuCI 设置覆盖 |
| 路由器专用 | `routing-mark` `iptables` `tun` `profile.store-selected` `ntp.enable` | `routing-mark` 设为 6666 或删除；`iptables` 删除；`tun` 整段替换（TUN 模式）或删除；`store-selected: true`；启用 NTP |
| DNS | `dns.enable` `dns.ipv6` `dns.listen` `dns.enhanced-mode` `dns.fake-ip-range` `dns.respect-rules` `dns.use-hosts` | DNS 监听 `0.0.0.0:7874` 并接管 dnsmasq 的转发；模式与 fake-ip 网段来自 LuCI |
| 嗅探 | `sniffer` | 启用嗅探（默认）时整段替换为 OpenClash 内置配置，否则只把 `enable` 设为 false |

**不列入 `managed`，写进预设说明**（它们只在特定条件下发生，或是追加而不是覆盖）：

- proxy 与 rule provider 的 `path` 被移到 `./proxy_provider/`、`./rule_provider/` 下（`init.d/openclash`）。生成器写出的相对路径不受影响，只是落盘位置变化。
- `nameserver`、`fallback`、`default-nameserver`、`proxy-server-nameserver` 中的 `system` 被删除（防回环，`enable_redirect_dns` 默认 1）；`nameserver` 为空时补公共 DNS；`respect-rules` 为真而 `proxy-server-nameserver` 为空时自动补上。
- 自定义 DNS、hosts、`authentication`、`fake-ip-filter`、`nameserver-policy`、绕过大陆等功能只在 LuCI 中开启后才合并进配置；`hosts` 会追加 `openwrt.lan` 等路由器主机名。
- 「路由器自身流量」开启时在 `rules` 头部插入 `SRC-IP-CIDR,<路由器 IP>/32,DIRECT`；自定义规则插在 `MATCH` 之前（`yml_rules_change.sh`）。
- `ebpf`、`auto-redir` 也会被删除，但 mihomo v1.19.33 已没有这两个键，目录中不收录。

## 刻意不做

- **不替用户删除接管字段**：用户可能在 OpenClash 中关闭了对应的覆写，或把同一份订阅给其他客户端用。
- **不预设表单初值**：OpenClash 不强制 `unified-delay`、`tcp-concurrent` 等选项，给初值属于主观取舍。
- **不模拟 OpenClash 的改写**：内核检查针对生成器的输出本身；OpenClash 改写后的结果由维护者在路由器上导入验证。

## 未验证

- 未在 OpenWrt 上实际导入（由维护者验证）。
- Alpha 构建与 v1.19.33 之间的差异只按 compare 的文件列表与提交说明判断，未逐行阅读 sudoku 重构。
