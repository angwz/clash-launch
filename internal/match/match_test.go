package match

import (
	"bytes"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"

	P "github.com/metacubex/mihomo/constant/provider"
	"github.com/metacubex/mihomo/rules/provider"
)

// mrsOf 用内核导出的 ConvertToMrs 把 text 规则转换为 MRS（与 mihomo convert-ruleset 相同的路径）。
func mrsOf(t testing.TB, behavior P.RuleBehavior, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := provider.ConvertToMrs([]byte(text), behavior, P.TextRule, &buf); err != nil {
		t.Fatalf("ConvertToMrs: %v", err)
	}
	return buf.Bytes()
}

func mustCompile(t testing.TB, behavior, format string, body []byte) *Set {
	t.Helper()
	s, err := Compile("t", behavior, format, body, Env{})
	if err != nil {
		t.Fatalf("Compile(%s, %s): %v", behavior, format, err)
	}
	return s
}

func req(t testing.TB, input string) Request {
	t.Helper()
	r, err := NewRequest(input, "", "")
	if err != nil {
		t.Fatalf("NewRequest(%q): %v", input, err)
	}
	return r
}

// traceSet 用一条 RULE-SET 行与兜底追踪单个规则集。
func traceSet(set *Set, r Request, params ...string) Step {
	raw := strings.Join(append([]string{"RULE-SET", "s", "X"}, params...), ",")
	lines := []Line{ParseLine(0, raw), ParseLine(1, "MATCH,F")}
	return Trace(lines, map[string]*Set{"s": set}, r, Env{}).Steps[0]
}

const domainText = `# 注释
// 另一种注释
+.example.com
.sub.example.net
exact.example.org

`

const domainYAML = `payload:
  # 注释
  - '+.example.com'
  - '.sub.example.net'
  - 'exact.example.org'
`

func TestDomainFormats(t *testing.T) {
	sets := map[string]*Set{
		"text": mustCompile(t, "domain", "text", []byte(domainText)),
		"yaml": mustCompile(t, "domain", "yaml", []byte(domainYAML)),
		"mrs":  mustCompile(t, "domain", "mrs", mrsOf(t, P.Domain, domainText)),
	}
	cases := []struct {
		host, result, evidence string
	}{
		{"example.com", ResultHit, "+.example.com"},
		{"www.example.com", ResultHit, "+.example.com"},
		{"a.sub.example.net", ResultHit, ".sub.example.net"},
		{"sub.example.net", ResultMiss, ""}, // . 前缀只覆盖子域名
		{"exact.example.org", ResultHit, "exact.example.org"},
		{"www.exact.example.org", ResultMiss, ""},
		{"example.net", ResultMiss, ""},
	}
	for name, set := range sets {
		if set.Count != 3 {
			t.Errorf("%s: Count = %d，期望 3", name, set.Count)
		}
		if set.IPDependent() {
			t.Errorf("%s: domain 规则集不应依赖 IP", name)
		}
		for _, c := range cases {
			got := traceSet(set, req(t, c.host))
			if got.Result != c.result || got.Evidence != c.evidence {
				t.Errorf("%s %s: 得到 %s %q，期望 %s %q", name, c.host, got.Result, got.Evidence, c.result, c.evidence)
			}
		}
	}
}

const cidrText = `192.0.2.0/25
192.0.2.128/25
2001:db8::/32
`

