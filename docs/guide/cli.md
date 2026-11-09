# 命令行：离线渲染方案

`clash-launch render` 读取一份方案文档，按字段目录合并表单层与原始编辑层，渲染为最终 YAML。它不联网、不读写数据库，结果确定：同一份文件两次渲染逐字节相同。

```sh
clash-launch render [选项] <方案文件>
```

| 选项 | 说明 |
| --- | --- |
| `-o <文件>` | 写入文件；默认写到标准输出 |
| `-check` | 渲染后用 `mihomo -t` 检查；需要 `-mihomo` 与 `-geodata`（或环境变量 `CLASH_LAUNCH_MIHOMO`、`CLASH_LAUNCH_GEODATA`）。检查不通过或无法执行时退出码为 1 |
| `-updated <RFC 3339>` | 文件头 `UPDATED` 的时间；默认取方案文件的修改时间 |
| `-lang zh\|en` | 诊断语言 |
| `-q` | 只输出 error 级诊断 |
| `-inputs <文件>` | 工作区文件：源与规则源的定义及其快照（见下文） |

诊断写到标准错误，按严重程度排序，每行是「级别 代码 [路径]：消息」：

```text
warning target.removed [global-client-fingerprint]：global-client-fingerprint 在内核 1.19.27 中已移除，所选内核为 1.19.33
info    kernel.passed：已通过 mihomo v1.19.33 -t 检查
info    layer.off_catalog [experimental.some-new-field]：experimental.some-new-field 不在字段目录中，按原样输出
```

## 方案文档

```yaml
clash-launch/profile: v1
name: M1 · 两层编辑
target: { pack: mihomo, preset: mihomo, core: "1.19.33" }
output: { timezone: Asia/Shanghai }
form:                       # 表单层：只写目录中的字段
  mixed-port: 7890
  dns: { enable: true, enhanced-mode: fake-ip }
raw: |                      # 原始编辑（后备隐藏能源）：任意 YAML，带合并标签
  rules: !prepend
    - DOMAIN,intranet.example.com,DIRECT
  experimental:
    some-new-field: true
```

完整示例见 [`testdata/profiles/`](../../testdata/profiles/)，合并标签见 [merge](../design/merge.md)。

## 工作区：源与规则源

方案只按 `ref` 引用源与规则源；命令行离线渲染时，它们的定义与快照由工作区文件提供（服务端由数据库提供同样的内容）。路径相对工作区文件所在目录。

```yaml
clash-launch/workspace: v1
sources:
  - id: src_airport_a
    name: 机场 A
    kind: subscription            # single | batch | subscription | file
    snapshot: snapshots/airport-a.txt
    fetched: 2026-11-08T22:00:00Z # 快照时间，写入文件头 UPDATED
    processing: { exclude: 测试, flags: prefix }
  - id: src_my_vps
    name: 我的 VPS
    kind: single
    content: |
      - {name: 自建, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example}
rule-sources:
  - id: rs_domainrouter
    groups: rules/groups.toml
    rulesets: rules/rulesets.toml  # 或 ini: rules/ACL4SSR_Online.ini
```

```sh
clash-launch render -inputs workspace.yaml -check -mihomo … -geodata … profile.yaml
```

订阅按在方案 `sources` 中出现的顺序编号 0、1、2……；`processing` 的字段见 [nodes](../design/nodes.md#整理)。完整示例在 [`testdata/workspace/`](../../testdata/workspace/)。

## 导出字段目录

```sh
clash-launch catalog export <目录>
```

生成 `<目录>/mihomo/` 下的 `sections.json`（表单元数据）、`schema.en.json`、`schema.zh-CN.json`（JSON Schema draft 2020-12，可用于本地编辑器补全）与 `messages.*.json`（i18n 源消息）。
