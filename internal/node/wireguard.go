package node

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// WireGuard 配置文件（wg-quick 格式）到 mihomo wireguard 节点的映射（反馈 #40）。
// 格式依据 wg(8) 与 wg-quick(8) 手册：INI 风格，[Interface] 一节加若干 [Peer]，键不区分大小写，
// # 开始注释，多值用逗号分隔。mihomo 字段依据 adapter/outbound/wireguard.go（v1.19.33）：
//
//	[Interface] PrivateKey → private-key；Address 中第一个 IPv4 / IPv6 → ip / ipv6（mihomo 接受带前缀的写法）；
//	            MTU → mtu；AmneziaWG 的 Jc、Jmin、Jmax、S1–S4、H1–H4、I1–I5 → amnezia-wg-option
//	[Peer]      Endpoint → server + port；PublicKey → public-key；PresharedKey → pre-shared-key；
//	            AllowedIPs → allowed-ips；PersistentKeepalive → persistent-keepalive（mihomo 只有顶层一个值）
//
// 只有一个 [Peer] 时写在顶层；多个时写成 peers 列表（mihomo 要求此时每个 peer 都有 allowed-ips）。
// 没有对应关系的键逐个报告，不静默丢弃：DNS 在 mihomo 中只在 remote-dns-resolve 开启时生效，
// 两者语义不同，不自动转换；ListenPort、FwMark、Table、PreUp 等是本机网络设置，不适用于代理节点。
const FormatWireGuard Format = "wireguard"

var wgSection = regexp.MustCompile(`(?im)^\s*\[interface\]\s*$`)

// looksLikeWireGuard 报告内容是否是 wg-quick 配置（有 [Interface] 一节）。
func looksLikeWireGuard(b []byte) bool { return wgSection.Match(b) }

var wgAmnezia = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"}

type wgSectionData struct {
	name string // interface | peer
	line int
	kv   map[string]string // 小写键
	keys []string          // 原始键（按出现顺序，报告用）
}