func TestIPCIDRFormats(t *testing.T) {
	sets := map[string]*Set{
		"text": mustCompile(t, "ipcidr", "text", []byte(cidrText)),
		"yaml": mustCompile(t, "ipcidr", "yaml", []byte("payload:\n  - 192.0.2.0/25\n  - 192.0.2.128/25\n  - 2001:db8::/32\n")),
		"mrs":  mustCompile(t, "ipcidr", "mrs", mrsOf(t, P.IPCIDR, cidrText)),
	}
	for name, set := range sets {
		if !set.IPDependent() {
			t.Errorf("%s: ipcidr 规则集应依赖 IP", name)
		}
		got := traceSet(set, req(t, "192.0.2.200"))
		// MRS 保存的是合并后的网段，依据因此是合并结果。
		want := "192.0.2.128/25"
		if name == "mrs" {
			want = "192.0.2.0/24"
		}
		if got.Result != ResultHit || got.Evidence != want {
			t.Errorf("%s: 得到 %s %q，期望 hit %q", name, got.Result, got.Evidence, want)
		}
		if got := traceSet(set, req(t, "[2001:db8::1]:443")); got.Result != ResultHit {
			t.Errorf("%s: IPv6 得到 %s", name, got.Result)
		}
		if got := traceSet(set, req(t, "198.51.100.1")); got.Result != ResultMiss {
			t.Errorf("%s: 不在网段内得到 %s", name, got.Result)
		}
		// 域名请求：没有 no-resolve 时内核会在这里解析 DNS，追踪记为 needs-ip。
		out := Trace([]Line{ParseLine(0, "RULE-SET,s,X"), ParseLine(1, "MATCH,F")}, map[string]*Set{"s": set}, req(t, "www.example.com"), Env{})
		if out.Steps[0].Result != ResultNeedsIP || out.NeedsIPAt != 0 || !slices.Equal(out.IPCandidates, []int{0}) || out.Hit != 1 {
			t.Errorf("%s: 域名请求 %+v", name, out)
		}
		// no-resolve：不触发解析，确定不命中。
		out = Trace([]Line{ParseLine(0, "RULE-SET,s,X,no-resolve"), ParseLine(1, "MATCH,F")}, map[string]*Set{"s": set}, req(t, "www.example.com"), Env{})
		if out.Steps[0].Result != ResultMiss || out.NeedsIPAt != -1 || len(out.IPCandidates) != 0 {
			t.Errorf("%s: no-resolve %+v", name, out)
		}
	}
}

const classicalText = `DOMAIN-SUFFIX,example.com
IP-CIDR,192.0.2.0/24,no-resolve
PROCESS-NAME,app.example
AND,((RULE-SET,other),(DOMAIN-SUFFIX,example.net))
AND,((DST-PORT,443),(NETWORK,UDP))
MATCH
RULE-SET,other
`

func TestClassical(t *testing.T) {
	yamlBody := "payload:\n"
	for _, l := range strings.Split(strings.TrimSpace(classicalText), "\n") {
		yamlBody += "  - '" + l + "'\n"
	}
	for format, body := range map[string]string{"text": classicalText, "yaml": yamlBody} {
		set := mustCompile(t, "classical", format, []byte(body))
		// MATCH、RULE-SET 在 classical 中不被内核接受，不计数。
		if set.Count != 5 {
			t.Errorf("%s: Count = %d，期望 5", format, set.Count)
		}
		if !set.IPDependent() {
			t.Errorf("%s: 含 IP-CIDR 的 classical 应依赖 IP", format)
		}
		cases := []struct {
			input, process, network  string
			result, evidence, reason string
		}{
			{"www.example.com", "", "", ResultHit, "DOMAIN-SUFFIX,example.com", ""},
			{"192.0.2.1", "", "", ResultHit, "IP-CIDR,192.0.2.0/24,no-resolve", ""},
			{"other.example.net", "app.example", "", ResultHit, "PROCESS-NAME,app.example", ""},
			{"other.example.net", `C:\Apps\app.example`, "", ResultHit, "PROCESS-NAME,app.example", ""},
			// 没填应用：PROCESS-NAME 无法判断；逻辑规则里的 RULE-SET 同样无法判断，报告第一个原因。
			{"other.example.net", "", "", ResultUnknown, "", ReasonNeedsProcess},
			{"other.example.net", "other.example", "", ResultUnknown, "", ReasonLogicRuleSet},
			// AND 的另一项已确定不成立时，逻辑规则可以判断。
			{"other.example.net", "other.example", "udp", ResultUnknown, "", ReasonLogicRuleSet},
			{"other.example.net:443", "other.example", "udp", ResultHit, "AND,((DST-PORT,443),(NETWORK,UDP))", ""},
		}
		for _, c := range cases {
			r, err := NewRequest(c.input, c.process, c.network)
			if err != nil {
				t.Fatal(err)
			}
			got := traceSet(set, r)
			if got.Result != c.result || got.Evidence != c.evidence || got.Reason != c.reason {
				t.Errorf("%s %s/%s/%s: 得到 %+v，期望 %s %q %q", format, c.input, c.process, c.network, got, c.result, c.evidence, c.reason)
			}
		}
	}
}

