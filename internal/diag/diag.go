// Package diag 定义贯穿整条管线的诊断类型。
//
// 诊断是「不静默丢弃」原则的载体：任何被跳过、改写、删除或无法验证的内容
// 都以一条诊断记录下来，管线继续执行，让一次运行就能看到全部问题。
// 诊断不携带面向用户的文本：消息由 Code 对应的 i18n 键与 Args 生成，
// 因此 Code 一经发布就不改名（见 codes.go）。
package diag

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Severity 是诊断的严重程度。
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Diagnostic 是一条机器可读的诊断。
type Diagnostic struct {
	Code     Code              `json:"code"`
	Severity Severity          `json:"severity"`
	Path     string            `json:"path,omitempty"` // 配置路径（dns.nameserver[2]）或输入位置（source:3）
	Args     map[string]string `json:"args,omitempty"` // 消息参数；生成时已脱敏
}

// String 用于日志与测试输出；界面文本走 i18n。
func (d Diagnostic) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s", d.Severity, d.Code)
	if d.Path != "" {
		fmt.Fprintf(&b, " @%s", d.Path)
	}
	if len(d.Args) > 0 {
		keys := make([]string, 0, len(d.Args))
		for k := range d.Args {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, " %s=%q", k, d.Args[k])
		}
	}
	return b.String()
}

// List 收集诊断。零值可用。
type List struct {
	items []Diagnostic
}

// Add 追加一条诊断。args 以键值对给出：Add(c, path, "name", n, "line", "3")。
// 未登记的 Code 在测试中会被 TestCodesRegistered 捕获；这里直接 panic，
// 因为它只可能来自程序缺陷，而非用户输入。
func (l *List) Add(code Code, path string, kv ...string) {
	info, ok := registry[code]
	if !ok {
		panic("diag: 未登记的诊断代码 " + string(code))
	}
	if len(kv)%2 != 0 {
		panic("diag: 参数必须成对出现")
	}
	var args map[string]string
	if len(kv) > 0 {
		args = make(map[string]string, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			args[kv[i]] = kv[i+1]
		}
	}
	l.items = append(l.items, Diagnostic{Code: code, Severity: info.Severity, Path: path, Args: args})
}

// Append 合并另一组诊断。
func (l *List) Append(ds ...Diagnostic) { l.items = append(l.items, ds...) }

// Items 返回诊断的副本，顺序为产生顺序。
func (l *List) Items() []Diagnostic { return slices.Clone(l.items) }

// Len 返回诊断条数。
func (l *List) Len() int { return len(l.items) }

// HasErrors 报告是否存在 error 级诊断。
func (l *List) HasErrors() bool {
	return slices.ContainsFunc(l.items, func(d Diagnostic) bool { return d.Severity == Error })
}

// Has 报告是否存在指定代码的诊断（测试与界面提示用）。
func (l *List) Has(code Code) bool {
	return slices.ContainsFunc(l.items, func(d Diagnostic) bool { return d.Code == code })
}

// Sorted 返回按严重程度、代码、路径排序的副本，供界面稳定展示。
// 管线内部保留产生顺序；排序只用于呈现。
func Sorted(ds []Diagnostic) []Diagnostic {
	rank := map[Severity]int{Error: 0, Warning: 1, Info: 2}
	out := slices.Clone(ds)
	slices.SortStableFunc(out, func(a, b Diagnostic) int {
		return cmp.Or(cmp.Compare(rank[a.Severity], rank[b.Severity]), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Path, b.Path))
	})
	return out
}

// Message 按语言渲染诊断消息（"zh" 或其他均视为英文）。占位符 {name} 用 Args 替换。
func Message(d Diagnostic, lang string) string {
	info, ok := registry[d.Code]
	if !ok {
		return string(d.Code)
	}
	msg := info.EN
	if strings.HasPrefix(lang, "zh") {
		msg = info.ZH
	}
	for k, v := range d.Args {
		msg = strings.ReplaceAll(msg, "{"+k+"}", v)
	}
	return msg
}
