// Package source 是唯一联网的包：拉取订阅与规则源、SSRF 防护、出站代理、快照（第 5.4、14.5、14.6 节）。
package source

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
)

// ErrBlocked 表示请求因安全策略被拒绝（SSRF 防护）。
var ErrBlocked = errors.New("source: 地址被安全策略拒绝")

// blockedPrefixes 是除 netip 内置判断（回环、私网、链路本地、组播、未指定）之外需要拒绝的网段：
// CGNAT、保留、文档与基准测试地址、IPv4 广播，以及常被用作 fake-ip 池的 198.18.0.0/15。
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
	"64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "fec0::/10",
)

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// Guard 是 SSRF 策略。零值即默认策略（无白名单）。
type Guard struct {
	AllowPrefixes []netip.Prefix // 管理员白名单：网段
	AllowHosts    []string       // 管理员白名单：主机名（精确匹配，不区分大小写）
}

// ParseAllowlist 解析 CLASH_LAUNCH_FETCH_ALLOWLIST：逗号分隔的网段、IP 或主机名。
func ParseAllowlist(s string) (Guard, error) {
	var g Guard
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
		case strings.Contains(item, "/"):
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return g, fmt.Errorf("白名单网段 %q 无效: %w", item, err)
			}
			g.AllowPrefixes = append(g.AllowPrefixes, p.Masked())
		default:
			if a, err := netip.ParseAddr(item); err == nil {
				g.AllowPrefixes = append(g.AllowPrefixes, netip.PrefixFrom(a, a.BitLen()))
			} else {
				g.AllowHosts = append(g.AllowHosts, strings.ToLower(item))
			}
		}
	}
	return g, nil
}

func (g Guard) allowedAddr(a netip.Addr) bool {
	for _, p := range g.AllowPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// CheckAddr 判断一个 IP 能否被连接。IPv4 映射与嵌入形式的 IPv6（::ffff:a.b.c.d、NAT64、6to4）按其中的 IPv4 判断。
func (g Guard) CheckAddr(a netip.Addr) error {
	a = a.Unmap()
	if g.allowedAddr(a) {
		return nil
	}
	if inner, ok := embeddedV4(a); ok {
		if err := g.CheckAddr(inner); err != nil {
			return err
		}
	}
	switch {
	case !a.IsValid(), a.IsUnspecified(), a.IsLoopback(), a.IsPrivate(), a.IsLinkLocalUnicast(),
		a.IsLinkLocalMulticast(), a.IsInterfaceLocalMulticast(), a.IsMulticast():
		return fmt.Errorf("%w：%s 是内部或特殊地址", ErrBlocked, a)
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return fmt.Errorf("%w：%s 属于保留网段 %s", ErrBlocked, a, p)
		}
	}
	return nil
}

var (
	nat64   = netip.MustParsePrefix("64:ff9b::/96")
	sixTo4  = netip.MustParsePrefix("2002::/16")
	teredo  = netip.MustParsePrefix("2001::/32")
	compat4 = netip.MustParsePrefix("::/96")
)

func embeddedV4(a netip.Addr) (netip.Addr, bool) {
	if !a.Is6() {
		return netip.Addr{}, false
	}
	b := a.As16()
	switch {
	case nat64.Contains(a), compat4.Contains(a):
		return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
	case sixTo4.Contains(a):
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	case teredo.Contains(a):
		return netip.AddrFrom4([4]byte{b[12] ^ 0xff, b[13] ^ 0xff, b[14] ^ 0xff, b[15] ^ 0xff}), true
	}
	return netip.Addr{}, false
}

// CheckURL 检查 URL 本身：只允许 http/https；拒绝 localhost 类主机名与内部的字面 IP。
// 这只是连接前的快速拒绝；域名解析后的真实地址由 Dialer.Control 在每次连接时再检查，
// 因此 DNS 重绑定与重定向到内网都会在连接阶段被拦下。
func (g Guard) CheckURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w：只允许 http 与 https（得到 %q）", ErrBlocked, u.Scheme)
	}
	return g.CheckHost(u.Hostname())
}

// CheckHost 检查主机名或字面 IP。
func (g Guard) CheckHost(host string) error {
	h := strings.TrimSuffix(strings.ToLower(host), ".")
	for _, a := range g.AllowHosts {
		if h == a {
			return nil
		}
	}
	if h == "" {
		return fmt.Errorf("%w：缺少主机名", ErrBlocked)
	}
	if a, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return g.CheckAddr(a)
	}
	if isLegacyIPv4(h) {
		return fmt.Errorf("%w：%s 是非标准写法的 IPv4 地址", ErrBlocked, host)
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || h == "localhost.localdomain" ||
		strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".lan") || !strings.Contains(h, ".") {
		return fmt.Errorf("%w：%s 是本地或内部主机名", ErrBlocked, host)
	}
	return nil
}

// isLegacyIPv4 识别 0x7f.1、2130706433、0177.0.0.1 这类被部分解析器当作 IPv4 的写法。
func isLegacyIPv4(h string) bool {
	parts := strings.Split(h, ".")
	if len(parts) > 4 {
		return false
	}
	for _, p := range parts {
		digits := "0123456789"
		if strings.HasPrefix(p, "0x") {
			p, digits = p[2:], "0123456789abcdef" // 只有带 0x 前缀的部分才按十六进制理解
		}
		if p == "" || strings.Trim(p, digits) != "" {
			return false
		}
	}
	return true
}

// control 在建立连接前检查实际要连接的地址（net.Dialer.Control），防止 DNS rebinding：
// 即使域名在检查之后被改为解析到内网，连接时仍会被拒绝。
func (g Guard) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w：%w", ErrBlocked, err)
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w：%s 不是 IP", ErrBlocked, host)
	}
	return g.CheckAddr(a)
}
