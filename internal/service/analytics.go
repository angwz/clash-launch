package service

import (
	"slices"
	"sort"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 订阅分析（反馈 #74）：把一段时间内的请求记录汇总成图表与表格需要的数据。纯函数，便于测试。
//
// 时间按用户的时区分桶（每天、星期 × 小时），未设置时按调用方传入的浏览器时区或 UTC。
// 「拉取间隔」取同一 IP 相邻两次成功请求间隔的中位数，忽略 10 秒内的重复（客户端重试、并发）。
// 「新出现的 IP」：在统计区间内第一次出现、且时间在最近 24 小时内的 IP（区间不足一天时不判断）。

// AnalyticsMaxRows 是一次分析最多读取的请求数；超过时只分析最新的这些条，并在结果中注明。
const AnalyticsMaxRows = 50_000

// Analytics 是分析结果。
type Analytics struct {
	From      time.Time    `json:"from"`
	To        time.Time    `json:"to"`
	Zone      string       `json:"zone"`
	Truncated bool         `json:"truncated"`
	Totals    AnTotals     `json:"totals"`
	Days      []string     `json:"days"`  // YYYY-MM-DD（所选时区）
	Daily     []int        `json:"daily"` // 与 Days 对齐
	Heat      [7][24]int   `json:"heat"`  // 星期一为 0
	Countries []AnCount    `json:"countries"`
	Orgs      []AnCount    `json:"orgs"`
	Clients   []AnCount    `json:"clients"`
	Kinds     []AnCount    `json:"kinds"`
	Links     []AnCount    `json:"links"`  // key = 链接 ID
	Shorts    []AnCount    `json:"shorts"` // key = 短链接 ID
	IPs       []AnIP       `json:"ips"`
	Recent    []AnRequest  `json:"recent"`
	NewIPs    int          `json:"newIps"`
	Peak      *AnPeakCount `json:"peak,omitempty"`
	// Friends 是好友经由授权短链接的使用情况（反馈 #75），由调用方填入
	Friends []AnFriend `json:"friends"`
}

// AnTotals 是区间内的总数。
type AnTotals struct {
	Requests  int `json:"requests"`
	OK        int `json:"ok"`
	Fail      int `json:"fail"`
	IPs       int `json:"ips"`
	Countries int `json:"countries"`
	Clients   int `json:"clients"`
}

// AnCount 是一个排行项。
type AnCount struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

// AnPeakCount 是请求最多的一天。
type AnPeakCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// AnIP 是一个 IP 的明细。
type AnIP struct {
	IP       string    `json:"ip"`
	Country  string    `json:"country"`
	ASN      string    `json:"asn"`
	Org      string    `json:"org"`
	Client   string    `json:"client"`
	Count    int       `json:"count"`
	OK       int       `json:"ok"`
	Fail     int       `json:"fail"`
	First    time.Time `json:"first"`
	Last     time.Time `json:"last"`
	Interval int64     `json:"interval"` // 秒；0 表示样本不足
	New      bool      `json:"new"`
}

// AnRequest 是一条请求（最近的请求列表）。
type AnRequest struct {
	At      time.Time `json:"at"`
	IP      string    `json:"ip"`
	Country string    `json:"country"`
	Org     string    `json:"org"`
	Client  string    `json:"client"`
	UA      string    `json:"ua"`
	Status  int       `json:"status"`
	Kind    string    `json:"kind"`
	LinkID  string    `json:"linkId"`
	ShortID string    `json:"shortId,omitempty"`
}

// Analyze 汇总请求。rs 按时间正序；[from, to) 是统计区间；loc 是分桶用的时区。
func Analyze(rs []store.SubRequest, from, to time.Time, loc *time.Location, truncated bool) *Analytics {
	a := &Analytics{From: from, To: to, Zone: loc.String(), Truncated: truncated}
	// 按所选时区逐日列出区间内的日期
	idx := map[string]int{}
	for d := from.In(loc); d.Before(to); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		if _, ok := idx[k]; !ok {
			idx[k] = len(a.Days)
			a.Days = append(a.Days, k)
		}
	}
	if k := to.Add(-time.Millisecond).In(loc).Format("2006-01-02"); len(a.Days) == 0 || a.Days[len(a.Days)-1] != k {
		idx[k] = len(a.Days)
		a.Days = append(a.Days, k)
	}
	a.Daily = make([]int, len(a.Days))

	countries, orgs, clients, kinds, links, shorts := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	orgLabel := map[string]string{}
	ips := map[string]*AnIP{}
	times := map[string][]time.Time{}
	for _, q := range rs {
		a.Totals.Requests++
		if q.Status < 400 {
			a.Totals.OK++
		} else {
			a.Totals.Fail++
		}
		t := q.At.In(loc)
		if i, ok := idx[t.Format("2006-01-02")]; ok {
			a.Daily[i]++
		}
		a.Heat[(int(t.Weekday())+6)%7][t.Hour()]++
		if q.Country != "" {
			countries[q.Country]++
		}
		if q.ASN != "" {
			orgs[q.ASN]++
			orgLabel[q.ASN] = q.Org
		}
		if q.Client != "" {
			clients[q.Client]++
		}
		kind := q.Kind
		if q.ShortID != "" && kind == "config" {
			kind = "short"
		}
		kinds[kind]++
		links[q.LinkID]++
		if q.ShortID != "" {
			shorts[q.ShortID]++
		}
		x := ips[q.IP]
		if x == nil {
			x = &AnIP{IP: q.IP, First: q.At}
			ips[q.IP] = x
		}
		x.Count++
		if q.Status < 400 {
			x.OK++
			times[q.IP] = append(times[q.IP], q.At)
		} else {
			x.Fail++
		}
		x.Last = q.At
		// 归属地与客户端取最近一次非空的值
		if q.Country != "" {
			x.Country = q.Country
		}
		if q.ASN != "" {
			x.ASN, x.Org = q.ASN, q.Org
		}
		if q.Client != "" {
			x.Client = q.Client
		}
	}
	a.Totals.IPs, a.Totals.Countries, a.Totals.Clients = len(ips), len(countries), len(clients)
	a.Countries, a.Clients, a.Kinds, a.Links, a.Shorts = rank(countries, 30), rank(clients, 20), rank(kinds, 10), rank(links, 50), rank(shorts, 50)
	a.Orgs = rank(orgs, 20)
	for i := range a.Orgs {
		a.Orgs[i].Label = orgLabel[a.Orgs[i].Key]
	}

	recentCut := to.Add(-24 * time.Hour)
	judgeNew := to.Sub(from) > 24*time.Hour
	for ip, x := range ips {
		x.Interval = medianGap(times[ip])
		x.New = judgeNew && !x.First.Before(recentCut)
		if x.New {
			a.NewIPs++
		}
		a.IPs = append(a.IPs, *x)
	}
	sort.Slice(a.IPs, func(i, j int) bool {
		if a.IPs[i].Count != a.IPs[j].Count {
			return a.IPs[i].Count > a.IPs[j].Count
		}
		return a.IPs[i].IP < a.IPs[j].IP
	})
	if len(a.IPs) > 200 {
		a.IPs = a.IPs[:200]
	}

	for i, n := range a.Daily {
		if n > 0 && (a.Peak == nil || n > a.Peak.Count) {
			a.Peak = &AnPeakCount{Day: a.Days[i], Count: n}
		}
	}
	for i := len(rs) - 1; i >= 0 && len(a.Recent) < 100; i-- {
		q := rs[i]
		a.Recent = append(a.Recent, AnRequest{At: q.At, IP: q.IP, Country: q.Country, Org: q.Org, Client: q.Client, UA: q.UA, Status: q.Status, Kind: q.Kind, LinkID: q.LinkID, ShortID: q.ShortID})
	}
	return a
}

