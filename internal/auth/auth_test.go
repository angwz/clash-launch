package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPoW(t *testing.T) {
	p := NewPoW(8)
	now := time.Unix(1_800_000_000, 0)
	ch := p.Challenge("register", now)
	nonce := Solve(ch, 8)
	if err := p.Verify("recovery", ch, nonce, now); err == nil {
		t.Error("用途不同应拒绝")
	}
	if err := p.Verify("register", ch, nonce, now.Add(6*time.Minute)); err == nil {
		t.Error("过期应拒绝")
	}
	if err := p.Verify("register", ch+"x", nonce, now); err == nil {
		t.Error("签名被篡改应拒绝")
	}
	if err := p.Verify("register", ch, nonce, now); err != nil {
		t.Fatalf("有效的证明被拒绝：%v", err)
	}
	if err := p.Verify("register", ch, nonce, now); err == nil {
		t.Error("同一挑战不能使用两次")
	}
	other := NewPoW(8)
	if err := other.Verify("register", ch, nonce, now); err == nil {
		t.Error("其他密钥签发的挑战应拒绝")
	}
	if err := p.Verify("register", ch, "-1", now); err == nil {
		t.Error("非数字 nonce 应拒绝")
	}
}

func TestLeadingZeroBits(t *testing.T) {
	var h [32]byte
	if LeadingZeroBits(h) != 256 {
		t.Error("全零")
	}
	h[0] = 0x0f
	if LeadingZeroBits(h) != 4 {
		t.Error("0x0f")
	}
	h[0], h[9] = 0, 0x80
	if LeadingZeroBits(h) != 72 {
		t.Errorf("第 9 字节：%d", LeadingZeroBits(h))
	}
}

