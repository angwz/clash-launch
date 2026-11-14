package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/angwz/clash-launch/internal/store/storetest"
)

// 账号安全相关的仓储：TOTP 防重放、恢复码一次性、通行密钥唯一、加密列可轮换、配额默认值与用量、新设备识别。
func TestSecurityRepos(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	k, _, err := LoadKeyring(dir, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	d, err := Open(ctx, storetest.DSN(t, dir), k)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	if _, err := d.Migrate(ctx, ""); err != nil {
		t.Fatal(err)
	}
	u := &User{Username: "alice", PasswordHash: "x", Role: "user"}
	if err := d.Users().Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	// 安全问题：整体替换
	_ = d.Questions().Set(ctx, u.ID, []Question{{0, "b:1", "h1"}, {1, "b:2", "h2"}, {2, "c:自定义", "h3"}})
	_ = d.Questions().Set(ctx, u.ID, []Question{{0, "b:4", "h4"}, {1, "b:5", "h5"}, {2, "b:6", "h6"}})
	if qs, err := d.Questions().List(ctx, u.ID); err != nil || len(qs) != 3 || qs[0].Question != "b:4" {
		t.Fatalf("安全问题：%+v %v", qs, err)
	}

	// TOTP：密钥加密存储；时间步只能前进
	if err := d.TOTP().Put(ctx, &TOTP{UserID: u.ID, Secret: "JBSWY3DPEHPK3PXP", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := d.TOTP().Advance(ctx, u.ID, 100); err != nil {
		t.Fatal(err)
	}
	if err := d.TOTP().Advance(ctx, u.ID, 100); !errors.Is(err, ErrConflict) {
		t.Errorf("同一时间步应被拒绝（防重放）：%v", err)
	}
	_ = d.TOTP().ReplaceCodes(ctx, u.ID, []string{"c1", "c2"})
	if err := d.TOTP().UseCode(ctx, u.ID, "c1", now); err != nil {
		t.Fatal(err)
	}
	if err := d.TOTP().UseCode(ctx, u.ID, "c1", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("恢复码只能用一次：%v", err)
	}
	if n, _ := d.TOTP().RemainingCodes(ctx, u.ID); n != 1 {
		t.Errorf("剩余恢复码：%d", n)
	}

	// 通行密钥：凭据 ID 全局唯一
	pk := &Passkey{UserID: u.ID, CredentialID: "cred-1", PublicKey: []byte{1, 2, 3}, Name: "手机"}
	if err := d.Passkeys().Create(ctx, pk); err != nil {
		t.Fatal(err)
	}
	if err := d.Passkeys().Create(ctx, &Passkey{UserID: u.ID, CredentialID: "cred-1", PublicKey: []byte{9}}); !errors.Is(err, ErrExists) {
		t.Errorf("重复的凭据应拒绝：%v", err)
	}
	_ = d.Passkeys().Used(ctx, pk.ID, 7, now)
	if got, err := d.Passkeys().ByCredential(ctx, "cred-1"); err != nil || got.SignCount != 7 || got.UserID != u.ID {
		t.Fatalf("按凭据查找：%+v %v", got, err)
	}
	if err := d.Passkeys().Delete(ctx, "someone-else", pk.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("不能删除别人的通行密钥：%v", err)
	}

	// SMTP：口令加密存储
	_ = d.SMTP().Put(ctx, &SMTP{UserID: u.ID, Host: "smtp.example.com", Port: 465, Security: "tls", Username: "a", Password: "fake-password", From: "a@example.com", To: "a@example.com"})

	// 轮换后新增的加密列仍可解密
	if _, err := AppendKey(filepath.Join(dir, "master.key"), k); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReEncrypt(ctx); err != nil {
		t.Fatal(err)
	}
	if tt, err := d.TOTP().Get(ctx, u.ID); err != nil || tt.Secret != "JBSWY3DPEHPK3PXP" || !tt.Enabled {
		t.Errorf("轮换后的 TOTP：%+v %v", tt, err)
	}
	if s, err := d.SMTP().Get(ctx, u.ID); err != nil || s.Password != "fake-password" {
		t.Errorf("轮换后的 SMTP：%+v %v", s, err)
	}

	// 配额：默认全为 -1；用量按所有者统计
	if q, err := d.Quotas().Get(ctx, u.ID); err != nil || q.Sources != -1 || q.Storage != -1 {
		t.Errorf("默认配额：%+v %v", q, err)
	}
	_ = d.Quotas().Put(ctx, &Quota{UserID: u.ID, Sources: 3, Profiles: -1, Links: -1, MinInterval: 600, Storage: 1 << 20})
	if q, _ := d.Quotas().Get(ctx, u.ID); q.Sources != 3 || q.MinInterval != 600 {
		t.Errorf("配额：%+v", q)
	}
	_ = d.Sources().Create(ctx, u.ID, &Source{Name: "s", Kind: "single", Content: []byte("ss://example")})
	if us, err := d.Quotas().Usage(ctx, u.ID); err != nil || us.Sources != 1 || us.Bytes <= 0 {
		t.Errorf("用量：%+v %v", us, err)
	}

	// 已知设备
	if isNew, _ := d.Devices().Seen(ctx, u.ID, "fp", now); !isNew {
		t.Error("第一次出现应为新设备")
	}
	if isNew, _ := d.Devices().Seen(ctx, u.ID, "fp", now); isNew {
		t.Error("第二次不是新设备")
	}
}
