package ruleset

import (
	"os"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/diag"
)

// routingOf 把 ToRouting 的结果按方案文档的方式解出（YAML 的整数是 int），再交给 FromRouting。
func routingOf(t *testing.T, src *Source) *Source {
	t.Helper()
	g, r := ToRouting(src)
	var groups, rulesets []map[string]any
	if err := g.Decode(&groups); err != nil {
		t.Fatal(err)
	}
	if err := r.Decode(&rulesets); err != nil {
		t.Fatal(err)
	}
	var l diag.List
	back := FromRouting(groups, rulesets, &l)
	for _, d := range l.Items() {
		if d.Severity == diag.Error {
			t.Errorf("往返后出现错误：%s", d)
		}
	}
	return back
}

func clean(s *Source) *Source {
	c := &Source{}
	for _, g := range s.Groups {
		g.Where, g.Select = "", nil // 选择器编译后的正则不可比较；SelectRaw 参与比较
		c.Groups = append(c.Groups, g)
	}
	for _, r := range s.Rulesets {
		r.Where = ""
		c.Rulesets = append(c.Rulesets, r)
	}
	return c
}

func TestRoutingRoundTrip(t *testing.T) {
	var l diag.List
	gt, _ := os.ReadFile("../../testdata/rules/domainrouter/groups.toml")
	rt, _ := os.ReadFile("../../testdata/rules/domainrouter/rulesets.toml")
	src := &Source{Groups: ParseGroups(gt, "groups.toml", &l), Rulesets: ParseRulesets(rt, "rulesets.toml", &l)}
	src.Groups = append(src.Groups, Group{Name: "SEL", Type: "url-test", Rule: []string{".*"}, Use: []string{"p"}, SelectRaw: map[string]any{"region": []any{"HK"}, "rate": map[string]any{"max": 1.5}},
		Extra: map[string]any{"url": "https://www.gstatic.com/generate_204", "interval": int64(300), "tolerance": int64(50), "lazy": false, "x-custom": "keep"}})
	DetectBundles(src.Rulesets)
	back := routingOf(t, src)
	if !reflect.DeepEqual(clean(src), clean(back)) {
		t.Errorf("FromRouting(ToRouting(x)) 与 x 不等价")
	}
	// 相同输入逐字节相同；键顺序固定
	g1, r1 := ToRouting(src)
	g2, r2 := ToRouting(src)
	b1, _ := yaml.Marshal([]*yaml.Node{g1, r1})
	b2, _ := yaml.Marshal([]*yaml.Node{g2, r2})
	if string(b1) != string(b2) {
		t.Error("两次输出不同")
	}
	last := g1.Content[len(g1.Content)-1]
	var keys []string
	for i := 0; i < len(last.Content); i += 2 {
		keys = append(keys, last.Content[i].Value)
	}
	if !reflect.DeepEqual(keys, []string{"name", "type", "rule", "select", "use", "url", "interval", "tolerance", "lazy", "x-custom"}) {
		t.Errorf("组的键顺序：%v", keys)
	}
}

func TestDetectBundles(t *testing.T) {
	u := func(kind, file string) string {
		return "https://raw.githubusercontent.com/angwz/domainrouter_v2/release/" + kind + "/" + file
	}
	list := []Ruleset{
		{Group: "DIRECT", Ruleset: u("clash-domain", "Direct.yaml")},
		{Group: "DIRECT", Ruleset: u("clash-ipcidr", "Direct-ipcidr.yaml")},
		{Group: "DIRECT", Ruleset: u("clash-classic", "Direct-classic.yaml")},
		{Group: "P", Ruleset: u("clash-domain", "AI.mrs")},
		{Group: "Q", Ruleset: u("clash-classic", "AI-classic.yaml")}, // 走向不同：不合并
		{Group: "P", Ruleset: "https://example.com/rules/Other.list"},
		{Group: "P", Ruleset: "[]MATCH"},
	}
	DetectBundles(list)
	var got []string
	for _, r := range list {
		got = append(got, r.Bundle)
	}
	if !reflect.DeepEqual(got, []string{"Direct", "Direct", "Direct", "", "", "", ""}) {
		t.Errorf("合集：%v", got)
	}
	// 合集被拆散（不相邻或走向不同）时提醒，输出照原顺序
	var l diag.List
	src := FromRouting(nil, []map[string]any{
		{"group": "A", "ruleset": "[]DOMAIN,a.example.com", "bundle": "X"},
		{"group": "A", "ruleset": "[]DOMAIN,b.example.com"},
		{"group": "A", "ruleset": "[]DOMAIN,c.example.com", "bundle": "X"},
		{"group": "A", "ruleset": "[]MATCH", "bundle": "Y", "interval": 300},
	}, &l)
	if len(src.Rulesets) != 4 || src.Rulesets[3].Interval != 300 {
		t.Errorf("YAML 的整数应按 TOML 相同处理：%+v", src.Rulesets)
	}
	n := 0
	for _, d := range l.Items() {
		if d.Code == diag.RoutingBundleSplit {
			n++
		}
	}
	if n != 1 {
		t.Errorf("bundle_split：%v", l.Items())
	}
}

func TestForkINI(t *testing.T) {
	ini, _ := os.ReadFile("../../testdata/rules/sample.ini")
	var l diag.List
	n := Fork(nil, nil, ini, "sample", "rs_sample", "2026-11-15T00:00:00Z", &l)
	var r struct {
		From             struct{ Rules, Digest, At string }
		Ops              []string `yaml:"ini-node-ops"`
		Groups, Rulesets []map[string]any
		Base             struct{ Groups, Rulesets []map[string]any }
	}
	if err := n.Decode(&r); err != nil {
		t.Fatal(err)
	}
	if r.From.Rules != "rs_sample" || len(r.From.Digest) != 71 || len(r.Groups) != 6 || len(r.Rulesets) != 5 || !reflect.DeepEqual(r.Groups, r.Base.Groups) {
		t.Errorf("Fork：%+v", r.From)
	}
	if !reflect.DeepEqual(r.Ops, []string{"rename=IEPL@专线", "emoji=(港|HK),🇭🇰", "include_remarks=(港|日|新)", "exclude_remarks=(测试|过期)"}) {
		t.Errorf("ini-node-ops：%q", r.Ops)
	}
	// 节点整理行与 ini 中的语义相同
	var l2 diag.List
	ops := ParseNodeOps(append(r.Ops, "ruleset=X,[]MATCH"), "routing.ini-node-ops", &l2)
	want := ParseINI(ini, "sample.ini", &diag.List{}).NodeOps
	if !reflect.DeepEqual(ops, want) {
		t.Errorf("ParseNodeOps：%+v，期望 %+v", ops, want)
	}
	if len(l2.Items()) != 1 || l2.Items()[0].Code != diag.IniUnsupported {
		t.Errorf("非节点整理的行应报告：%v", l2.Items())
	}
	// 上游没变，摘要不变
	n2 := Fork(nil, nil, ini, "sample", "rs_sample", "2027-01-01T00:00:00Z", &diag.List{})
	var r2 struct{ From struct{ Digest string } }
	_ = n2.Decode(&r2)
	if r2.From.Digest != r.From.Digest {
		t.Error("摘要应只取决于上游内容")
	}
}
