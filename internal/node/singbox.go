package node

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// sing-box 出站到 mihomo 节点的映射（第 5.2 节）。字段名对照 sing-box 官方文档
// （sing-box.sagernet.org/configuration/outbound/*、shared/tls、shared/v2ray-transport，2026-11-09），
// 映射表见 docs/design/singbox-mapping.md。没有对应关系的字段逐个报告，不静默丢弃。

// 不是代理节点的出站类型：跳过并说明。
var singboxNonProxy = map[string]string{
	"direct": "not a proxy", "block": "not a proxy", "dns": "not a proxy",
	"selector": "groups are rebuilt from the rule source", "urltest": "groups are rebuilt from the rule source",
}

var singboxType = map[string]string{
	"shadowsocks": "ss", "vmess": "vmess", "vless": "vless", "trojan": "trojan", "hysteria2": "hysteria2",
	"hysteria": "hysteria", "tuic": "tuic", "socks": "socks5", "http": "http", "anytls": "anytls",
}

type sbConv struct {
	tag  string
	src  map[string]any
	used map[string]bool
	out  *yaml.Node
}

func (c *sbConv) get(k string) (any, bool) {
	v, ok := c.src[k]
	if ok {
		c.used[k] = true
	}
	return v, ok
}

func (c *sbConv) set(k string, v any) {
	n, err := yamlx.FromAny(v)
	if err == nil {
		yamlx.Set(c.out, k, n)
	}
}

// copy 把 sing-box 字段 from 原值写为 mihomo 字段 to。
func (c *sbConv) copy(from, to string) {
	if v, ok := c.get(from); ok {
		c.set(to, v)
	}
}

