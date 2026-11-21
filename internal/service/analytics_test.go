package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

func TestAnalyze(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	to := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC) // 上海 08:00
	from := to.AddDate(0, 0, -7)
	at := func(h int) time.Time { return to.Add(time.Duration(-h) * time.Hour) }
	rs := []store.SubRequest{
		{LinkID: "l1", At: at(72), IP: "192.0.2.1", Status: 200, Kind: "config", Client: "mihomo", Country: "CN", ASN: "AS4134", Org: "Example Telecom"},
		{LinkID: "l1", At: at(66), IP: "192.0.2.1", Status: 200, Kind: "config", Client: "mihomo", Country: "CN", ASN: "AS4134", Org: "Example Telecom"},
		{LinkID: "l1", At: at(60), IP: "192.0.2.1", Status: 200, Kind: "config", Client: "mihomo", Country: "CN", ASN: "AS4134", Org: "Example Telecom"},
		{LinkID: "l1", At: at(60).Add(3 * time.Second), IP: "192.0.2.1", Status: 200, Kind: "config", Client: "mihomo"}, // 重试，不计入间隔
		{LinkID: "l1", At: at(2), IP: "192.0.2.9", Status: 404, Kind: "config", ShortID: "s1", Country: "JP"},
	}
	a := Analyze(rs, map[string]string{"s1": "u9"}, from, to, sh)
	if a.Totals.Requests != 5 || a.Totals.OK != 4 || a.Totals.Fail != 1 || a.Totals.IPs != 2 || a.Totals.Countries != 2 {
		t.Errorf("总数：%+v", a.Totals)
	}
	if len(a.Days) != 8 || a.Days[len(a.Days)-1] != "2026-11-15" {
		t.Errorf("日期按上海时区：%v", a.Days)
	}
	sum := 0
	for _, n := range a.Daily {
		sum += n
	}
	if sum != 5 {
		t.Errorf("每日合计：%v", a.Daily)
	}
	// at(2) = 11-14 22:00 UTC = 11-15 06:00 上海，星期日 → 下标 6
	if a.Heat[6][6] != 1 {
		t.Errorf("热力图按上海时区：%v", a.Heat[6])
	}
	if a.IPs[0].IP != "192.0.2.1" || a.IPs[0].Interval != 6*3600 || a.IPs[0].New {
		t.Errorf("IP 明细：%+v", a.IPs[0])
	}
	if !a.IPs[1].New || a.NewIPs != 1 {
		t.Errorf("新出现的 IP：%+v %d", a.IPs[1], a.NewIPs)
	}
	if a.Orgs[0].Key != "AS4134" || a.Orgs[0].Label != "Example Telecom" || a.Shorts[0].Key != "s1" {
		t.Errorf("排行：%+v %+v", a.Orgs, a.Shorts)
	}
	if a.Recent[0].IP != "192.0.2.9" {
		t.Errorf("最近的请求应倒序：%+v", a.Recent[0])
	}
	var kinds = map[string]int{}
	for _, k := range a.Kinds {
		kinds[k.Key] = k.Count
	}
	if kinds["short"] != 1 || kinds["config"] != 4 {
		t.Errorf("类型：%v", kinds)
	}
}

func TestAnalyzeFriends(t *testing.T) {
	to := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -2)
	rs := []store.SubRequest{
		{LinkID: "l1", At: from.Add(time.Hour), IP: "192.0.2.1", Status: 200, Kind: "config", ShortID: "s1", Country: "JP", Org: "Example Net", Client: "mihomo"},
		{LinkID: "l1", At: from.Add(30 * time.Hour), IP: "192.0.2.2", Status: 404, Kind: "config", ShortID: "s2"},
		{LinkID: "l1", At: from.Add(31 * time.Hour), IP: "192.0.2.3", Status: 200, Kind: "config"}, // 不是经好友的短链接
	}
	a := Analyze(rs, map[string]string{"s1": "u9", "s2": "u9"}, from, to, time.UTC)
	if len(a.Friends) != 1 {
		t.Fatalf("好友：%+v", a.Friends)
	}
	f := a.Friends[0]
	if f.UserID != "u9" || f.Requests != 2 || f.OK != 1 || f.Fail != 1 || f.IPs != 2 || len(f.Shorts) != 2 || f.Daily[0] != 1 || f.Daily[1] != 1 {
		t.Errorf("好友汇总：%+v", f)
	}
	if f.Countries[0] != "JP" || f.Orgs[0] != "Example Net" || f.Clients[0] != "mihomo" {
		t.Errorf("好友的归属地与客户端：%+v", f)
	}
}

// 流式汇总不保留全部请求：拉取间隔只看每个 IP 最近 gapSamples 次成功请求。
func TestAnalyzerGapSamples(t *testing.T) {
	to := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -30)
	z := NewAnalyzer(from, to, time.UTC, nil)
	at := from
	q := store.SubRequest{LinkID: "l1", IP: "192.0.2.1", Status: 200, Kind: "config"}
	for i := 0; i < 3*gapSamples; i++ { // 早期每分钟一次
		at = at.Add(time.Minute)
		q.At = at
		z.Add(&q)
	}
	for i := 0; i < gapSamples; i++ { // 最近每两分钟一次：恰好占满全部样本间隔
		at = at.Add(2 * time.Minute)
		q.At = at
		z.Add(&q)
	}
	a := z.Result(nil)
	if a.Totals.Requests != 4*gapSamples || a.IPs[0].Count != 4*gapSamples {
		t.Fatalf("计数不受样本上限影响：%+v", a.Totals)
	}
	if a.IPs[0].Interval != 120 {
		t.Errorf("间隔应只看最近的样本，得到 %d", a.IPs[0].Interval)
	}
	if len(a.Recent) != 0 {
		t.Errorf("最近的请求由调用方提供：%+v", a.Recent)
	}
}

// BenchmarkAnalyzer 测 Go 侧汇总的开销：33 万条请求（与 tools/dbbench 重度用户近 30 天的分布相近）。
func BenchmarkAnalyzer(b *testing.B) {
	to := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -30)
	sh, _ := time.LoadLocation("Asia/Shanghai")
	countries := []string{"CN", "HK", "JP", "SG", "US"}
	const n = 330_000
	rs := make([]store.SubRequest, n)
	step := to.Sub(from) / n
	for i := range rs {
		rs[i] = store.SubRequest{LinkID: "l" + strconv.Itoa(i%3+1), At: from.Add(time.Duration(i) * step), IP: "192.0.2." + strconv.Itoa(i%254+1),
			Status: 200, Kind: "config", Client: "mihomo", Country: countries[i%5], ASN: "AS64500", Org: "Example Net"}
		if i%7 == 0 {
			rs[i].ShortID = "s" + strconv.Itoa(i%50)
		}
	}
	grants := map[string]string{"s1": "u9", "s8": "u10"}
	b.ResetTimer()
	for range b.N {
		z := NewAnalyzer(from, to, sh, grants)
		for i := range rs {
			z.Add(&rs[i])
		}
		_ = z.Result(nil)
	}
}
