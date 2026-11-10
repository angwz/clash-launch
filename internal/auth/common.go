package auth

import (
	_ "embed"
	"strings"
	"sync"
)

// 常见口令表（第 14.1 节：随程序发布，不联网查询）。由 tools/commonpw 从固定版本的 NCSC 口令表生成，
// 只含长度不少于 10 个字符的条目（更短的口令本来就会被拒绝）。

//go:embed common_passwords.txt
var commonRaw string

var commonSet = sync.OnceValue(func() map[string]struct{} {
	m := make(map[string]struct{}, 10000)
	for _, l := range strings.Split(commonRaw, "\n") {
		if l != "" && !strings.HasPrefix(l, "# ") {
			m[l] = struct{}{}
		}
	}
	return m
})

// IsCommonPassword 报告口令（不区分大小写）是否在常见口令表中。
func IsCommonPassword(pw string) bool {
	_, ok := commonSet()[strings.ToLower(pw)]
	return ok
}
