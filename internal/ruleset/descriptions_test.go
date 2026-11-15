package ruleset

import (
	"os"
	"strings"
	"testing"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/diag"
)

// presetAddresses 收集预设会引用到的规则地址（离线）：预设里内联的 rulesets、domainrouter_v2 发布的两份
// rulesets（testdata 副本），以及 ACL4SSR Online 系列 ini 引用的地址清单（testdata）。
func presetAddresses(t *testing.T) []string {
	t.Helper()
	var out []string
	add := func(rs []Ruleset) {
		for _, r := range rs {
			if !strings.HasPrefix(r.Ruleset, "[]") {
				out = append(out, r.Ruleset)
			}
		}
	}
	ps, err := LoadPresets(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	var l diag.List
	for _, p := range ps {
		for _, v := range p.Variants {
			if v.RulesetsText != "" {
				add(ParseRulesets([]byte(v.RulesetsText), p.ID+"/rulesets", &l))
			}
		}
	}
	for _, f := range []string{"rulesets.toml", "rulesets-mrs.toml"} {
		b, err := os.ReadFile("../../testdata/rules/domainrouter/" + f)
		if err != nil {
			t.Fatal(err)
		}
		add(ParseRulesets(b, f, &l))
	}
	b, err := os.ReadFile("../../testdata/rules/acl4ssr-online-rulesets.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func TestRulesetDescriptions(t *testing.T) {
	d, err := LoadDescriptions(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	addrs := presetAddresses(t)
	if len(addrs) < 100 {
		t.Fatalf("预设地址只有 %d 个，收集方式可能失效", len(addrs))
	}
	for _, s := range d.Sources {
		for j, set := range s.Sets {
			n := 0
			for _, a := range addrs {
				if set.re.MatchString(a) {
					n++
				}
			}
			if n == 0 {
				t.Errorf("%s#%d（%s）没有匹配任何预设地址", s.ID, j, set.Match)
			}
		}
	}
	// 一个地址最多匹配一条说明，否则 Describe 的结果取决于书写顺序。
	described := 0
	for _, a := range addrs {
		hits := 0
		for _, s := range d.Sources {
			for _, set := range s.Sets {
				if set.re.MatchString(a) {
					hits++
				}
			}
		}
		if hits > 1 {
			t.Errorf("%s 被 %d 条说明匹配", a, hits)
		}
		if hits == 1 {
			described++
		}
	}
	t.Logf("预设地址 %d 个，有说明的 %d 个", len(addrs), described)

	got := d.Describe("https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/BanAD.list")
	if got == nil || got.Source != "acl4ssr" || got.License != "CC-BY-SA-4.0" || got.Text["zh"] == "" || got.Text["en"] == "" {
		t.Errorf("Describe: %+v", got)
	}
	if d.Describe("https://example.com/rules.yaml") != nil {
		t.Error("未收录的地址应返回 nil")
	}
}
