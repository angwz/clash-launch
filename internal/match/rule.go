package match

import (
	"fmt"
	"slices"
	"strings"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/rules"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/logic"
)

// 无法判断的原因代码（Step.Reason，前端按代码显示文案）。
const (
	ReasonLogicRuleSet    = "logic-ruleset"    // 逻辑规则里引用 RULE-SET：内核经全局 tunnel 查找，服务器无法复现
	ReasonSubRule         = "sub-rule"         // SUB-RULE：取决于子规则表
	ReasonNeedsProcess    = "needs-process"    // 规则取决于应用，追踪时没有填写（或只填了名字而规则看路径）
	ReasonNeedsConnection = "needs-connection" // 取决于来源地址、入站、端口、DSCP、UID 等追踪不掌握的连接信息
	ReasonNoContent       = "no-content"       // 规则集内容不可用（只有 path、拉取失败、超出上限）
	ReasonGeodata         = "geodata"          // 没有可用的地理数据
	ReasonParse           = "parse"            // 内核无法解析这一行
)

// Env 是追踪所处的环境：地理数据与最终配置里的 geodata-mode（true 时 GEOIP 用 geoip.dat，否则用 MMDB）。
type Env struct {
	Geo         *Geo
	GeodataMode bool
}

// ruleInfo 是编译期得到的标记。reason 是与请求无关的无法判断原因；process、path、port 表示
// 规则（或其子规则）取决于应用名、应用路径、目的端口，是否可判断要看请求有没有提供。
type ruleInfo struct {
	reason              string
	process, path, port bool
	ip                  bool // 可能因目的 IP 命中（不论 no-resolve：一旦前面的规则触发解析，后面的 no-resolve 规则也会看到 IP）
	resolves            bool // 含有因缺少地理数据而被替换、但在内核中会触发 DNS 解析的成分（没有 no-resolve 的 GEOIP、IP-ASN）
}

func (i *ruleInfo) flag(reason string) {
	if i.reason == "" {
		i.reason = reason
	}
}

// uncertain 返回在请求 req 下该规则含有的无法判断原因；空字符串表示可以确定地判断。
func (i ruleInfo) uncertain(req *Request) string {
	switch {
	case i.reason != "":
		return i.reason
	case i.process && req.Process == "", i.path && req.ProcessPath == "":
		return ReasonNeedsProcess
	case i.port && req.Port == 0:
		return ReasonNeedsConnection
	}
	return ""
}

// entry 是一条普通规则或 classical 规则集里的一条。
type entry struct {
	raw                 string
	tp, payload, target string
	params              []string
	rule                C.Rule // req 为 nil 时编译的版本，只在 info.uncertain(req) 为空时使用
	info                ruleInfo
}

// eval 判断一条规则。含有无法判断的成分时，把这些成分分别假定为「不命中」与「命中」各算一次：
// 两次结果相同就是确定的（例如 AND 中另一项已不成立），不同才是无法判断。
func (e *entry) eval(md *C.Metadata, h C.RuleMatchHelper, req *Request, env Env) (matched bool, reason string) {
	reason = e.info.uncertain(req)
	if reason == "" {
		m, _ := e.rule.Match(md, h)
		return m, ""
	}
	r0, _, err0 := build(e.tp, e.payload, e.target, e.params, env, req, false)
	r1, _, err1 := build(e.tp, e.payload, e.target, e.params, env, req, true)
	if err0 != nil || err1 != nil {
		return false, ReasonParse
	}
	m0, _ := r0.Match(md, h)
	m1, _ := r1.Match(md, h)
	if m0 == m1 {
		return m0, ""
	}
	return false, reason
}

// 只依赖连接信息、追踪中拿不到的规则类型。
var connectionTypes = []string{
	"SRC-IP-CIDR", "SRC-IP-SUFFIX", "SRC-IP-ASN", "SRC-GEOIP", "SRC-PORT", "IN-PORT",
	"IN-TYPE", "IN-USER", "IN-NAME", "DSCP", "UID", "REMATCH-NAME",
}

