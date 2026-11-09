package ruleset

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// matchRange 与 fork 的行为逐条对照（subexport.cpp，见 matcher.go 的注释）。
func TestMatchRange(t *testing.T) {
	cases := []struct {
		rng    string
		target int
		want   bool
	}{
		{"0", 0, true}, {"0", 1, false}, {"1,3", 3, true}, {"1,3", 2, false},
		{"1-3", 2, true}, {"1-3", 4, false}, {"3-1", 2, false},
		{"!1", 1, false}, {"!1", 2, true},
		{"0,!1", 0, true},  // !1 覆盖：先置 true，0≠1 保持 true
		{"1,!1", 1, false}, // !1 覆盖前面的命中
		{"!1,1", 1, true},  // 后面的精确匹配重新置 true
		{"!-3", 3, false},  // 负号被丢弃：比较的是 3
		{"!-3", -3, true},
		{"!1-3", 2, false}, {"!1-3", 5, true},
		{"3-", 2, true}, {"3-", 3, true}, {"3-", 4, false}, // n- 表示 ≤ n
		{"3+", 4, true}, {"3+", 3, true}, {"3+", 2, false}, // n+ 表示 ≥ n
		{"-1", -1, true}, {"abc", 0, false}, {"", 0, false},
	}
	for _, c := range cases {
		if got := matchRange(c.rng, c.target); got != c.want {
			t.Errorf("matchRange(%q, %d) = %v，期望 %v", c.rng, c.target, got, c.want)
		}
	}
}

func FuzzMatchRange(f *testing.F) {
	f.Add("1-3,!2,5+", 4)
	f.Fuzz(func(t *testing.T, rng string, target int) {
		_ = matchRange(rng, target)
		_ = parseMatcher("!!GROUPID=" + rng + "!!x")
	})
}

func TestParseGroupsValidation(t *testing.T) {
	src := `
[[custom_groups]]
name = "A"
type = "select"
rule = ["[]B", "[]DIRECT", "[]Missing"]

[[custom_groups]]
name = "B"
type = "url-test"
rule = ["[]A"]

[[custom_groups]]
name = "C,bad"
type = "relay"
rule = ["x"]

[[custom_groups]]
type = "select"
rule = ["x"]

[[custom_groups]]
name = "D"
type = "fallback"
url = "https://www.gstatic.com/generate_204"
interval = 300

[[custom_groups]]
name = "D"
type = "ssid"
rule = []
`
	var l diag.List
	groups := ParseGroups([]byte(src), "groups.toml", &l)
	CheckReferences(groups, &l)
	got := map[diag.Code]int{}
	for _, d := range l.Items() {
		got[d.Code]++
	}
	want := map[diag.Code]int{diag.GroupInvalid: 6, diag.GroupReference: 1, diag.GroupCycle: 1, diag.GroupTypeMihomo: 2}
	for c, n := range want {
		if got[c] != n {
			t.Errorf("%s：%d 条，期望 %d；全部：%v", c, got[c], n, l.Items())
		}
	}
}

func TestBuildRules(t *testing.T) {
	list := []Ruleset{
		{Group: "DIRECT", Ruleset: "https://r.example.com/clash-domain/Direct.yaml", Type: "clash-domain", Interval: 21600},
		{Group: "DIRECT", Ruleset: "https://r.example.com/clash-ipcidr/Direct.mrs", Type: "clash-ipcidr", Interval: 21600},
		{Group: "P", Ruleset: "https://r.example.com/x/direct.list", Type: "surge-ruleset", Interval: 86400},
		{Group: "P", Ruleset: "https://r.example.com/x/Bad.mrs", Type: "clash-classic", Interval: 86400},
		{Group: "P", Ruleset: "https://r.example.com/x/q.list", Type: "quantumultx", Interval: 86400},
		{Group: "DIRECT", Ruleset: "[]GEOIP,CN,no-resolve"},
		{Group: "P", Ruleset: "[]AND,((DOMAIN,a.example),(NETWORK,UDP))"},
		{Group: "🐟", Ruleset: "[]FINAL"},
	}
	var l diag.List
	providers, rules := BuildRules(list, &l)
	want := []string{
		"RULE-SET,Direct,DIRECT",
		"RULE-SET,Direct_2,DIRECT,no-resolve",
		"RULE-SET,direct_3,P",
		"GEOIP,CN,DIRECT,no-resolve",
		"AND,((DOMAIN,a.example),(NETWORK,UDP)),P",
		"MATCH,🐟",
	}
	if !slices.Equal(rules, want) {
		t.Fatalf("规则：\n%s\n期望：\n%s", strings.Join(rules, "\n"), strings.Join(want, "\n"))
	}
	if f := yamlx.Get(yamlx.Get(providers, "Direct_2"), "format"); f == nil || f.Value != "mrs" {
		t.Error("Direct_2 应为 format: mrs")
	}
	if f := yamlx.Get(yamlx.Get(providers, "direct_3"), "format"); f == nil || f.Value != "text" {
		t.Error("direct_3 应为 format: text")
	}
	if p := yamlx.Get(yamlx.Get(providers, "Direct"), "path"); p.Value != "./ruleset/Direct.yaml" {
		t.Errorf("path：%s", p.Value)
	}
	if !l.Has(diag.RulesetInvalid) || !l.Has(diag.RulesetRenamed) || !l.Has(diag.RulesetLimit) {
		t.Errorf("诊断不全：%v", l.Items())
	}
}

