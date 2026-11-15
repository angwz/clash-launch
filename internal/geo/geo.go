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

	"github.com/oschwald/maxminddb-golang"
)

// DB 是打开的地理数据库。零值不可用；nil 表示没有可用的数据，Lookup 返回空结果。可并发使用。
type DB struct {
	country *maxminddb.Reader
	kind    string
	asn     *maxminddb.Reader
}

// Info 是一次查询的结果。字段为空表示数据库中没有该地址（如内网、保留地址）。
type Info struct {
	Country string // ISO 3166-1 alpha-2，大写，如 CN
	ASN     string // 如 AS4134
	Org     string // 运营商或组织名
}

// Open 打开目录中的 geoip.metadb 与 ASN.mmdb。两个都不存在时返回 (nil, nil)。
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
	if d.country == nil && d.asn == nil {
		return nil, nil
	}
	return d, nil
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