// rank 按次数降序（相同按键名）取前 n 项。
func rank(m map[string]int, n int) []AnCount {
	out := make([]AnCount, 0, len(m))
	for k, v := range m {
		out = append(out, AnCount{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// medianGap 返回相邻时间间隔的中位数（秒），忽略 10 秒内的重复；样本不足两个间隔时返回 0。
func medianGap(ts []time.Time) int64 {
	var gaps []int64
	for i := 1; i < len(ts); i++ {
		if g := ts[i].Sub(ts[i-1]); g >= 10*time.Second {
			gaps = append(gaps, int64(g/time.Second))
		}
	}
	if len(gaps) < 2 {
		return 0
	}
	slices.Sort(gaps)
	return gaps[len(gaps)/2]
}

// AnFriend 是一位好友通过授权的短链接使用你的配置的情况（反馈 #75）。
type AnFriend struct {
	UserID    string    `json:"userId"`
	Requests  int       `json:"requests"`
	OK        int       `json:"ok"`
	Fail      int       `json:"fail"`
	IPs       int       `json:"ips"`
	Countries []string  `json:"countries"` // 出现过的国家（按次数降序）
	Orgs      []string  `json:"orgs"`      // 出现过的运营商（按次数降序，最多 5 个）
	Clients   []string  `json:"clients"`   // 出现过的客户端（按次数降序）
	Shorts    []string  `json:"shorts"`    // 经由的短链接 ID
	Daily     []int     `json:"daily"`     // 与 Analytics.Days 对齐（区间起点是所选时区的零点，按 24 小时切分）
	First     time.Time `json:"first"`
	Last      time.Time `json:"last"`
	Interval  int64     `json:"interval"` // 典型拉取间隔（秒），0 表示样本不足
}

// FriendUsage 按好友汇总经由授权短链接的请求。granteeOf 是短链接 ID → 好友 ID。
// 返回值按请求次数降序；没有请求的好友不列出（界面另外列出全部授权）。
func FriendUsage(rs []store.SubRequest, granteeOf map[string]string, from, to time.Time) []AnFriend {
	type acc struct {
		f                        *AnFriend
		ips                      map[string]bool
		countries, orgs, clients map[string]int
		shorts                   map[string]bool
		times                    []time.Time
	}
	days := int(to.Sub(from).Hours()/24 + 0.5)
	by := map[string]*acc{}
	var order []string
	for _, q := range rs {
		uid := granteeOf[q.ShortID]
		if uid == "" {
			continue
		}
		a := by[uid]
		if a == nil {
			a = &acc{f: &AnFriend{UserID: uid, First: q.At, Daily: make([]int, max(days, 1))}, ips: map[string]bool{}, countries: map[string]int{}, orgs: map[string]int{}, clients: map[string]int{}, shorts: map[string]bool{}}
			by[uid] = a
			order = append(order, uid)
		}
		f := a.f
		f.Requests++
		if q.Status < 400 {
			f.OK++
			a.times = append(a.times, q.At)
		} else {
			f.Fail++
		}
		f.Last = q.At
		a.ips[q.IP] = true
		if q.Country != "" {
			a.countries[q.Country]++
		}
		if q.Org != "" {
			a.orgs[q.Org]++
		}
		if q.Client != "" {
			a.clients[q.Client]++
		}
		a.shorts[q.ShortID] = true
		if i := int(q.At.Sub(from).Hours() / 24); i >= 0 && i < len(f.Daily) {
			f.Daily[i]++
		}
	}
	keys := func(m map[string]int, n int) []string {
		out := []string{}
		for _, c := range rank(m, n) {
			out = append(out, c.Key)
		}
		return out
	}
	out := make([]AnFriend, 0, len(order))
	for _, uid := range order {
		a := by[uid]
		a.f.IPs = len(a.ips)
		a.f.Countries, a.f.Orgs, a.f.Clients = keys(a.countries, 20), keys(a.orgs, 5), keys(a.clients, 10)
		for id := range a.shorts {
			a.f.Shorts = append(a.f.Shorts, id)
		}
		sort.Strings(a.f.Shorts)
		a.f.Interval = medianGap(a.times)
		out = append(out, *a.f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}