func TestTraceLines(t *testing.T) {
	dom := mustCompile(t, "domain", "text", []byte("+.example.com\n"))
	cidr := mustCompile(t, "ipcidr", "text", []byte("192.0.2.0/24\n"))
	sets := map[string]*Set{"dom": dom, "cidr": cidr}
	rules := []string{
		"DOMAIN-SUFFIX,example.net,A",               // 0
		"AND,((DST-PORT,443),(NETWORK,UDP)),REJECT", // 1 常见的屏蔽 QUIC 写法
		"AND,((RULE-SET,dom),(NETWORK,TCP)),B",      // 2
		"PROCESS-NAME,app.example,C",                // 3
		"RULE-SET,missing,D",                        // 4
		"RULE-SET,dom,E,src",                        // 5
		"SUB-RULE,(NETWORK,TCP),sub",                // 6
		"RULE-SET,cidr,F,no-resolve",                // 7
		"RULE-SET,dom,G",                            // 8
		"GEOIP,CN,H",                                // 9
		"IP-CIDR,198.51.100.0/24,I,no-resolve",      // 10
		"NOT,((DOMAIN,keep.example.org)),J",         // 11
		"MATCH,K",                                   // 12
	}
	lines := make([]Line, len(rules))
	for i, r := range rules {
		lines[i] = ParseLine(i, r)
	}
	results := func(o Outcome) string {
		var b strings.Builder
		for _, s := range o.Steps {
			fmt.Fprintf(&b, "%d:%s", s.Index, s.Result)
			if s.Reason != "" {
				b.WriteString("/" + s.Reason)
			}
			b.WriteByte(' ')
		}
		return strings.TrimSpace(b.String())
	}
	cases := []struct {
		input, network string
		want           string
		needsIP        int
		candidates     []int
	}{
		{"www.example.com", "", "0:miss 1:miss 2:unknown/logic-ruleset 3:unknown/needs-process 4:unknown/no-content 5:unknown/needs-connection 6:unknown/sub-rule 7:miss 8:hit", -1, nil},
		// UDP 未知端口：屏蔽 QUIC 的行无法判断；RULE-SET 所在的 AND 因 NETWORK 不成立而确定不命中。
		{"www.example.com", "udp", "0:miss 1:unknown/needs-connection 2:miss 3:unknown/needs-process 4:unknown/no-content 5:unknown/needs-connection 6:unknown/sub-rule 7:miss 8:hit", -1, nil},
		{"www.example.com:443", "udp", "0:miss 1:hit", -1, nil},
		// 没有地理数据：GEOIP 无法判断，但内核会在这里解析，之后的 no-resolve IP 规则成为候选。
		{"keep.example.org", "", "0:miss 1:miss 2:unknown/logic-ruleset 3:unknown/needs-process 4:unknown/no-content 5:unknown/needs-connection 6:unknown/sub-rule 7:miss 8:miss 9:unknown/geodata 10:needs-ip 11:miss 12:hit", 9, []int{9, 10}},
		{"198.51.100.7", "", "0:miss 1:miss 2:unknown/logic-ruleset 3:unknown/needs-process 4:unknown/no-content 5:unknown/needs-connection 6:unknown/sub-rule 7:miss 8:miss 9:unknown/geodata 10:hit", -1, nil},
	}
	for _, c := range cases {
		r, err := NewRequest(c.input, "", c.network)
		if err != nil {
			t.Fatal(err)
		}
		out := Trace(lines, sets, r, Env{})
		if got := results(out); got != c.want {
			t.Errorf("%s %s:\n得到 %s\n期望 %s", c.input, c.network, got, c.want)
		}
		if out.NeedsIPAt != c.needsIP || !slices.Equal(out.IPCandidates, c.candidates) {
			t.Errorf("%s: NeedsIPAt=%d IPCandidates=%v，期望 %d %v", c.input, out.NeedsIPAt, out.IPCandidates, c.needsIP, c.candidates)
		}
		if last := out.Steps[len(out.Steps)-1]; last.Result != ResultHit || out.Hit != last.Index {
			t.Errorf("%s: 最后一步应为命中：%+v", c.input, out)
		}
	}
	// 命中依据：普通规则给出类型与载荷，规则集给出命中的条目。
	out := Trace(lines, sets, req(t, "www.example.com"), Env{})
	if e := out.Steps[len(out.Steps)-1].Evidence; e != "+.example.com" {
		t.Errorf("依据 = %q", e)
	}
	// TraceFrom：调用方因目标不可用跳过命中行后从下一行继续。
	if o := TraceFrom(lines, 9, sets, req(t, "www.example.com"), Env{}); o.Steps[0].Index != 9 || o.Hit != 11 {
		t.Errorf("TraceFrom: %+v", o)
	}
}

