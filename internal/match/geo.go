package match

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/metacubex/mihomo/component/geodata"
	"github.com/metacubex/mihomo/component/geodata/router"
	_ "github.com/metacubex/mihomo/component/geodata/standard" // 注册 standard 加载器
	"github.com/metacubex/mihomo/component/mmdb"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/oschwald/maxminddb-golang"
)

// 为什么不用 rules.ParseRule 构造 GEOIP、GEOSITE、IP-ASN（第 0 步的验证结论，写入 0007 的「实测」）：
//   - NewGEOIP / NewGEOSITE / NewIPASN 先调用 geodata.InitGeoIP 等：文件不存在或校验失败时**从网上下载**，
//     违反「运行时出站只有用户配置的订阅与规则源」；
//   - mmdb.IPInstance / ASNInstance 打不开文件时 log.Fatalln，整个服务进程退出；
//   - 数据路径（C.Path）、geodata-mode、加载器都是进程级全局变量，多用户的服务器上不同配置会互相影响。
//
// 因此本文件只用内核导出的、无全局状态的部分：加载器的 LoadSiteByBytes / LoadIPByBytes、
// router 的匹配器构造函数、mmdb.ASNReader.LookupASN；三种规则的 Match 逐行移植（出处见各函数注释）。

// Geo 是只读加载的地理数据。nil 或缺少某个文件时，对应规则在追踪中标为无法判断。可并发使用。
type Geo struct {
	mmdb     *maxminddb.Reader
	mmdbKind uint8
	asn      *maxminddb.Reader
	ipDat    string // geoip.dat 路径
	siteDat  string // geosite.dat 路径

	mu    sync.Mutex
	sites map[string]router.DomainMatcher
	ips   map[string]router.IPMatcher
}

const (
	kindMaxmind uint8 = iota // 与内核 component/mmdb 的 databaseType 相同
	kindSing
	kindMetaV0
)

// OpenGeo 只读打开目录中的地理数据。文件名与内核 constant/path.go 的查找规则相同（不区分大小写）：
// MMDB 为 Country.mmdb、geoip.db 或 geoip.metadb，另有 GeoIP.dat、GeoSite.dat、ASN.mmdb。
// 目录为空时返回 (nil, nil)；某个文件缺失只让对应规则无法判断。
func OpenGeo(dir string) (*Geo, error) {
	if dir == "" {
		return nil, nil
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取地理数据目录：%w", err)
	}
	g := &Geo{sites: map[string]router.DomainMatcher{}, ips: map[string]router.IPMatcher{}}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name, path := f.Name(), filepath.Join(dir, f.Name())
		switch {
		case g.mmdb == nil && (strings.EqualFold(name, "Country.mmdb") || strings.EqualFold(name, "geoip.db") || strings.EqualFold(name, "geoip.metadb")):
			r, err := maxminddb.Open(path)
			if err != nil {
				g.Close()
				return nil, fmt.Errorf("打开 %s：%w", name, err)
			}
			g.mmdb = r
			switch r.Metadata.DatabaseType {
			case "sing-geoip":
				g.mmdbKind = kindSing
			case "Meta-geoip0":
				g.mmdbKind = kindMetaV0
			default:
				g.mmdbKind = kindMaxmind
			}
		case strings.EqualFold(name, "ASN.mmdb"):
			r, err := maxminddb.Open(path)
			if err != nil {
				g.Close()
				return nil, fmt.Errorf("打开 %s：%w", name, err)
			}
			g.asn = r
		case strings.EqualFold(name, "GeoIP.dat"):
			g.ipDat = path
		case strings.EqualFold(name, "GeoSite.dat"):
			g.siteDat = path
		}
	}
	return g, nil
}

// Close 关闭打开的数据库。
func (g *Geo) Close() {
	if g == nil {
		return
	}
	if g.mmdb != nil {
		_ = g.mmdb.Close()
	}
	if g.asn != nil {
		_ = g.asn.Close()
	}
}

