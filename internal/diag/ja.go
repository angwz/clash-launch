package diag

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// 日语消息（R37b）：英文原文 → 日语，见 ja.json。以英文原文为键：英文改动后旧译文失配，
// JACoverage 报告缺失与多余（测试要求都为空），不会显示过期的意思。缺译文的代码在日语界面下显示英文。
//
//go:embed ja.json
var jaJSON []byte

func init() {
	var ja map[string]string
	if err := json.Unmarshal(jaJSON, &ja); err != nil {
		panic("diag: 解析 ja.json: " + err.Error())
	}
	jaTable = ja
	for c, info := range registry {
		info.JA = ja[info.EN]
		registry[c] = info
	}
}

var jaTable map[string]string

// JACoverage 返回没有日语译文的代码，以及译文表中已不再使用的英文原文（都按字典序）。
func JACoverage() (missing []Code, stale []string) {
	used := map[string]bool{}
	for c, info := range registry {
		used[info.EN] = true
		if info.JA == "" {
			missing = append(missing, c)
		}
	}
	for en := range jaTable {
		if !used[en] {
			stale = append(stale, en)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	sort.Strings(stale)
	return missing, stale
}
