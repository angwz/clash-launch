# 链式代理

方案的 `chains` 让选中的节点先经过一个前置再连出去：流量路径为 **本机 → 前置 → 节点 → 目标**。输出只用 mihomo 的 `dialer-proxy`，实现在 [`internal/engine/chain.go`](../../internal/engine/chain.go)。

## 依据（mihomo v1.19.33 源码）

| 事实 | 位置 |
| --- | --- |
| `relay` 类型的策略组已移除，加载时报错并提示改用 `dialer-proxy` | `adapter/outboundgroup/parser.go` |
| `dialer-proxy` 写在节点上，值为节点名或策略组名；写在策略组上无效，只记录错误日志 | `adapter/outbound/base.go`、`parser.go` |
| proxy-provider 的 `dialer-proxy` 套用到其中每个节点；`override.dialer-proxy` 在其后生效 | `adapter/provider/provider.go`、官方示例 `provider2` |
| 内核只检查前置是否存在，以及「节点 → 节点」的直接循环；经策略组绕回自身的循环能通过 `-t` | `config/utils.go` `validateDialerProxies`；已用 mihomo v1.19.33 实测 |

## 写法

```yaml
chains:
  - via: 🎐 自建 VPS                              # 前置：节点名或策略组名
    select: { source: [机场 A], region: [HK] }    # 现代选择器的键；省略表示全部节点
  - via: 🛫 香港专线
    select: { name: 自建 }
    disabled: true                                # 停用但保留
```

`select` 与 groups.toml 的[现代选择器](rules.md#现代选择器)键相同（`region`、`source`、`protocol`、`tags`、`name`、`rate`、`exclude`），`source` 可写源名称或 `#编号`。

## 套用规则

1. 规则按顺序套用，后面的覆盖前面的（`chain.override`，信息）。
2. 「写入配置」的节点：逐个写入 `dialer-proxy`。前置本身即使被选中也不对自己设置（`chain.self`）。
3. 「交给客户端」的源：服务器看不到其中的节点，只能整体设置。只有 `source` 条件时在 provider 上写 `dialer-proxy`；含按节点属性的条件时不套用并警告（`chain.provider_partial`）。
4. 前置不是已生成的节点或策略组时警告（`chain.via_missing`），仍照写，可在原始编辑中补上，最终由内核检查判定。
5. 没有选中任何节点或 provider 时警告（`chain.empty`）。

## 循环检查

把节点 → `dialer-proxy`、策略组 → 静态成员看作依赖图，套用每条边之前从前置出发查找当前节点：

| 情况 | 处理 |
| --- | --- |
| 只经过节点就回到自身（内核会拒绝加载） | error `chain.cycle`，这一条不套用 |
| 经过策略组回到自身（组选中它时才循环，`-t` 查不出） | warning `chain.loop_risk`，照常套用，诊断中给出完整路径 |

`use` 引用的 provider 成员在服务器上不可见，不参与检查。

## 界面

方案编辑器「源与规则」页下方的「链式代理」：每条规则选前置（候选为预览中的节点与策略组）、点选来源、填地区代码与名称正则；下方显示 `本机 → 前置 → N 个节点 → 目标` 与实际选中的节点。实际结果与候选名字来自预览接口的 `chains`、`outbounds`，前端不推算。

## 验证

- `internal/engine/chain_test.go`：按来源套用、覆盖与跳过自身、直接循环、经组循环、provider 整体与部分条件、无效输入。
- `testdata/workspace/chains.profile.yaml` → golden，`go test -tags kernel ./internal/kernel/` 通过 mihomo v1.19.33 检查。
