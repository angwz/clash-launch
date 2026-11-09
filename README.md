# Clash, Launch!

> *Genshin Impact, Launch!* → *Clash, Launch!* 生成一份配置，然后——启动。

一个由字段目录驱动的 YAML 生成器。第一个目标是 Mihomo 系 Clash 配置：在网页中填入节点、订阅和规则源（如 [domainrouter_v2](https://github.com/angwz/domainrouter_v2) 的 `rulesets.toml`、`groups.toml`），选择目标客户端，按官方文档逐字段设置，得到一条长期有效的订阅链接。官方新增的字段在目录跟进之前，可以直接在原始编辑模式中写入。

> **状态** 实施中 · 引擎、输入与服务端（M1–M3）可用：API、订阅链接、后台拉取、加密存储；网页界面在 M4。进度见 [docs/development/progress.md](docs/development/progress.md)，实施规范见 [docs/spec/](docs/spec/index.md)。

```text
节点 · 订阅 · 批量节点 ─┐
rulesets.toml ──────────┼─▶ 表单 ─▶ 原始编辑 ─▶ 目标适配 ─▶ mihomo -t ─▶ /s/<token>
groups.toml ────────────┘
```

## 计划中的特点

- **字段即目录。** 选定客户端后展示官方文档中的全部字段，附说明、默认值、示例与重要程度；每周自动对比 mihomo 新版本的字段。
- **格式第一。** 输出的 YAML 缩进规整、顺序遵循官方示例，每次都经过格式自检与 `mihomo -t` 检查。
- **后备隐藏能源。** 一处不显眼的原始编辑入口，可以写入任意字段，包括目录里还没有的新字段。
- **有序订阅与现代选择器。** 多个订阅按顺序编号，兼容 `!!GROUPID=`；也可以按地区、源、协议、标签组合挑选节点。
- **不会认错的国旗。** 地区识别会忽略「广港」这类线路词，取真正的落地地；单个节点可以手动指定。
- **第三方规则。** 除 domainrouter_v2 外，支持 ACL4SSR 等社区主流规则。
- **主流 Mihomo 系客户端。** Clash Verge Rev、FlClash、Sparkle、ClashMi、OpenClash、Stash 等都有预设。
- **自托管、多用户。** 单个 Docker 容器；凭据加密存储；订阅拉取带 SSRF 防护。
- **为全球而做。** 从简体中文与英文起步，界面结构支持波斯语等从右往左的语言；运行时不依赖任何第三方服务。

## 许可

计划以 GNU GPL v3.0 或更新版本（`GPL-3.0-or-later`）发布。
