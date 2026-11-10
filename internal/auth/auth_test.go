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
