package service

import (
	"net/netip"
	"slices"
	"sort"
	"time"

	"github.com/angwz/clash-launch/internal/geo"
	"github.com/angwz/clash-launch/internal/store"
)

// 订阅分析（反馈 #74）：把一段时间内的请求记录汇总成图表与表格需要的数据。
//
// 汇总是流式的（需求 R14c）：存储层按时间正序逐条交给 Analyzer.Add，这里只保留各维度的计数，
// 不保留请求本身，所以没有条数上限。内存与「不同的 IP、国家、客户端……的个数」成正比，
// 另外每个 IP（每位好友）最多保留 gapSamples 个最近的成功请求时间，用来求拉取间隔。
// 不读时钟、不依赖 map 迭代顺序：相同输入得到相同结果。
//
// 时间按用户的时区分桶（每天、星期 × 小时），未设置时按调用方传入的浏览器时区或 UTC。
// 「拉取间隔」取同一 IP 最近相邻两次成功请求间隔的中位数，忽略 10 秒内的重复（客户端重试、并发）。
// 「新出现的 IP」：在统计区间内第一次出现、且时间在最近 24 小时内的 IP（区间不足一天时不判断）。

// gapSamples 是求拉取间隔时每个 IP 最多使用的最近成功请求数。
const gapSamples = 1000

// RecentRequests 是「最近的请求」列表的条数，由调用方另外查询（需要 UA 等完整列）。
const RecentRequests = 100

// Analytics 是分析结果。
type Analytics struct {
	From      time.Time    `json:"from"`
	To        time.Time    `json:"to"`
	Zone      string       `json:"zone"`
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
	// Friends 是好友经由授权短链接的使用情况（反馈 #75）
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
	// Place 是省市与运营商（O7，按当前的本地地理库查询，见 Locate）；查不到时省略
	Place *geo.Place `json:"place,omitempty"`
}

