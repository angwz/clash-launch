# 两层编辑与合并语义

表单层是默认的编辑方式，只接受字段目录中的字段；原始编辑层（后备隐藏能源）是一段 YAML，合并在表单结果之上，可以写任何字段，包括目录里还没有的新字段。本文是合并语义的契约，每一条都对应 [`internal/layer/merge_test.go`](../../internal/layer/merge_test.go) 中的一个用例。

## 表单层

- 输入是生成器产出的分节（`proxies`、`proxy-groups`、`proxy-providers`、`rule-providers`、`rules`）加上用户在表单中设置的字段。
- 只输出用户设置过的字段；把字段设回默认值也算设置，照样输出；「恢复默认」才删除字段。
- 校验（`layer.ValidateForm`）：不在目录中的字段、类型不符的值、不在枚举中的取值都被去掉，并以 error 诊断说明；生成分节不能在表单中设置（[0001](../decisions/0001-deviations-from-spec-1.0.md) #8）。字符串与枚举字段的值一律按字符串保存，`"1.0"` 不会变成浮点数。

## 原始编辑层

| 情况 | 行为 |
| --- | --- |
| 映射对映射 | 递归合并：同名键继续合并，新键追加 |
| 标量 | 替换 |
| 列表 | 替换 |
| 值为 `null` | 设为 null（不是删除） |
| 映射替换为标量（或反之） | 替换 |

合并标签只能出现在映射的值上，以及 `!patch-by` 列表的元素上：

| 标签 | 适用 | 行为 |
| --- | --- | --- |
| `!replace` | 映射、列表 | 整体替换，不递归 |
| `!append` | 列表 | 追加到原列表末尾；原列表不存在时等于设置 |
| `!prepend` | 列表 | 插入到原列表开头；原列表不存在时等于设置 |
| `!delete` | 任意 | 删除该键（值被忽略，写 `!delete ~`）；键不存在时给出 info |
| `!patch-by:<键>` | 元素为映射的列表 | 按键找到元素并递归合并，找不到则追加；元素带 `!delete` 时删除该元素 |

```yaml
dns:
  nameserver: !replace
    - https://dns.example.com/dns-query
rules: !prepend
  - DOMAIN,intranet.example.com,DIRECT
proxy-groups: !patch-by:name
  - name: 🐳 漏网の鱼
    type: fallback
  - !delete {name: 📱 TikTok}
experimental:
  some-new-field: true          # 目录外字段：原样输出，并给出 info
```

错误（标签被忽略，该键保持原值，渲染继续）：

| 情况 | 诊断 |
| --- | --- |
| 未知标签，如 `!merge` | `layer.unknown_tag` |
| 标签出现在键上、普通列表的元素上、或 `!patch-by` 元素上的非 `!delete` 标签 | `layer.tag_placement` |
| `!replace` 用于标量；`!append`/`!prepend` 的值不是列表，或原值不是列表 | `layer.tag_kind` |
| `!patch-by` 的目标不是映射列表 | `layer.patch_target` |
| `!patch-by` 的元素缺少标量键 | `layer.patch_key` |

规则：

- 写成 `!patch-by:name` 而不是 `!patch-by name`：标签后的空格会把 `name` 变成节点内容。
- 新插入的映射内部的标签同样生效，等价于合并到「空」之上：`dns: {nameserver: !append [x]}` 在没有 `dns` 时得到 `dns: {nameserver: [x]}`。
- 锚点与别名在解析时展开（展开后的节点数受限），输出中不出现锚点与别名。
- 合并不修改输入，同一输入结果确定（`FuzzMerge`）。
- 键顺序：合并只追加新键；最终顺序由渲染器统一按目录排列，目录外的键保持出现顺序排在所在映射的末尾，`map` 类型（键是用户数据，如 `hosts`）保持原顺序。
- 原始编辑写入或修改过的路径记录在 `Touched` 中，界面据此标「⚡」；目标适配不会删除这些路径上的字段（用户明确要求输出）。

## 导入兼容（M4）

Clash Verge Rev Merge 与 Stash 覆写在导入时一次性转换为上述标签，原文另存备查；转换规则在实现时补入本节。
