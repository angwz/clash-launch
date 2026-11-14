package api

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 统计接口（反馈 #66–#68）。

// statsDays 是趋势图覆盖的天数。
const statsDays = 90

// dayRange 返回 [今天-n+1, 今天] 的 YYYYMMDD 区间与逐日列表（UTC）。
func dayRange(now time.Time, n int) (from, to int, days []int) {
	for i := n - 1; i >= 0; i-- {
		days = append(days, service.Day(now.AddDate(0, 0, -i)))
	}
	return days[0], days[len(days)-1], days
}

// series 把按天计数展开成与 days 对齐的数组（没有记录的天为 0）。
func series(rows []store.DayCount, days []int) []int64 {
	idx := map[int]int{}
	for i, d := range days {
		idx[d] = i
	}
	out := make([]int64, len(days))
	for _, r := range rows {
		if i, ok := idx[r.Day]; ok {
			out[i] += r.Count
		}
	}
	return out
}

type eventsReq struct {
	Events []service.Event `json:"events"`
}

// postEvents 接收前端上报的使用事件（打开网站、切换页面、改值、展开说明）。去重在服务端完成。
func (s *Server) postEvents(w http.ResponseWriter, r *http.Request, p *principal) {
	var req eventsReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Stats().Record(p.User.ID, req.Events, time.Now()); err != nil {
		if errors.Is(err, service.ErrEvent) {
			s.fail(w, badRequest("invalid events"))
			return
		}
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getFieldScores 返回设置项热度快照（每周重算一次）。
func (s *Server) getFieldScores(w http.ResponseWriter, r *http.Request, _ *principal) {
	fs, err := s.Svc.FieldScores(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

type countItem struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func topN(m map[string]int64, n int) []countItem {
	out := make([]countItem, 0, len(m))
	for k, v := range m {
		out = append(out, countItem{k, v})
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

func sum(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

type ipDTO struct {
	IP     string    `json:"ip"`
	First  time.Time `json:"first"`
	Last   time.Time `json:"last"`
	OK     int64     `json:"ok"`
	Fail   int64     `json:"fail"`
	Client string    `json:"client"`
}

func toIPs(xs []store.SubIP) []ipDTO {
	out := make([]ipDTO, 0, len(xs))
	for _, x := range xs {
		out = append(out, ipDTO{x.IP, x.First, x.Last, x.OK, x.Fail, x.Client})
	}
	return out
}

// adminStats 汇总全站：访问、登录、订阅请求（总数永久累计），近 90 天趋势，热门页面、客户端与设置项，
// 以及令牌无效的请求来源（用于发现试探）。
func (s *Server) adminStats(w http.ResponseWriter, r *http.Request, _ *principal) {
	ctx := r.Context()
	st := s.Svc.Store.Stats()
	s.Svc.FlushStats(ctx) // 先写入内存中的计数，管理员看到的是最新数字
	from, to, days := dayRange(time.Now(), statsDays)
	type out struct {
		Days    []int              `json:"days"`
		Totals  map[string]int64   `json:"totals"`
		Series  map[string][]int64 `json:"series"`
		Pages   []countItem        `json:"pages"`
		Clients []countItem        `json:"clients"`
		Fields  []countItem        `json:"fields"`
		Probes  []ipDTO            `json:"probes"`
	}
	o := out{Days: days, Totals: map[string]int64{}, Series: map[string][]int64{}}
	for _, scope := range []string{"visit", "uv", "login", "sub_all", "sub_fail", "pv"} {
		tot, err := st.Totals(ctx, scope, nil)
		if err != nil {
			s.fail(w, err)
			return
		}
		o.Totals[scope] = sum(tot)
		if scope == "pv" {
			o.Pages = topN(tot, 20)
		}
		rows, err := st.Daily(ctx, scope, nil, from, to)
		if err != nil {
			s.fail(w, err)
			return
		}
		o.Series[scope] = series(rows, days)
	}
	clients, err := st.Totals(ctx, "sub_client", nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	o.Clients = topN(clients, 12)
	fs, err := s.Svc.FieldScores(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	o.Fields = topN(fs.Scores, 15)
	probes, err := st.IPs(ctx, "", "")
	if err != nil {
		s.fail(w, err)
		return
	}
	o.Probes = toIPs(probes)
	writeJSON(w, http.StatusOK, o)
}

type linkStatsDTO struct {
	ID        string    `json:"id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
	Total     int64     `json:"total"`
	First     time.Time `json:"first,omitzero"`
	Last      time.Time `json:"last,omitzero"`
	Series    []int64   `json:"series"`
	IPs       []ipDTO   `json:"ips"`
}

// profileStats 是一份配置的订阅统计：配置创建时间、各链接的总请求数、首次与最近一次请求、
// 近 90 天趋势，以及请求过它的 IP（成功与失败次数）。只有配置的所有者可以查看。
func (s *Server) profileStats(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	prof, err := s.Svc.Store.Profiles().Get(ctx, p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	links, err := s.Svc.Store.Links().List(ctx, p.User.ID, prof.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.Svc.FlushStats(ctx)
	st := s.Svc.Store.Stats()
	ids := make([]string, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.ID)
	}
	from, to, days := dayRange(time.Now(), statsDays)
	totals := map[string]int64{}
	var rows []store.DayCount
	if len(ids) > 0 {
		if totals, err = st.Totals(ctx, "sub", ids); err != nil {
			s.fail(w, err)
			return
		}
		if rows, err = st.Daily(ctx, "sub", ids, from, to); err != nil {
			s.fail(w, err)
			return
		}
	}
	byLink := map[string][]store.DayCount{}
	for _, x := range rows {
		byLink[x.Key] = append(byLink[x.Key], x)
	}
	out := struct {
		CreatedAt time.Time      `json:"createdAt"`
		Days      []int          `json:"days"`
		Links     []linkStatsDTO `json:"links"`
	}{CreatedAt: prof.CreatedAt, Days: days, Links: []linkStatsDTO{}}
	for _, l := range links {
		ips, err := st.IPs(ctx, p.User.ID, l.ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		d := linkStatsDTO{ID: l.ID, Note: l.Note, CreatedAt: l.CreatedAt, Total: totals[l.ID], Series: series(byLink[l.ID], days), IPs: toIPs(ips)}
		for _, x := range ips {
			if d.First.IsZero() || x.First.Before(d.First) {
				d.First = x.First
			}
			if x.Last.After(d.Last) {
				d.Last = x.Last
			}
		}
		out.Links = append(out.Links, d)
	}
	writeJSON(w, http.StatusOK, out)
}

type requestDTO struct {
	At     time.Time `json:"at"`
	IP     string    `json:"ip"`
	UA     string    `json:"ua"`
	Client string    `json:"client"`
	Status int       `json:"status"`
	Bytes  int64     `json:"bytes"`
	Kind   string    `json:"kind"`
}

// linkRequests 分页返回一条链接的请求记录（新的在前）。
func (s *Server) linkRequests(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	l, err := s.Svc.Store.Links().Get(ctx, p.User.ID, r.PathValue("lid"))
	if err != nil || l.ProfileID != r.PathValue("id") {
		s.fail(w, store.ErrNotFound)
		return
	}
	s.Svc.FlushStats(ctx)
	pg := page(r)
	items, err := s.Svc.Store.Stats().Requests(ctx, p.User.ID, l.ID, pg)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[requestDTO]{Items: []requestDTO{}}
	for _, q := range items {
		out.Items = append(out.Items, requestDTO{q.At, q.IP, q.UA, q.Client, q.Status, q.Bytes, q.Kind})
	}
	if len(items) > 0 && len(items) == pageSize(r) {
		out.Next = store.RequestCursor(items[len(items)-1])
	}
	writeJSON(w, http.StatusOK, out)
}
