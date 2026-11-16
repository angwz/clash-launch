package api

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/service"
)

// 规则内容由本机的测试服务器提供（拉取器的白名单允许 127.0.0.1）。
const (
	rsDomain = "+.example.com\n.cdn.example.net\n"
	rsCIDR   = "192.0.2.0/24\n"
)

func ruleServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/clash-domain/Proxy.list":
			_, _ = w.Write([]byte(rsDomain))
		case "/clash-ipcidr/Proxy-ipcidr.list":
			_, _ = w.Write([]byte(rsCIDR))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// routingProfile 建立一个引用本机规则内容的规则源与方案，返回方案文档。
func routingProfile(t *testing.T, h *harness, c *client, base string) (string, string) {
	t.Helper()
	groups := `[[custom_groups]]
name = "PROXY"
type = "select"
rule = ["[]AUTO", "[]DIRECT", ".*"]

[[custom_groups]]
name = "AUTO"
type = "url-test"
url = "http://www.gstatic.com/generate_204"
rule = [".*"]

[[custom_groups]]
name = "SPARE"
type = "fallback"
rule = ["[]DIRECT"]
`
	rulesets := `[[rulesets]]
group = "REJECT"
ruleset = "[]DOMAIN-SUFFIX,ads.example.org"
[[rulesets]]
group = "PROXY"
ruleset = "` + base + `/clash-domain/Proxy.list"
type = "clash-domain"
[[rulesets]]
group = "PROXY"
ruleset = "` + base + `/clash-ipcidr/Proxy-ipcidr.list"
type = "clash-ipcidr"
[[rulesets]]
group = "PROXY"
ruleset = "` + base + `/missing.list"
type = "clash-domain"
[[rulesets]]
group = "DIRECT"
ruleset = "[]MATCH"
`
	src := decode[sourceDTO](t, c.want(201, "POST", "/api/v1/sources", map[string]any{
		"name": "我的 VPS", "kind": "batch",
		"content": "- {name: 香港 01, type: ss, server: 192.0.2.1, port: 8388, cipher: aes-128-gcm, password: example-pw}\n- {name: 日本 02, type: ss, server: 192.0.2.2, port: 8388, cipher: aes-128-gcm, password: example-pw}\n",
	}))
	rs := decode[ruleSourceDTO](t, c.want(201, "POST", "/api/v1/rule-sources", map[string]any{"name": "local", "kind": "toml", "groupsText": groups, "rulesetsText": rulesets}))
	doc := "clash-launch/profile: v1\nname: 分流\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n  - ref: " + src.ID + "\nrules: {ref: " + rs.ID + "}\noutput: {empty-groups: keep}\n"
	p := decode[profileDTO](t, c.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	return p.ID, doc
}

func TestRoutingGraphTraceHealth(t *testing.T) {
	h := newHarness(t, nil)
	c, _ := h.setup()
	rsrv, hits := ruleServer(t)
	pid, doc := routingProfile(t, h, c, rsrv.URL)
	path := "/api/v1/profiles/" + pid + "/routing/"

	// 地图：第一次取时规则内容在后台拉取；稍后再取得到条数与样例
	var g service.GraphDTO
	for i := 0; i < 50; i++ {
		g = decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": doc}))
		if g.Pending == 0 && g.Rules[1].Set != nil && g.Rules[1].Set.Count != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if g.State != "follow" || !g.Editable || !g.TraceEnabled || len(g.Rules) != 5 {
		t.Fatalf("地图：%+v", g)
	}
	dom := g.Rules[1]
	if dom.Origin != "rules" || dom.Set == nil || dom.Set.Behavior != "domain" || *dom.Set.Count != 2 {
		t.Fatalf("domain 规则集：%+v %+v", dom, dom.Set)
	}
	// 样例按服务器追踪的结果给出命中行
	if !slices.Equal(samples(dom.Set), []string{"example.com@1", "www.cdn.example.net@1"}) {
		t.Errorf("样例：%v", samples(dom.Set))
	}
	if cidr := g.Rules[2]; !slices.Contains(cidr.Flags, "no-resolve") || !slices.Equal(samples(cidr.Set), []string{"192.0.2.1@2"}) {
		t.Errorf("ipcidr：%+v %v", cidr, samples(cidr.Set))
	}
	if g.Rules[3].Set.Count != nil {
		t.Errorf("拉取失败的规则集不应有条数：%+v", g.Rules[3].Set)
	}
	auto := group(g, "AUTO")
	for _, want := range []string{"group.tolerance_zero", "group.lazy_stale", "group.latency_only", "group.http_probe"} {
		if !slices.Contains(auto.Lints, want) {
			t.Errorf("AUTO 缺少提醒 %s：%v", want, auto.Lints)
		}
	}
	if spare := group(g, "SPARE"); !slices.Contains(spare.Lints, "group.unused") || !slices.Contains(spare.Lints, "group.single_member") {
		t.Errorf("SPARE 的提醒：%v", spare.Lints)
	}
	if px := group(g, "PROXY"); px.Members[0].Kind != "group" || px.Members[1].Kind != "builtin" || px.Members[2].Kind != "node" {
		t.Errorf("成员类型：%+v", px.Members)
	}
	if !slices.ContainsFunc(g.Exits, func(e service.ExitDTO) bool {
		return e.Name == "🇭🇰 香港 01" && e.Region == "HK" && e.Source == "我的 VPS" && e.Protocol == "ss"
	}) {
		t.Errorf("出口：%+v", g.Exits)
	}

	// 追踪：命中规则集，沿默认路径 PROXY → AUTO → 第一个节点
	before := *hits
	tr := decode[struct{ Current service.TraceDTO }](t, c.want(200, "POST", path+"trace", map[string]string{"doc": doc, "host": "https://www.example.com/a"})).Current
	if tr.Hit != 1 || tr.Steps[1].Evidence != "+.example.com" || len(tr.Chain) != 2 || tr.Chain[1].Group != "AUTO" || tr.Exit.Kind != "node" || tr.Exit.Name != "🇭🇰 香港 01" || tr.Exit.Region != "HK" {
		t.Fatalf("追踪：%+v", tr)
	}
	if *hits != before {
		t.Errorf("规则内容已缓存，追踪不应重新拉取（%d → %d）", before, *hits)
	}
	// 没命中任何规则集的域名：ipcidr 规则集带 no-resolve（生成器自动加），不需要 IP；拉取失败的规则集无法判断，兜底直连
	tr = decode[struct{ Current service.TraceDTO }](t, c.want(200, "POST", path+"trace", map[string]string{"doc": doc, "host": "other.example.org"})).Current
	if tr.Hit != 4 || tr.NeedsIP != nil || tr.Steps[2].Result != "miss" || tr.Steps[3].Result != "unknown" || tr.Steps[3].Reason != "no-content" || tr.Exit.Name != "DIRECT" {
		t.Errorf("兜底：%+v", tr)
	}
	c.want(400, "POST", path+"trace", map[string]string{"doc": doc, "host": "a b"})

	// 体检：每个网站一项，按列表顺序
	hl := decode[struct{ Items []service.HealthItem }](t, c.want(200, "POST", path+"health", map[string]string{"doc": doc}))
	if len(hl.Items) < 20 || hl.Items[0].Host != "chatgpt.com" || hl.Items[0].Rule != "MATCH,DIRECT" {
		t.Errorf("体检：%+v", hl.Items[:2])
	}

	// 原始编辑：局部追加只标记来源，整段替换时地图只读
	g = decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": doc + "raw: |\n  rules: !prepend\n    - DOMAIN,intranet.example.com,DIRECT\n"}))
	if !g.Editable || g.Rules[0].Origin != "raw" || g.Rules[1].Origin != "rules" {
		t.Errorf("局部追加：editable=%v %+v", g.Editable, g.Rules[:2])
	}
	g = decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": doc + "raw: |\n  rules:\n    - MATCH,DIRECT\n"}))
	if g.Editable || g.ReadonlyReason != "raw_override" || g.Rules[0].Origin != "rules" {
		t.Errorf("整段替换：%+v", g)
	}

	// 关闭追踪：地图照常，追踪与体检返回 404 feature_disabled
	h.svc.Trace.Disabled = true
	if g := decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": doc})); g.TraceEnabled || g.Rules[1].Set.Count != nil {
		t.Errorf("关闭后地图不应给出规则内容：%+v", g.Rules[1].Set)
	}
	if b := c.want(404, "POST", path+"trace", map[string]string{"doc": doc, "host": "www.example.com"}); !strings.Contains(string(b), "feature_disabled") {
		t.Errorf("关闭后追踪：%s", b)
	}
	c.want(404, "POST", path+"health", map[string]string{"doc": doc})
}

// 普通用户只能使用公共代码托管站上的规则地址（反馈 #72）：本机地址即使已被管理员缓存，也不交给普通用户。
func TestRoutingTraceHostsAndQuota(t *testing.T) {
	h := newHarness(t, nil)
	a, _ := h.setup()
	rsrv, _ := ruleServer(t)
	_, adoc := routingProfile(t, h, a, rsrv.URL)
	_ = adoc
	a.want(200, "PUT", "/api/v1/admin/registration", map[string]bool{"open": true})
	anon := h.client()
	anon.want(201, "POST", "/api/v1/register", registerBody(t, anon, "bob", ""))
	b := h.client()
	b.login("bob", "a long enough password")
	// bob 的方案只引用内联规则（他保存不了本机地址的规则源）
	src := decode[sourceDTO](t, b.want(201, "POST", "/api/v1/sources", map[string]any{"name": "n", "kind": "batch", "content": "- {name: n1, type: ss, server: 192.0.2.9, port: 8388, cipher: aes-128-gcm, password: example-pw}\n"}))
	rs := decode[ruleSourceDTO](t, b.want(201, "POST", "/api/v1/rule-sources", map[string]any{"name": "r", "kind": "toml", "groupsText": groupsToml, "rulesetsText": "[[rulesets]]\ngroup = \"PROXY\"\nruleset = \"[]DOMAIN-SUFFIX,example.com\"\n" + rulesetsToml}))
	doc := "clash-launch/profile: v1\nname: b\ntarget: {pack: mihomo, preset: mihomo, core: \"1.19.33\"}\nsources:\n  - ref: " + src.ID + "\nrules: {ref: " + rs.ID + "}\n"
	p := decode[profileDTO](t, b.want(201, "POST", "/api/v1/profiles", map[string]string{"doc": doc}))
	path := "/api/v1/profiles/" + p.ID + "/routing/trace"
	// 原始编辑加入一个指向本机的规则集：普通用户的追踪中它无法判断
	raw := doc + "raw: |\n  rule-providers:\n    loc: {type: http, behavior: domain, format: text, url: " + rsrv.URL + "/clash-domain/Proxy.list}\n  rules: !prepend\n    - RULE-SET,loc,REJECT\n"
	tr := decode[struct{ Current service.TraceDTO }](t, b.want(200, "POST", path, map[string]string{"doc": raw, "host": "www.example.com"})).Current
	if tr.Steps[0].Result != "unknown" || tr.Steps[0].Reason != "no-content" || tr.Hit != 1 {
		t.Errorf("白名单外的地址：%+v", tr)
	}
	// 每分钟 60 次
	for i := 1; i < 60; i++ {
		b.want(200, "POST", path, map[string]string{"doc": doc, "host": "www.example.com"})
	}
	b.want(429, "POST", path, map[string]string{"doc": doc, "host": "www.example.com"})
	// 别人的方案：404
	b.want(404, "POST", strings.Replace(path, p.ID, "00000000-0000-0000-0000-000000000000", 1), map[string]string{"doc": doc, "host": "www.example.com"})
}

func samples(s *service.RuleSetDTO) []string {
	var out []string
	if s == nil {
		return out
	}
	for _, x := range s.Samples {
		out = append(out, x.Host+"@"+itoa(x.Hit))
	}
	return out
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func group(g service.GraphDTO, name string) service.GroupDTO {
	for _, x := range g.Groups {
		if x.Name == name {
			return x
		}
	}
	return service.GroupDTO{}
}

// 开始自己管理（routing 第 13 节第 3 步）：fork 得到的 routing 段写入文档后输出不变；地图标为 own；
// 草稿改了走向时，追踪同时给出应用前后的结果。
func TestRoutingForkOwnDraft(t *testing.T) {
	h := newHarness(t, nil)
	c, _ := h.setup()
	rsrv, _ := ruleServer(t)
	pid, doc := routingProfile(t, h, c, rsrv.URL)
	path := "/api/v1/profiles/" + pid + "/routing/"
	fk := decode[struct{ Routing string }](t, c.want(200, "POST", path+"fork", map[string]string{"doc": doc}))
	if !strings.Contains(fk.Routing, "digest: sha256:") || !strings.Contains(fk.Routing, "base:") {
		t.Fatalf("fork：%s", fk.Routing)
	}
	own := doc + "routing:\n" + indent(fk.Routing)
	before := decode[previewResp](t, c.want(200, "POST", "/api/v1/profiles/"+pid+"/preview", map[string]string{"doc": doc}))
	after := decode[previewResp](t, c.want(200, "POST", "/api/v1/profiles/"+pid+"/preview", map[string]string{"doc": own}))
	if dropUpdated(before.YAML) != dropUpdated(after.YAML) {
		t.Errorf("开始自己管理后输出变了：\n%s\n----\n%s", before.YAML, after.YAML)
	}
	g := decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": own}))
	if g.State != "own" || g.Rules[1].Origin != "routing" || g.Rules[1].Entry == nil || *g.Rules[1].Entry != 1 {
		t.Errorf("自己管理的地图：%+v", g.Rules[1])
	}
	if px := group(g, "PROXY"); px.Members[0].Entry == nil || *px.Members[0].Entry != 0 || px.Members[2].Entry != nil {
		t.Errorf("成员的条目：%+v", px.Members)
	}
	// 草稿：把 Proxy.list 改为直连
	draft := strings.Replace(fk.Routing, "group: PROXY\n    ruleset: "+rsrv.URL+"/clash-domain/Proxy.list", "group: DIRECT\n    ruleset: "+rsrv.URL+"/clash-domain/Proxy.list", 1)
	if draft == fk.Routing {
		t.Fatalf("草稿没有改动：%s", fk.Routing)
	}
	tr := decode[struct{ Current, Draft service.TraceDTO }](t, c.want(200, "POST", path+"trace", map[string]string{"doc": own, "draft": draft, "host": "www.example.com"}))
	if tr.Current.Exit.Kind != "node" || tr.Draft.Exit.Name != "DIRECT" || tr.Draft.Hit != tr.Current.Hit {
		t.Errorf("草稿前后：%+v / %+v", tr.Current.Exit, tr.Draft.Exit)
	}
	c.want(400, "POST", path+"trace", map[string]string{"doc": own, "draft": "- not a mapping", "host": "www.example.com"})
	// 没有规则源时不能开始自己管理
	noRules := strings.Replace(doc, "rules: {ref: ", "#", 1)
	if b := c.want(400, "POST", path+"fork", map[string]string{"doc": noRules}); !strings.Contains(string(b), "routing.no_rules") {
		t.Errorf("没有规则源：%s", b)
	}
}

func indent(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		if l != "" {
			b.WriteString("  ")
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

func dropUpdated(y string) string {
	var out []string
	for _, l := range strings.Split(y, "\n") {
		if !strings.HasPrefix(l, "# UPDATED:") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// 上游同步与期望（routing 第 13 节第 4 步）。
func TestRoutingUpstreamChecks(t *testing.T) {
	h := newHarness(t, nil)
	c, _ := h.setup()
	rsrv, _ := ruleServer(t)
	pid, doc := routingProfile(t, h, c, rsrv.URL)
	path := "/api/v1/profiles/" + pid + "/routing/"
	fk := decode[struct{ Routing string }](t, c.want(200, "POST", path+"fork", map[string]string{"doc": doc}))
	ipcidr := rsrv.URL + "/clash-ipcidr/Proxy-ipcidr.list"
	// 用户自己的改动：ipcidr 规则集改为直连
	mine := strings.Replace(fk.Routing, "group: PROXY\n    ruleset: "+ipcidr, "group: DIRECT\n    ruleset: "+ipcidr, 1)
	if mine == fk.Routing {
		t.Fatal("没有改到 ipcidr 规则集")
	}
	own := doc + "routing:\n" + indent(mine)
	up := decode[service.UpstreamDTO](t, c.want(200, "POST", path+"upstream", map[string]string{"doc": own}))
	if len(up.Changes) != 0 {
		t.Fatalf("上游没变时不应有变化：%+v", up.Changes)
	}

	// 上游更新：domain 规则集改走 AUTO（可直接采纳）；ipcidr 规则集改走 SPARE（与用户的改动冲突）；新增一条内联规则
	var rs ruleSourceDTO
	for _, x := range decode[list[ruleSourceDTO]](t, c.want(200, "GET", "/api/v1/rule-sources", nil)).Items {
		if x.Name == "local" {
			rs = x
		}
	}
	full := decode[map[string]any](t, c.want(200, "GET", "/api/v1/rule-sources/"+rs.ID, nil))
	text := full["rulesetsText"].(string)
	text = strings.Replace(text, "group = \"PROXY\"\nruleset = \""+rsrv.URL+"/clash-domain/Proxy.list\"", "group = \"AUTO\"\nruleset = \""+rsrv.URL+"/clash-domain/Proxy.list\"", 1)
	text = strings.Replace(text, "group = \"PROXY\"\nruleset = \""+ipcidr+"\"", "group = \"SPARE\"\nruleset = \""+ipcidr+"\"", 1)
	text = "[[rulesets]]\ngroup = \"PROXY\"\nruleset = \"[]DOMAIN,new.example.com\"\n" + text
	body := map[string]any{"name": full["name"], "kind": full["kind"], "groupsText": full["groupsText"], "rulesetsText": text}
	c.want(200, "PUT", "/api/v1/rule-sources/"+rs.ID, body, "If-Match", `"v`+itoa(int(full["version"].(float64)))+`"`)

	up = decode[service.UpstreamDTO](t, c.want(200, "POST", path+"upstream", map[string]string{"doc": own}))
	got := map[string]bool{}
	for _, ch := range up.Changes {
		got[ch.Op+" "+ch.Key+" "+ch.Field] = ch.Conflict
	}
	want := map[string]bool{
		"add []DOMAIN,new.example.com ":                         false,
		"modify " + rsrv.URL + "/clash-domain/Proxy.list group": false,
		"modify " + ipcidr + " group":                           true,
	}
	for k, v := range want {
		if c, ok := got[k]; !ok || c != v {
			t.Errorf("缺少或冲突标记不对：%q（得到 %v）", k, got)
		}
	}
	if len(up.Rulesets) != 6 || !strings.HasPrefix(up.Digest, "sha256:") {
		t.Errorf("上游内容：%d 条，摘要 %s", len(up.Rulesets), up.Digest)
	}
	g := decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": own}))
	if g.UpstreamChanges != len(up.Changes) {
		t.Errorf("地图上的上游变化数：%d", g.UpstreamChanges)
	}

	// 期望：先追踪一次让规则内容进入缓存（地图里的期望只用已缓存的内容判断）
	c.want(200, "POST", path+"trace", map[string]string{"doc": own, "host": "www.example.com"})
	checks := own + "  checks:\n    - {host: www.example.com, via: AUTO}\n    - {host: www.example.com, via: DIRECT}\n    - {host: www.example.com, via: NOPE}\n"
	g = decode[service.GraphDTO](t, c.want(200, "POST", path+"graph", map[string]string{"doc": checks}))
	if len(g.Checks) != 3 || g.Checks[0].OK == nil || !*g.Checks[0].OK || g.Checks[1].OK == nil || *g.Checks[1].OK || g.Checks[2].OK != nil {
		t.Fatalf("期望：%+v", g.Checks)
	}
	codes := map[string]bool{}
	for _, d := range g.Diagnostics {
		codes[string(d.Code)] = true
	}
	if !codes["routing.check_failed"] || !codes["routing.check_unknown"] || !codes["routing.upstream_changed"] {
		t.Errorf("诊断：%v", codes)
	}
	// 跟随上游时没有可比对的基线
	c.want(400, "POST", path+"upstream", map[string]string{"doc": doc})
}