func TestInsertGroup(t *testing.T) {
	cases := map[string]string{
		"GEOIP,CN":                        "GEOIP,CN,G",
		"GEOIP,CN,no-resolve":             "GEOIP,CN,G,no-resolve",
		"IP-CIDR,192.0.2.0/24,no-resolve": "IP-CIDR,192.0.2.0/24,G,no-resolve",
		"MATCH":                           "MATCH,G",
		"SRC-IP-CIDR,192.0.2.1/32,src":    "SRC-IP-CIDR,192.0.2.1/32,G,src",
		"OR,((DOMAIN,a),(DOMAIN,b))":      "OR,((DOMAIN,a),(DOMAIN,b)),G",
	}
	for in, want := range cases {
		if got := InsertGroup(in, "G"); got != want {
			t.Errorf("InsertGroup(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestINI(t *testing.T) {
	b, err := os.ReadFile("../../testdata/rules/sample.ini")
	if err != nil {
		t.Fatal(err)
	}
	var l diag.List
	src := ParseINI(b, "sample.ini", &l)
	if len(src.Rulesets) != 5 || len(src.Groups) != 6 {
		t.Fatalf("rulesets %d、groups %d；%v", len(src.Rulesets), len(src.Groups), l.Items())
	}
	r := src.Rulesets[1]
	if r.Type != "clash-domain" || r.Interval != 43200 || r.Ruleset != "https://rules.example.com/clash-domain/Reject.yaml" {
		t.Errorf("带类型前缀与间隔的 ruleset：%+v", r)
	}
	auto := src.Groups[1]
	if auto.Extra["url"] != "http://www.gstatic.com/generate_204" || auto.Extra["interval"] != int64(300) || auto.Extra["tolerance"] != int64(50) {
		t.Errorf("url-test 的时间参数：%+v", auto.Extra)
	}
	hk := src.Groups[2]
	if !slices.Equal(hk.Use, []string{"机场 B"}) || !slices.Equal(hk.Rule, []string{"(港|HK)"}) || hk.Extra["timeout"] != int64(5) {
		t.Errorf("!!PROVIDER 与 fallback：%+v", hk)
	}
	ops := src.NodeOps
	if len(ops.Rename) != 1 || len(ops.RegionRules) != 1 || ops.RegionRules[0].Region != "HK" || ops.Include != "(港|日|新)" || ops.Exclude != "(测试|过期)" {
		t.Errorf("节点整理映射：%+v", ops)
	}
	if !l.Has(diag.IniUnsupported) {
		t.Error("clash_rule_base 应报告为不支持")
	}
}

// TestACL4SSR 读取本机的 ACL4SSR 参考仓库（.cache/ref/acl4ssr），没有时跳过。
func TestACL4SSR(t *testing.T) {
	files, _ := filepath.Glob("../../.cache/ref/acl4ssr/Clash/config/ACL4SSR_Online*.ini")
	if len(files) == 0 {
		t.Skip("没有 ACL4SSR 参考仓库")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var l diag.List
		src := ParseINI(b, filepath.Base(f), &l)
		CheckReferences(src.Groups, &l)
		if len(src.Groups) == 0 || len(src.Rulesets) == 0 {
			t.Errorf("%s：没有解析出组或规则", f)
		}
		for _, d := range l.Items() {
			if d.Severity == diag.Error {
				t.Errorf("%s：%s", filepath.Base(f), d)
			}
		}
	}
}

func TestDomainrouterRelease(t *testing.T) {
	var l diag.List
	g, _ := os.ReadFile("../../testdata/rules/domainrouter/groups.toml")
	r, _ := os.ReadFile("../../testdata/rules/domainrouter/rulesets.toml")
	groups := ParseGroups(g, "groups.toml", &l)
	CheckReferences(groups, &l)
	rs := ParseRulesets(r, "rulesets.toml", &l)
	providers, rules := BuildRules(rs, &l)
	for _, d := range l.Items() {
		if d.Severity == diag.Error {
			t.Errorf("真实规则源不应有错误：%s", d)
		}
	}
	if len(groups) != 23 || len(rs) != 66 || len(rules) != 66 || len(providers.Content)/2 != 59 {
		t.Errorf("groups %d、rulesets %d、rules %d、providers %d", len(groups), len(rs), len(rules), len(providers.Content)/2)
	}
	if rules[len(rules)-1] != "MATCH,🐳 漏网の鱼" {
		t.Errorf("最后一条规则：%s", rules[len(rules)-1])
	}
}

func testNodes() []*node.Node {
	mk := func(name, typ, region string, src int, gid int, srcName string, rate float64, tags ...string) *node.Node {
		return &node.Node{Name: name, Type: typ, Server: "192.0.2.1", Port: 443, Mapping: yamlx.Map("name", name, "type", typ), Region: node.Region{Code: region}, SourceIndex: src, GroupID: gid, SourceName: srcName, Rate: rate, Tags: tags}
	}
	return []*node.Node{
		mk("🇭🇰 港 01", "ss", "HK", 0, 0, "机场 A", 1),
		mk("🇯🇵 日 01", "vmess", "JP", 0, 0, "机场 A", 2),
		mk("🇭🇰 HK IEPL", "trojan", "HK", 1, -1, "我的 VPS", 1, "IEPL"),
		mk("🇭🇰 港 02 premium", "hysteria2", "HK", 2, 1, "机场 B", 0.5),
		mk("🇺🇸 美 01", "vless", "US", 2, 1, "机场 B", 1),
	}
}

func names(g *yaml.Node, key string) []string {
	var out []string
	if s := yamlx.Get(g, key); s != nil {
		for _, it := range s.Content {
			out = append(out, it.Value)
		}
	}
	return out
}

func TestExpand(t *testing.T) {
	src := `
[[custom_groups]]
name = "订阅 0 香港"
type = "select"
rule = ["!!GROUPID=0!!(港|HK)"]

[[custom_groups]]
name = "订阅 1"
type = "select"
rule = ["!!GROUPID=1"]

[[custom_groups]]
name = "香港"
type = "url-test"
url = "https://www.gstatic.com/generate_204"
interval = 300
tolerance = 50
timeout = 5
icon = "https://example.com/hk.png"
weird = 1
rule = ["[]DIRECT", "@select", "(港)"]

[custom_groups.select]
region = ["HK"]
exclude = { name = "(?i)premium" }

[[custom_groups]]
name = "低倍率"
type = "select"
rule = ["[]DIRECT"]

[custom_groups.select]
rate = { max = 0.5 }
sort = "rate"

[[custom_groups]]
name = "空"
type = "select"
rule = ["(不存在)"]

[[custom_groups]]
name = "协议"
type = "select"
rule = ["!!TYPE=VMESS|VLESS", "!!PORT=443", "!!SERVER=^192"]
`
	var l diag.List
	groups := ParseGroups([]byte(src), "g.toml", &l)
	out := Expand(ExpandInput{Groups: groups, Nodes: testNodes(), GroupField: func(_, k string) bool { return k != "weird" }}, &l)
	byName := map[string]*yaml.Node{}
	for _, g := range out.Content {
		byName[yamlx.Get(g, "name").Value] = g
	}
	check := func(group string, want ...string) {
		t.Helper()
		if got := names(byName[group], "proxies"); !slices.Equal(got, want) {
			t.Errorf("%s：%v，期望 %v", group, got, want)
		}
	}
	check("订阅 0 香港", "🇭🇰 港 01")
	check("订阅 1", "🇭🇰 港 02 premium", "🇺🇸 美 01")
	check("香港", "DIRECT", "🇭🇰 港 01", "🇭🇰 HK IEPL", "🇭🇰 港 02 premium")
	check("低倍率", "DIRECT", "🇭🇰 港 02 premium")
	check("空", "COMPATIBLE")
	// 条目顺序：TYPE 先命中日、美，PORT 再补上其余节点
	check("协议", "🇯🇵 日 01", "🇺🇸 美 01", "🇭🇰 港 01", "🇭🇰 HK IEPL", "🇭🇰 港 02 premium")
	hk := byName["香港"]
	if yamlx.Get(hk, "timeout") != nil || yamlx.Get(hk, "icon") == nil || yamlx.Get(hk, "tolerance") == nil {
		t.Error("timeout 应被忽略，icon 与 tolerance 应透传")
	}
	for _, c := range []diag.Code{diag.GroupKeyIgnored, diag.GroupOffCatalog, diag.GroupEmpty, diag.GroupIDShift} {
		if !l.Has(c) {
			t.Errorf("缺少诊断 %s", c)
		}
	}
}

func TestExpandProviderMode(t *testing.T) {
	src := `
[[custom_groups]]
name = "香港"
type = "select"
rule = ["(港|HK)", "!!GROUPID=1"]

[custom_groups.select]
protocol = ["ss"]
region = ["HK"]
`
	var l diag.List
	groups := ParseGroups([]byte(src), "g.toml", &l)
	ps := []ProviderSource{{Name: "机场 A", SourceName: "机场 A", GroupID: 0}, {Name: "机场 B", SourceName: "机场 B", GroupID: 1}}
	out := Expand(ExpandInput{Groups: groups, Providers: ps}, &l)
	g := out.Content[0]
	if got := names(g, "use"); !slices.Equal(got, []string{"机场 A", "机场 B"}) {
		t.Errorf("use：%v", got)
	}
	if f := yamlx.Get(g, "filter"); f == nil || f.Value != "(港|HK)" {
		t.Errorf("filter：%v", f)
	}
	if et := yamlx.Get(g, "exclude-type"); et == nil || slices.Contains(strings.Split(et.Value, "|"), "ss") || !slices.Contains(strings.Split(et.Value, "|"), "vmess") {
		t.Errorf("exclude-type 应为 ss 的补集：%v", et)
	}
	if !l.Has(diag.GroupProviderNote) {
		t.Error("region 条件在 provider 模式下应警告")
	}
}

// 每个预设都能加载；内联模板解析、校验、展开均无错误。
func TestPresets(t *testing.T) {
	ps, err := LoadPresets(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) < 6 || ps[0].ID != "domainrouter" {
		t.Fatalf("预设：%d 个，首个 %s", len(ps), ps[0].ID)
	}
	for _, p := range ps {
		for _, v := range p.Variants {
			if v.GroupsText == "" {
				continue
			}
			var l diag.List
			groups := ParseGroups([]byte(v.GroupsText), p.ID+"/groups", &l)
			CheckReferences(groups, &l)
			rs := ParseRulesets([]byte(v.RulesetsText), p.ID+"/rulesets", &l)
			_, rules := BuildRules(rs, &l)
			_ = Expand(ExpandInput{Groups: groups, Nodes: testNodes()}, &l)
			for _, d := range l.Items() {
				if d.Severity == diag.Error {
					t.Errorf("%s/%s：%s", p.ID, v.ID, d)
				}
			}
			if len(rules) == 0 || !strings.HasPrefix(rules[len(rules)-1], "MATCH,") {
				t.Errorf("%s/%s：最后一条规则应为 MATCH", p.ID, v.ID)
			}
		}
	}
}

// forkRules 按 angwz/subconverter@a33d49e templates.cpp 的原生 provider 路径计算 rules：
// provider 名取自 findFileName，重名（区分大小写）追加 _2；ipcidr 追加 ,no-resolve；
// 内联规则追加组名后，含三个及以上逗号时套用 ^(.*?,.*?)(,.*)(,.*)$ → $1$3$2；FINAL 改为 MATCH。
func forkRules(list []Ruleset) []string {
	var rules, names []string
	moveRe := regexp.MustCompile(`^(.*?,.*?)(,.*)(,.*)$`)
	for _, r := range list {
		if strings.HasPrefix(r.Ruleset, "[]") {
			line := r.Ruleset[2:]
			if strings.HasPrefix(line, "FINAL") {
				line = "MATCH" + line[5:]
			}
			line += "," + r.Group
			if strings.Count(line, ",") >= 3 {
				line = moveRe.ReplaceAllString(line, "$1$3$2")
			}
			rules = append(rules, line)
			continue
		}
		name, _ := fileOf(r.Ruleset)
		base := name
		for i := 2; slices.Contains(names, name); i++ {
			name = base + "_" + strconv.Itoa(i)
		}
		names = append(names, name)
		line := "RULE-SET," + name + "," + r.Group
		if r.Type == "clash-ipcidr" {
			line += ",no-resolve"
		}
		rules = append(rules, line)
	}
	return rules
}

// 差异测试（第 6.6 节）：真实规则源上，本项目的 rules 与 fork 的结果逐条一致。
// 有意的差异（provider 的 path、format，重名判断不区分大小写）不影响 rules 本身，见 subconverter-parity。
func TestParityRules(t *testing.T) {
	check := func(name string, list []Ruleset) {
		var l diag.List
		_, ours := BuildRules(list, &l)
		fork := forkRules(list)
		if !slices.Equal(ours, fork) {
			for i := range max(len(ours), len(fork)) {
				var a, b string
				if i < len(ours) {
					a = ours[i]
				}
				if i < len(fork) {
					b = fork[i]
				}
				if a != b {
					t.Errorf("%s 第 %d 条：本项目 %q，fork %q", name, i+1, a, b)
				}
			}
		}
	}
	var l diag.List
	r, _ := os.ReadFile("../../testdata/rules/domainrouter/rulesets.toml")
	check("domainrouter rulesets.toml", ParseRulesets(r, "rulesets.toml", &l))
	r2, _ := os.ReadFile("../../testdata/rules/domainrouter/rulesets-mrs.toml")
	check("domainrouter rulesets-mrs.toml", ParseRulesets(r2, "rulesets-mrs.toml", &l))
	for _, f := range []string{"ACL4SSR_Online.ini", "ACL4SSR_Online_Full.ini"} {
		b, err := os.ReadFile("../../.cache/ref/acl4ssr/Clash/config/" + f)
		if err != nil {
			continue // 没有本机参考仓库时只比较 domainrouter
		}
		check(f, ParseINI(b, f, &l).Rulesets)
	}
}

// forkMembers 按 fork 的 groupGenerate 计算组成员：[]名称 直接追加（不去重）；
// 其余条目按节点顺序匹配，节点名不在列表中才追加；结果为空且没有 use 时写 DIRECT。
func forkMembers(g Group, nodes []*node.Node) []string {
	var list []string
	for _, r := range g.Rule {
		if strings.HasPrefix(r, "[]") {
			list = append(list, r[2:])
			continue
		}
		var l diag.List
		for _, n := range parseMatcher(r).pick(nodes, g.Name, g.Where, &l) {
			if !slices.Contains(list, n.Name) {
				list = append(list, n.Name)
			}
		}
	}
	if len(list) == 0 && len(g.Use) == 0 {
		list = []string{"DIRECT"}
	}
	return list
}

func TestParityGroups(t *testing.T) {
	var l diag.List
	b, _ := os.ReadFile("../../testdata/rules/domainrouter/groups.toml")
	groups := ParseGroups(b, "groups.toml", &l)
	nodes := testNodes()
	out := Expand(ExpandInput{Groups: groups, Nodes: nodes}, &l)
	for i, g := range groups {
		ours := names(out.Content[i], "proxies")
		fork := forkMembers(g, nodes)
		emptyDiff := slices.Equal(fork, []string{"DIRECT"}) && slices.Equal(ours, []string{"COMPATIBLE"}) // 有意的差异，见 subconverter-parity
		if !slices.Equal(ours, fork) && !emptyDiff {
			t.Errorf("%s：本项目 %v，fork %v", g.Name, ours, fork)
		}
	}
}
