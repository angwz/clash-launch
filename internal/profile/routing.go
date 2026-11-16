package profile

import "time"

// Routing 是方案自己管理的分流（docs/design/routing.md 第 4.1 节，决定 0007 第 6 条）。
// groups 与 groups.toml 的 [[custom_groups]] 同一组键，rulesets 与 rulesets.toml 同一组键并多一个 bundle；
// 两者保持原始映射，由 ruleset.FromRouting 校验与转换，与规则源走同一条路径。
type Routing struct {
	From       RoutingFrom      `yaml:"from"`
	ININodeOps []string         `yaml:"ini-node-ops,omitempty"` // 上游为 ini 时影响节点的行（rename、emoji、include_remarks……）
	Groups     []map[string]any `yaml:"groups"`
	Rulesets   []map[string]any `yaml:"rulesets"`
	Base       *RoutingBase     `yaml:"base,omitempty"` // 开始管理时上游的规范化副本，上游同步的三方比对基准
	Checks     []RoutingCheck   `yaml:"checks,omitempty"`
}

// RoutingFrom 记录开始自己管理时的上游。
type RoutingFrom struct {
	Rules  string    `yaml:"rules"`  // 规则源 ref，与 rules.ref 相同
	Digest string    `yaml:"digest"` // 上游 groups + rulesets 规范化 JSON 的 sha256
	At     time.Time `yaml:"at"`
}

// RoutingBase 是上游的规范化副本。
type RoutingBase struct {
	Groups   []map[string]any `yaml:"groups"`
	Rulesets []map[string]any `yaml:"rulesets"`
}

// RoutingCheck 是一条期望：追踪 host 时，路径上必须出现 via（组名或出口名）。
type RoutingCheck struct {
	Host string `yaml:"host"`
	Via  string `yaml:"via"`
}
