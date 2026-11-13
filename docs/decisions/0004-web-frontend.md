# 0004 · Web 前端的工程选择

- 状态：已采纳（2026-11-10，M4）
- 范围：第 15、16 节在 M4 中的具体选择；视觉风格待维护者用截图确认（第 16.5 节）

## 依赖与版本

| # | 主题 | 决定 |
| ---: | --- | --- |
| 1 | 框架 | Svelte 5（runes）+ TypeScript strict + Vite 8；`web/package.json` 精确固定版本，`package-lock.json` 入库 |
| 2 | 版本策略 | 每个包取 2026-10-26 之前发布的最新稳定版（距实施至少两周），降低刚发布版本被篡改的风险；升级时同样按发布时间挑选 |
| 3 | TypeScript | 5.9.3 而不是 7.x：`svelte-check` 4.7 与 `openapi-typescript` 7.13 只支持 5–6，TypeScript 7 是 Go 原生实现，JS API 尚不完整 |
| 4 | 路由与状态 | 不引入库。路由是 History API + 路径模式（`lib/router.svelte.ts`，约 100 行）；状态是 `.svelte.ts` 模块中的 runes 类。前端路径避开后端保留的 `/api`、`/s`、`/p` |
| 5 | API 类型 | `openapi-typescript` 从 `api/openapi.yaml` 生成 `schema.d.ts`（只有类型，零运行时）；手写一层 30 行的 fetch 封装处理 CSRF、`If-Match: "vN"` 与 RFC 9457 错误 |
| 6 | 方案文档 | `yaml`（ISC）的 Document API 只做结构操作：读写 `form` 的一个路径、`sources` 的顺序与写法、`raw` 原文（块字面量）。未修改时原样返回原文，修改后保留注释。合并、校验与渲染全部经后端预览完成 |
| 7 | 多语言 | `intl-messageformat`（ICU MessageFormat）；语言包按需加载；伪 RTL 语言 `ar-XB` 由 `en` 生成（RLO…PDF），只用于测试，不出现在语言选择器中 |
| 8 | 原始编辑器 | CodeMirror 6，按需加载（约 136 KiB gzip，不计入首屏）；挂在 Shadow DOM 中，见 #11 |
| 9 | 二维码 | `qrcode-generator`（MIT）在本地计算矩阵，渲染为 SVG 路径；不调用任何二维码服务 |
| 10 | 图标 | 自绘的线性图标（`components/Icon.svelte`），不引入图标库，没有额外的许可义务 |

## 安全与 CSP

| # | 主题 | 决定 |
| ---: | --- | --- |
| 11 | CSP 不放宽 | 后端 CSP 保持 `script-src 'self'; style-src 'self'`。CodeMirror 的样式库 style-mod 只在 ShadowRoot 上使用可构造样式表，挂在 document 上会插入 `<style>` 而被拦下；因此编辑器挂在 Shadow DOM 中（设计 token 作为 CSS 变量穿过影子边界）。不采用每请求 nonce：它需要服务端模板化 `index.html`，且 nonce 会暴露在 DOM 中 |
| 12 | 内联样式 | 组件不写 `style="…"` 属性；动态尺寸用 class 或 CSSOM。端到端测试把任何 CSP 违规与页面错误视为失败 |
| 13 | 静态文件 | `assets/` 带内容哈希、长期缓存，缺失时 404（不回退到页面）；`index.html` 每次重新验证；`/api/`、`/s/`、`/p/` 下未注册的路径返回 JSON 404；点文件不提供；构建清单移出 `dist`（`all:` 嵌入会包含点目录） |

## 界面结构

| # | 主题 | 决定 |
| ---: | --- | --- |
| 14 | 分节导航 | 目录 `pack.yaml` 新增 `nav`：每个顶层键恰好属于一个分节（加载时校验，`TestNavCoversTopLevel`）；`generated: sources/rules` 的分节由源生成，表单只给入口 |
| 15 | 订阅编号 | 预览响应新增 `numbering`（GROUPID，引擎计算）与 `nodes`；界面不自行推算编号 |
| 16 | 字段控件 | 标量：开关、分段（≤ 3 个取值或 `ui.widget: segmented`）、下拉、整数、文本（敏感值可显示 / 隐藏）；标量列表一行一项；映射、对象列表与 `any/anyOf/oneOf` 用 YAML 片段。写入前只检查类型与格式 |
| 17 | 诊断显示 | 参数中的源 ID 显示为源名称；`provider.not_equivalent` 的 `setting` 参数改为整理设置的字段名（`flags`、`keepInfo` 等），界面翻译 |
| 18 | 响应式 | ≥ 1280 三栏；768–1279 预览栏为抽屉；< 768 单栏，分节导航为下拉，底部切换「设置 / 预览」。只用逻辑属性，`styles.test.ts` 检查不出现 left/right 与不对称的四值简写 |
| 19 | 首屏预算 | `scripts/budget.mjs` 按构建清单计算入口、登录页与较大语言包的 gzip 体积，超过 150 KiB 失败；实测见 progress |
| 20 | 字段的位置与默认值 | 字段按重要程度固定在「常用」或「更多设置」中，设置后不移动；「更多设置」默认展开。未设置的字段以虚线框与「默认」标注显示默认值，点击后实心选中（反馈 #17、#18） |
| 21 | 总开关 | 对象有 bool 型 `enable` 时：关闭时同组其余字段降低不透明度（仍可编辑，悬停恢复）；打开时按目录的 `recommended` 填入未设置的子字段并提示数量 |
| 22 | 动效 | 只用于状态变化与内容出现：按钮按下、开关弹性滑动、设置标记展开、分组与「更多设置」展开收起、页面与分节淡入。系统要求减少动画时缩短时长并去掉弹性与位移，不完全取消（状态变化需要可察觉的反馈） |
| 23 | 字体 | 系统字体栈 + 随程序发布的 Twemoji Country Flags（`unicode-range` 只覆盖国旗；Windows 自带字体没有国旗）+ 系统彩色 emoji 字体；不访问第三方 |
| 24 | 节点写法 | 预览上方切换「节点：单行 / 多行」，默认多行（与官方示例一致），单行写入方案的 `output.proxies-flow: true`；1.0 的 `output.proxies-block` 仍可解析，切换时移除（反馈 #23） |

## 顺带的后端修正

- `store.ErrExists`：唯一约束冲突与版本冲突分开，API 分别返回 409 `exists` 与 412 `conflict`（此前用户名重复返回 412「已在别处修改」）。
- 诊断与 `touched` 等数组字段在为空时编码为 `[]`（此前没有原始编辑的方案预览返回 `touched: null`，前端因此报错）；契约测试检查预览响应中没有 `null` 数组。

## 未验证

- 视觉风格尚待维护者确认；确认前不视为 M4 完成。
- 只在 Windows 上的 Edge 157 中运行过端到端测试；Firefox、Safari 与 Linux 未运行。Windows 的表情字体不含国旗，地区国旗显示为字母，这是系统字体的限制。
