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
