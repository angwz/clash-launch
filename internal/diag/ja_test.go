package diag

import (
	"regexp"
	"slices"
	"testing"
)

// 日语诊断消息（R37b）：每个代码都有译文，译文表没有多余条目，占位符与英文一致。
func TestJapaneseMessages(t *testing.T) {
	missing, stale := JACoverage()
	if len(missing) > 0 {
		t.Errorf("缺少日语译文（internal/diag/ja.json）：%v", missing)
	}
	if len(stale) > 0 {
		t.Errorf("ja.json 中有已不再使用的原文：%q", stale)
	}
	ph := regexp.MustCompile(`\{[a-z_]+\}`)
	for c, info := range All() {
		a, b := ph.FindAllString(info.EN, -1), ph.FindAllString(info.JA, -1)
		slices.Sort(a)
		slices.Sort(b)
		if !slices.Equal(a, b) {
			t.Errorf("%s：日语占位符 %v 与英文 %v 不同", c, b, a)
		}
	}
}
