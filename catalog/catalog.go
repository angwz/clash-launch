// Package catalogdata 嵌入 catalog/ 下的数据文件：字段目录、客户端预设、地区表、规则源预设与规则集说明。
// 数据只在这里嵌入；解析与校验在 internal/catalog 等包中完成。
package catalogdata

import "embed"

// FS 是嵌入的目录数据。路径以 mihomo/、regions/、rulesources/、rulesets/ 开头。
//
//go:embed mihomo regions rulesources rulesets
var FS embed.FS