// AnRequest 是一条请求（最近的请求列表）。
type AnRequest struct {
	At      time.Time  `json:"at"`
	IP      string     `json:"ip"`
	Country string     `json:"country"`
	Org     string     `json:"org"`
	Client  string     `json:"client"`
	UA      string     `json:"ua"`
	Status  int        `json:"status"`
	Kind    string     `json:"kind"`
	LinkID  string     `json:"linkId"`
	ShortID string     `json:"shortId,omitempty"`
	Place   *geo.Place `json:"place,omitempty"`
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

// Analyzer 流式汇总请求。用法：NewAnalyzer → 按时间正序多次 Add → Result。
type Analyzer struct {
	from, to  time.Time
	loc       *time.Location
	granteeOf map[string]string // 短链接 ID → 好友 ID
	a         *Analytics
	dayIdx    map[int]int // 所选时区的 YYYYMMDD → Days 下标

	countries, orgs, clients, kinds, links, shorts map[string]int
	orgLabel                                       map[string]string
	ips                                            map[string]*ipAcc
	friends                                        map[string]*friendAcc
	friendOrder                                    []string
	friendDays                                     int
}

type ipAcc struct {
	x     AnIP
	times tail
}

type friendAcc struct {
	f                        AnFriend
	ips                      map[string]bool
	countries, orgs, clients map[string]int
	shorts                   map[string]bool
	times                    tail
}

// tail 保留最近 gapSamples+1 个时间（摊还 O(1)：满两倍时把后一半挪到前面）。
type tail struct{ ts []time.Time }

func (t *tail) add(x time.Time) {
	if len(t.ts) == 2*(gapSamples+1) {
		t.ts = t.ts[:copy(t.ts, t.ts[gapSamples+1:])]
	}
	t.ts = append(t.ts, x)
}

func (t *tail) last() []time.Time {
	if n := len(t.ts) - (gapSamples + 1); n > 0 {
		return t.ts[n:]
	}
	return t.ts
}

// NewAnalyzer 准备汇总 [from, to) 内的请求；loc 是分桶用的时区，granteeOf 是为好友生成的短链接 → 好友。
func NewAnalyzer(from, to time.Time, loc *time.Location, granteeOf map[string]string) *Analyzer {
	z := &Analyzer{
		from: from, to: to, loc: loc, granteeOf: granteeOf,
		a:         &Analytics{From: from, To: to, Zone: loc.String()},
		dayIdx:    map[int]int{},
		countries: map[string]int{}, orgs: map[string]int{}, clients: map[string]int{}, kinds: map[string]int{}, links: map[string]int{}, shorts: map[string]int{},
		orgLabel: map[string]string{}, ips: map[string]*ipAcc{}, friends: map[string]*friendAcc{},
		friendDays: max(int(to.Sub(from).Hours()/24+0.5), 1),
	}
	// 按所选时区逐日列出区间内的日期
	addDay := func(t time.Time) {
		if k := dayKey(t); !hasKey(z.dayIdx, k) {
			z.dayIdx[k] = len(z.a.Days)
			z.a.Days = append(z.a.Days, t.Format("2006-01-02"))
		}
	}
	for d := from.In(loc); d.Before(to); d = d.AddDate(0, 0, 1) {
		addDay(d)
	}
	addDay(to.Add(-time.Millisecond).In(loc))
	z.a.Daily = make([]int, len(z.a.Days))
	return z
}

func hasKey(m map[int]int, k int) bool {
	_, ok := m[k]
	return ok
}

func dayKey(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}

// Add 汇总一条请求。调用方可以在返回后复用 q。
func (z *Analyzer) Add(q *store.SubRequest) {
	a := z.a
	ok := q.Status < 400
	a.Totals.Requests++
	if ok {
		a.Totals.OK++
	} else {
		a.Totals.Fail++
	}
	t := q.At.In(z.loc)
	if i, found := z.dayIdx[dayKey(t)]; found {
		a.Daily[i]++
	}
	a.Heat[(int(t.Weekday())+6)%7][t.Hour()]++
	if q.Country != "" {
		z.countries[q.Country]++
	}
	if q.ASN != "" {
		z.orgs[q.ASN]++
		z.orgLabel[q.ASN] = q.Org
	}
	if q.Client != "" {
		z.clients[q.Client]++
	}
	kind := q.Kind
	if q.ShortID != "" && kind == "config" {
		kind = "short"
	}
	z.kinds[kind]++
	z.links[q.LinkID]++
	if q.ShortID != "" {
		z.shorts[q.ShortID]++
	}

	x := z.ips[q.IP]
	if x == nil {
		x = &ipAcc{x: AnIP{IP: q.IP, First: q.At}}
		z.ips[q.IP] = x
	}
	x.x.Count++
	if ok {
		x.x.OK++
		x.times.add(q.At)
	} else {
		x.x.Fail++
	}
	x.x.Last = q.At
	// 归属地与客户端取最近一次非空的值
	if q.Country != "" {
		x.x.Country = q.Country
	}
	if q.ASN != "" {
		x.x.ASN, x.x.Org = q.ASN, q.Org
	}
	if q.Client != "" {
		x.x.Client = q.Client
	}

	z.addFriend(q, ok)
}

func (z *Analyzer) addFriend(q *store.SubRequest, ok bool) {
	uid := z.granteeOf[q.ShortID]
	if uid == "" {
		return
	}
	fa := z.friends[uid]
	if fa == nil {
		fa = &friendAcc{f: AnFriend{UserID: uid, First: q.At, Daily: make([]int, z.friendDays)}, ips: map[string]bool{}, countries: map[string]int{}, orgs: map[string]int{}, clients: map[string]int{}, shorts: map[string]bool{}}
		z.friends[uid] = fa
		z.friendOrder = append(z.friendOrder, uid)
	}
	f := &fa.f
	f.Requests++
	if ok {
		f.OK++
		fa.times.add(q.At)
	} else {
		f.Fail++
	}
	f.Last = q.At
	fa.ips[q.IP] = true
	if q.Country != "" {
		fa.countries[q.Country]++
	}
	if q.Org != "" {
		fa.orgs[q.Org]++
	}
	if q.Client != "" {
		fa.clients[q.Client]++
	}
	fa.shorts[q.ShortID] = true
	if i := int(q.At.Sub(z.from).Hours() / 24); i >= 0 && i < len(f.Daily) {
		f.Daily[i]++
	}
}

// Result 生成结果。recent 是区间内最新的请求（新的在前），原样列出。Result 之后不要再 Add。
func (z *Analyzer) Result(recent []store.SubRequest) *Analytics {
	a := z.a
	a.Totals.IPs, a.Totals.Countries, a.Totals.Clients = len(z.ips), len(z.countries), len(z.clients)
	a.Countries, a.Clients, a.Kinds, a.Links, a.Shorts = rank(z.countries, 30), rank(z.clients, 20), rank(z.kinds, 10), rank(z.links, 50), rank(z.shorts, 50)
	a.Orgs = rank(z.orgs, 20)
	for i := range a.Orgs {
		a.Orgs[i].Label = z.orgLabel[a.Orgs[i].Key]
	}

	recentCut := z.to.Add(-24 * time.Hour)
	judgeNew := z.to.Sub(z.from) > 24*time.Hour
	a.IPs = make([]AnIP, 0, len(z.ips))
	for _, x := range z.ips {
		x.x.New = judgeNew && !x.x.First.Before(recentCut)
		if x.x.New {
			a.NewIPs++
		}
		a.IPs = append(a.IPs, x.x)
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
	// 中位数只为列出的 IP 计算
	for i := range a.IPs {
		a.IPs[i].Interval = medianGap(z.ips[a.IPs[i].IP].times.last())
	}

	for i, n := range a.Daily {
		if n > 0 && (a.Peak == nil || n > a.Peak.Count) {
			a.Peak = &AnPeakCount{Day: a.Days[i], Count: n}
		}
	}
	a.Recent = make([]AnRequest, 0, len(recent))
	for _, q := range recent {
		a.Recent = append(a.Recent, AnRequest{At: q.At, IP: q.IP, Country: q.Country, Org: q.Org, Client: q.Client, UA: q.UA, Status: q.Status, Kind: q.Kind, LinkID: q.LinkID, ShortID: q.ShortID})
	}
	a.Friends = z.friendResult()
	return a
}

// friendResult 按请求次数降序列出好友；没有请求的好友不列出（界面另外列出全部授权）。
func (z *Analyzer) friendResult() []AnFriend {
	keys := func(m map[string]int, n int) []string {
		out := []string{}
		for _, c := range rank(m, n) {
			out = append(out, c.Key)
		}
		return out
	}
	out := make([]AnFriend, 0, len(z.friendOrder))
	for _, uid := range z.friendOrder {
		fa := z.friends[uid]
		f := fa.f
		f.IPs = len(fa.ips)
		f.Countries, f.Orgs, f.Clients = keys(fa.countries, 20), keys(fa.orgs, 5), keys(fa.clients, 10)
		for id := range fa.shorts {
			f.Shorts = append(f.Shorts, id)
		}
		sort.Strings(f.Shorts)
		f.Interval = medianGap(fa.times.last())
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}

// Analyze 一次性汇总内存中的请求（rs 按时间正序）；最近的请求取 rs 末尾。用于测试与小数据量。
func Analyze(rs []store.SubRequest, granteeOf map[string]string, from, to time.Time, loc *time.Location) *Analytics {
	z := NewAnalyzer(from, to, loc, granteeOf)
	for i := range rs {
		z.Add(&rs[i])
	}
	recent := make([]store.SubRequest, 0, RecentRequests)
	for i := len(rs) - 1; i >= 0 && len(recent) < RecentRequests; i-- {
		recent = append(recent, rs[i])
	}
	return z.Result(recent)
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

// Locate 为 IP 明细与最近请求补上省市与运营商（O7，复用登录设备所在地的分层查询，R7）。
// 不写入请求表：千万行的明细表加列会让分析查询离开覆盖索引，而这两个列表都有上限（200 个 IP、100 条请求），
// 读取时查本地库只需几百次内存查找。代价是显示的是按当前地理库的结果，而不是请求当时的。
func (s *Service) Locate(a *Analytics) {
	if s.Geo == nil {
		return
	}
	at := func(ip string) *geo.Place {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return nil
		}
		p := s.Geo.Place(addr)
		if p.Empty() {
			return nil
		}
		return &p
	}
	for i := range a.IPs {
		a.IPs[i].Place = at(a.IPs[i].IP)
	}
	for i := range a.Recent {
		a.Recent[i].Place = at(a.Recent[i].IP)
	}
}
