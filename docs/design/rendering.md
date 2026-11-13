# 渲染与格式自检

优先级：**格式绝对正确 → 字段顺序与排版美观 → 客户端导入不报错 → 每个字段都有效**。

## 输出样式

```yaml
# NAME: 手机 · Stash
# TARGET: stash · mihomo 1.19.33
# GENERATOR: Clash, Launch! 1.0.0 · https://github.com/angwz/clash-launch
# UPDATED: 2026-11-09 06:08:31 (UTC+8)

mixed-port: 7890
mode: rule

dns:
  enable: true
  fake-ip-filter:
    - '*.lan'

proxies:
  - name: 🇸🇬 广港 IEPL 新加坡 01
    type: vmess
    server: 192.0.2.10
    port: 443
    network: ws
    ws-opts:
      path: /ray

  - name: 🇯🇵 日本 02
    type: ss
    server: 192.0.2.11
    port: 8388

proxy-groups:
  - name: 节点选择
    type: select
    proxies:
      - 🇸🇬 广港 IEPL 新加坡 01
      - 🇯🇵 日本 02
```

- 缩进 2 个空格，块序列缩进在父键之下；LF 换行，文件以单个换行结尾。
- 空行：连续的顶层简单键（标量、空集合）挨着写，映射与列表分节前后空一行；`proxies` 与 `proxy-groups` 的条目之间空一行，与官方示例一致（反馈 #23）。
- `proxies` 默认用块格式，每个键一行；方案设置 `output.proxies-flow: true`（界面：预览上方「节点：单行」）时每个节点写成一行流式映射，条目之间不空行。其余部分使用块格式，空集合写 `{}`、`[]`。
- 预览与订阅输出走同一个 `service.Render`，除文件头的 `UPDATED` 外逐字节一致（`TestPreviewMatchesSubscription`）。
- 文件头键名大写；`UPDATED` 取自输入快照，时区取方案设置（默认 UTC），没有快照时间时省略。

## 引号

| 写法 | 何时使用 |
| --- | --- |
| 普通 | 值以普通写法解析回同一类型，且不触发下面任何一条 |
| 单引号 | 需要引号、且只含可打印字符：以指示符开头（`*.lan`、`!x`）、含 `: ` 或 ` #`、首尾空白、空串，以及在 YAML 1.1 解析器中有歧义的值（`yes`、`off`、`0123`、`1:20`、`1_000`、`2026-11-09`） |
| 双引号 | 含控制字符、制表符、BOM、U+2028/U+2029、U+FFFE 等不可打印字符，或在流式上下文中含换行 |
| 字面块 | 块上下文中的多行字符串；只用 `|`（一个结尾换行）与 `|-`（无结尾换行）；首行以空格开头时加缩进指示 `|2` |

- emoji 与非 ASCII 字符原样输出，不转义。
- 流式上下文（节点行）中，ASCII 字符只允许字母、数字、空格与 `._/+@()=^~$;!-` 出现在普通写法里，其余一律加引号：libyaml 在流式上下文中对 `:`、`?` 等的限制比规范更多。
- 非字符串标量（数字、布尔、null）保留原文；原文不能以普通写法解析回同一类型时显式写出标签。
- 超过 1000 字节的键改用显式键 `? 键` 写法；含这类键的节点不用流式写法。

## 键顺序

- 顶层与分节内部按字段目录的顺序；目录顺序与 mihomo 官方示例 `docs/config.yaml` 一致，由 `TestTopLevelOrderFollowsExample` 检查（示例顺序由 `catalog-extract order` 提取，保存在 `testdata/catalog/example-order.json`）。
- 节点：前四个键统一为 `name, type, server, port`，其余按该协议在官方示例中的顺序（[0001](../decisions/0001-deviations-from-spec-1.0.md) #4）。
- 目录外的键保持出现顺序，排在所在映射的末尾；`map` 类型与目录未知的位置保持原顺序；列表元素的顺序从不改变。

## 格式自检

每次渲染都执行，失败即引擎缺陷，不发布：

1. 输出不含 CR，以单个换行结尾，没有制表符缩进；
2. 用 yaml.v3（与 mihomo 同一解析器）重新解析，必须是单个文档、没有重复键；
3. 与渲染前的树逐节点比较种类、标签、值与顺序；
4. 输出中不存在 YAML 1.1 有歧义的普通标量。

超过 4096 个字符的行给出警告（不阻止发布）。为什么不用 yaml.v3 的 Encoder，见 [0001](../decisions/0001-deviations-from-spec-1.0.md#取舍)。
