package service

import (
	"os"
	"regexp"
	"testing"
)

// 前端路由的第一段都必须是保留路径，否则同名短码会遮住页面（反馈 #75）。
func TestReservedCoversFrontendRoutes(t *testing.T) {
	b, err := os.ReadFile("../../web/src/lib/router.svelte.ts")
	if err != nil {
		t.Fatal(err)
	}
	ms := regexp.MustCompile(`\["[a-z-]+", "/([a-z-]+)`).FindAllStringSubmatch(string(b), -1)
	if len(ms) < 10 {
		t.Fatalf("没有解析出前端路由：%d", len(ms))
	}
	for _, m := range ms {
		if !IsReservedCode(m[1]) {
			t.Errorf("前端路由 /%s 未保留", m[1])
		}
	}
	for _, c := range []string{"api", "admin", "LOGIN", "assets", "well-known", "favicon.ico"} {
		if !IsReservedCode(c) {
			t.Errorf("%s 应保留", c)
		}
	}
	if IsReservedCode("home-wifi") {
		t.Error("普通短码不应保留")
	}
}
