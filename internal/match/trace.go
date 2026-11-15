package match

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/provider"
)

// Request 是一次追踪的输入。用 NewRequest 从用户输入构造。
type Request struct {
	Host        string     // 小写，已去掉协议、路径、端口与末尾的点；输入是 IP 时为空
	IP          netip.Addr // 输入的是 IP 时有效
	Port        uint16     // 输入中带端口时有效；0 表示未知（DST-PORT 规则无法判断）
	Process     string     // 应用名，可选
	ProcessPath string     // 输入的应用名含路径分隔符时为完整路径
	Network     string     // tcp | udp
}

// NewRequest 规范化用户输入：接受域名、IP、带协议或路径的地址（https://www.example.com:8443/a）。
// process 含 / 或 \ 时视为路径，同时取最后一段作为应用名。network 为空时按 tcp。
func NewRequest(input, process, network string) (Request, error) {
	var r Request
	s := strings.ToLower(strings.TrimSpace(input))
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	host, port := s, ""
	switch {
	case strings.HasPrefix(s, "["): // [2001:db8::1]:443
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return r, errors.New("地址缺少 ]")
		}
		host, port = s[1:end], strings.TrimPrefix(s[end+1:], ":")
	case strings.Count(s, ":") == 1:
		host, port = s[:strings.IndexByte(s, ':')], s[strings.IndexByte(s, ':')+1:]
	}
	if port != "" {
		p, err := strconv.ParseUint(port, 10, 16)
		if err != nil || p == 0 {
			return r, fmt.Errorf("端口无效：%s", port)
		}
		r.Port = uint16(p)
	}
	host = strings.TrimSuffix(host, ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		r.IP = ip.Unmap().WithZone("")
	} else {
		if host == "" || len(host) > 253 || strings.ContainsAny(host, " \t,:[]") {
			return r, fmt.Errorf("不是有效的域名或 IP：%q", input)
		}
		r.Host = host
	}
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "", "tcp":
		r.Network = "tcp"
	case "udp":
		r.Network = "udp"
	default:
		return r, fmt.Errorf("网络类型只能是 tcp 或 udp：%q", network)
	}
	process = strings.TrimSpace(process)
	if strings.ContainsAny(process, `/\`) {
		r.ProcessPath = process
		process = process[strings.LastIndexAny(process, `/\`)+1:]
	}
	r.Process = process
	return r, nil
}

// Line 是最终配置 rules 中的一行。
type Line struct {
	Index                 int
	Raw                   string // 原文，例如 RULE-SET,Direct,DIRECT,no-resolve
	Type, Payload, Target string
	Params                []string
}

// ParseLine 按内核 ParseRulePayload（needTarget）拆分 rules 中的一行。
func ParseLine(index int, raw string) Line {
	tp, payload, target, params := common.ParseRulePayload(raw, true)
	return Line{Index: index, Raw: raw, Type: tp, Payload: payload, Target: target, Params: params}
}

// Step 是一行的判断结果。
type Step struct {
	Index    int
	Result   string // miss | hit | needs-ip | unknown
	Evidence string // 命中的那一条，例如 "+.example.com"、"IP-CIDR,192.0.2.0/24"；定位不到时为空
	Reason   string // unknown 的原因代码（Reason* 常量）
}

const (
	ResultMiss    = "miss"
	ResultHit     = "hit"
	ResultNeedsIP = "needs-ip"
	ResultUnknown = "unknown"
)

// Outcome 是一次追踪的结果。
type Outcome struct {
	Steps        []Step // 从第 0 行到命中为止
	Hit          int    // 命中的行号；-1 表示没有命中
	NeedsIPAt    int    // 第一次需要 IP 的行号，-1 表示不需要
	IPCandidates []int  // 从 NeedsIPAt 起、Hit 之前可能因解析出的 IP 而命中的行
}

// Trace 按 mihomo tunnel.match 的顺序逐行判断，返回第一条命中的行。
//
// 与内核的差别只在「拿不到的信息」上：
//   - 不解析 DNS（决定 0007 #4）。规则请求解析时只记录 NeedsIPAt，IP 类规则因没有 IP 而不命中，追踪继续向下；
//     内核解析一次后 DstIP 对后续所有行可见（含 no-resolve 的行），因此从 NeedsIPAt 起依赖 IP 的行都进入 IPCandidates；
//   - 无法判断的行（Reason* 常量）视为不命中并继续。
//
// 命中后，内核还会因目标不存在、PASS、REMATCH 或节点不支持 UDP 而跳过该行（tunnel.go），这些取决于
// 策略组与节点，由调用方（service）检查后从下一行起再次调用 TraceFrom。
func Trace(lines []Line, sets map[string]*Set, req Request, env Env) Outcome {
	return TraceFrom(lines, 0, sets, req, env)
}

// TraceFrom 与 Trace 相同，但从第 start 行开始（之前的行视为已判断为不命中）。
func TraceFrom(lines []Line, start int, sets map[string]*Set, req Request, env Env) Outcome {
	out := Outcome{Hit: -1, NeedsIPAt: -1}
	md := &C.Metadata{Host: req.Host, DstIP: req.IP, DstPort: req.Port, Process: req.Process, ProcessPath: req.ProcessPath, NetWork: C.TCP}
	if req.Network == "udp" {
		md.NetWork = C.UDP
	}
	resolved := false
	h := C.RuleMatchHelper{ResolveIP: func() { resolved = true }, FindProcess: func() {}}

	for i := start; i < len(lines); i++ {
		l := lines[i]
		resolved = false
		step := Step{Index: l.Index}
		matched, ipDep := false, false
		switch l.Type {
		case "MATCH":
			matched = true
		case "SUB-RULE":
			step.Reason = ReasonSubRule
		case "RULE-SET":
			set := sets[l.Payload]
			isSrc, noResolve := common.ParseParams(l.Params)
			switch {
			case set == nil:
				step.Reason = ReasonNoContent
			case isSrc:
				step.Reason = ReasonNeedsConnection
			default:
				hh := h
				if noResolve { // 与内核 rule_set.go 相同
					hh.ResolveIP = nil
				}
				matched, step.Evidence, step.Reason = set.match(md, hh, &req, env)
				ipDep = set.ip
			}
		default:
			// 顶层行带目标，按 ParseLine（needTarget）的拆分结果编译；每次追踪重新编译，代价可以忽略。
			e := &entry{raw: l.Raw, tp: l.Type, payload: l.Payload, target: l.Target, params: l.Params}
			r, info, err := build(l.Type, l.Payload, l.Target, l.Params, env, nil, false)
			if err != nil {
				step.Reason = ReasonParse
				break
			}
			e.rule, e.info = r, info
			matched, step.Reason = e.eval(md, h, &req, env)
			ipDep = e.info.ip
			resolved = resolved || (e.info.resolves && !matched)
			if matched {
				step.Evidence = l.Type + "," + l.Payload
			}
		}

		switch {
		case matched:
			step.Result, step.Reason = ResultHit, ""
		case step.Reason != "":
			step.Result = ResultUnknown
		default:
			step.Result = ResultMiss
		}
		if !req.IP.IsValid() && !matched {
			if resolved && out.NeedsIPAt < 0 {
				out.NeedsIPAt = l.Index
			}
			if ipDep && out.NeedsIPAt >= 0 {
				out.IPCandidates = append(out.IPCandidates, l.Index)
				if step.Result == ResultMiss {
					step.Result = ResultNeedsIP
				}
			}
		}
		out.Steps = append(out.Steps, step)
		if matched {
			out.Hit = l.Index
			break
		}
	}
	return out
}

// match 判断规则集是否命中，并定位依据。
func (s *Set) match(md *C.Metadata, h C.RuleMatchHelper, req *Request, env Env) (matched bool, evidence, reason string) {
	if s.entries != nil || s.s == nil { // classical
		for _, e := range s.entries {
			m, why := e.eval(md, h, req, env)
			if m {
				return true, e.raw, ""
			}
			if e.info.resolves && h.ResolveIP != nil { // 被替换的 GEOIP、IP-ASN 在内核中会触发解析
				h.ResolveIP()
			}
			if why != "" && reason == "" {
				reason = why
			}
		}
		return false, "", reason
	}
	if !s.s.Match(md, h) {
		return false, "", ""
	}
	return true, s.locate(md), ""
}

// locate 找出覆盖请求的第一条条目。逐条用单条目的内核策略对象判断，语义与整体匹配完全相同；
// 先用廉价的文本条件筛掉不可能的条目。定位不到时返回空字符串，不影响结果。
func (s *Set) locate(md *C.Metadata) string {
	found := ""
	try := func(item string) bool { // 返回 false 停止遍历（与 DumpMrs 的约定一致）
		if s.Behavior == "domain" {
			if !mayCover(item, md.RuleHost()) {
				return true
			}
			one := provider.NewDomainStrategy()
			one.Insert(item)
			one.FinishInsert()
			if one.Match(md, C.RuleMatchHelper{}) {
				found = item
				return false
			}
			return true
		}
		if p, err := netip.ParsePrefix(item); err == nil {
			if p.Contains(md.DstIP.WithZone("")) {
				found = item
				return false
			}
			return true
		}
		one := provider.NewIPCidrStrategy()
		one.Reset()
		one.Insert(item)
		one.FinishInsert()
		if one.Match(md, C.RuleMatchHelper{}) {
			found = item
			return false
		}
		return true
	}
	if s.lines != nil {
		for _, l := range s.lines {
			if !try(l) {
				break
			}
		}
	} else {
		s.s.DumpMrs(try)
	}
	return found
}

// mayCover 是域名条目的粗筛：去掉 +.、.、*. 前缀后，含通配符的条目一律交给精确判断，
// 否则只有等于主机名或是其后缀的条目才可能覆盖。
func mayCover(item, host string) bool {
	if host == "" {
		return false
	}
	rest := strings.ToLower(item)
	for _, p := range []string{"+.", ".", "*."} {
		if strings.HasPrefix(rest, p) {
			rest = rest[len(p):]
			break
		}
	}
	if strings.ContainsAny(rest, "*+") {
		return true
	}
	return host == rest || strings.HasSuffix(host, "."+rest)
}
