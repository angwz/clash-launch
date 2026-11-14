package node

import (
	"strings"
	"testing"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/yamlx"
)

// 密钥为虚构内容（"example-X-key-not-real-000000" 的 base64）；地址用文档保留段与 example.com。
const wgConf = `# 示例
[Interface]
PrivateKey = ZXhhbXBsZS1BLWtleS1ub3QtcmVhbC0wMDAwMDA=
Address = 10.0.0.2/32, 2001:db8::2/128
DNS = 192.0.2.53
MTU = 1280
ListenPort = 51820

[Peer]
PublicKey = ZXhhbXBsZS1CLWtleS1ub3QtcmVhbC0wMDAwMDA=
PresharedKey = ZXhhbXBsZS1DLWtleS1ub3QtcmVhbC0wMDAwMDA=
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

func TestParseWireGuard(t *testing.T) {
	var l diag.List
	nodes, f := Parse([]byte(wgConf), "src_wg", cat(t), &l)
	if f != FormatWireGuard || len(nodes) != 1 {
		t.Fatalf("格式 %s，节点 %d；%v", f, len(nodes), l.Items())
	}
	n := nodes[0]
	if n.Type != "wireguard" || n.Server != "vpn.example.com" || n.Port != 51820 || n.Name != "WireGuard vpn.example.com:51820" {
		t.Errorf("节点：%+v", n)
	}
	want := map[string]string{"ip": "10.0.0.2/32", "ipv6": "2001:db8::2/128", "mtu": "1280", "persistent-keepalive": "25", "udp": "true",
		"public-key": "ZXhhbXBsZS1CLWtleS1ub3QtcmVhbC0wMDAwMDA=", "pre-shared-key": "ZXhhbXBsZS1DLWtleS1ub3QtcmVhbC0wMDAwMDA="}
	for k, v := range want {
		if got := yamlx.Get(n.Mapping, k); got == nil || got.Value != v {
			t.Errorf("%s = %v，期望 %s", k, got, v)
		}
	}
	if a := yamlx.Get(n.Mapping, "allowed-ips"); a == nil || len(a.Content) != 2 {
		t.Errorf("allowed-ips：%v", a)
	}
	// DNS 与 ListenPort 不转换，逐个报告
	var fields []string
	for _, d := range l.Items() {
		if d.Code == diag.NodeWireGuardField {
			fields = append(fields, d.Args["field"])
		}
	}
	if strings.Join(fields, ",") != "DNS,ListenPort" {
		t.Errorf("未转换的设置：%v", fields)
	}
}

func TestParseWireGuardPeers(t *testing.T) {
	conf := "[Interface]\nPrivateKey = ZXhhbXBsZS1BLWtleS1ub3QtcmVhbC0wMDAwMDA=\nAddress = 10.0.0.2/32\n" +
		"[Peer]\nPublicKey = ZXhhbXBsZS1CLWtleS1ub3QtcmVhbC0wMDAwMDA=\nEndpoint = 192.0.2.1:51820\nAllowedIPs = 10.1.0.0/16\n" +
		"[Peer]\nPublicKey = ZXhhbXBsZS1ELWtleS1ub3QtcmVhbC0wMDAwMDA=\nEndpoint = [2001:db8::1]:51821\nAllowedIPs = 10.2.0.0/16\n"
	var l diag.List
	nodes, _ := Parse([]byte(conf), "src_wg", cat(t), &l)
	if len(nodes) != 1 {
		t.Fatalf("节点 %d；%v", len(nodes), l.Items())
	}
	peers := yamlx.Get(nodes[0].Mapping, "peers")
	if peers == nil || len(peers.Content) != 2 || yamlx.Get(peers.Content[1], "server").Value != "2001:db8::1" {
		t.Errorf("peers：%v", peers)
	}
}

func TestParseWireGuardMissingPeer(t *testing.T) {
	var l diag.List
	nodes, f := Parse([]byte("[Interface]\nPrivateKey = x\n"), "src_wg", cat(t), &l)
	if f != FormatWireGuard || len(nodes) != 0 || !l.Has(diag.NodeUnrecognized) {
		t.Errorf("缺少 [Peer] 应报告：%d %v", len(nodes), l.Items())
	}
}
