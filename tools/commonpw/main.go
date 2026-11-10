// Command commonpw 生成随程序发布的常见口令表 internal/auth/common_passwords.txt（第 14.1 节）。
//
// 输入：SecLists 的 Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt（MIT），
// 固定在提交 a1a06ab57c3f4efc450d80738a72c11597aac81a，SHA-256 见 expectSHA。
// 只保留长度不少于 10 个字符（口令的最短长度）的条目：更短的口令本来就会被拒绝。
// 转为小写、去重、排序；另外加入几条项目自己的补充。
//
//	curl -LO https://raw.githubusercontent.com/danielmiessler/SecLists/a1a06ab57c3f4efc450d80738a72c11597aac81a/Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt
//	go run ./tools/commonpw -in 100k-most-used-passwords-NCSC.txt
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

const expectSHA = "c2e5696882c603b76bb67a47ee970897e5a76fc4c3f5547abe3d0ca340c576e0"

// extra 是 NCSC 表中没有、但在中文环境里常见的长口令。
var extra = []string{
	"1234567890", "12345678910", "qwertyuiop", "password123", "password1234", "iloveyou123", "0123456789",
	"1qaz2wsx3edc", "qwerty12345", "abc1234567", "administrator", "letmein1234", "1111111111", "0000000000",
	"a123456789", "woaini1314", "woaini520", "5201314520", "1314520520", "qq123456789", "aa123456789",
}

func main() {
	in := flag.String("in", "", "NCSC 口令表")
	out := flag.String("o", "internal/auth/common_passwords.txt", "输出")
	flag.Parse()
	b, err := os.ReadFile(*in)
	if err != nil {
		fail(err)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != expectSHA {
		fail(fmt.Errorf("SHA-256 不匹配：%s", got))
	}
	set := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		set[strings.ToLower(strings.TrimRight(sc.Text(), "\r"))] = true
	}
	for _, e := range extra {
		set[e] = true
	}
	var list []string
	for p := range set {
		if utf8.RuneCountInString(p) >= 10 && !strings.ContainsAny(p, "\r\n") {
			list = append(list, p)
		}
	}
	slices.Sort(list)
	var w strings.Builder
	w.WriteString("# 常见口令表（长度 ≥ 10，小写，排序）。由 tools/commonpw 生成，不要手工编辑。\n")
	w.WriteString("# 来源：SecLists 100k-most-used-passwords-NCSC.txt @ a1a06ab（MIT），以及 tools/commonpw 中的补充。\n")
	for _, p := range list {
		w.WriteString(p + "\n")
	}
	if err := os.WriteFile(*out, []byte(w.String()), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("已写入 %s：%d 条\n", *out, len(list))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "commonpw:", err)
	os.Exit(1)
}
