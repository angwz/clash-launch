package kernel

import (
	"reflect"
	"testing"
)

// mihomo v1.19.34 起，network=httpupgrade 的分享链接转为 ws + ws-opts.v2ray-http-upgrade: true
// （common/convert 的 converter.go、v.go）。走 ws 分支后同样会写入随机的浏览器 UA，这里也要可复现。
func TestParseLinkHTTPUpgrade(t *testing.T) {
	links := []string{
		"trojan://example-pw@192.0.2.10:443?type=httpupgrade&path=%2Fup&host=up.example.com&sni=up.example.com#T",
		"vless://00000000-0000-4000-8000-000000000001@192.0.2.11:443?type=httpupgrade&path=%2Fup&host=up.example.com&security=tls&sni=up.example.com#V",
	}
	for _, l := range links {
		a := ParseLink(l, 1)
		if a.Err != "" {
			t.Fatalf("%s：%s", l, a.Err)
		}
		if a.Mapping["network"] != "ws" {
			t.Errorf("%s：network = %v，应为 ws", l, a.Mapping["network"])
		}
		ws, _ := a.Mapping["ws-opts"].(map[string]any)
		if ws["v2ray-http-upgrade"] != true || ws["path"] != "/up" {
			t.Errorf("%s：ws-opts = %v", l, ws)
		}
		if b := ParseLink(l, 1); !reflect.DeepEqual(a.Mapping, b.Mapping) {
			t.Errorf("%s：两次解析结果不同（UA 应按内容固定）：%v vs %v", l, a.Mapping, b.Mapping)
		}
	}
}
