package source

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP 是一个极简的 SMTP 服务器：useTLS 时为隐式 TLS；记录认证与正文。
type fakeSMTP struct {
	ln   net.Listener
	mu   sync.Mutex
	auth string
	data string
}

func startFakeSMTP(t *testing.T, cfg *tls.Config) *fakeSMTP {
	t.Helper()
	var ln net.Listener
	var err error
	if cfg != nil {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", cfg)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	r := bufio.NewReader(c)
	w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
	w("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		switch up := strings.ToUpper(cmd); {
		case strings.HasPrefix(up, "EHLO"):
			w("250-fake")
			w("250 AUTH PLAIN")
		case strings.HasPrefix(up, "AUTH PLAIN"):
			f.mu.Lock()
			f.auth = strings.TrimSpace(cmd[len("AUTH PLAIN"):])
			f.mu.Unlock()
			w("235 ok")
		case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
			w("250 ok")
		case up == "DATA":
			w("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			w("250 queued")
		case up == "QUIT":
			w("221 bye")
			return
		default:
			w("502 unknown")
		}
	}
}

func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: k}}, MinVersion: tls.VersionTLS12}, pool
}

func portOf(ln net.Listener) int { return ln.Addr().(*net.TCPAddr).Port }

func TestMailer(t *testing.T) {
	srvCfg, pool := selfSigned(t)
	srv := startFakeSMTP(t, srvCfg)
	guard, _ := ParseAllowlist("127.0.0.1")
	m := &Mailer{Guard: guard, Ports: []int{portOf(srv.ln)}, TLS: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	mail := Mail{Host: "127.0.0.1", Port: portOf(srv.ln), Security: "tls", Username: "alice@example.com", Password: "fake-password", From: "alice@example.com", To: "alice@example.com", Subject: "新设备登录提醒", Body: "有一个新设备登录了你的账号。"}
	if err := m.Send(context.Background(), mail); err != nil {
		t.Fatalf("发送：%v", err)
	}
	srv.mu.Lock()
	data, auth := srv.data, srv.auth
	srv.mu.Unlock()
	raw, _ := base64.StdEncoding.DecodeString(auth)
	if string(raw) != "\x00alice@example.com\x00fake-password" {
		t.Errorf("AUTH PLAIN：%q", raw)
	}
	if !strings.Contains(data, "Subject: =?utf-8?q?") || !strings.Contains(data, "Content-Transfer-Encoding: base64") {
		t.Errorf("邮件头：%s", data)
	}

	// 拒绝的情况：端口、换行注入、带显示名的地址、未加入白名单的内网地址
	bad := map[string]func(Mail) Mail{
		"端口":   func(x Mail) Mail { x.Port = 2525; return x },
		"换行注入": func(x Mail) Mail { x.Subject = "hi\r\nBcc: evil@example.com"; return x },
		"显示名":  func(x Mail) Mail { x.To = "Alice <alice@example.com>"; return x },
		"安全方式": func(x Mail) Mail { x.Security = "none"; return x },
	}
	for name, f := range bad {
		if err := m.Send(context.Background(), f(mail)); !errors.Is(err, ErrSMTP) {
			t.Errorf("%s：应拒绝（%v）", name, err)
		}
	}
	strict := &Mailer{Guard: Guard{}, Ports: []int{portOf(srv.ln)}}
	if err := strict.Send(context.Background(), mail); !errors.Is(err, ErrSMTP) || !errors.Is(err, ErrBlocked) {
		t.Errorf("未加白名单的回环地址应被 SSRF 防护拦下：%v", err)
	}
}

// 明文服务器不支持 STARTTLS：不发送口令。
func TestMailerRequiresTLS(t *testing.T) {
	srv := startFakeSMTP(t, nil)
	guard, _ := ParseAllowlist("127.0.0.1")
	m := &Mailer{Guard: guard, Ports: []int{portOf(srv.ln)}}
	err := m.Send(context.Background(), Mail{Host: "127.0.0.1", Port: portOf(srv.ln), Security: "starttls", Username: "a", Password: "fake-password", From: "a@example.com", To: "a@example.com", Subject: "x", Body: "y"})
	if !errors.Is(err, ErrSMTP) || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("应拒绝明文：%v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.auth != "" {
		t.Error("口令不应被发送")
	}
}
