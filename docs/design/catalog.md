# 字段目录

字段目录是产品的骨架：表单、补全、校验、目标适配与渲染时的键顺序都从这里生成，前后端都不硬编码字段。数据在 [`catalog/mihomo/`](../../catalog/mihomo/)，加载与导出在 [`internal/catalog`](../../internal/catalog/)。

## 文件

| 文件 | 内容 |
| --- | --- |
| `pack.yaml` | 格式包元数据、文档根地址、支持的内核区间、**顶层键顺序** `order`、文件列表 |
| `general.yaml` | 端口、局域网、模式、地理数据、日志、TLS、外部控制、连接参数、`experimental`、`hosts`、`profile`、`tunnels`、`ntp` |
| `dns.yaml` · `tun.yaml` · `sniffer.yaml` | 对应分节 |
| `inbound.yaml` · `platform.yaml` | `listeners` 与旧式入站；`iptables`、`clash-for-android` |
| `proxies.yaml` | 节点：按 `type` 区分的分支，每个分支按官方示例顺序取用片段字段 |
| `proxies/common.yaml` · `proxies/protocols.yaml` | 共享片段（头部、拨号、smux、TLS、传输）与各协议专属片段 |
| `proxy-groups.yaml` · `providers.yaml` · `rules.yaml` | 策略组、两种 provider、规则与子规则 |
| `presets/` | 目标客户端预设（第 10 节） |

## 字段定义

```yaml
- key: enhanced-mode
  type: enum                       # string|int|float|bool|enum|duration|list|map|object|oneOf|anyOf|any
  enum:
    - {value: fake-ip, en: Answer with fake IPs, zh: 用假 IP 应答}
  default: redir-host              # 内核默认值，取自源码；没有依据就不写
  importance: recommended          # core | recommended | optional | expert
  stability: stable                # stable | experimental | deprecated（deprecated 必须写 replacement）
  since: "1.19.33"                 # 不写表示版本未知
  removed: "1.19.27"
  doc: "dns#enhanced-mode"         # 相对 doc-root 的页面与锚点
  related: [dns.fake-ip-range]
  sensitive: false                 # 敏感字段：加密存储、界面打码、日志脱敏
  targets: {stash: unsupported}    # 与预设默认不同时才写
  zh: [增强模式, "一句话摘要"]       # [标题, 摘要]；摘要加引号，避免其中的逗号与冒号被 YAML 解析
  en: [Enhanced mode, "One-line summary"]
  detail: {zh: 详细说明, en: Details}
```

- `list` 的元素类型写在 `items`，`map` 的值类型写在 `values`，`anyOf` 的候选写在 `of`，`object` 的子字段写在 `fields`，`oneOf` 写 `discriminator` 与 `variants`。
- `type: any` 只用于目录暂时无法精确描述的位置，必须写 `reason`。
- 片段（`fragments`）在 `fields` 中以 `{include: 片段名}` 或 `{include: 片段名, keys: [...]}` 引用；片段字段的 ID 以片段为根（`fragments.tls-verify.skip-cert-verify`），所有引用处共用一份说明。
- 加载时校验：每个字段都有中英文标题与摘要、`importance`、文档链接；枚举有取值；ID 不重复；`order` 与文件中的顶层字段一一对应。

## 顺序的来源

1. 顶层顺序取自官方示例 `docs/config.yaml`，包括其中被注释掉的键；示例中没有的键按 wiki 页面的介绍顺序放在相关字段旁边，并在 `pack.yaml` 中注明。
2. 分节内部与节点各协议同样以示例为准，示例没有出现的字段插在相关字段之后。
3. `catalog-extract order` 从示例提取各位置的键顺序（`testdata/catalog/example-order.json`）；测试检查目录是它的保序扩展。

## 文档链接

`doc` 写成 `页面#锚点`，基于 `doc-root`（`https://wiki.metacubex.one/config/`）。wiki 的锚点由标题生成：字段名本身（`#enhanced-mode`），嵌套字段为父键与子键直接拼接（`#reality-optspublic-key`）；以中文为标题的页面锚点不稳定（`#_3`），这些字段只链接到页面。`testdata/catalog/wiki-anchors.txt` 是各页面锚点的快照，测试检查每个锚点都存在。更新快照：抓取页面 HTML，提取 `<h2>`–`<h4>` 的 `id`（去掉以 `_` 开头的）。

## 提取与跟进

```sh
go run ./tools/catalog-extract scan -src <mihomo 源码> -version vX -o vX.json   # 字段全集与类型
go run ./tools/catalog-extract diff old.json new.json                           # 新增、移除、类型变化；有差异时退出码 3
go run ./tools/catalog-extract coverage -o cov.json vX.json                      # 每条路径在目录中的状态；摘要写到标准错误
```

扫描范围：`config.RawConfig`（`yaml` tag）、`adapter.ParseProxy` 分派的选项结构体（`proxy`）、`outboundgroup.ParseProxyGroup`（`group`）、两种 provider 的 schema（`provider`）、`listener.ParseListener`（`inbound`）。类型是 Go 类型的粗略映射（例如枚举在 Go 中是整数），只作初稿，由人工核对后写入目录。

`coverage` 把 scan 的每条路径放到目录中解析，分为五类：**covered**（类型化收录，可在表单中配置）、**free**（源码本身就是 `any` 或 `map[string]any`，目录如实以 `any` 收录）、**skipped**（有意不收录，原因登记在 `tools/catalog-extract/coverage.go` 的 `skipped`）、**passthrough**（某个祖先以 `any` 透传）、**missing**（目录中没有）。路径分段与 scan 一致：列表与 map 的元素层不占段，按 `type` 分派处以判别值占一段。

## 覆盖率

以 `catalog-extract coverage` 对 mihomo v1.19.33 的 scan 结果（2075 条路径）为准。

| 时间 | covered | free | skipped | passthrough | missing | 本次补齐 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 2026-11-09（M1 结束，free 尚未单列，计入 passthrough） | 793 | 0 | 0 | 873 | 409 | — |
| 2026-11-09 | 1012 | 4 | 1 | 781 | 277 | xhttp 全部细项（含 download-settings、reuse-settings）、shadow-tls/restls/jls、realm、AmneziaWG、ip-stack、vmess 的 mkcp/mekya/tlsmirror、hysteria 的 hop-interval 与 Stash 别名、override-expr |
| 2026-11-09 | 1793 | 3 | 2 | 0 | 277 | listeners 全部 20 种类型（含 tun、hysteria2-realm）的逐类型字段；tun 字段改为片段 tun-fields，与 listeners 的 tun 共用（默认值不同的两个字段单独定义） |

早期记录（按目录中带说明的字段计数）：

| 时间 | 带说明的字段 | 范围 |
| --- | ---: | --- |
| 2026-11-09（M1） | 484 | 全部顶层键与通用、dns、tun、sniffer、ntp、experimental、profile、tunnels、tuic-server、iptables；节点 17 种协议（含 TLS、传输、smux）；策略组 4 种；两种 provider；rules、sub-rules。`listeners` 与部分节点子选项（xhttp 细项、shadow-tls/restls/jls、realm、AmneziaWG）暂以 `any` 透传，M6 补齐 |
