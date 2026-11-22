package geo

import (
	"net/netip"
	"strings"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

// regionFile 是 ip2region 的 IPv4 库（lionsoul2014/ip2region，Apache-2.0 或 MIT），由 toolsync 下载到地理数据目录。
// 镜像只带 IPv4 库：IPv6 库约 37 MB，体积代价过大（O8）。需要 IPv6 地址的省市时，把 region6File 放进地理数据目录即可；
// 没有它时 IPv6 地址退回国家库与 ASN 库。
const (
	regionFile  = "ip2region_v4.xdb"
	region6File = "ip2region_v6.xdb"
)

// Place 是一个地址所在的地方（需求 R7），用于登录设备的展示。字段为空表示查不到。
//
// 分层：国内（以及 ip2region 覆盖到的其他地址）的省、市与运营商来自 ip2region；国家代码优先取国家库，
// 国家库没有时用 ip2region 的；运营商在 ip2region 没有时退回 ASN 库的组织名。
// ip2region 的国内地名是中文，其他国家多为英文，原样保留；国家名由前端按界面语言生成。
type Place struct {
	Country  string `json:"cc,omitempty"` // ISO 3166-1 alpha-2，大写
	Province string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	ISP      string `json:"isp,omitempty"`
}

// regionFor 选择地址对应的 ip2region 库；没有时返回 nil。
func (d *DB) regionFor(a netip.Addr) *xdb.Searcher {
	if a.Is4() && d.region != nil {
		return d.region
	}
	if a.Is6() && d.region6 != nil {
		return d.region6
	}
	return nil
}

// Empty 报告是否什么都没查到。
func (p Place) Empty() bool { return p == Place{} }

// Place 查询地址所在的地方。查不到的字段留空，不返回错误。
func (d *DB) Place(a netip.Addr) Place {
	var p Place
	if d == nil || !a.IsValid() {
		return p
	}
	a = a.Unmap()
	info := d.Lookup(a)
	p.Country, p.ISP = info.Country, info.Org
	if s := d.regionFor(a); s != nil {
		if f := d.searchRegion(s, a); f != nil {
			if p.Country == "" && len(f[4]) == 2 {
				p.Country = strings.ToUpper(f[4])
			}
			// ip2region 与国家库判断的国家不同时，省市不可信：只保留国家库的结论
			if p.Country == "" || strings.EqualFold(p.Country, f[4]) {
				p.Province, p.City = f[1], f[2]
				if f[3] != "" {
					p.ISP = f[3]
				}
			}
		}
	}
	if p.City == p.Province { // 直辖市：省与市同名（如「上海市」），只显示一次
		p.City = ""
	}
	return p
}

// searchRegion 返回 ip2region 的五个字段：国家|省|市|运营商|国家代码。「0」表示空，统一换成空串；
// 保留地址（国家为 Reserved）与格式不符的记录返回 nil。
func (d *DB) searchRegion(sr *xdb.Searcher, a netip.Addr) []string {
	d.regionMu.Lock()
	s, err := sr.Search(a.String())
	d.regionMu.Unlock()
	if err != nil {
		return nil
	}
	f := strings.Split(s, "|")
	if len(f) != 5 || f[0] == "Reserved" {
		return nil
	}
	for i := range f {
		if f[i] == "0" {
			f[i] = ""
		}
	}
	return f
}