// codes 移植 component/mmdb/reader.go 的 IPReader.LookupCode（内核用未导出的 databaseType 区分三种库）。
func (g *Geo) codes(ip netip.Addr) []string {
	switch g.mmdbKind {
	case kindSing:
		var code string
		_ = g.mmdb.Lookup(ip.AsSlice(), &code)
		if code == "" {
			return []string{}
		}
		return []string{code}
	case kindMetaV0:
		var record any
		_ = g.mmdb.Lookup(ip.AsSlice(), &record)
		switch record := record.(type) {
		case string:
			return []string{record}
		case []any:
			out := make([]string, 0, len(record))
			for _, item := range record {
				if s, ok := item.(string); ok { // 内核直接断言；这里不让异常数据导致 panic
					out = append(out, s)
				}
			}
			return out
		}
		return []string{}
	default:
		var country struct {
			Country struct {
				IsoCode string `maxminddb:"iso_code"`
			} `maxminddb:"country"`
		}
		_ = g.mmdb.Lookup(ip.AsSlice(), &country)
		if country.Country.IsoCode == "" {
			return []string{}
		}
		return []string{strings.ToLower(country.Country.IsoCode)}
	}
}

func loader() (geodata.Loader, error) { return geodata.GetGeoDataLoader("standard") }

// ipMatcher 移植 geodata.LoadGeoIPMatcher：! 前缀取反，代码不区分大小写。
func (g *Geo) ipMatcher(country string) (router.IPMatcher, error) {
	not := strings.HasPrefix(country, "!")
	country = strings.ToLower(strings.TrimPrefix(country, "!"))
	g.mu.Lock()
	defer g.mu.Unlock()
	m, ok := g.ips[country]
	if !ok {
		body, err := os.ReadFile(g.ipDat)
		if err != nil {
			return nil, err
		}
		l, err := loader()
		if err != nil {
			return nil, err
		}
		cidrs, err := l.LoadIPByBytes(body, country)
		if err != nil {
			return nil, err
		}
		if m, err = router.NewGeoIPMatcher(cidrs); err != nil {
			return nil, err
		}
		g.ips[country] = m
	}
	if not {
		return router.NewNotIpMatcherGroup(m), nil
	}
	return m, nil
}

// siteMatcher 移植 geodata.LoadGeoSiteMatcher：! 取反、@属性筛选，匹配器用内核默认的 succinct
// （geosite-matcher: mph 只影响性能，不影响结果）。
func (g *Geo) siteMatcher(code string) (router.DomainMatcher, error) {
	if code == "" {
		return nil, fmt.Errorf("country code could not be empty")
	}
	not := false
	if code[0] == '!' {
		not, code = true, code[1:]
		if code == "" {
			return nil, fmt.Errorf("country code could not be empty")
		}
	}
	code = strings.ToLower(code)
	parts := strings.Split(code, "@")
	list := strings.TrimSpace(parts[0])
	if list == "" {
		return nil, fmt.Errorf("empty listname in rule: %s", code)
	}
	var attrs []string // 移植 geodata/attr.go 的 parseAttrs 与 AttributeList.Match（全部属性都要有）
	for _, a := range parts[1:] {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			attrs = append(attrs, a)
		}
	}
	key := list
	if len(attrs) > 0 {
		key += "@" + strings.Join(attrs, ",")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	m, ok := g.sites[key]
	if !ok {
		body, err := os.ReadFile(g.siteDat)
		if err != nil {
			return nil, err
		}
		l, err := loader()
		if err != nil {
			return nil, err
		}
		domains, err := l.LoadSiteByBytes(body, list)
		if err != nil {
			return nil, err
		}
		if len(attrs) > 0 {
			kept := make([]*router.Domain, 0, len(domains))
			for _, d := range domains {
				if hasAttrs(d, attrs) {
					kept = append(kept, d)
				}
			}
			domains = kept
		}
		if m, err = router.NewSuccinctMatcherGroup(domains); err != nil {
			return nil, err
		}
		g.sites[key] = m
	}
	if not {
		return router.NewNotDomainMatcherGroup(m), nil
	}
	return m, nil
}

func hasAttrs(d *router.Domain, attrs []string) bool {
	for _, want := range attrs {
		if !slices.ContainsFunc(d.Attribute, func(a *router.Domain_Attribute) bool { return strings.EqualFold(a.GetKey(), want) }) {
			return false
		}
	}
	return true
}

// geoIPRule 移植 rules/common/geoip.go 的 GEOIP.Match（目的地址；src 在 build 中已标为无法判断）。
type geoIPRule struct {
	common.Base
	country, adapter string
	noResolve        bool
	geo              *Geo
	dat              bool // geodata-mode
	m                router.IPMatcher
}

