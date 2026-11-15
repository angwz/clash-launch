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

	// 请求记录带归属地，Range 按时间正序
	now := time.Now()
	reqs := []SubRequest{
		{LinkID: "l1", OwnerID: a.ID, At: now.Add(-2 * time.Hour), IP: "192.0.2.1", Status: 200, Country: "CN", ASN: "AS4134", Org: "Example Telecom"},
		{LinkID: "l1", OwnerID: a.ID, At: now.Add(-time.Hour), IP: "192.0.2.2", Status: 200, ShortID: "s1"},
	}
	if err := d.Stats().AddRequests(ctx, reqs); err != nil {
		t.Fatal(err)
	}
	rs, cut, err := d.Stats().Range(ctx, a.ID, now.Add(-3*time.Hour), now, 10)
	if err != nil || cut || len(rs) != 2 || rs[0].Country != "CN" || rs[0].Org != "Example Telecom" || rs[1].ShortID != "s1" {
		t.Fatalf("Range：%+v %v %v", rs, cut, err)
	}
	if rs, cut, _ := d.Stats().Range(ctx, a.ID, now.Add(-3*time.Hour), now, 1); !cut || len(rs) != 1 || rs[0].IP != "192.0.2.2" {
		t.Errorf("截断时应保留最新的：%+v %v", rs, cut)
	}
	ips, _ := d.Stats().IPs(ctx, a.ID, "l1")
	if len(ips) != 2 {
		t.Fatalf("IP 汇总：%+v", ips)
	}
}
