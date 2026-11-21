package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 反馈 #74：短链接只能指向自己的链接、短码唯一且不区分大小写；消息只对收发双方可见，各自隐藏互不影响。
func TestSocialRepos(t *testing.T) {
	ctx := context.Background()
	d := newDB(t)
	a := &User{Username: "alice", PasswordHash: "h", Role: "user"}
	b := &User{Username: "bob", PasswordHash: "h", Role: "user"}
	c := &User{Username: "carol", PasswordHash: "h", Role: "user"}
	for _, u := range []*User{a, b, c} {
		if err := d.Users().Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	p := &Profile{Name: "p", Doc: []byte("x")}
	if err := d.Profiles().Create(ctx, a.ID, p, ""); err != nil {
		t.Fatal(err)
	}
	l := &Link{ProfileID: p.ID, Token: "token-example-aaaaaaaaaaaaaaaa", TokenHash: "h1"}
	if err := d.Links().Create(ctx, a.ID, l); err != nil {
		t.Fatal(err)
	}

	// 短链接
	sl := &ShortLink{LinkID: l.ID, Code: "Home-Wifi"}
	if err := d.ShortLinks().Create(ctx, a.ID, sl); err != nil {
		t.Fatal(err)
	}
	if err := d.ShortLinks().Create(ctx, b.ID, &ShortLink{LinkID: l.ID, Code: "steal"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("不能指向别人的链接：%v", err)
	}
	if err := d.ShortLinks().Create(ctx, a.ID, &ShortLink{LinkID: l.ID, Code: "home-wifi"}); !errors.Is(err, ErrExists) {
		t.Errorf("短码不区分大小写且唯一：%v", err)
	}
	got, err := d.ShortLinks().ByCode(ctx, "HOME-WIFI")
	if err != nil || got.ID != sl.ID || got.Code != "home-wifi" || !got.Active(time.Now()) {
		t.Fatalf("ByCode：%+v %v", got, err)
	}
	if _, err := d.ShortLinks().Get(ctx, b.ID, sl.ID); !errors.Is(err, ErrNotFound) {
		t.Error("别人不能读取")
	}
	got.Disabled, got.ExpiresAt = true, time.Now().Add(-time.Minute)
	if err := d.ShortLinks().Update(ctx, a.ID, got); err != nil {
		t.Fatal(err)
	}
	if x, _ := d.ShortLinks().ByCode(ctx, "home-wifi"); x.Active(time.Now()) {
		t.Error("停用后不可用")
	}
	if n, _ := d.ShortLinks().Count(ctx, a.ID); n != 1 {
		t.Errorf("数量：%d", n)
	}
	// 删除配置链接时短链接一并删除
	if err := d.Links().Delete(ctx, a.ID, l.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.ShortLinks().Count(ctx, a.ID); n != 0 {
		t.Errorf("链接删除后短链接应一并删除：%d", n)
	}

	// 消息
	m := &Message{SenderID: a.ID, RecipientID: b.ID, Kind: "text", Body: []byte(`{"text":"你好 👋"}`)}
	if err := d.Messages().Create(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Messages().Get(ctx, c.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Error("第三人不能读取")
	}
	got2, err := d.Messages().Get(ctx, b.ID, m.ID)
	if err != nil || string(got2.Body) != `{"text":"你好 👋"}` {
		t.Fatalf("收件人读取：%+v %v", got2, err)
	}
	if n, _ := d.Messages().Unread(ctx, b.ID); n != 1 {
		t.Errorf("未读：%d", n)
	}
	_ = d.Messages().MarkRead(ctx, b.ID, a.ID, time.Now())
	if n, _ := d.Messages().Unread(ctx, b.ID); n != 0 {
		t.Errorf("已读后未读：%d", n)
	}
	if err := d.Messages().Hide(ctx, b.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	if th, _ := d.Messages().Thread(ctx, b.ID, a.ID, Page{}); len(th) != 0 {
		t.Error("收件人隐藏后看不到")
	}
	if th, _ := d.Messages().Thread(ctx, a.ID, b.ID, Page{}); len(th) != 1 {
		t.Error("发件人仍然看得到")
	}
	if err := d.Messages().Hide(ctx, c.ID, m.ID); !errors.Is(err, ErrNotFound) {
		t.Error("第三人不能隐藏")
	}
	if n, _ := d.Messages().SentSince(ctx, a.ID, time.Now().Add(-time.Hour)); n != 1 {
		t.Errorf("发送计数：%d", n)
	}
	if err := d.Messages().SetAdded(ctx, a.ID, m.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Error("只有收件人能记录加入的源")
	}

	// 请求记录带归属地；EachRequest 按时间正序，LatestRequests 新的在前，筛选条件生效
	now := time.Now()
	reqs := []SubRequest{
		{LinkID: "l1", OwnerID: a.ID, At: now.Add(-2 * time.Hour), IP: "192.0.2.1", UA: "mihomo/1.19.33", Status: 200, Country: "CN", ASN: "AS4134", Org: "Example Telecom"},
		{LinkID: "l1", OwnerID: a.ID, At: now.Add(-time.Hour), IP: "192.0.2.2", Status: 200, ShortID: "s1"},
		{LinkID: "l2", OwnerID: a.ID, At: now.Add(-30 * time.Minute), IP: "192.0.2.3", Status: 404, ShortID: "s2"},
	}
	if err := d.Stats().AddRequests(ctx, reqs); err != nil {
		t.Fatal(err)
	}
	each := func(f RequestFilter) []SubRequest {
		t.Helper()
		var out []SubRequest
		if err := d.Stats().EachRequest(ctx, a.ID, now.Add(-3*time.Hour), now, f, func(q *SubRequest) error {
			out = append(out, *q) // 回调的指针会被复用，必须复制
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	rs := each(RequestFilter{})
	if len(rs) != 3 || rs[0].Country != "CN" || rs[0].Org != "Example Telecom" || rs[0].UA != "" || rs[1].ShortID != "s1" || rs[2].Status != 404 {
		t.Fatalf("EachRequest：%+v", rs)
	}
	if rs := each(RequestFilter{LinkID: "l2"}); len(rs) != 1 || rs[0].IP != "192.0.2.3" {
		t.Errorf("按链接筛选：%+v", rs)
	}
	if rs := each(RequestFilter{Shorts: []string{"s1", "s2"}}); len(rs) != 2 {
		t.Errorf("按一组短链接筛选：%+v", rs)
	}
	if rs := each(RequestFilter{Shorts: []string{}}); len(rs) != 0 {
		t.Errorf("空的短链接集合不应匹配任何请求：%+v", rs)
	}
	if rs, err := d.Stats().LatestRequests(ctx, a.ID, now.Add(-3*time.Hour), now, RequestFilter{ShortID: "s1"}, 5); err != nil || len(rs) != 1 || rs[0].IP != "192.0.2.2" {
		t.Errorf("LatestRequests 按短链接：%+v %v", rs, err)
	}
	if rs, _ := d.Stats().LatestRequests(ctx, a.ID, now.Add(-3*time.Hour), now, RequestFilter{}, 2); len(rs) != 2 || rs[0].IP != "192.0.2.3" || rs[1].UA != "" || rs[0].Status != 404 {
		t.Errorf("LatestRequests 应新的在前：%+v", rs)
	}
	ips, _ := d.Stats().IPs(ctx, a.ID, "l1")
	if len(ips) != 2 {
		t.Fatalf("IP 汇总：%+v", ips)
	}
}
