package match

import (
	"net/netip"
	"strings"

	"github.com/metacubex/mihomo/rules/common"
)

// Samples 从规则集内容中取最多 n 个代表性的请求目标（域名或 IP），在条目中均匀分布，结果确定。
// 用于分流地图的粒子样例与规则集说明（routing 第 7、10 节）。取不到时返回空切片。
//
// 只从能直接还原为一个请求目标的条目取：domain 的精确与 +. 条目（. 前缀取 www. 子域名），
// ipcidr 的网段（取第一个可用地址），classical 的 DOMAIN、DOMAIN-SUFFIX、IP-CIDR、IP-CIDR6。
// 通配、关键字、正则等条目跳过：从它们构造的主机名是猜的，不能代表规则集。
func (s *Set) Samples(n int) []string {
	s.sampleOnce.Do(func() { s.samples = s.collectSamples(8) })
	if n > len(s.samples) {
		n = len(s.samples)
	}
	return s.samples[:n]
}

func (s *Set) collectSamples(n int) []string {
	var cands []string
	add := func(t string) {
		if t != "" {
			cands = append(cands, t)
		}
	}
	switch {
	case s.entries != nil || s.s == nil: // classical
		for _, e := range s.entries {
			switch e.tp {
			case "DOMAIN", "DOMAIN-SUFFIX":
				add(hostSample(e.payload))
			case "IP-CIDR", "IP-CIDR6":
				if isSrc, _ := common.ParseParams(e.params); !isSrc {
					add(ipSample(e.payload))
				}
			}
		}
	default:
		each := func(item string) bool {
			if s.Behavior == "domain" {
				add(domainSample(item))
			} else {
				add(ipSample(item))
			}
			return true
		}
		if s.lines != nil {
			for _, l := range s.lines {
				each(l)
			}
		} else {
			s.s.DumpMrs(each)
		}
	}
	if len(cands) <= n {
		return cands
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, cands[i*len(cands)/n])
	}
	return out
}

// domainSample 把 domain 规则集的一个条目还原为主机名。
func domainSample(item string) string {
	switch {
	case strings.HasPrefix(item, "+."):
		return hostSample(item[2:])
	case strings.HasPrefix(item, "."):
		if h := hostSample(item[1:]); h != "" {
			return "www." + h
		}
		return ""
	}
	return hostSample(item)
}

// hostSample 只接受至少两段、不含通配的主机名（单独的顶级域名不是一个可访问的网站）。
func hostSample(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if !strings.Contains(h, ".") || strings.ContainsAny(h, "*+/ ") {
		return ""
	}
	return h
}

func ipSample(item string) string {
	p, err := netip.ParsePrefix(strings.TrimSpace(item))
	if err != nil {
		if a, err := netip.ParseAddr(strings.TrimSpace(item)); err == nil {
			return a.String()
		}
		return ""
	}
	a := p.Masked().Addr()
	if p.Bits() < a.BitLen() {
		a = a.Next()
	}
	return a.String()
}
