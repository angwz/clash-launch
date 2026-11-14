package ruleset

import (
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
)

// 只有香港节点：「新加坡」「台湾」两个地区组为空（反馈 #45）。
// 「台湾」只在「台湾服务」里作为选项，「台湾服务」因此也变空；「新加坡」被规则直接使用。
const pruneGroups = `
[[custom_groups]]
name = "节点选择"
type = "select"
rule = ["[]香港", "[]新加坡", "[]台湾", "[]DIRECT"]

[[custom_groups]]
name = "香港"
type = "select"
rule = ["(港)"]

[[custom_groups]]
name = "新加坡"
type = "select"
rule = ["(新加坡)"]

[[custom_groups]]
name = "台湾"
type = "select"
rule = ["(台湾)"]

[[custom_groups]]
name = "台湾服务"
type = "select"
rule = ["[]台湾"]
`

var pruneRules = []string{"DOMAIN-SUFFIX,sg.example.com,新加坡", "MATCH,节点选择"}

func pruneRun(t *testing.T, mode string) (map[string][]string, []string, *diag.List) {
	t.Helper()
	var l diag.List
	groups := ParseGroups([]byte(pruneGroups), "g.toml", &l)
	empty := map[string]bool{}
	seq := Expand(ExpandInput{Groups: groups, Nodes: testNodes(), Empty: empty}, &l)
	gone := PruneEmpty(seq, pruneRules, empty, mode, map[string]string{}, &l)
	got := map[string][]string{}
	for _, g := range seq.Content {
		got[g.Content[1].Value] = names(g, "proxies")
	}
	return got, gone, &l
}

func TestPruneEmpty(t *testing.T) {
	got, gone, l := pruneRun(t, "")
	if strings.Join(gone, ",") != "台湾,台湾服务" {
		t.Errorf("删除：%v", gone)
	}
	// 被规则使用的「新加坡」保留并指向 MATCH 目标；「节点选择」中不再列出空组，避免形成环
	if strings.Join(got["新加坡"], ",") != "节点选择" || strings.Join(got["节点选择"], ",") != "香港,DIRECT" {
		t.Errorf("组：%v", got)
	}
	if !l.Has(diag.GroupPruned) || !l.Has(diag.GroupEmptyFallback) || l.Has(diag.GroupEmpty) {
		t.Errorf("诊断：%v", l.Items())
	}
}

func TestPruneEmptyFallbackAndKeep(t *testing.T) {
	got, gone, _ := pruneRun(t, EmptyFallback)
	if len(gone) != 0 || strings.Join(got["台湾服务"], ",") != "节点选择" || strings.Join(got["台湾"], ",") != "节点选择" {
		t.Errorf("fallback：%v %v", gone, got)
	}
	got, gone, l := pruneRun(t, EmptyKeep)
	if len(gone) != 0 || strings.Join(got["台湾"], ",") != "COMPATIBLE" || !l.Has(diag.GroupEmpty) {
		t.Errorf("keep：%v %v", gone, got)
	}
}