// build 把一条规则解析为规则对象，并收集 ruleInfo。逻辑规则的子规则也经过这里（传给 logic.NewXXX 的解析函数），
// 因此子规则中的地理规则、RULE-SET 同样被拦截，不会走到内核依赖全局状态的构造函数。
//
// req 为 nil 时（编译期）取决于请求的成分（应用、端口）按真实规则编译并只做标记；
// req 不为 nil 时，请求缺少的信息与其他无法判断的成分一样替换为常量规则，常量取 assume。
func build(tp, payload, target string, params []string, env Env, req *Request, assume bool) (C.Rule, ruleInfo, error) {
	var info ruleInfo
	var parse common.ParseRuleFunc
	unknown := func(reason, tp, payload, target string) (C.Rule, error) {
		info.flag(reason)
		return &constRule{tp: tp, payload: payload, adapter: target, value: assume}, nil
	}
	parse = func(tp, payload, target string, params []string, _ map[string][]C.Rule) (C.Rule, error) {
		if tp != "MATCH" && payload == "" { // 与 rules.ParseRule 的检查相同
			return nil, fmt.Errorf("missing subsequent parameters: %s", tp)
		}
		isSrc, noResolve := common.ParseParams(params)
		switch {
		case tp == "AND":
			return logic.NewAND(payload, target, parse)
		case tp == "OR":
			return logic.NewOR(payload, target, parse)
		case tp == "NOT":
			return logic.NewNOT(payload, target, parse)
		case tp == "SUB-RULE":
			return unknown(ReasonSubRule, tp, payload, target)
		case tp == "RULE-SET": // 只会出现在逻辑规则里：顶层 RULE-SET 由 Trace 处理
			return unknown(ReasonLogicRuleSet, tp, payload, target)
		case slices.Contains(connectionTypes, tp):
			return unknown(ReasonNeedsConnection, tp, payload, target)
		case strings.HasPrefix(tp, "PROCESS-NAME"):
			info.process = true
			if req != nil && req.Process == "" {
				return unknown(ReasonNeedsProcess, tp, payload, target)
			}
		case strings.HasPrefix(tp, "PROCESS-PATH"):
			info.path = true
			if req != nil && req.ProcessPath == "" {
				return unknown(ReasonNeedsProcess, tp, payload, target)
			}
		case tp == "DST-PORT":
			info.port = true
			if req != nil && req.Port == 0 {
				return unknown(ReasonNeedsConnection, tp, payload, target)
			}
		case tp == "IP-CIDR", tp == "IP-CIDR6", tp == "IP-SUFFIX", tp == "IP-ASN", tp == "GEOIP":
			if isSrc { // src 参数：按来源地址匹配
				return unknown(ReasonNeedsConnection, tp, payload, target)
			}
			info.ip = true
			missing := func() (C.Rule, error) {
				info.resolves = info.resolves || !noResolve
				return unknown(ReasonGeodata, tp, payload, target)
			}
			switch tp {
			case "GEOIP":
				return newGeoIP(payload, target, noResolve, env, missing)
			case "IP-ASN":
				return newIPASN(payload, target, noResolve, env, missing)
			}
		case tp == "GEOSITE":
			return newGeoSite(payload, target, env, func() (C.Rule, error) { return unknown(ReasonGeodata, tp, payload, target) })
		}
		return rules.ParseRule(tp, payload, target, params, nil)
	}
	r, err := parse(tp, payload, target, params, nil)
	return r, info, err
}

// constRule 代替无法判断的成分，Match 恒为 value。
type constRule struct {
	common.Base
	tp, payload, adapter string
	value                bool
}

func (r *constRule) RuleType() C.RuleType { return C.MATCH }
func (r *constRule) Match(*C.Metadata, C.RuleMatchHelper) (bool, string) {
	return r.value, r.adapter
}
func (r *constRule) Adapter() string { return r.adapter }
func (r *constRule) Payload() string { return r.payload }
