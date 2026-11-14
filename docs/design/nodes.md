# 节点输入与整理

[`internal/node`](../../internal/node/) 把一个源的原文快照变成校验过的节点列表。它是纯函数：不联网、不读写数据库；拉取在 [`internal/source`](../../internal/source/)，见 [network](network.md)。

## 内容识别

按以下顺序判断，第一个成功的为准：

0. **WireGuard 配置文件**（wg-quick 格式）：有 `[Interface]` 一节；先于 YAML 判断，因为 INI 不是 YAML。映射与不转换的设置见 `internal/node/wireguard.go` 的文件注释：只有一个 `[Peer]` 时写在顶层，多个时写成 `peers`；`DNS` 在 mihomo 中需要配合 `remote-dns-resolve`，语义不同，不自动转换，逐项报告 `node.wireguard_field`。一个文件产出一个节点，名称取第一个对端地址。
1. **Clash/Mihomo YAML**：顶层有 `proxies` 列表；或是节点映射的列表；或是带 `name`、`type` 的单个节点映射。
2. **sing-box JSON**：顶层有 `outbounds` 数组，映射见 [singbox-mapping](singbox-mapping.md)。
3. **base64**：整体解码（标准与 URL-safe 两种字母表，容忍缺失的填充与换行），解码结果是 UTF-8 文本时按行重新识别。
4. **分享链接**：每行一条。

都失败时报告 `node.unrecognized`，附开头 200 字节的脱敏预览（URL 中的用户信息与查询串、形似密钥的长串被替换为 `***`）。

## 解析与校验

- 分享链接由 mihomo 的 `common/convert.ConvertsV2Ray` 解析，与内核行为一致；它一次处理多行时会静默跳过坏行并给重名节点改名，因此**逐行调用**，失败的行记入 `node.line_skipped`（行号、scheme、原因，不含链接内容）。
- 每个节点再经 mihomo 的 `adapter.ParseProxy` 校验，构造的出站对象立即关闭；失败者记入 `node.invalid`，原因中的凭据值被替换为 `***`。
- 分享链接产出的字段按目录类型修正（端口等由字符串转为整数）；用户提供的 YAML 保持原样。
- mihomo 转换器会为 ws 传输**随机**注入一个浏览器 User-Agent；本项目把它改为按链接内容（不含名称）哈希选取同一张表中的条目，保证渲染确定（[0001](../decisions/0001-deviations-from-spec-1.0.md) #12）。
- mihomo 的日志级别被设为 `SILENT`：它的转换器会把解析失败的整行写进日志。
- 支持的 scheme 以 v1.19.33 源码为准：`hysteria`、`hysteria2`/`hy2`/`hysteria2+realm`/`hy2+realm`、`tuic`、`trojan`、`vless`、`vmess`、`ss`、`ssr`、`socks`/`socks5`/`socks5h`/`http`/`https`、`anytls`、`mierus`。WireGuard 没有正式的分享链接规范，通过 Clash YAML 输入。

## 整理

每个源按固定顺序执行，每一步去掉或改动的节点都写入诊断：

| 步骤 | 设置（`node.Processing`） | 说明 |
| --- | --- | --- |
| 过滤 | `include`、`exclude`、`keep-info`、`info-patterns` | 正则为 regexp2（与 mihomo `filter` 一致），多个用反引号分隔，超时 100 ms。「去掉信息节点」默认开启，模式表在 [`catalog/regions/info.yaml`](../../catalog/regions/info.yaml) |
| 重命名 | `rename`（正则 → 替换）、`prefix` | 按顺序应用 |
| 地区与国旗 | `region-rules`、`overrides`、`flags`、`no-decor` | 见下文 |
| 去重 | `keep-duplicates` | 除 `name` 外全部字段相同即为重复，保留首个 |
| 排序 | `sort` | `source`（默认，保持原序）或 `name` |

之后，全部源的节点按源的顺序连接，跨源同名时后出现的追加「 2」「 3」……。

**指纹**：协议 + 服务器 + 端口 + 凭据的 SHA-256 前 16 位。手动指定的地区、emoji、标签、倍率都以指纹为键，机场改名后依然有效；找不到指纹时再按名称匹配。

**倍率**：从名称识别 `x2`、`2x`、`2倍`、`0.5×`、`×1.5`、`倍率：3`，识别不到时为 1；可以手动指定。数字与 `x` 之间不允许空格，避免把「04 x2」读成 4 倍。

**标签**：源级标签与手动指定的标签，供现代选择器使用。

## 地区识别

识别结果是节点的属性 `region`（ISO 3166-1 两位代码）与理由；国旗只是它的一种显示方式。优先级从高到低：

1. **单个节点的手动指定**（`overrides` 中按指纹或名称）；
2. **源级规则**（`region-rules`：正则 → 地区，第一个命中为准；外部配置 ini 中的国旗 `emoji=` 规则也转为这一级）；
3. **内置识别**；
4. **未识别**：不加国旗，改用装饰 emoji。

内置识别不是「第一个命中的正则」，而是对整个名称打分（[`region.go`](../../internal/node/region.go)，数据在 [`catalog/regions/regions.yaml`](../../catalog/regions/regions.yaml)）：

1. 名称中已有国旗 emoji：直接采用，且不再重复添加。
2. 方向符号（`→`、`->`、`=>`、`⇒`、`➡`、`转`、`至`）：从最后一个起向前尝试，取第一个右侧有地区线索的切分点（「转」也可能是线路词「中转」的一部分）。
3. 在名称中找出全部地区线索与线路词，**较长的词优先并占用位置**，等长时地区名优先于线路词。线路词包括「城市简称 + 地区简称」的组合（广港、沪日、京德……，由 `route-cities` × 各地区的 `short` 生成）以及 IPLC、IEPL、BGP、中转、专线等；它们被剔除并记入理由。地区线索包括中文名与别称、主要城市、英文名（词边界、不区分大小写）、ISO 两位与三位代码（词边界；只有 HK、TW、JP、SG、US、KR、UK 允许小写）、单字简称。
4. 多个地区线索时取最靠后的一个（机场惯例是线路在前、落地在后），并标为「推断」。
5. 只剩线路词时，用线路词中的地区，标为「推断」。

[`testdata/regions/tricky.yaml`](../../testdata/regions/tricky.yaml) 是刁钻名称表（中转、混合语言、代码冲突、已有国旗、单字简称、信息节点），共 55 条，每条写明期望结果；识别规则改动时它就是回归测试。

**国旗写法**：`prefix`（默认，「🇸🇬 原名称」，名称已有国旗时不重复添加）、`replace`（替换名称中已有的国旗）、`off`。

**装饰 emoji**：开启国旗时，未识别的节点从 [`catalog/regions/decor.yaml`](../../catalog/regions/decor.yaml) 的精选表中选一个——按指纹的 FNV-1a 哈希取模，同一节点每次得到同一个；与同一源内上一个节点相同时顺延一个。表中不含国旗、地区旗与易被误认为地区的符号；可以只保留自己喜欢的几个，也可以为单个节点手动指定。
