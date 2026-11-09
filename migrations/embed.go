// Package migrations 嵌入数据库迁移脚本：每种数据库一个目录，版本号一一对应，只向前。
// 两套脚本必须得到相同的逻辑结构（表、列、主键、唯一约束），由 store 的测试检查。
package migrations

import "embed"

// FS 包含 sqlite/ 与 postgres/ 两个目录，文件名形如 0001_init.sql。
//
//go:embed sqlite/*.sql postgres/*.sql
var FS embed.FS
