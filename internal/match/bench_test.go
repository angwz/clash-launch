package match

import (
	"fmt"
	"strings"
	"testing"
)

func domains(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "+.d%d.example.com\n", i)
	}
	return []byte(b.String())
}

// BenchmarkCompileDomain100k：10 万条 domain 规则（text）的编译耗时与内存，决定规则内容缓存的代价。
func BenchmarkCompileDomain100k(b *testing.B) {
	body := domains(100_000)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Compile("t", "domain", "text", body, Env{}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTrace：与维护者的 domainrouter_v2 配置同等规模（66 条规则，其中 60 条 RULE-SET，每个规则集 2000 条），
// 请求一路未命中直到兜底，是一次追踪的最坏情况。
func BenchmarkTrace(b *testing.B) {
	sets := map[string]*Set{}
	var lines []Line
	for i := 0; i < 60; i++ {
		name := fmt.Sprintf("s%d", i)
		var body strings.Builder
		for j := 0; j < 2000; j++ {
			fmt.Fprintf(&body, "+.r%d-%d.example.com\n", i, j)
		}
		behavior := "domain"
		if i%5 == 4 {
			behavior = "ipcidr"
			body.Reset()
			for j := 0; j < 2000; j++ {
				fmt.Fprintf(&body, "2001:db8:%x:%x::/64\n", i, j)
			}
		}
		s, err := Compile(name, behavior, "text", []byte(body.String()), Env{})
		if err != nil {
			b.Fatal(err)
		}
		sets[name] = s
		lines = append(lines, ParseLine(len(lines), "RULE-SET,"+name+",G"))
	}
	for _, r := range []string{"DOMAIN-SUFFIX,example.net,A", "DOMAIN-KEYWORD,never,B", "AND,((DST-PORT,443),(NETWORK,UDP)),REJECT", "IP-CIDR,192.0.2.0/24,C,no-resolve", "GEOIP,CN,D", "MATCH,F"} {
		lines = append(lines, ParseLine(len(lines), r))
	}
	r := req(b, "www.example.org:443")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if o := Trace(lines, sets, r, Env{}); o.Hit != len(lines)-1 {
			b.Fatalf("Hit = %d", o.Hit)
		}
	}
}
