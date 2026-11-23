// Package match 在服务器上复现 mihomo 的规则匹配，用于分流追踪（docs/design/routing.md 第 5 节，决定 0007 #1–#4）。
//
// 判断尽量直接调用内核导出的对象：domain、ipcidr 规则集用 provider.NewDomainStrategy / NewIPCidrStrategy，
// 普通规则用 rules.ParseRule 与 logic.NewAND/OR/NOT。以下几处内核实现依赖进程级全局状态，本包改为移植或标记：
//   - RULE-SET 经全局 tunnel 查找规则集 → 由 Trace 直接调用本包持有的 Set；
//   - GEOIP、GEOSITE、IP-ASN 的构造函数会在文件缺失时自动下载、MMDB 打不开时 log.Fatalln → 见 geo.go；
//   - 逻辑规则内的 RULE-SET、SUB-RULE → 标为无法判断。
//
// 本包不发起任何网络连接，不读当前时间与随机数；规则内容由调用方提供。
package match

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/metacubex/mihomo/common/yaml"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
	mlog "github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/rules/common"
	"github.com/metacubex/mihomo/rules/provider"
)

func init() {
	// 策略对象遇到无效条目时用 mihomo 的日志打警告；本服务不使用 mihomo 的日志输出（与 internal/kernel 一致）。
	mlog.SetLevel(mlog.SILENT)
}

// 上限（routing.md 第 5.1 节）：单文件与拉取上限一致；条数超过时整个规则集无法判断。
const (
	MaxBytes   = 20 << 20
	MaxEntries = 2_000_000
	// MRS 解压后的上限，防止压缩炸弹。200 万条域名的二进制前缀树约几十 MiB，留足余量。
	maxMrsDecoded = 256 << 20
)

// strategy 是 domain、ipcidr 策略对象共有的导出方法（内核 rules/provider 的 ruleStrategy 与 mrsRuleStrategy）。
type strategy interface {
	Match(*C.Metadata, C.RuleMatchHelper) bool
	Count() int
	Reset()
	Insert(string)
	FinishInsert()
	FromMrs(io.Reader, int) error
	DumpMrs(func(string) bool)
}

// Set 是一个编译好的规则集。判断用内核的策略对象；lines 只用于定位依据（命中的那一行）。
type Set struct {
	Name, Behavior, Format string // behavior: domain | ipcidr | classical；format: text | yaml | mrs | inline
	Count                  int    // 有效条目数，与内核 ruleCount 相同

	s       strategy // domain、ipcidr
	lines   []string // text、yaml、inline 的条目原文；mrs 为空，依据改用 DumpMrs 遍历
	entries []*entry // classical：按原顺序
	ip      bool     // 是否可能因目的 IP 命中（ipcidr，或 classical 中含 IP 类条目）

	sampleOnce sync.Once
	samples    []string
}

// IPDependent 报告规则集是否可能因目的 IP 命中。
func (s *Set) IPDependent() bool { return s.ip }

// Compile 编译规则文件。behavior、format 的取值与 rule-providers 相同（format 为空时按内核默认的 yaml）。
func Compile(name, behavior, format string, body []byte, env Env) (*Set, error) {
	if len(body) > MaxBytes {
		return nil, fmt.Errorf("规则集 %s 超过 %d 字节", name, MaxBytes)
	}
	b, err := P.ParseBehavior(behavior)
	if err != nil {
		return nil, err
	}
	f, err := P.ParseRuleFormat(format)
	if err != nil {
		return nil, err
	}
	set := &Set{Name: name, Behavior: behavior, Format: map[P.RuleFormat]string{P.YamlRule: "yaml", P.TextRule: "text", P.MrsRule: "mrs"}[f]}
	if f == P.MrsRule {
		if b == P.Classical {
			return nil, fmt.Errorf("规则集 %s：classical 没有 mrs 格式", name)
		}
		set.s = newStrategy(b)
		if err := readMrs(body, set.s, b); err != nil {
			return nil, fmt.Errorf("规则集 %s：%w", name, err)
		}
		set.Count = set.s.Count()
		set.ip = b == P.IPCIDR
		return set, nil
	}
	lines, err := parseLines(body, f)
	if err != nil {
		return nil, fmt.Errorf("规则集 %s：%w", name, err)
	}
	set.fill(b, lines, env)
	return set, nil
}

// CompileInline 编译内联规则集（rule-providers 的 type: inline 与 payload）。
func CompileInline(name, behavior string, payload []string, env Env) (*Set, error) {
	b, err := P.ParseBehavior(behavior)
	if err != nil {
		return nil, err
	}
	if len(payload) > MaxEntries {
		return nil, fmt.Errorf("规则集 %s 超过 %d 条", name, MaxEntries)
	}
	// 移植 rulesParseInline：只跳过空字符串，不去空白、不识别注释。
	lines := make([]string, 0, len(payload))
	for _, r := range payload {
		if r != "" {
			lines = append(lines, r)
		}
	}
	set := &Set{Name: name, Behavior: behavior, Format: "inline"}
	set.fill(b, lines, env)
	return set, nil
}

func (set *Set) fill(b P.RuleBehavior, lines []string, env Env) {
	set.lines = lines
	if b == P.Classical {
		for _, raw := range lines {
			if e := compileEntry(raw, env); e != nil {
				set.entries = append(set.entries, e)
				set.ip = set.ip || e.info.ip
			}
		}
		set.Count = len(set.entries)
		return
	}
	set.s = newStrategy(b)
	set.s.Reset()
	for _, l := range lines {
		set.s.Insert(l)
	}
	set.s.FinishInsert()
	set.Count = set.s.Count()
	set.ip = b == P.IPCIDR
}

