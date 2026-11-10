package node

import (
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/diag"
)

// 正则灾难性回溯（第 14.5 节）：超时记为诊断并按未命中处理，不会卡住渲染。
func TestRegexTimeout(t *testing.T) {
	re, err := Compile(`^(a+)+$`)
	if err != nil {
		t.Fatal(err)
	}
	var l diag.List
	start := time.Now()
	if Match(re, strings.Repeat("a", 40)+"!", "include", &l) {
		t.Error("超时应按未命中处理")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("匹配耗时 %v，超时没有生效", d)
	}
	items := l.Items()
	if len(items) != 1 || items[0].Code != diag.RegexTimeout {
		t.Errorf("应记为 regex.timeout：%+v", items)
	}
}