func newGeoIP(country, adapter string, noResolve bool, env Env, missing func() (C.Rule, error)) (C.Rule, error) {
	country = strings.ToLower(country)
	r := &geoIPRule{country: country, adapter: adapter, noResolve: noResolve, geo: env.Geo, dat: env.GeodataMode}
	if country == "lan" {
		return r, nil
	}
	if env.Geo == nil || (env.GeodataMode && env.Geo.ipDat == "") || (!env.GeodataMode && env.Geo.mmdb == nil) {
		return missing()
	}
	if env.GeodataMode { // 内核在构造时试加载，代码不存在时这一行无效
		m, err := env.Geo.ipMatcher(country)
		if err != nil {
			return nil, fmt.Errorf("[GeoIP] %w", err)
		}
		r.m = m
	}
	return r, nil
}

func (r *geoIPRule) RuleType() C.RuleType { return C.GEOIP }
func (r *geoIPRule) Adapter() string      { return r.adapter }
func (r *geoIPRule) Payload() string      { return r.country }

func (r *geoIPRule) Match(md *C.Metadata, h C.RuleMatchHelper) (bool, string) {
	if !r.noResolve && h.ResolveIP != nil {
		h.ResolveIP()
	}
	ip := md.DstIP
	if !ip.IsValid() {
		return false, ""
	}
	if r.country == "lan" {
		return ip.IsPrivate() || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast(), r.adapter
	}
	if r.dat {
		if slices.Contains(md.DstGeoIP, r.country) {
			return true, r.adapter
		}
		ok := r.m.Match(ip)
		if ok {
			md.DstGeoIP = append(md.DstGeoIP, r.country)
		}
		return ok, r.adapter
	}
	if md.DstGeoIP != nil {
		return slices.Contains(md.DstGeoIP, r.country), r.adapter
	}
	md.DstGeoIP = r.geo.codes(ip)
	if slices.Contains(md.DstGeoIP, r.country) {
		return true, r.adapter
	}
	return false, ""
}

// geoSiteRule 移植 rules/common/geosite.go 的 GEOSITE.Match。
type geoSiteRule struct {
	common.Base
	code, adapter string
	m             router.DomainMatcher
}

func newGeoSite(code, adapter string, env Env, missing func() (C.Rule, error)) (C.Rule, error) {
	if env.Geo == nil || env.Geo.siteDat == "" {
		return missing()
	}
	m, err := env.Geo.siteMatcher(code) // 内核同样在构造时试加载
	if err != nil {
		return nil, fmt.Errorf("load GeoSite data error, %w", err)
	}
	return &geoSiteRule{code: code, adapter: adapter, m: m}, nil
}

func (r *geoSiteRule) RuleType() C.RuleType { return C.GEOSITE }
func (r *geoSiteRule) Adapter() string      { return r.adapter }
func (r *geoSiteRule) Payload() string      { return r.code }

func (r *geoSiteRule) Match(md *C.Metadata, _ C.RuleMatchHelper) (bool, string) {
	host := md.RuleHost()
	return host != "" && r.m.ApplyDomain(host), r.adapter
}

// ipASNRule 移植 rules/common/ipasn.go 的 ASN.Match（目的地址），查询用内核导出的 mmdb.ASNReader。
type ipASNRule struct {
	common.Base
	asn, adapter string
	noResolve    bool
	r            mmdb.ASNReader
}

func newIPASN(asn, adapter string, noResolve bool, env Env, missing func() (C.Rule, error)) (C.Rule, error) {
	if env.Geo == nil || env.Geo.asn == nil {
		return missing()
	}
	return &ipASNRule{asn: asn, adapter: adapter, noResolve: noResolve, r: mmdb.ASNReader{Reader: env.Geo.asn}}, nil
}

func (r *ipASNRule) RuleType() C.RuleType { return C.IPASN }
func (r *ipASNRule) Adapter() string      { return r.adapter }
func (r *ipASNRule) Payload() string      { return r.asn }

func (r *ipASNRule) Match(md *C.Metadata, h C.RuleMatchHelper) (bool, string) {
	if !r.noResolve && h.ResolveIP != nil {
		h.ResolveIP()
	}
	if !md.DstIP.IsValid() {
		return false, ""
	}
	asn, aso := r.r.LookupASN(md.DstIP.AsSlice())
	md.DstIPASN = asn + " " + aso
	return r.asn == asn, r.adapter
}