func newStrategy(b P.RuleBehavior) strategy {
	if b == P.Domain {
		return provider.NewDomainStrategy()
	}
	return provider.NewIPCidrStrategy()
}

// compileEntry 移植 classicalStrategy.payloadToRule：MATCH、RULE-SET、SUB-RULE 不允许出现在 classical 规则集里，
// 解析失败的条目被内核跳过且不计数，这里同样返回 nil。
//
// classical 的匹配在内核中就是按顺序逐条调用 Match（classical_strategy.go），本包自己遍历，
// 以便对含有无法判断成分的条目单独处理，结果与内核的布尔值一致。
func compileEntry(raw string, env Env) *entry {
	tp, payload, target, params := common.ParseRulePayload(raw, false)
	switch tp {
	case "MATCH", "RULE-SET", "SUB-RULE":
		return nil
	}
	e := &entry{raw: raw, tp: tp, payload: payload, target: target, params: params}
	r, info, err := build(tp, payload, target, params, env, nil, false)
	if err != nil {
		return nil
	}
	e.rule, e.info = r, info
	return e
}

var (
	errNoPayload     = errors.New("file must have a `payload` field") // 与内核 ErrNoPayload 文字相同
	errInvalidFormat = errors.New("invalid format")
)

// rulePayload 对应内核 RulePayload。
type rulePayload struct {
	Payload []string `yaml:"payload"`
	Rules   []string `yaml:"rules"`
}

// parseLines 逐行移植 mihomo v1.19.34 rules/provider/provider.go 的 rulesParse（text、yaml 分支），
// 只把原本交给 strategy.Insert 的字符串收集起来。yaml 的逐行解析方式（先找 payload 头，再逐行拼上头部解析）
// 与内核保持一致，因此对不规范文件的容错也相同。
func parseLines(buf []byte, format P.RuleFormat) ([]string, error) {
	schema := &rulePayload{}
	var first bytes.Buffer
	firstLineLength := 0
	var out []string

	s := 0
	for s < len(buf) {
		line := buf[s:]
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			i += s
			line = buf[s : i+1]
			s = i + 1
		} else {
			s = len(buf)
			if firstLineLength == 0 && format == P.YamlRule { // 没有头部，或只有一行
				return nil, errNoPayload
			}
		}
		var str string
		switch format {
		case P.TextRule:
			str = strings.TrimSpace(string(line))
			if len(str) == 0 || str[0] == '#' || strings.HasPrefix(str, "//") {
				continue
			}
		case P.YamlRule:
			trim := bytes.TrimSpace(line)
			if len(trim) == 0 || trim[0] == '#' {
				continue
			}
			first.Write(line)
			if firstLineLength == 0 {
				firstLineLength = first.Len()
				first.WriteString("  - ''") // 内核用一行测试条目判断这是不是 payload 头
				err := yaml.Unmarshal(first.Bytes(), schema)
				first.Truncate(firstLineLength)
				if err == nil && (len(schema.Rules) > 0 || len(schema.Payload) > 0) {
					continue
				}
				first.Truncate(0)
				firstLineLength = 0
				continue
			}
			err := yaml.Unmarshal(first.Bytes(), schema)
			first.Truncate(firstLineLength)
			if err != nil {
				continue
			}
			if len(schema.Rules) > 0 {
				str = schema.Rules[0]
			}
			if len(schema.Payload) > 0 {
				str = schema.Payload[0]
			}
		default:
			return nil, errInvalidFormat
		}
		if str == "" {
			continue
		}
		if len(out) >= MaxEntries {
			return nil, fmt.Errorf("超过 %d 条", MaxEntries)
		}
		out = append(out, str)
	}
	return out, nil
}

// readMrs 移植 mihomo v1.19.34 rules/provider/mrs_reader.go 的 rulesMrsParse：zstd 解压后依次是
// 魔数 MRS\x01、行为字节、条数（int64 大端）、扩展长度与内容，之后交给策略对象的 FromMrs。
func readMrs(buf []byte, s strategy, b P.RuleBehavior) error {
	dec, err := zstd.NewReader(bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer dec.Close()
	r := &capReader{r: dec, left: maxMrsDecoded}

	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	if header != provider.MrsMagicBytes {
		return errors.New("invalid MrsMagic bytes")
	}
	var behavior [1]byte
	if _, err := io.ReadFull(r, behavior[:]); err != nil {
		return err
	}
	if behavior[0] != b.Byte() {
		return errors.New("invalid behavior")
	}
	var count int64
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return err
	}
	if count < 0 || count > MaxEntries {
		return fmt.Errorf("条数 %d 超出范围（上限 %d）", count, MaxEntries)
	}
	var length int64
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return err
	}
	if length < 0 || length > maxMrsDecoded {
		return errors.New("length is invalid")
	}
	if length > 0 {
		if _, err := io.CopyN(io.Discard, r, length); err != nil {
			return err
		}
	}
	return s.FromMrs(r, int(count))
}

// capReader 在解压内容超过上限时返回错误（而不是静默截断）。
type capReader struct {
	r    io.Reader
	left int64
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		return 0, fmt.Errorf("mrs 解压后超过 %d 字节", maxMrsDecoded)
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}
