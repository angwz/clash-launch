// Package geo 用本机的地理数据库查询 IP 的国家与运营商（反馈 #74）。
//
// 数据来自内核检查同一份地理数据目录（CLASH_LAUNCH_GEODATA，由 tools/toolsync 按 tools.lock.json 下载并校验）：
//   - geoip.metadb：MetaCubeX 的国家库（MMDB，数据库类型 Meta-geoip0 / sing-geoip / GeoLite2-Country），值为国家代码；
//   - ASN.mmdb：GeoLite2-ASN（自治系统编号与组织名）。
//
// 读取方式与 mihomo 的 component/mmdb 相同（同一个库 oschwald/maxminddb-golang v1.12.0）。
// 运行时不访问任何外部服务。数据库缺失或无法打开时返回 nil 的 *DB，查询结果为空，调用方照常工作。
package geo

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"github.com/oschwald/maxminddb-golang"
)

// DB 是打开的地理数据库。零值不可用；nil 表示没有可用的数据，Lookup 返回空结果。可并发使用。
type DB struct {
	country *maxminddb.Reader
	kind    string
	asn     *maxminddb.Reader
	// region 是 ip2region 的 IPv4 库（需求 R7），整个读入内存；它的 Searcher 不是并发安全的，查询时加锁
	region   *xdb.Searcher
	regionMu sync.Mutex
	// region6 是可选的 ip2region IPv6 库（O8）：约 37 MB，不随镜像分发；放进地理数据目录即启用。
	// 只把向量索引读入内存（约 512 KB），记录按需从文件读取；同样不是并发安全的，共用 regionMu
	region6 *xdb.Searcher
}

// Info 是一次查询的结果。字段为空表示数据库中没有该地址（如内网、保留地址）。
type Info struct {
	Country string // ISO 3166-1 alpha-2，大写，如 CN
	ASN     string // 如 AS4134
	Org     string // 运营商或组织名
}

// ErrOptional 标记只影响可选数据（ip2region 的 IPv6 库）的错误：Open 同时返回可用的 DB，调用方记录警告后照常使用。
var ErrOptional = errors.New("geo: 可选数据不可用")

// Open 打开目录中的 geoip.metadb、ASN.mmdb、ip2region_v4.xdb 与可选的 ip2region_v6.xdb。都不存在时返回 (nil, nil)。
// IPv6 库有问题时只停用它：返回其余可用的 DB 与包装了 ErrOptional 的错误。
func Open(dir string) (*DB, error) {
	if dir == "" {
		return nil, nil
	}
	d := &DB{}
	if r, err := maxminddb.Open(filepath.Join(dir, "geoip.metadb")); err == nil {
		d.country, d.kind = r, r.Metadata.DatabaseType
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("geo: 打开 geoip.metadb：%w", err)
	}
	if r, err := maxminddb.Open(filepath.Join(dir, "ASN.mmdb")); err == nil {
		d.asn = r
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("geo: 打开 ASN.mmdb：%w", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, regionFile)); err == nil {
		s, err := xdb.NewWithBuffer(xdb.IPv4, b)
		if err != nil {
			return nil, fmt.Errorf("geo: 打开 %s：%w", regionFile, err)
		}
		d.region = s
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("geo: 读取 %s：%w", regionFile, err)
	}
	s6, err6 := openRegion6(filepath.Join(dir, region6File))
	d.region6 = s6
	if d.country == nil && d.asn == nil && d.region == nil && d.region6 == nil {
		if err6 != nil {
			return nil, err6
		}
		return nil, nil
	}
	if err6 != nil {
		return d, fmt.Errorf("%w：%w", ErrOptional, err6)
	}
	return d, nil
}

// openRegion6 打开可选的 IPv6 库；文件不存在时返回 (nil, nil)。文件头声明的不是 IPv6 库（例如把 v4 库改了名）时报错，
// 而不是在查询时得到错误的结果。
func openRegion6(path string) (*xdb.Searcher, error) {
	h, err := xdb.LoadHeaderFromFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("geo: 读取 %s：%w", region6File, err)
	}
	v, err := xdb.VersionFromHeader(h)
	if err != nil || v.Id != xdb.IPv6.Id {
		return nil, fmt.Errorf("geo: %s 不是 ip2region 的 IPv6 库", region6File)
	}
	idx, err := xdb.LoadVectorIndexFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("geo: 读取 %s 的索引：%w", region6File, err)
	}
	s, err := xdb.NewWithVectorIndex(xdb.IPv6, path, idx)
	if err != nil {
		return nil, fmt.Errorf("geo: 打开 %s：%w", region6File, err)
	}
	return s, nil
}

// Close 释放数据库。
func (d *DB) Close() error {
	if d == nil {
		return nil
	}
	var errs []error
	if d.country != nil {
		errs = append(errs, d.country.Close())
	}
	if d.asn != nil {
		errs = append(errs, d.asn.Close())
	}
	if d.region6 != nil {
		d.region6.Close()
	}
	return errors.Join(errs...)
}

// Lookup 查询地址。查不到的字段留空；不会返回错误（统计用途，不能因为查询失败影响请求）。
func (d *DB) Lookup(a netip.Addr) Info {
	var out Info
	if d == nil || !a.IsValid() {
		return out
	}
	ip := net.IP(a.Unmap().AsSlice())
	if d.country != nil {
		// 只接受两位国家代码：Meta-geoip0 对内网与保留地址返回 private 等标记
		if c := d.lookupCountry(ip); len(c) == 2 {
			out.Country = strings.ToUpper(c)
		}
	}
	if d.asn != nil {
		var r struct {
			Number uint32 `maxminddb:"autonomous_system_number"`
			Org    string `maxminddb:"autonomous_system_organization"`
		}
		if err := d.asn.Lookup(ip, &r); err == nil && r.Number != 0 {
			out.ASN = fmt.Sprintf("AS%d", r.Number)
			out.Org = r.Org
		}
	}
	return out
}

// lookupCountry 按数据库类型解析国家代码（与 mihomo component/mmdb 的 LookupCode 一致，取第一个代码）。
func (d *DB) lookupCountry(ip net.IP) string {
	switch d.kind {
	case "sing-geoip":
		var code string
		_ = d.country.Lookup(ip, &code)
		return code
	case "Meta-geoip0":
		var rec any
		_ = d.country.Lookup(ip, &rec)
		switch v := rec.(type) {
		case string:
			return v
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok && len(s) == 2 {
					return s
				}
			}
		}
		return ""
	default: // MaxMind GeoLite2-Country 及兼容格式
		var r struct {
			Country struct {
				ISO string `maxminddb:"iso_code"`
			} `maxminddb:"country"`
		}
		_ = d.country.Lookup(ip, &r)
		return r.Country.ISO
	}
}
