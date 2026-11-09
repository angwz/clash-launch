package node

import (
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// 地区识别（第 5.8 节）。结果是节点的属性（ISO 3166-1 两位代码），国旗只是它的一种显示方式。
//
// 内置识别按以下步骤打分，而不是「第一个命中的正则」：
//  1. 名称中已有国旗 emoji：直接采用；
//  2. 出现方向符号（→、->、转……）时只看符号之后的部分；
//  3. 在名称中找出所有地区线索与线路词（广港、IEPL……），较长的词优先并占用位置，
//     等长时地区名优先于线路词；线路词被剔除，记入理由；
//  4. 多个地区线索时取最靠后的一个（机场惯例：线路在前、落地在后），标为「推断」；
//  5. 只剩线路词时，用线路词中的地区，标为「推断」。

// RegionKind 是识别结果的来源。
type RegionKind string

const (
	RegionFlag     RegionKind = "flag"    // 名称中已有国旗
	RegionKeyword  RegionKind = "keyword" // 命中地区关键词
	RegionRoute    RegionKind = "route"   // 只有线路词，取其中的地区
	RegionManual   RegionKind = "manual"  // 单个节点的手动指定
	RegionBySource RegionKind = "rule"    // 源级规则
	RegionUnknown  RegionKind = "unknown" // 未识别
)

// Region 是识别结果与理由。
type Region struct {
	Code     string     `json:"code,omitempty"` // 未识别时为空
	Kind     RegionKind `json:"kind"`
	Match    string     `json:"match,omitempty"`    // 命中的文字
	Ignored  []string   `json:"ignored,omitempty"`  // 被剔除的线路词
	Inferred bool       `json:"inferred,omitempty"` // 多个候选或只有线路词
}

// Explain 返回人能读懂的理由，用于诊断与 CLI（界面另由 i18n 组合）。
func (r Region) Explain() string {
	var b strings.Builder
	switch r.Kind {
	case RegionFlag:
		b.WriteString("名称中已有国旗")
	case RegionKeyword:
		fmt.Fprintf(&b, "命中关键词 %s", r.Match)
	case RegionRoute:
		fmt.Fprintf(&b, "只有线路词 %s", r.Match)
	case RegionManual:
		b.WriteString("手动指定")
	case RegionBySource:
		fmt.Fprintf(&b, "源级规则 %s", r.Match)
	default:
		b.WriteString("未识别")
	}
	if len(r.Ignored) > 0 {
		fmt.Fprintf(&b, "；已忽略线路词 %s", strings.Join(r.Ignored, "、"))
	}
	if r.Inferred {
		b.WriteString("；推断")
	}
	return b.String()
}

type regionDef struct {
	Code   string   `yaml:"code"`
	ISO3   string   `yaml:"iso3"`
	Short  string   `yaml:"short"`
	ZH     []string `yaml:"zh"`
	EN     []string `yaml:"en"`
	Cities []string `yaml:"cities"`
	Codes  []string `yaml:"codes"`
}

type regionFile struct {
	Regions     []regionDef `yaml:"regions"`
	RouteCities []string    `yaml:"route-cities"`
	RouteWords  []string    `yaml:"route-words"`
	Arrows      []string    `yaml:"arrows"`
}

type termKind int

// 等长时的优先级：数值越大越优先。
const (
	kindShort termKind = iota // 单字简称
	kindCode                  // ISO 代码与额外代码
	kindRoute                 // 线路词
	kindEN                    // 英文名、英文城市
	kindZH                    // 中文名、中文城市
)

type term struct {
	text     string
	code     string // 线路词：其中地区简称对应的代码（可能为空）
	kind     termKind
	fold     bool // 不区分大小写
	boundary bool // 需要词边界（前后不是 ASCII 字母）
	runes    int
}

// Detector 是加载好的识别器。只读，可并发使用。
type Detector struct {
	terms  []term
	arrows []string
	codes  map[string]bool
}

// lowerCodes 是允许小写出现的常见代码（hk-01、jp 02）；其余代码只认大写，避免 in、is、no 这类英文单词。
var lowerCodes = map[string]bool{"HK": true, "TW": true, "JP": true, "SG": true, "US": true, "KR": true, "UK": true}

// LoadDetector 从数据目录（catalog/regions/regions.yaml）加载识别器。
func LoadDetector(fsys fs.FS, path string) (*Detector, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	var f regionFile
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("node: 解析 %s: %w", path, err)
	}
	d := &Detector{arrows: f.Arrows, codes: map[string]bool{}}
	add := func(t term) {
		t.runes = utf8.RuneCountInString(t.text)
		d.terms = append(d.terms, t)
	}
	shorts := map[string]string{}
	for _, r := range f.Regions {
		if len(r.Code) != 2 || d.codes[r.Code] {
			return nil, fmt.Errorf("node: 地区代码 %q 无效或重复", r.Code)
		}
		d.codes[r.Code] = true
		for _, z := range r.ZH {
			add(term{text: z, code: r.Code, kind: kindZH})
		}
		for _, c := range r.Cities {
			if isASCII(c) {
				add(term{text: c, code: r.Code, kind: kindEN, fold: true, boundary: true})
			} else {
				add(term{text: c, code: r.Code, kind: kindZH})
			}
		}
		for _, e := range r.EN {
			add(term{text: e, code: r.Code, kind: kindEN, fold: true, boundary: true})
		}
		add(term{text: r.Code, code: r.Code, kind: kindCode, boundary: true, fold: lowerCodes[r.Code]})
		if r.ISO3 != "" {
			add(term{text: r.ISO3, code: r.Code, kind: kindCode, boundary: true})
		}
		for _, c := range r.Codes {
			add(term{text: c, code: r.Code, kind: kindCode, boundary: true, fold: lowerCodes[c]})
		}
		if r.Short != "" {
			add(term{text: r.Short, code: r.Code, kind: kindShort})
			shorts[r.Short] = r.Code
		}
	}
	for _, c := range f.RouteCities {
		for s, code := range shorts {
			add(term{text: c + s, code: code, kind: kindRoute})
		}
	}
	for _, w := range f.RouteWords {
		add(term{text: w, kind: kindRoute, fold: isASCII(w), boundary: isASCII(w)})
	}
	// 固定顺序：长度降序、优先级降序、文本——保证结果与 map 迭代顺序无关。
	sort.SliceStable(d.terms, func(i, j int) bool {
		a, b := d.terms[i], d.terms[j]
		if a.runes != b.runes {
			return a.runes > b.runes
		}
		if a.kind != b.kind {
			return a.kind > b.kind
		}
		return a.text < b.text
	})
	return d, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Known 报告代码是否在地区表中。
func (d *Detector) Known(code string) bool { return d.codes[code] }

type span struct {
	start, end int // 字节区间
	t          term
}

// Detect 识别名称的落地地区。
func (d *Detector) Detect(name string) Region {
	if code, ok := FlagCode(name); ok {
		return Region{Code: code, Kind: RegionFlag}
	}
	// 方向符号：从最后一个起向前尝试，取第一个「之后有地区线索」的切分点。
	// 「转」也可能是线路词「中转」的一部分，因此切分后没有线索时继续向前，而不是直接放弃。
	var cuts []int
	for _, a := range d.arrows {
		for from := 0; ; {
			i := strings.Index(name[from:], a)
			if i < 0 {
				break
			}
			cuts = append(cuts, from+i+len(a))
			from += i + len(a)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(cuts)))
	for _, c := range cuts {
		if r := d.scan(name[c:]); r.Code != "" && r.Kind == RegionKeyword {
			return r
		}
	}
	return d.scan(name)
}

func (d *Detector) scan(text string) Region {
	lower := strings.ToLower(text)
	var taken []span
	overlaps := func(s, e int) bool {
		for _, x := range taken {
			if s < x.end && x.start < e {
				return true
			}
		}
		return false
	}
	for _, t := range d.terms {
		hay, needle := text, t.text
		if t.fold {
			hay, needle = lower, strings.ToLower(t.text)
		}
		for from := 0; from < len(hay); {
			i := strings.Index(hay[from:], needle)
			if i < 0 {
				break
			}
			s := from + i
			e := s + len(needle)
			from = s + 1
			if t.boundary && !boundaryOK(text, s, e) {
				continue
			}
			if overlaps(s, e) {
				continue
			}
			taken = append(taken, span{start: s, end: e, t: t})
		}
	}
	sort.Slice(taken, func(i, j int) bool { return taken[i].start < taken[j].start })
	var res Region
	var cands []span
	var lastRoute *span
	for i := range taken {
		x := taken[i]
		if x.t.kind == kindRoute {
			res.Ignored = append(res.Ignored, text[x.start:x.end])
			if x.t.code != "" {
				lastRoute = &taken[i]
			}
			continue
		}
		cands = append(cands, x)
	}
	switch {
	case len(cands) > 0:
		last := cands[len(cands)-1]
		res.Code, res.Kind, res.Match = last.t.code, RegionKeyword, text[last.start:last.end]
		for _, c := range cands {
			if c.t.code != last.t.code {
				res.Inferred = true
			}
		}
	case lastRoute != nil:
		res.Code, res.Kind, res.Match, res.Inferred = lastRoute.t.code, RegionRoute, text[lastRoute.start:lastRoute.end], true
	default:
		res.Kind = RegionUnknown
	}
	return res
}

// boundaryOK 要求匹配的前后都不是 ASCII 字母（数字、空格、符号与非 ASCII 字符都算边界）。
func boundaryOK(s string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if r < 0x80 && unicode.IsLetter(r) {
			return false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if r < 0x80 && unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// —— 国旗 ——

const regionalA = 0x1F1E6 // REGIONAL INDICATOR SYMBOL LETTER A

func isRegional(r rune) bool { return r >= regionalA && r < regionalA+26 }

// Flag 返回地区代码对应的国旗 emoji；代码无效时返回空串。
func Flag(code string) string {
	if len(code) != 2 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < 2; i++ {
		c := code[i]
		if c < 'A' || c > 'Z' {
			return ""
		}
		b.WriteRune(rune(regionalA + int(c-'A')))
	}
	return b.String()
}

// FlagCode 返回名称中第一个国旗 emoji 对应的代码。
func FlagCode(name string) (string, bool) {
	rs := []rune(name)
	for i := 0; i+1 < len(rs); i++ {
		if isRegional(rs[i]) && isRegional(rs[i+1]) {
			return string([]rune{'A' + rs[i] - regionalA, 'A' + rs[i+1] - regionalA}), true
		}
	}
	return "", false
}

// StripFlags 去掉名称中的全部国旗 emoji 及其后紧跟的空白。
func StripFlags(name string) string {
	rs := []rune(name)
	var out []rune
	for i := 0; i < len(rs); i++ {
		if i+1 < len(rs) && isRegional(rs[i]) && isRegional(rs[i+1]) {
			i++
			for i+1 < len(rs) && rs[i+1] == ' ' {
				i++
			}
			continue
		}
		out = append(out, rs[i])
	}
	return strings.TrimSpace(string(out))
}

// FlagMode 是国旗的写法。
type FlagMode string

const (
	FlagPrefix  FlagMode = "prefix"  // 默认：🇸🇬 原名称；名称已有国旗时不重复添加
	FlagReplace FlagMode = "replace" // 替换名称中已有的国旗
	FlagOff     FlagMode = "off"
)

// Decor 是装饰 emoji 的选择器。
type Decor struct{ list []string }

// LoadDecor 加载装饰 emoji 表；allowed 非空时只保留其中的条目（用户偏好）。
func LoadDecor(fsys fs.FS, path string, allowed []string) (*Decor, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Emoji []string `yaml:"emoji"`
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	list := f.Emoji
	if len(allowed) > 0 {
		list = slices.DeleteFunc(slices.Clone(list), func(e string) bool { return !slices.Contains(allowed, e) })
	}
	for _, e := range list {
		if _, ok := FlagCode(e); ok {
			return nil, fmt.Errorf("node: 装饰 emoji 不能是国旗：%s", e)
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("node: 装饰 emoji 表为空")
	}
	return &Decor{list: list}, nil
}

// Pick 按指纹的哈希稳定地挑选 emoji；与 avoid 相同时顺延一个，使相邻节点尽量不重复。
func (d *Decor) Pick(fingerprint, avoid string) string {
	var h uint32 = 2166136261 // FNV-1a
	for i := 0; i < len(fingerprint); i++ {
		h ^= uint32(fingerprint[i])
		h *= 16777619
	}
	i := int(h % uint32(len(d.list))) //nolint:gosec // 表长只有十几项，远小于 2^32
	if d.list[i] == avoid && len(d.list) > 1 {
		i = (i + 1) % len(d.list)
	}
	return d.list[i]
}

// List 返回可用的装饰 emoji。
func (d *Decor) List() []string { return slices.Clone(d.list) }
