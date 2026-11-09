package diag

// Code 是稳定的诊断代码，形如 "area.reason"。发布后不改名、不复用。
type Code string

// CodeInfo 是诊断代码的登记信息。消息使用 ICU MessageFormat 占位符（{name}），
// 由前端与 CLI 按语言渲染；en 是翻译的源语言（第 15.2 节）。
type CodeInfo struct {
	Severity Severity
	EN       string
	ZH       string
}

// 诊断代码按管线阶段分组登记。新增代码时同时补齐两种语言的消息，
// docs/design/diagnostics.md 由测试检查与此表一致。
var registry = map[Code]CodeInfo{}

func reg(c Code, s Severity, en, zh string) Code {
	if _, dup := registry[c]; dup {
		panic("diag: 重复登记 " + string(c))
	}
	registry[c] = CodeInfo{Severity: s, EN: en, ZH: zh}
	return c
}

// Lookup 返回代码的登记信息。
func Lookup(c Code) (CodeInfo, bool) { i, ok := registry[c]; return i, ok }

// All 返回全部已登记代码（无序）。
func All() map[Code]CodeInfo {
	out := make(map[Code]CodeInfo, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// —— 方案文档 ——
var (
	ProfileInvalid     = reg("profile.invalid", Error, "Profile document is invalid: {reason}", "方案文档无效：{reason}")
	ProfileVersion     = reg("profile.version", Error, "Unsupported profile document version {version}", "不支持的方案文档版本 {version}")
	ProfileUnknownKey  = reg("profile.unknown_key", Warning, "Unknown key {key} in profile document; ignored", "方案文档中的未知键 {key}，已忽略")
	ProfileSourceRef   = reg("profile.source_ref", Error, "Source {ref} referenced by the profile does not exist", "方案引用的源 {ref} 不存在")
	ProfileRuleRef     = reg("profile.rule_ref", Error, "Rule source {ref} referenced by the profile does not exist", "方案引用的规则源 {ref} 不存在")
	ProfileSnapshotOld = reg("profile.snapshot_stale", Warning, "Source {source} has failed since {since}; using the last good snapshot", "源 {source} 自 {since} 起拉取失败，沿用上次成功的快照")
)

// —— YAML 输入 ——
var (
	YAMLSyntax      = reg("yaml.syntax", Error, "YAML syntax error: {reason}", "YAML 语法错误：{reason}")
	YAMLTooLarge    = reg("yaml.too_large", Error, "YAML document exceeds {limit}", "YAML 文档超过上限 {limit}")
	YAMLTooManyNode = reg("yaml.too_many_nodes", Error, "YAML document has more than {limit} nodes", "YAML 文档节点数超过 {limit}")
	YAMLDupKey      = reg("yaml.duplicate_key", Error, "Duplicate key {key}", "重复的键 {key}")
	YAMLNotMapping  = reg("yaml.not_mapping", Error, "Expected a mapping at the top level", "顶层必须是映射")
)

// —— 两层编辑与合并 ——
var (
	LayerUnknownTag   = reg("layer.unknown_tag", Error, "Unknown tag {tag}", "未知标签 {tag}")
	LayerTagPlacement = reg("layer.tag_placement", Error, "Tag {tag} is not allowed here; tags apply to mapping values and !patch-by elements", "此处不允许标签 {tag}：标签只能用于映射的值与 !patch-by 列表的元素")
	LayerTagKind      = reg("layer.tag_kind", Error, "Tag {tag} cannot be applied to a {kind}", "标签 {tag} 不能用于{kind}")
	LayerPatchKey     = reg("layer.patch_key", Error, "Element of !patch-by:{key} has no scalar {key}", "!patch-by:{key} 的元素缺少标量键 {key}")
	LayerPatchTarget  = reg("layer.patch_target", Error, "!patch-by:{key} applies only to lists of mappings", "!patch-by:{key} 只能用于元素为映射的列表")
	LayerDeleteMiss   = reg("layer.delete_missing", Info, "!delete on {path}: nothing to delete", "{path} 上的 !delete：没有可删除的内容")
	LayerRawField     = reg("layer.raw_field", Info, "{path} comes from raw edit", "{path} 来自原始编辑")
	LayerOffCatalog   = reg("layer.off_catalog", Info, "{path} is not in the field catalog; written as-is", "{path} 不在字段目录中，按原样输出")
	LayerAlias        = reg("layer.alias_expanded", Info, "Anchors and aliases were expanded in the output", "输出中的锚点与别名已展开")
	LayerDanglingRef  = reg("layer.dangling_ref", Warning, "{path} references {name}, which does not exist", "{path} 引用的 {name} 不存在")
	LayerCanPromote   = reg("layer.can_promote", Info, "{path} is now a catalog field and can move to the form", "{path} 已是目录字段，可以转为表单字段")
	FormValueType     = reg("form.value_type", Error, "{path} expects {want}, got {got}", "{path} 需要 {want}，实际为 {got}")
	FormUnknownField  = reg("form.unknown_field", Error, "{path} is not a catalog field; use raw edit for fields outside the catalog", "{path} 不是目录字段；目录外字段请用原始编辑")
	FormEnumValue     = reg("form.enum_value", Error, "{path} must be one of {values}", "{path} 只能取 {values}")
	FormGenerated     = reg("form.generated_section", Error, "{path} is produced by the node and rule generators; change it in raw edit", "{path} 由节点与规则生成器产出；如需修改请使用原始编辑")
	FormNotMapping    = reg("form.not_mapping", Error, "The form layer must be a mapping", "表单层必须是映射")
)

// —— 目标适配 ——
var (
	TargetUnknown     = reg("target.unknown", Error, "Unknown target preset {preset}", "未知的目标预设 {preset}")
	TargetUnsupported = reg("target.unsupported_field", Warning, "{path} is not supported by {preset}; removed", "{preset} 不支持 {path}，已删除")
	TargetManaged     = reg("target.managed_field", Info, "{path} is managed by {preset} and may be overridden by the client", "{path} 由 {preset} 接管，可能被客户端覆盖")
	TargetSince       = reg("target.since", Warning, "{path} requires core {since} or later; selected core is {core}", "{path} 需要内核 {since} 或更新，所选内核为 {core}")
	TargetRemoved     = reg("target.removed", Warning, "{path} was removed in core {removed}; selected core is {core}", "{path} 在内核 {removed} 中已移除，所选内核为 {core}")
	TargetDeprecated  = reg("target.deprecated", Info, "{path} is deprecated; use {replacement}", "{path} 已弃用，请改用 {replacement}")
	TargetCoreNoCheck = reg("target.no_kernel", Info, "No kernel check is available for {preset}; only catalog and format checks ran", "{preset} 没有可用的内核检查，只做了目录与格式检查")
)

// —— 渲染与检查 ——
var (
	RenderSelfCheck   = reg("render.self_check", Error, "Format self-check failed at {path}: {reason}", "格式自检失败（{path}）：{reason}")
	RenderLongLine    = reg("render.long_line", Warning, "Line {line} is {length} characters long", "第 {line} 行长度为 {length} 个字符")
	RenderFallback    = reg("render.layout_fallback", Info, "Section spacing was disabled to keep a block scalar exact", "为保证块标量逐字节一致，已取消分节空行")
	KernelFailed      = reg("kernel.failed", Error, "mihomo -t rejected the configuration: {reason}", "mihomo -t 未通过：{reason}")
	KernelUnavailable = reg("kernel.unavailable", Warning, "Kernel check skipped: {reason}", "未进行内核检查：{reason}")
	KernelTimeout     = reg("kernel.timeout", Error, "Kernel check timed out after {timeout}", "内核检查超时（{timeout}）")
	KernelPassed      = reg("kernel.passed", Info, "mihomo {version} -t passed", "已通过 mihomo {version} -t 检查")
)

// —— 节点输入与整理（第 5 节）——
var (
	NodeUnrecognized = reg("node.unrecognized", Error, "Content of {source} is not recognised (expected Clash YAML, sing-box JSON, base64 or share links); preview: {preview}", "无法识别 {source} 的内容（应为 Clash YAML、sing-box JSON、base64 或分享链接）；预览：{preview}")
	NodeLineSkipped  = reg("node.line_skipped", Warning, "Line {line} of {source} skipped: {reason} ({scheme})", "{source} 第 {line} 行已跳过：{reason}（{scheme}）")
	NodeInvalid      = reg("node.invalid", Warning, "Proxy {name} from {source} rejected by mihomo: {reason}", "{source} 中的节点 {name} 未通过 mihomo 校验：{reason}")
	NodeNotProxy     = reg("node.not_proxy", Warning, "Item {index} of {source} is not a proxy mapping with name and type", "{source} 的第 {index} 项不是带 name 与 type 的节点映射")
	NodeInfoRemoved  = reg("node.info_removed", Info, "Information entry {name} removed from {source}", "已从 {source} 去掉信息节点 {name}")
	NodeFiltered     = reg("node.filtered", Info, "{name} from {source} filtered out by {rule}", "{source} 中的 {name} 被 {rule} 过滤")
	NodeDuplicate    = reg("node.duplicate", Info, "{name} from {source} duplicates {first} and was removed", "{source} 中的 {name} 与 {first} 重复，已去掉")
	NodeRenamed      = reg("node.renamed", Info, "{name} renamed to {new} to keep names unique", "{name} 已改名为 {new} 以保持名称唯一")
	NodeEmptySource  = reg("node.empty_source", Warning, "{source} produced no proxies", "{source} 没有产出任何节点")
	NodeRegionInfer  = reg("node.region_inferred", Info, "Region of {name} inferred as {code}: {reason}", "{name} 的地区推断为 {code}：{reason}")
	NodeRegionManual = reg("node.region_manual_unknown", Warning, "Manual region {code} for {name} is not a known region", "为 {name} 手动指定的地区 {code} 不在地区表中")
	NodeSingboxSkip  = reg("node.singbox_unsupported", Warning, "sing-box outbound {tag} ({type}) is not converted: {reason}", "未转换 sing-box 出站 {tag}（{type}）：{reason}")
	NodeSingboxField = reg("node.singbox_field", Warning, "sing-box field {field} of {tag} is not converted", "未转换 {tag} 的 sing-box 字段 {field}")
	RegexInvalid     = reg("regex.invalid", Error, "Invalid regular expression at {where}: {reason}", "{where} 处的正则无效：{reason}")
	RegexTimeout     = reg("regex.timeout", Warning, "Regular expression at {where} timed out on {name}; treated as no match", "{where} 处的正则在 {name} 上超时，按未命中处理")
	ProviderNoEquiv  = reg("provider.not_equivalent", Warning, "{setting} of {source} cannot be expressed in provider mode and has no effect", "{source} 的 {setting} 在「交给客户端」模式下无法等价表达，不会生效")
)

// —— 规则源（第 6 节）——
var (
	RulesSyntax       = reg("rules.syntax", Error, "{file}: {reason}", "{file}：{reason}")
	GroupInvalid      = reg("group.invalid", Error, "Group {name}: {reason}", "策略组 {name}：{reason}")
	GroupReference    = reg("group.reference", Error, "Group {name} references {ref}, which does not exist", "策略组 {name} 引用的 {ref} 不存在")
	GroupCycle        = reg("group.cycle", Error, "Groups reference each other in a cycle: {cycle}", "策略组之间存在循环引用：{cycle}")
	GroupTypeMihomo   = reg("group.type_unsupported", Error, "Group {name} has type {type}, which mihomo does not support: {reason}", "策略组 {name} 的类型 {type} 不被 mihomo 支持：{reason}")
	GroupEmpty        = reg("group.empty", Warning, "Group {name} matched no proxies; {fallback} written instead", "策略组 {name} 没有匹配到任何节点，已写入 {fallback}")
	GroupOffCatalog   = reg("group.off_catalog_key", Warning, "Group {name}: key {key} is not a mihomo group field; passed through as-is", "策略组 {name}：键 {key} 不是 mihomo 的组字段，按原样透传")
	GroupKeyIgnored   = reg("group.key_ignored", Warning, "Group {name}: key {key} is not used for Clash output: {reason}", "策略组 {name}：键 {key} 不用于 Clash 输出：{reason}")
	GroupMatcher      = reg("group.matcher_unsupported", Error, "Group {name}: matcher {matcher} cannot be implemented equivalently: {reason}", "策略组 {name}：匹配器 {matcher} 无法等价实现：{reason}")
	GroupMatcherNote  = reg("group.matcher_note", Warning, "Group {name}: matcher {matcher}: {reason}", "策略组 {name}：匹配器 {matcher}：{reason}")
	GroupIDShift      = reg("group.groupid_shift", Info, "Group {name} uses GROUPID {range}; subscription numbering: {numbering}", "策略组 {name} 使用 GROUPID {range}；当前订阅编号：{numbering}")
	GroupProviderRef  = reg("group.provider_ref", Warning, "Group {name} uses provider {ref}, which is not defined (raw edit may still add it)", "策略组 {name} 引用的 provider {ref} 未定义（原始编辑中仍可补上）")
	GroupProviderNote = reg("group.provider_partial", Warning, "Group {name}: {condition} depends on server-side data and has no effect for provider sources", "策略组 {name}：{condition} 依赖服务器端信息，对「交给客户端」的源不生效")
	SelectInvalid     = reg("select.invalid", Error, "Group {name}: select table: {reason}", "策略组 {name}：select 表：{reason}")
	RulesetInvalid    = reg("ruleset.invalid", Error, "Ruleset {index}: {reason}", "第 {index} 条 ruleset：{reason}")
	RulesetRenamed    = reg("ruleset.renamed", Info, "Rule provider {name} renamed to {new} to keep names unique", "rule-provider {name} 已改名为 {new} 以保持名称唯一")
	RulesetLimit      = reg("ruleset.limitation", Warning, "Ruleset {index}: {reason}", "第 {index} 条 ruleset：{reason}")
	IniUnsupported    = reg("ini.unsupported_key", Warning, "External config key {key} is not supported and was ignored", "外部配置的键 {key} 不受支持，已忽略")
	IniInvalid        = reg("ini.invalid", Error, "External config line {line}: {reason}", "外部配置第 {line} 行：{reason}")
)

// —— 拉取与网络（第 5.4、14.5、14.6 节）——
var (
	FetchFailed    = reg("fetch.failed", Warning, "Fetching {source} failed: {reason}", "拉取 {source} 失败：{reason}")
	FetchBlocked   = reg("fetch.blocked", Error, "Fetching {source} was blocked: {reason}", "已阻止拉取 {source}：{reason}")
	FetchStaleLong = reg("fetch.stale_long", Warning, "{source} has been failing for more than {days} days", "{source} 已连续失败超过 {days} 天")
)