func fromWireGuard(b []byte, where string, cat *catalog.Catalog, out *diag.List) []*Node {
	var secs []*wgSectionData
	sc := bufio.NewScanner(bytes.NewReader(b))
	ln := 0
	for sc.Scan() {
		ln++
		s := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(s, '#'); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			secs = append(secs, &wgSectionData{name: strings.ToLower(strings.TrimSpace(s[1 : len(s)-1])), line: ln, kv: map[string]string{}})
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok || len(secs) == 0 {
			out.Add(diag.NodeWireGuardField, fmt.Sprintf("%s:%d", where, ln), "field", s, "reason", "not a key = value line")
			continue
		}
		cur := secs[len(secs)-1]
		key := strings.TrimSpace(k)
		cur.kv[strings.ToLower(key)] = strings.TrimSpace(v)
		cur.keys = append(cur.keys, key)
	}

	var iface *wgSectionData
	var peers []*wgSectionData
	for _, s := range secs {
		switch s.name {
		case "interface":
			if iface != nil {
				out.Add(diag.NodeWireGuardField, fmt.Sprintf("%s:%d", where, s.line), "field", "[Interface]", "reason", "only the first [Interface] is used")
				continue
			}
			iface = s
		case "peer":
			peers = append(peers, s)
		default:
			out.Add(diag.NodeWireGuardField, fmt.Sprintf("%s:%d", where, s.line), "field", "["+s.name+"]", "reason", "unknown section")
		}
	}
	origin := where + ":wireguard"
	if iface == nil || len(peers) == 0 {
		out.Add(diag.NodeUnrecognized, where, "source", where, "preview", "WireGuard config needs [Interface] and at least one [Peer]")
		return nil
	}

	m := yamlx.Map()
	set := func(n *yaml.Node, k string, v any) {
		if x, err := yamlx.FromAny(v); err == nil {
			yamlx.Set(n, k, x)
		}
	}
	report := func(s *wgSectionData, used map[string]bool, why map[string]string) {
		for _, k := range s.keys {
			lk := strings.ToLower(k)
			if used[lk] {
				continue
			}
			reason := why[lk]
			if reason == "" {
				reason = "no mihomo equivalent"
			}
			out.Add(diag.NodeWireGuardField, origin, "field", k, "reason", reason)
		}
	}

	// [Interface]
	used := map[string]bool{}
	take := func(k string) (string, bool) {
		v, ok := iface.kv[k]
		if ok {
			used[k] = true
		}
		return v, ok
	}
	set(m, "name", "")
	set(m, "type", "wireguard")
	if v, ok := take("privatekey"); ok {
		set(m, "private-key", v)
	}
	if v, ok := take("address"); ok {
		var v4, v6 string
		for _, a := range splitList(v) {
			ip := a
			if i := strings.IndexByte(ip, '/'); i >= 0 {
				ip = ip[:i]
			}
			p := net.ParseIP(ip)
			switch {
			case p == nil:
				out.Add(diag.NodeWireGuardField, origin, "field", "Address "+a, "reason", "not an IP address")
			case p.To4() != nil && v4 == "":
				v4 = a
			case p.To4() == nil && v6 == "":
				v6 = a
			default:
				out.Add(diag.NodeWireGuardField, origin, "field", "Address "+a, "reason", "mihomo takes one IPv4 and one IPv6 address")
			}
		}
		if v4 != "" {
			set(m, "ip", v4)
		}
		if v6 != "" {
			set(m, "ipv6", v6)
		}
	}
	if v, ok := take("mtu"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			set(m, "mtu", n)
		} else {
			used["mtu"] = false
		}
	}
	amn := yamlx.Map()
	for _, k := range wgAmnezia {
		if v, ok := take(k); ok {
			if k[0] == 'h' || k[0] == 'i' {
				set(amn, k, v) // mihomo 中 H、I 为字符串
			} else if n, err := strconv.Atoi(v); err == nil {
				set(amn, k, n)
			} else {
				used[k] = false
			}
		}
	}
	if len(amn.Content) > 0 {
		yamlx.Set(m, "amnezia-wg-option", amn)
	}
	report(iface, used, map[string]string{
		"dns":        "mihomo applies per-node dns only with remote-dns-resolve; set it in the node if needed",
		"listenport": "local interface setting", "fwmark": "local interface setting", "table": "local interface setting",
		"preup": "local script", "postup": "local script", "predown": "local script", "postdown": "local script", "saveconfig": "local interface setting",
	})

	// [Peer]
	keepalive := ""
	peerNode := func(p *wgSectionData, dst *yaml.Node) bool {
		pu := map[string]bool{}
		get := func(k string) (string, bool) {
			v, ok := p.kv[k]
			if ok {
				pu[k] = true
			}
			return v, ok
		}
		ep, ok := get("endpoint")
		if !ok {
			out.Add(diag.NodeWireGuardField, fmt.Sprintf("%s:%d", where, p.line), "field", "[Peer]", "reason", "missing Endpoint")
			return false
		}
		host, port, err := net.SplitHostPort(ep)
		pn, perr := strconv.Atoi(port)
		if err != nil || perr != nil {
			out.Add(diag.NodeWireGuardField, fmt.Sprintf("%s:%d", where, p.line), "field", "Endpoint "+ep, "reason", "expected host:port")
			return false
		}
		set(dst, "server", host)
		set(dst, "port", pn)
		if v, ok := get("publickey"); ok {
			set(dst, "public-key", v)
		}
		if v, ok := get("presharedkey"); ok {
			set(dst, "pre-shared-key", v)
		}
		if v, ok := get("allowedips"); ok {
			set(dst, "allowed-ips", splitList(v))
		}
		if v, ok := get("persistentkeepalive"); ok {
			if keepalive != "" && keepalive != v {
				pu["persistentkeepalive"] = false
			} else {
				keepalive = v
			}
		}
		report(p, pu, map[string]string{"persistentkeepalive": "mihomo has one persistent-keepalive for all peers"})
		return true
	}
	if len(peers) == 1 {
		if !peerNode(peers[0], m) {
			return nil
		}
	} else {
		list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, p := range peers {
			pm := yamlx.Map()
			if peerNode(p, pm) {
				list.Content = append(list.Content, pm)
			}
		}
		if len(list.Content) == 0 {
			return nil
		}
		yamlx.Set(m, "peers", list)
	}
	if keepalive != "" {
		if n, err := strconv.Atoi(keepalive); err == nil && n > 0 {
			set(m, "persistent-keepalive", n)
		}
	}
	set(m, "udp", true)

	// 名称：配置文件里没有名字，用第一个对端地址；导入后可在源的整理设置中改名
	first := peers[0].kv["endpoint"]
	set(m, "name", "WireGuard "+first)
	Coerce(m, proxyField(cat))
	if n := build(m, where, origin, out); n != nil {
		return []*Node{n}
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}
