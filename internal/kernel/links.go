package kernel

import (
	"fmt"
	"strings"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/common/convert"
	mlog "github.com/metacubex/mihomo/log"
)

func init() {
	// mihomo 的转换器解析失败时会把整行（可能含凭据）写进它的日志；本服务从不使用 mihomo 的日志输出。
	mlog.SetLevel(mlog.SILENT)
}

// Schemes 是 mihomo v1.19.33 common/convert.ConvertsV2Ray 处理的 scheme（2026-11-09 对照源码）。
// wireguard 没有正式的分享链接规范，不在其中；WireGuard 节点通过 Clash YAML 输入。
var Schemes = []string{
	"hysteria", "hysteria2", "hy2", "hysteria2+realm", "hy2+realm", "tuic", "trojan", "vless", "vmess",
	"ss", "ssr", "socks", "socks5", "socks5h", "http", "https", "anytls", "mierus",
}

// LinkResult 是一行分享链接的解析结果。
type LinkResult struct {
	Line    int            // 输入中的行号（从 1 开始）
	Scheme  string         // 小写的 scheme
	Mapping map[string]any // 解析成功时的 Clash 节点字段
	Err     string         // 失败原因（不含链接内容与凭据）
}

// ParseLink 用 mihomo 的转换器解析单条分享链接。
//
// ConvertsV2Ray 一次处理多行时会静默跳过无法解析的行，并对重名节点自动改名；
// 为了「不静默丢弃」与保留原始名称，这里逐行调用，失败的行由调用方记入跳过清单。
func ParseLink(line string, lineNo int) (res LinkResult) {
	res.Line = lineNo
	scheme, _, ok := strings.Cut(strings.TrimSpace(line), "://")
	if !ok {
		res.Err = "not a share link"
		return res
	}
	res.Scheme = strings.ToLower(scheme)
	if !isSupported(res.Scheme) {
		res.Err = "unsupported scheme"
		return res
	}
	defer func() {
		if r := recover(); r != nil {
			res.Mapping = nil
			res.Err = "parser panic" // 不输出 panic 内容：其中可能带有链接片段
		}
	}()
	out, err := convert.ConvertsV2Ray([]byte(strings.TrimSpace(line)))
	switch {
	case err != nil:
		res.Err = "malformed link"
	case len(out) == 0:
		res.Err = "malformed link"
	case len(out) > 1:
		res.Err = "one line produced several proxies"
	default:
		res.Mapping = out[0]
		stabilizeUA(res.Mapping, line)
	}
	return res
}

// stabilizeUA 让 ws 传输的 User-Agent 可复现。
//
// mihomo 的转换器在解析 ws 传输的 trojan、vless、vmess 链接时，会从内置的浏览器 UA 表中随机挑一个
// 写入 ws-opts.headers.User-Agent（common/convert 的 RandUserAgent）。分享链接本身不携带 UA，
// 因此该值只可能来自转换器；这里保留其意图（模拟浏览器），把随机选择改为按链接内容
// （不含 # 之后的名称）哈希选择同一张表中的条目，使相同输入得到相同输出（第 7.1 节）。
func stabilizeUA(m map[string]any, line string) {
	ws, ok := m["ws-opts"].(map[string]any)
	if !ok {
		return
	}
	h, ok := ws["headers"].(map[string]any)
	if !ok {
		return
	}
	ua, ok := h["User-Agent"].(string)
	if !ok || !isMihomoUA(ua) {
		return
	}
	body, _, _ := strings.Cut(strings.TrimSpace(line), "#")
	var sum uint32 = 2166136261 // FNV-1a
	for i := 0; i < len(body); i++ {
		sum ^= uint32(body[i])
		sum *= 16777619
	}
	h["User-Agent"] = mihomoUserAgents[sum%uint32(len(mihomoUserAgents))] //nolint:gosec // 表长 250，远小于 2^32
}

func isMihomoUA(ua string) bool {
	for _, x := range mihomoUserAgents {
		if x == ua {
			return true
		}
	}
	return false
}

func isSupported(s string) bool {
	for _, x := range Schemes {
		if x == s {
			return true
		}
	}
	return false
}

// ValidateProxy 用 mihomo 的节点解析（adapter.ParseProxy）校验节点字段，与内核加载配置时的行为一致。
// 构造出的出站对象立即关闭；构造过程不建立连接。返回的错误信息来自 mihomo，可能包含字段名与取值，
// 调用方在写入诊断前须经 Redact 脱敏。
func ValidateProxy(m map[string]any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("proxy parser panic")
		}
	}()
	p, err := adapter.ParseProxy(m)
	if err != nil {
		return err
	}
	return p.Close()
}

// Redact 去掉错误信息中可能出现的凭据：URL 中的用户信息与形似令牌的长串。
func Redact(msg string) string {
	msg = userinfo.ReplaceAllString(msg, "://***@")
	msg = longToken.ReplaceAllString(msg, "***")
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return msg
}
