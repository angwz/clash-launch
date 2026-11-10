package yamlx

import (
	"errors"
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
)

// YAML 炸弹（第 14.5 节）：体积上限、别名展开后的节点上限、递归别名、重复键都被拒绝并记为诊断。
func TestParseLimits(t *testing.T) {
	// 十亿笑：每层引用上一层 9 次，9 层后展开为 9^9 个节点
	var b strings.Builder
	b.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i <= 9; i++ {
		b.WriteString("a" + string(rune('0'+i)) + ": &a" + string(rune('0'+i)) + " [")
		for j := 0; j < 9; j++ {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString("*a" + string(rune('0'+i-1)))
		}
		b.WriteString("]\n")
	}
	cases := map[string]struct {
		text string
		lim  Limits
	}{
		"别名炸弹": {b.String(), DefaultLimits},
		"体积":   {strings.Repeat("a: b\n", 100), Limits{MaxBytes: 100}},
		"节点数":  {"[1, 2, 3, 4, 5, 6, 7, 8, 9, 10]", Limits{MaxNodes: 5}},
		"重复键":  {"a: 1\na: 2\n", DefaultLimits},
	}
	for name, c := range cases {
		var l diag.List
		if _, err := Parse([]byte(c.text), c.lim, "t", &l); !errors.Is(err, ErrInvalid) || len(l.Items()) == 0 {
			t.Errorf("%s：应拒绝并记为诊断（%v，%d 条诊断）", name, err, len(l.Items()))
		}
	}
	var l diag.List
	if _, err := Parse([]byte("a: &x [1, 2]\nb: *x\n"), DefaultLimits, "t", &l); err != nil {
		t.Errorf("正常的别名应接受：%v", err)
	}
}
