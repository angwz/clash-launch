package match

import (
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/component/geodata"
	"github.com/metacubex/mihomo/component/geodata/router"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules"
)

// geodataDir 返回本机的地理数据目录（tools/toolsync 下载并校验）；没有时跳过。
func geodataDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("CLASH_LAUNCH_GEODATA")
	if dir == "" {
		dir = filepath.Join("..", "..", ".cache", "tools", "geodata")
	}
	if _, err := os.Stat(filepath.Join(dir, "geosite.dat")); err != nil {
		t.Skipf("没有地理数据（%s）：运行 go run ./tools/toolsync 或设置 CLASH_LAUNCH_GEODATA", dir)
	}
	return dir
}

// kernelHome 把地理数据复制到临时目录并设为内核的数据目录，供对照组使用。
// 复制是因为内核在校验失败时会删除并重新下载文件；下载地址同时指向不可达的本机端口，确保测试不联网。
func kernelHome(t *testing.T, dir string) {
	t.Helper()
	home := t.TempDir()
	for _, name := range []string{"geosite.dat", "geoip.dat", "geoip.metadb", "ASN.mmdb"} {
		src, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		dst, err := os.Create(filepath.Join(home, name))
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(dst, src)
		_ = src.Close()
		if cerr := dst.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, set := range []func(string){geodata.SetGeoIpUrl, geodata.SetMmdbUrl, geodata.SetGeoSiteUrl, geodata.SetASNUrl} {
		set("http://127.0.0.1:1/blocked")
	}
	C.SetHomeDir(home)
}

// TestGeoMatchesKernel 是第 0 步「进程内加载地理数据」的验证：同一份数据上，本包的 GEOSITE、GEOIP（MMDB 与
// geodata 两种模式）、IP-ASN 与内核自己的规则对象（rules.ParseRule，走全局加载器）逐个比较。
// 测试用的域名与 IP 从数据本身取出，不写入任何真实地址。
func TestGeoMatchesKernel(t *testing.T) {
	dir := geodataDir(t)
	geo, err := OpenGeo(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer geo.Close()
	kernelHome(t, dir)

	l, err := loader()
	if err != nil {
		t.Fatal(err)
	}
	site, err := os.ReadFile(filepath.Join(dir, "geosite.dat"))
	if err != nil {
		t.Fatal(err)
	}
	ipdat, err := os.ReadFile(filepath.Join(dir, "geoip.dat"))
	if err != nil {
		t.Fatal(err)
	}

	// 域名样本：取 cn 列表中前若干条 Domain / Full 条目本身及其子域名，另加保留域名。
	cnDomains, err := l.LoadSiteByBytes(site, "cn")
	if err != nil {
		t.Fatal(err)
	}
	hosts := []string{"www.example.com", "example.net"}
	for _, d := range cnDomains {
		if len(hosts) > 400 {
			break
		}
		switch d.Type {
		case router.Domain_Domain:
			hosts = append(hosts, d.Value, "sub."+d.Value)
		case router.Domain_Full:
			hosts = append(hosts, d.Value)
		}
	}
	// IP 样本：cn、us 网段中的地址，另加文档地址。
	ips := []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")}
	for _, cc := range []string{"cn", "us"} {
		cidrs, err := l.LoadIPByBytes(ipdat, cc)
		if err != nil {
			t.Fatal(err)
		}
		for i, c := range cidrs {
			if i >= 150 {
				break
			}
			if a, ok := netip.AddrFromSlice(c.Ip); ok {
				ips = append(ips, a.Unmap().Next())
			}
		}
	}

	compare := func(t *testing.T, raw string, env Env, inputs []Request) {
		t.Helper()
		tp, payload, target := strings.SplitN(raw, ",", 3)[0], strings.SplitN(raw, ",", 3)[1], "X"
		kr, kerr := rules.ParseRule(tp, payload, target, nil, nil)
		ours, info, oerr := build(tp, payload, target, nil, env, nil, false)
		if (kerr == nil) != (oerr == nil) {
			t.Fatalf("%s: 内核错误 %v，本包错误 %v", raw, kerr, oerr)
		}
		if kerr != nil {
			return
		}
		if info.reason != "" {
			t.Fatalf("%s: 有数据时不应无法判断：%s", raw, info.reason)
		}
		n := 0
		for _, r := range inputs {
			mdK := &C.Metadata{Host: r.Host, DstIP: r.IP}
			mdO := &C.Metadata{Host: r.Host, DstIP: r.IP}
			k, _ := kr.Match(mdK, C.RuleMatchHelper{})
			o, _ := ours.Match(mdO, C.RuleMatchHelper{})
			if k != o {
				t.Errorf("%s %s%s: 内核 %v，本包 %v", raw, r.Host, r.IP, k, o)
			}
			if k {
				n++
			}
		}
		t.Logf("%s: %d 个样本中 %d 个命中", raw, len(inputs), n)
		if n == 0 && !strings.HasPrefix(payload, "!") {
			t.Errorf("%s: 样本全部不命中，对照没有意义", raw)
		}
	}

	var hostReqs, ipReqs []Request
	for _, h := range hosts {
		hostReqs = append(hostReqs, Request{Host: h})
	}
	for _, ip := range ips {
		ipReqs = append(ipReqs, Request{IP: ip})
	}

	t.Run("GEOSITE", func(t *testing.T) {
		for _, raw := range []string{"GEOSITE,cn", "GEOSITE,CN", "GEOSITE,!cn"} {
			compare(t, raw, Env{Geo: geo}, hostReqs)
		}
	})
	t.Run("GEOIP-mmdb", func(t *testing.T) {
		geodata.SetGeodataMode(false)
		for _, raw := range []string{"GEOIP,CN", "GEOIP,us"} {
			compare(t, raw, Env{Geo: geo}, ipReqs)
		}
	})
	t.Run("GEOIP-geodata", func(t *testing.T) {
		geodata.SetGeodataMode(true)
		defer geodata.SetGeodataMode(false)
		for _, raw := range []string{"GEOIP,CN", "GEOIP,us", "GEOIP,!cn"} {
			compare(t, raw, Env{Geo: geo, GeodataMode: true}, ipReqs)
		}
	})
	t.Run("IP-ASN", func(t *testing.T) {
		// 取第一个有 ASN 的样本的编号作为规则载荷。
		asn := ""
		for _, ip := range ips {
			md := &C.Metadata{DstIP: ip}
			r, _, _ := build("IP-ASN", "0", "X", nil, Env{Geo: geo}, nil, false)
			r.Match(md, C.RuleMatchHelper{})
			if f := strings.Fields(md.DstIPASN); len(f) > 0 && f[0] != "0" {
				asn = f[0]
				break
			}
		}
		if asn == "" {
			t.Skip("样本中没有可查到 ASN 的地址")
		}
		compare(t, "IP-ASN,"+asn, Env{Geo: geo}, ipReqs)
	})
}