func TestNormalizeAnswer(t *testing.T) {
	cases := map[string]string{
		"  Beijing ":        "beijing",
		"ＢＥＩＪＩＮＧ":           "beijing",
		"New\t\tYork  City": "new york city",
		"北京　市":              "北京 市", // 全角空格
		"Ｍｒ．Ｗｈｉｓｋｅｒｓ":       "mr.whiskers",
	}
	for in, want := range cases {
		if got := NormalizeAnswer(in); got != want {
			t.Errorf("NormalizeAnswer(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestCheckQuestions(t *testing.T) {
	ok := []Question{{Builtin: 0}, {Builtin: 3}, {Builtin: -1, Text: "我的第一辆自行车"}}
	if err := CheckQuestions(ok, []string{"ab", "答案", "red"}); err != nil {
		t.Fatalf("合法的问题被拒绝：%v", err)
	}
	bad := map[string][][]Question{
		"重复":    {{{Builtin: 1}, {Builtin: 1}, {Builtin: 2}}},
		"越界":    {{{Builtin: 0}, {Builtin: 1}, {Builtin: BuiltinQuestions}}},
		"自定义太短": {{{Builtin: 0}, {Builtin: 1}, {Builtin: -1, Text: "x"}}},
		"数量":    {{{Builtin: 0}, {Builtin: 1}}},
	}
	for name, qss := range bad {
		for _, qs := range qss {
			if err := CheckQuestions(qs, []string{"ab", "cd", "ef"}[:len(qs)]); err == nil {
				t.Errorf("%s：应拒绝", name)
			}
		}
	}
	if err := CheckQuestions(ok, []string{"ab", " a ", "red"}); err == nil {
		t.Error("规范化后少于 2 个字符的答案应拒绝")
	}
}

func TestQuestionEncoding(t *testing.T) {
	for _, q := range []Question{{Builtin: 7}, {Builtin: -1, Text: "b:不是内置"}} {
		if got := DecodeQuestion(q.Encode()); got != q {
			t.Errorf("往返：%+v → %+v", q, got)
		}
	}
}

func TestDecoyQuestions(t *testing.T) {
	key := []byte("k")
	a := DecoyQuestions(key, "Nobody")
	b := DecoyQuestions(key, "nobody")
	if len(a) != QuestionCount || a[0] != b[0] || a[1] != b[1] || a[2] != b[2] {
		t.Errorf("同一用户名（不区分大小写）应得到相同的问题：%v %v", a, b)
	}
	seen := map[int]bool{}
	for _, q := range a {
		if q.Builtin < 0 || q.Builtin >= BuiltinQuestions || seen[q.Builtin] {
			t.Errorf("问题无效或重复：%v", a)
		}
		seen[q.Builtin] = true
	}
}

func TestBackoff(t *testing.T) {
	b := NewBackoff(3, time.Second, 15*time.Minute)
	now := time.Unix(0, 0)
	for i := 0; i < 3; i++ {
		if d := b.Fail("u", now); d != 0 {
			t.Fatalf("前 3 次不应等待：%v", d)
		}
	}
	waits := []time.Duration{}
	for i := 0; i < 4; i++ {
		waits = append(waits, b.Fail("u", now))
	}
	if strings.Join(durs(waits), ",") != "1s,2s,4s,8s" {
		t.Errorf("指数退避：%v", waits)
	}
	if b.Wait("u", now) != 8*time.Second || b.Wait("u", now.Add(9*time.Second)) != 0 {
		t.Error("等待时间")
	}
	for i := 0; i < 30; i++ {
		b.Fail("u", now)
	}
	if b.Wait("u", now) != 15*time.Minute {
		t.Errorf("上限：%v", b.Wait("u", now))
	}
	b.Reset("u")
	if b.Wait("u", now) != 0 {
		t.Error("成功后清零")
	}
}

func durs(ds []time.Duration) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.String()
	}
	return out
}

// RFC 6238 附录 B 的测试向量（SHA-1，密钥为 ASCII "12345678901234567890"，取 8 位结果的后 6 位）。
func TestTOTPVectors(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	vectors := map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037", 20000000000: "353130"}
	for ts, want := range vectors {
		got, err := TOTPCode(secret, ts/30)
		if err != nil || got != want {
			t.Errorf("T=%d：%s，期望 %s（%v）", ts, got, want, err)
		}
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret := NewTOTPSecret()
	now := time.Unix(1_800_000_000, 0)
	cur, _ := TOTPCode(secret, TOTPStep(now))
	prev, _ := TOTPCode(secret, TOTPStep(now)-1)
	old, _ := TOTPCode(secret, TOTPStep(now)-3)
	if s, ok := VerifyTOTP(secret, cur, now); !ok || s != TOTPStep(now) {
		t.Error("当前时间步应通过")
	}
	if _, ok := VerifyTOTP(secret, " "+prev[:3]+" "+prev[3:], now); !ok {
		t.Error("前一个时间步（含空格）应通过")
	}
	if _, ok := VerifyTOTP(secret, old, now); ok {
		t.Error("超出窗口的验证码应拒绝")
	}
	if !strings.HasPrefix(TOTPURI(secret, "Clash, Launch!", "alice"), "otpauth://totp/Clash%2C%20Launch%21:alice?") {
		t.Errorf("URI：%s", TOTPURI(secret, "Clash, Launch!", "alice"))
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes := NewRecoveryCodes(10)
	if len(codes) != 10 || len(hashes) != 10 {
		t.Fatal("数量")
	}
	if HashRecoveryCode(strings.ToUpper(strings.ReplaceAll(codes[0], "-", " "))) != hashes[0] {
		t.Error("大小写、空格与连字符不影响")
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 9 || c[4] != '-' || seen[c] {
			t.Errorf("格式或重复：%s", c)
		}
		seen[c] = true
	}
}

func TestTickets(t *testing.T) {
	tk := NewTickets()
	now := time.Unix(1_800_000_000, 0)
	x := tk.Issue("mfa", "user-1", 5*time.Minute, now)
	if sub, err := tk.Verify("mfa", x, now.Add(time.Minute)); err != nil || sub != "user-1" {
		t.Fatalf("有效凭证：%q %v", sub, err)
	}
	if _, err := tk.Verify("mfa", x, now.Add(6*time.Minute)); err == nil {
		t.Error("过期应拒绝")
	}
	if _, err := tk.Verify("other", x, now); err == nil {
		t.Error("用途不同应拒绝")
	}
	y := tk.Issue("pk", "user-1|challenge", time.Minute, now)
	if sub, err := tk.Verify("pk", y, now); err != nil || sub != "user-1|challenge" {
		t.Errorf("主体含 | 时应原样返回：%q %v", sub, err)
	}
	if _, err := NewTickets().Verify("mfa", x, now); err == nil {
		t.Error("其他密钥签发的凭证应拒绝")
	}
}
