package api

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// 强调色（R28）：偏好接口接受预设名称、拒绝其他值；名称与前端 lib/accents.ts 一致。
func TestPrefsAccent(t *testing.T) {
	h := newHarness(t, nil)
	c, _ := h.setup()
	c.want(200, "PUT", "/api/v1/account/prefs", map[string]any{"accent": "teal"})
	if p := decode[map[string]any](t, c.want(200, "GET", "/api/v1/account/prefs", nil)); p["accent"] != "teal" {
		t.Errorf("保存的强调色：%v", p)
	}
	c.want(400, "PUT", "/api/v1/account/prefs", map[string]any{"accent": "#ff0000"})

	b, err := os.ReadFile("../../web/src/lib/accents.ts")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`ACCENTS = \[([^\]]*)\]`).FindSubmatch(b)
	if m == nil {
		t.Fatal("没有找到 ACCENTS")
	}
	var fe []string
	for _, q := range regexp.MustCompile(`"([a-z]+)"`).FindAllSubmatch(m[1], -1) {
		fe = append(fe, string(q[1]))
	}
	if !slices.Equal(fe, accents) {
		t.Errorf("前端 %v，后端 %v", fe, accents)
	}
}