func fromSingbox(list *yaml.Node, where string, cat *catalog.Catalog, out *diag.List) []*Node {
	if list.Kind != yaml.SequenceNode {
		out.Add(diag.NodeNotProxy, where, "source", where, "index", "outbounds")
		return nil
	}
	var nodes []*Node
	for i, it := range list.Content {
		var ob map[string]any
		if err := it.Decode(&ob); err != nil {
			out.Add(diag.NodeNotProxy, where, "source", where, "index", strconv.Itoa(i))
			continue
		}
		typ, _ := ob["type"].(string)
		tag, _ := ob["tag"].(string)
		if tag == "" {
			tag = fmt.Sprintf("outbounds[%d]", i)
		}
		origin := fmt.Sprintf("%s:outbounds[%d]", where, i)
		if reason, ok := singboxNonProxy[typ]; ok {
			out.Add(diag.NodeSingboxSkip, origin, "tag", tag, "type", typ, "reason", reason)
			continue
		}
		mt, ok := singboxType[typ]
		if !ok {
			out.Add(diag.NodeSingboxSkip, origin, "tag", tag, "type", typ, "reason", "no mapping for this outbound type")
			continue
		}
		c := &sbConv{tag: tag, src: ob, used: map[string]bool{"type": true, "tag": true}, out: yamlx.Map()}
		c.set("name", tag)
		c.set("type", mt)
		c.copy("server", "server")
		c.copy("server_port", "port")
		if ok := c.protocol(typ, out, origin); !ok {
			continue
		}
		if v, ok := c.get("network"); ok {
			c.set("udp", v != "tcp")
		} else if mt != "http" {
			c.set("udp", true)
		}
		c.copy("detour", "dialer-proxy")
		var unused []string
		for k := range ob {
			if !c.used[k] {
				unused = append(unused, k)
			}
		}
		sort.Strings(unused)
		for _, k := range unused {
			out.Add(diag.NodeSingboxField, origin, "tag", tag, "field", k)
		}
		Coerce(c.out, proxyField(cat))
		if n := build(c.out, where, origin, out); n != nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

func (c *sbConv) protocol(typ string, out *diag.List, origin string) bool {
	switch typ {
	case "shadowsocks":
		c.copy("method", "cipher")
		c.copy("password", "password")
		if p, ok := c.get("plugin"); ok {
			opts, _ := c.get("plugin_opts")
			if !c.ssPlugin(fmt.Sprint(p), fmt.Sprint(opts)) {
				out.Add(diag.NodeSingboxField, origin, "tag", c.tag, "field", "plugin")
			}
		}
		if v, ok := c.get("udp_over_tcp"); ok {
			switch x := v.(type) {
			case bool:
				c.set("udp-over-tcp", x)
			case map[string]any:
				c.set("udp-over-tcp", x["enabled"] == true)
				if ver, ok := x["version"]; ok {
					c.set("udp-over-tcp-version", ver)
				}
			}
		}
	case "vmess":
		c.copy("uuid", "uuid")
		if _, ok := c.src["alter_id"]; ok {
			c.copy("alter_id", "alterId")
		} else {
			c.set("alterId", 0)
		}
		if _, ok := c.src["security"]; ok {
			c.copy("security", "cipher")
		} else {
			c.set("cipher", "auto")
		}
		c.copy("global_padding", "global-padding")
		c.copy("authenticated_length", "authenticated-length")
		c.copy("packet_encoding", "packet-encoding")
		c.tls("servername", true)
		c.transport()
	case "vless":
		c.copy("uuid", "uuid")
		c.copy("flow", "flow")
		c.copy("packet_encoding", "packet-encoding")
		c.tls("servername", true)
		c.transport()
	case "trojan":
		c.copy("password", "password")
		c.tls("sni", false)
		c.transport()
	case "hysteria2":
		c.ports()
		c.copy("hop_interval", "hop-interval")
		c.mbps("up_mbps", "up")
		c.mbps("down_mbps", "down")
		if o, ok := c.get("obfs"); ok {
			if m, ok := o.(map[string]any); ok {
				if t, ok := m["type"]; ok {
					c.set("obfs", t)
				}
				if p, ok := m["password"]; ok {
					c.set("obfs-password", p)
				}
			}
		}
		c.copy("password", "password")
		c.tls("sni", false)
	case "hysteria":
		c.ports()
		c.copy("up", "up")
		c.copy("down", "down")
		c.mbps("up_mbps", "up")
		c.mbps("down_mbps", "down")
		c.copy("obfs", "obfs")
		c.copy("auth", "auth")
		c.copy("auth_str", "auth-str")
		c.copy("recv_window_conn", "recv-window-conn")
		c.copy("recv_window", "recv-window")
		c.copy("disable_mtu_discovery", "disable-mtu-discovery")
		c.tls("sni", false)
	case "tuic":
		c.copy("uuid", "uuid")
		c.copy("password", "password")
		c.copy("congestion_control", "congestion-controller")
		c.copy("udp_relay_mode", "udp-relay-mode")
		c.copy("udp_over_stream", "udp-over-stream")
		c.tls("sni", false)
	case "socks":
		if v, ok := c.get("version"); ok && fmt.Sprint(v) != "5" {
			out.Add(diag.NodeSingboxSkip, origin, "tag", c.tag, "type", "socks", "reason", "only SOCKS5 is supported by mihomo")
			return false
		}
		c.copy("username", "username")
		c.copy("password", "password")
	case "http":
		c.copy("username", "username")
		c.copy("password", "password")
		if h, ok := c.get("headers"); ok {
			if m, ok := h.(map[string]any); ok {
				flat := map[string]any{}
				for k, v := range m {
					if l, ok := v.([]any); ok && len(l) > 0 {
						v = l[0]
					}
					flat[k] = fmt.Sprint(v)
				}
				c.set("headers", flat)
			}
		}
		c.tls("sni", true)
	case "anytls":
		c.copy("password", "password")
		c.seconds("idle_session_check_interval", "idle-session-check-interval")
		c.seconds("idle_session_timeout", "idle-session-timeout")
		c.copy("min_idle_session", "min-idle-session")
		c.copy("client_metadata", "client-metadata")
		c.tls("sni", false)
	}
	return true
}

// ssPlugin 映射 shadowsocks 插件：obfs-local（simple-obfs）与 v2ray-plugin。
func (c *sbConv) ssPlugin(name, opts string) bool {
	kv := map[string]string{}
	for _, part := range strings.Split(opts, ";") {
		k, v, _ := strings.Cut(part, "=")
		if k = strings.TrimSpace(k); k != "" {
			kv[k] = strings.TrimSpace(v)
		}
	}
	switch name {
	case "obfs-local", "simple-obfs":
		c.set("plugin", "obfs")
		c.set("plugin-opts", map[string]any{"mode": kv["obfs"], "host": kv["obfs-host"]})
	case "v2ray-plugin":
		mode := kv["mode"]
		if mode == "" {
			mode = "websocket"
		}
		po := map[string]any{"mode": mode, "host": kv["host"], "path": kv["path"]}
		if _, ok := kv["tls"]; ok {
			po["tls"] = true
		}
		c.set("plugin", "v2ray-plugin")
		c.set("plugin-opts", po)
	default:
		return false
	}
	return true
}

func (c *sbConv) ports() {
	v, ok := c.get("server_ports")
	if !ok {
		return
	}
	var parts []string
	switch x := v.(type) {
	case []any:
		for _, p := range x {
			parts = append(parts, strings.ReplaceAll(fmt.Sprint(p), ":", "-"))
		}
	case string:
		parts = append(parts, strings.ReplaceAll(x, ":", "-"))
	}
	if len(parts) > 0 {
		c.set("ports", strings.Join(parts, ","))
	}
}

func (c *sbConv) mbps(from, to string) {
	if v, ok := c.get(from); ok {
		c.set(to, fmt.Sprintf("%v Mbps", v))
	}
}

// seconds 把 sing-box 的时长字符串（"30s"、"1m"）转换为秒数。
func (c *sbConv) seconds(from, to string) {
	v, ok := c.get(from)
	if !ok {
		return
	}
	if d, err := time.ParseDuration(fmt.Sprint(v)); err == nil {
		c.set(to, int(d.Seconds()))
	}
}

// tls 映射 sing-box 的 tls 对象。sniKey 是该协议在 mihomo 中的 SNI 键；switchKey 表示协议用 tls: true 开启 TLS。
func (c *sbConv) tls(sniKey string, switchKey bool) {
	v, ok := c.get("tls")
	if !ok {
		return
	}
	t, ok := v.(map[string]any)
	if !ok {
		return
	}
	if switchKey {
		c.set("tls", t["enabled"] == true)
	}
	known := []string{"enabled", "server_name", "insecure", "alpn", "utls", "reality", "ech", "disable_sni"}
	if s, ok := t["server_name"]; ok {
		c.set(sniKey, s)
	}
	if b, ok := t["insecure"]; ok {
		c.set("skip-cert-verify", b)
	}
	if a, ok := t["alpn"]; ok {
		c.set("alpn", a)
	}
	if b, ok := t["disable_sni"]; ok {
		c.set("disable-sni", b)
	}
	if u, ok := t["utls"].(map[string]any); ok && u["enabled"] == true {
		if fp, ok := u["fingerprint"]; ok {
			c.set("client-fingerprint", fp)
		}
	}
	if r, ok := t["reality"].(map[string]any); ok && r["enabled"] == true {
		ro := map[string]any{}
		if pk, ok := r["public_key"]; ok {
			ro["public-key"] = pk
		}
		if sid, ok := r["short_id"]; ok {
			ro["short-id"] = sid
		}
		c.set("reality-opts", ro)
	}
	if e, ok := t["ech"].(map[string]any); ok && e["enabled"] == true {
		eo := map[string]any{"enable": true}
		if cfg, ok := e["config"]; ok {
			if l, ok := cfg.([]any); ok {
				var lines []string
				for _, x := range l {
					lines = append(lines, fmt.Sprint(x))
				}
				cfg = strings.Join(lines, "\n")
			}
			eo["config"] = cfg
		}
		if q, ok := e["query_server_name"]; ok {
			eo["query-server-name"] = q
		}
		c.set("ech-opts", eo)
	}
	for k := range t {
		if !slices.Contains(known, k) {
			c.used["tls."+k] = false
			c.src["tls."+k] = true // 让未转换的 TLS 子字段出现在报告中
		}
	}
}

// transport 映射 v2ray 传输：ws、httpupgrade、grpc、http（TLS 下为 h2）。quic 等没有对应。
func (c *sbConv) transport() {
	v, ok := c.get("transport")
	if !ok {
		return
	}
	t, ok := v.(map[string]any)
	if !ok {
		return
	}
	headers := func() map[string]any {
		h, _ := t["headers"].(map[string]any)
		return h
	}
	tlsOn := false
	if tl, ok := c.src["tls"].(map[string]any); ok {
		tlsOn = tl["enabled"] == true
	}
	switch t["type"] {
	case "ws", "httpupgrade":
		c.set("network", "ws")
		wo := map[string]any{}
		if p, ok := t["path"]; ok {
			wo["path"] = p
		}
		if h := headers(); h != nil {
			wo["headers"] = h
		}
		if t["type"] == "httpupgrade" {
			if h, ok := t["host"]; ok {
				wo["headers"] = map[string]any{"Host": h}
			}
			wo["v2ray-http-upgrade"] = true
		}
		if m, ok := t["max_early_data"]; ok {
			wo["max-early-data"] = m
		}
		if n, ok := t["early_data_header_name"]; ok {
			wo["early-data-header-name"] = n
		}
		c.set("ws-opts", wo)
	case "grpc":
		c.set("network", "grpc")
		c.set("grpc-opts", map[string]any{"grpc-service-name": t["service_name"]})
	case "http":
		if tlsOn {
			c.set("network", "h2")
			ho := map[string]any{}
			if h, ok := t["host"]; ok {
				ho["host"] = h
			}
			if p, ok := t["path"]; ok {
				ho["path"] = p
			}
			c.set("h2-opts", ho)
		} else {
			c.set("network", "http")
			ho := map[string]any{}
			if m, ok := t["method"]; ok {
				ho["method"] = m
			}
			if p, ok := t["path"]; ok {
				ho["path"] = []any{p}
			}
			c.set("http-opts", ho)
		}
	default:
		c.used["transport"] = false // 报告为未转换
	}
}
