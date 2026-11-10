package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// 安全问题（第 14.1 节）：注册时设置 3 个，找回时答对至少 2 个即可重设口令。
// 问题可以是内置问题（只存编号，文字由界面按语言显示）或自定义文字；答案规范化后以 argon2id 存储。

// BuiltinQuestions 是内置问题的数量；文字在前端消息 question.builtin.<编号> 中。
const BuiltinQuestions = 10

// QuestionCount 是每个账号的安全问题数；RecoverThreshold 是找回所需的最少正确数。
const (
	QuestionCount    = 3
	RecoverThreshold = 2
)

// ErrQuestions 表示安全问题不合规。
var ErrQuestions = errors.New("auth: 需要 3 个互不相同的安全问题，答案至少 2 个字符")

// Question 是一个安全问题：Builtin ≥ 0 时为内置问题编号，否则 Text 为自定义文字。
type Question struct {
	Builtin int    `json:"builtin"`
	Text    string `json:"text,omitempty"`
}

// Encode 把问题编码为存储形式：b:<编号> 或 c:<文字>。
func (q Question) Encode() string {
	if q.Builtin >= 0 {
		return "b:" + strconv.Itoa(q.Builtin)
	}
	return "c:" + q.Text
}

// DecodeQuestion 是 Encode 的逆操作。
func DecodeQuestion(s string) Question {
	if n, ok := strings.CutPrefix(s, "b:"); ok {
		if i, err := strconv.Atoi(n); err == nil {
			return Question{Builtin: i}
		}
	}
	return Question{Builtin: -1, Text: strings.TrimPrefix(s, "c:")}
}

// NormalizeAnswer 规范化答案：Unicode NFKC（统一全角与半角、兼容字符）、转为小写、
// 去掉首尾空白并把连续空白合并为一个空格。这样「Beijing」「 beijing 」「ＢＥＩＪＩＮＧ」视为相同。
func NormalizeAnswer(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

// CheckQuestions 校验一组问题与答案：数量为 3，问题互不相同（内置编号有效、自定义文字 2–100 字），
// 规范化后的答案至少 2 个字符、最多 200 个字符。
func CheckQuestions(qs []Question, answers []string) error {
	if len(qs) != QuestionCount || len(answers) != QuestionCount {
		return ErrQuestions
	}
	seen := map[string]bool{}
	for i, q := range qs {
		if q.Builtin >= BuiltinQuestions || (q.Builtin < 0 && !validCustom(q.Text)) {
			return ErrQuestions
		}
		if q.Builtin >= 0 {
			qs[i].Text = ""
		}
		k := qs[i].Encode()
		if seen[k] {
			return ErrQuestions
		}
		seen[k] = true
		n := utf8.RuneCountInString(NormalizeAnswer(answers[i]))
		if n < 2 || n > 200 {
			return ErrQuestions
		}
	}
	return nil
}

func validCustom(t string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(t))
	return n >= 2 && n <= 100 && strings.TrimSpace(t) == t
}

// DecoyQuestions 为不存在的用户名给出一组稳定的内置问题，与真实账号的响应形状一致，
// 避免通过找回接口探测用户名是否存在。同一用户名每次得到相同的问题。
func DecoyQuestions(key []byte, username string) []Question {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(strings.ToLower(username)))
	sum := m.Sum(nil)
	out := make([]Question, 0, QuestionCount)
	used := map[int]bool{}
	for i := 0; len(out) < QuestionCount; i++ {
		n := int(binary.BigEndian.Uint32(sum[(i*4)%28:]) % BuiltinQuestions)
		if !used[n] {
			used[n] = true
			out = append(out, Question{Builtin: n})
		}
	}
	return out
}
