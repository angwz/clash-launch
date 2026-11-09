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
