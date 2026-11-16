package workspace_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
	rd "github.com/angwz/clash-launch/internal/render"
	"github.com/angwz/clash-launch/internal/ruleset"
)

// TestRoutingForkGolden（routing 第 13 节第 3 步）：domainrouter 方案开始自己管理分流后（routing 段由 Fork 从同一份
// 规则源生成），输出与跟随上游时逐字节相同。带 routing 的方案文件由本测试在 -update 时生成，也进入工作区 golden 与内核检查。
func TestRoutingForkGolden(t *testing.T) {
	const dir = "../../testdata/workspace/"
	base, err := os.ReadFile(dir + "domainrouter.profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := os.ReadFile("../../testdata/rules/domainrouter/groups.toml")
	rulesets, _ := os.ReadFile("../../testdata/rules/domainrouter/rulesets.toml")
	var l diag.List
	node := ruleset.Fork(groups, rulesets, nil, "domainrouter", "rs_domainrouter", "2026-11-15T00:00:00Z", &l)
	for _, d := range l.Items() {
		if d.Severity == diag.Error {
			t.Fatalf("Fork：%s", d)
		}
	}
	body, err := rd.Render(node, rd.Options{}, &l)
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.Replace(string(base), "name: M2 · domainrouter_v2", "name: M2 · domainrouter_v2（自己管理）", 1)
	doc += "routing:\n"
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		if line != "" { // 空行不缩进，避免行尾空格
			line = "  " + line
		}
		doc += line + "\n"
	}
	file := dir + "domainrouter-routing.profile.yaml"
	if *update {
		if err := os.WriteFile(file, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	} else if got, _ := os.ReadFile(file); string(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))) != doc {
		t.Fatal("domainrouter-routing.profile.yaml 与 Fork 的结果不一致（go test ./internal/workspace/ -update 并审阅差异）")
	}
	a, b := render(t, dir+"domainrouter.profile.yaml"), render(t, file)
	// 文件头的配置名与更新时间不同（自己管理后规则在方案文档里，没有规则源的拉取时间），其余应逐字节相同
	strip := func(y []byte) string {
		lines := strings.Split(string(y), "\n")
		out := lines[:0]
		for _, x := range lines {
			if !strings.Contains(x, "M2 · domainrouter_v2") && !strings.HasPrefix(x, "# UPDATED:") {
				out = append(out, x)
			}
		}
		return strings.Join(out, "\n")
	}
	if strip(a.YAML) != strip(b.YAML) {
		t.Errorf("开始自己管理后输出变了：\n%s", firstDiff(strip(a.YAML), strip(b.YAML)))
	}
}

func firstDiff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la) && i < len(lb); i++ {
		if la[i] != lb[i] {
			return "第 " + itoa(i+1) + " 行：\n- " + la[i] + "\n+ " + lb[i]
		}
	}
	return "行数不同"
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
