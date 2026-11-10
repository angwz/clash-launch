package main

import (
	"testing"

	catalogdata "github.com/angwz/clash-launch/catalog"
	"github.com/angwz/clash-launch/internal/catalog"
)

// 分类规则：判别值占一段、列表与 map 透明、源码本身为 any 时计为 free、登记过的路径计为 skipped。
func TestCoverageClassify(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS, "mihomo")
	if err != nil {
		t.Fatal(err)
	}
	res := Result{Fields: []Field{
		{Path: "dns.enhanced-mode", Type: "string"},
		{Path: "proxies.vless.xhttp-opts.download-settings.reality-opts.public-key", Type: "string"},
		{Path: "proxies.wireguard.peers.public-key", Type: "string"},
		{Path: "proxies.ss.plugin-opts", Type: "map<any>"},
		{Path: "proxies.vless.ws-headers", Type: "map<string>"},
		{Path: "proxies.vless.no-such-field", Type: "string"},
	}}
	cv := coverage(res, cat)
	want := map[string]int{covCovered: 3, covFree: 1, covSkipped: 1, covMissing: 1}
	for k, n := range want {
		if cv.Counts[k] != n {
			t.Errorf("%s = %d，期望 %d（%v）", k, cv.Counts[k], n, cv.Counts)
		}
	}
}