func TestNewRequest(t *testing.T) {
	cases := []struct {
		in, process, network string
		want                 Request
		bad                  bool
	}{
		{in: "WWW.Example.COM.", want: Request{Host: "www.example.com", Network: "tcp"}},
		{in: "https://user@www.example.com:8443/a?b#c", want: Request{Host: "www.example.com", Port: 8443, Network: "tcp"}},
		{in: "192.0.2.1:53", network: "UDP", want: Request{IP: netip.MustParseAddr("192.0.2.1"), Port: 53, Network: "udp"}},
		{in: "::ffff:192.0.2.1", want: Request{IP: netip.MustParseAddr("192.0.2.1"), Network: "tcp"}},
		{in: "[2001:db8::1]", want: Request{IP: netip.MustParseAddr("2001:db8::1"), Network: "tcp"}},
		{in: "2001:db8::1", want: Request{IP: netip.MustParseAddr("2001:db8::1"), Network: "tcp"}},
		{in: "www.example.com", process: "/usr/bin/app", want: Request{Host: "www.example.com", Process: "app", ProcessPath: "/usr/bin/app", Network: "tcp"}},
		{in: "", bad: true},
		{in: "a b.example.com", bad: true},
		{in: "www.example.com:0", bad: true},
		{in: "www.example.com:70000", bad: true},
		{in: "www.example.com", network: "icmp", bad: true},
	}
	for _, c := range cases {
		got, err := NewRequest(c.in, c.process, c.network)
		if c.bad {
			if err == nil {
				t.Errorf("%q: 应当报错，得到 %+v", c.in, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%q: 得到 %+v %v，期望 %+v", c.in, got, err, c.want)
		}
	}
}

// TestParseParity 用内核自己的 rulesParse（经 ConvertToMrs 到达）对照本包移植的逐行解析：
// 同一份 text、yaml 内容，两条路径得到的条数与匹配结果一致。
func TestParseParity(t *testing.T) {
	bodies := []struct {
		behavior P.RuleBehavior
		name     string
		format   P.RuleFormat
		body     string
		probes   []string
	}{
		{P.Domain, "domain", P.TextRule, " +.example.com \r\n#x\n//y\n\n*.w.example.net\nexample.org\nbad/slash.example.com\n", []string{"a.example.com", "x.w.example.net", "example.org", "a.example.org"}},
		{P.Domain, "domain", P.YamlRule, "# head\npayload:\n  - '+.example.com'\n\n  - \"*.w.example.net\"\n  - example.org # tail\n", []string{"a.example.com", "x.w.example.net", "example.org"}},
		{P.IPCIDR, "ipcidr", P.TextRule, "192.0.2.0/24\nnot-a-cidr\n2001:db8::/48\n", []string{"192.0.2.9", "2001:db8::1", "2001:db8:1::1"}},
		{P.IPCIDR, "ipcidr", P.YamlRule, "payload:\n  - 192.0.2.0/24\n  - '2001:db8::/48'\n", []string{"192.0.2.9", "2001:db8::1"}},
	}
	for i, b := range bodies {
		var mrs bytes.Buffer
		if err := provider.ConvertToMrs([]byte(b.body), b.behavior, b.format, &mrs); err != nil {
			t.Fatalf("#%d ConvertToMrs: %v", i, err)
		}
		ours := mustCompile(t, b.name, map[P.RuleFormat]string{P.TextRule: "text", P.YamlRule: "yaml"}[b.format], []byte(b.body))
		kernel := mustCompile(t, b.name, "mrs", mrs.Bytes())
		if ours.Count != kernel.Count {
			t.Errorf("#%d: Count %d ≠ 内核 %d", i, ours.Count, kernel.Count)
		}
		for _, p := range b.probes {
			r := req(t, p)
			if a, k := traceSet(ours, r).Result, traceSet(kernel, r).Result; a != k {
				t.Errorf("#%d %s: %s ≠ 内核 %s", i, p, a, k)
			}
		}
	}
	// yaml 没有 payload 头时内核报错。
	if _, err := Compile("t", "domain", "yaml", []byte("example.com"), Env{}); err == nil {
		t.Error("没有 payload 头的单行 yaml 应当报错")
	}
}

func TestCompileErrors(t *testing.T) {
	cases := map[string]func() error{
		"过大": func() error {
			_, err := Compile("t", "domain", "text", make([]byte, MaxBytes+1), Env{})
			return err
		},
		"未知行为": func() error { _, err := Compile("t", "nope", "text", nil, Env{}); return err },
		"未知格式": func() error { _, err := Compile("t", "domain", "nope", nil, Env{}); return err },
		"classical 没有 mrs": func() error {
			_, err := Compile("t", "classical", "mrs", nil, Env{})
			return err
		},
		"不是 zstd": func() error { _, err := Compile("t", "domain", "mrs", []byte("plain"), Env{}); return err },
		"行为不符": func() error {
			_, err := Compile("t", "ipcidr", "mrs", mrsOf(t, P.Domain, "example.com\n"), Env{})
			return err
		},
	}
	for name, f := range cases {
		if f() == nil {
			t.Errorf("%s: 应当报错", name)
		}
	}
}

func TestCompileInline(t *testing.T) {
	set, err := CompileInline("t", "classical", []string{"DOMAIN-SUFFIX,example.com", "", " IP-CIDR,192.0.2.0/24"}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if set.Count != 2 || set.Format != "inline" {
		t.Errorf("得到 %+v", set)
	}
	if got := traceSet(set, req(t, "www.example.com")); got.Result != ResultHit {
		t.Errorf("得到 %+v", got)
	}
}

// 没有地理数据时，GEOIP、GEOSITE、IP-ASN 一律无法判断（不调用内核会自动下载的构造函数）。
func TestGeoWithoutData(t *testing.T) {
	for _, raw := range []string{"GEOIP,CN,X", "GEOSITE,cn,X", "IP-ASN,64496,X", "OR,((GEOSITE,cn),(DOMAIN,never.example.org)),X"} {
		out := Trace([]Line{ParseLine(0, raw), ParseLine(1, "MATCH,F")}, nil, req(t, "www.example.com"), Env{})
		if s := out.Steps[0]; s.Result != ResultUnknown || s.Reason != ReasonGeodata {
			t.Errorf("%s: %+v", raw, s)
		}
	}
	// GEOIP,lan 不需要地理数据。
	out := Trace([]Line{ParseLine(0, "GEOIP,lan,X"), ParseLine(1, "MATCH,F")}, nil, req(t, "192.0.2.1"), Env{})
	if s := out.Steps[0]; s.Result != ResultMiss {
		t.Errorf("GEOIP,lan: %+v", s)
	}
}

func TestSamples(t *testing.T) {
	dom := mustCompile(t, "domain", "text", []byte("+.example.com\n.sub.example.net\n*.w.example.org\n+.cn\nexact.example.org\n"))
	if got := dom.Samples(8); !slices.Equal(got, []string{"example.com", "www.sub.example.net", "exact.example.org"}) {
		t.Errorf("domain: %v", got)
	}
	mrs := mustCompile(t, "domain", "mrs", mrsOf(t, P.Domain, "+.example.com\nexact.example.org\n"))
	if got := mrs.Samples(8); len(got) != 2 {
		t.Errorf("mrs: %v", got)
	}
	cidr := mustCompile(t, "ipcidr", "text", []byte("192.0.2.0/24\n2001:db8::/32\n198.51.100.7/32\n"))
	if got := cidr.Samples(8); !slices.Equal(got, []string{"192.0.2.1", "2001:db8::1", "198.51.100.7"}) {
		t.Errorf("ipcidr: %v", got)
	}
	cl := mustCompile(t, "classical", "text", []byte("DOMAIN-KEYWORD,example\nDOMAIN-SUFFIX,example.com\nIP-CIDR,192.0.2.0/24,no-resolve\nSRC-IP-CIDR,192.0.2.0/24\n"))
	if got := cl.Samples(8); !slices.Equal(got, []string{"example.com", "192.0.2.1"}) {
		t.Errorf("classical: %v", got)
	}
	// 均匀分布：从 100 个候选中取 8 个
	var b strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "h%02d.example.com\n", i)
	}
	big := mustCompile(t, "domain", "text", []byte(b.String()))
	if got := big.Samples(8); len(got) != 8 || got[0] != "h00.example.com" || got[1] != "h12.example.com" || got[7] != "h87.example.com" {
		t.Errorf("均匀分布: %v", got)
	}
	if got := big.Samples(3); len(got) != 3 {
		t.Errorf("Samples(3): %v", got)
	}
}
