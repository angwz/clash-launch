package geo

import (
	"net/netip"
	"os"
	"testing"
)

// 需要本机地理数据（go run ./tools/toolsync）；没有时跳过。只检查格式，不依赖具体归属。
func TestLookup(t *testing.T) {
	dir := "../../.cache/tools/geodata"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("没有本机地理数据")
	}
	d, err := Open(dir)
	if err != nil || d == nil {
		t.Fatalf("打开：%v", err)
	}
	defer func() { _ = d.Close() }()
	// 8.8.8.8 与 1.1.1.1 是公开的知名地址，只用于检查数据库能读出国家与 ASN
	for _, s := range []string{"8.8.8.8", "1.1.1.1"} {
		info := d.Lookup(netip.MustParseAddr(s))
		t.Logf("%s → %+v", s, info)
		if len(info.Country) != 2 || info.ASN == "" {
			t.Errorf("%s 应查到国家与 ASN：%+v", s, info)
		}
	}
	if info := d.Lookup(netip.MustParseAddr("192.0.2.1")); info.Country != "" || info.ASN != "" {
		t.Errorf("文档地址不应有归属：%+v", info)
	}
	var nilDB *DB
	if info := nilDB.Lookup(netip.MustParseAddr("8.8.8.8")); info != (Info{}) {
		t.Error("nil 数据库应返回空结果")
	}
}
