package api

import (
	"encoding/csv"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// analyticsScope 是订阅分析与导出共用的范围与筛选条件：
// days（1–365，默认 30）、link、short、friend（只看某条链接、某个短链接、某位好友），
// tz（用户没有设置时区时由前端传入浏览器时区）。区间结束于所选时区的明天零点，开始于 days 天前的零点。
type analyticsScope struct {
	from, to  time.Time
	loc       *time.Location
	filter    store.RequestFilter
	codes     map[string]string // 短链接 ID → 短码
	friends   map[string]string // 好友 ID → 用户名
	granteeOf map[string]string // 为好友生成的短链接 ID → 好友 ID（反馈 #75）
}

func (s *Server) analyticsScope(r *http.Request, p *principal) (*analyticsScope, error) {
	ctx := r.Context()
	q := r.URL.Query()
	days, _ := strconv.Atoi(q.Get("days"))
	if days <= 0 || days > 365 {
		days = 30
	}
	zone := p.User.Timezone
	if zone == "" {
		zone = q.Get("tz")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" || zone == "Local" {
		loc = time.UTC
	}
	s.Svc.FlushStats(ctx)
	now := time.Now().In(loc)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	sc := &analyticsScope{from: to.AddDate(0, 0, -days), to: to, loc: loc, codes: map[string]string{}, friends: map[string]string{}, granteeOf: map[string]string{}}
	shorts, err := s.Svc.Store.ShortLinks().List(ctx, p.User.ID)
	if err != nil {
		return nil, err
	}
	for _, x := range shorts {
		sc.codes[x.ID] = x.Code
		if x.GranteeID != "" {
			sc.granteeOf[x.ID] = x.GranteeID
			if _, ok := sc.friends[x.GranteeID]; !ok {
				if u, err := s.Svc.Store.Users().ByID(ctx, x.GranteeID); err == nil {
					sc.friends[x.GranteeID] = u.Username
				} else {
					sc.friends[x.GranteeID] = "?"
				}
			}
		}
	}
	// 筛选条件交给数据库；只看某位好友时取他的全部短链接（没有则为空切片：必然没有请求）
	sc.filter = store.RequestFilter{LinkID: q.Get("link"), ShortID: q.Get("short")}
	if friend := q.Get("friend"); friend != "" {
		sc.filter.Shorts = []string{}
		for _, x := range shorts {
			if x.GranteeID == friend {
				sc.filter.Shorts = append(sc.filter.Shorts, x.ID)
			}
		}
	}
	return sc, nil
}

// exportAnalytics 把订阅分析导出为 CSV（需求 R39）：kind=requests 是请求明细（受明细保留期限制），
// kind=daily 是按天的汇总（同一范围与筛选）。流式写出，没有条数上限；时间按所选时区。
//
// 文件以 UTF-8 BOM 开头，表格软件按 UTF-8 打开中文。客户端名称、备注等可能由请求方或用户任意填写，
// 以 = + - @ 制表符或回车开头的单元格前加 '，防止在表格软件中被当作公式执行（CSV 注入）。
func (s *Server) exportAnalytics(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	sc, err := s.analyticsScope(r, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	labels, err := s.linkLabels(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "daily" {
		kind = "requests"
	}
	name := "subscription-" + kind + "-" + sc.from.Format("20060102") + "-" + sc.to.AddDate(0, 0, -1).Format("20060102") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	row := func(cells ...string) error {
		for i, c := range cells {
			cells[i] = csvSafe(c)
		}
		return cw.Write(cells)
	}
	if kind == "daily" {
		type day struct {
			total, ok, blocked int
			ips                map[string]bool
		}
		byDay := map[string]*day{}
		err = s.Svc.Store.Stats().EachRequest(ctx, p.User.ID, sc.from, sc.to, sc.filter, func(q *store.SubRequest) error {
			k := q.At.In(sc.loc).Format("2006-01-02")
			d := byDay[k]
			if d == nil {
				d = &day{ips: map[string]bool{}}
				byDay[k] = d
			}
			d.total++
			if q.Status < 400 {
				d.ok++
			} else {
				d.blocked++
			}
			d.ips[q.IP] = true
			return nil
		})
		if err == nil {
			err = row("date", "requests", "ok", "blocked", "unique_ips")
		}
		for t := sc.from; err == nil && t.Before(sc.to); t = t.AddDate(0, 0, 1) {
			k := t.Format("2006-01-02")
			d := byDay[k]
			if d == nil {
				d = &day{ips: map[string]bool{}}
			}
			err = row(k, strconv.Itoa(d.total), strconv.Itoa(d.ok), strconv.Itoa(d.blocked), strconv.Itoa(len(d.ips)))
		}
	} else {
		err = row("time", "config", "link", "short_link", "friend", "client", "ip", "country", "asn", "org", "status", "kind")
		if err == nil {
			err = s.Svc.Store.Stats().EachRequest(ctx, p.User.ID, sc.from, sc.to, sc.filter, func(q *store.SubRequest) error {
				l := labels[q.LinkID]
				return row(q.At.In(sc.loc).Format(time.RFC3339), l.ProfileName, l.Note, sc.codes[q.ShortID], sc.friends[sc.granteeOf[q.ShortID]],
					q.Client, q.IP, q.Country, q.ASN, q.Org, strconv.Itoa(q.Status), q.Kind)
			})
		}
	}
	cw.Flush()
	if err == nil {
		err = cw.Error()
	}
	if err != nil {
		// 已经开始写正文，不能再改状态码：记日志，客户端得到的是截断的文件
		s.Log.Warn("导出订阅分析失败", "user", p.User.ID, "error", err)
	}
}

// csvSafe 防止 CSV 注入：以公式起始字符开头的单元格前加一个单引号。
func csvSafe(c string) string {
	if c != "" && strings.ContainsRune("=+-@\t\r", rune(c[0])) {
		return "'" + c
	}
	return c
}
