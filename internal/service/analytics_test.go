package service

import (
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
	a := Analyze(rs, from, to, sh, false)
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
