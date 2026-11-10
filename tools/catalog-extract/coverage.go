package main

import (
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/angwz/clash-launch/internal/catalog"
)

// 覆盖状态：scan 得到的每条路径在目录中的解析结果。
const (
	covCovered     = "covered"     // 目录中有对应字段（类型化，可在表单中配置）
	covFree        = "free"        // 源码本身就是 any / map[string]any，目录以 any 收录是如实反映
	covPassthrough = "passthrough" // 目录在这条路径的某个祖先处以 any 透传：能输出，但表单无法逐项配置
	covMissing     = "missing"     // 目录中没有
	covSkipped     = "skipped"     // 有意不收录（见 skipped）
)

// skipped 是有意不收录的路径及原因。只登记查证过的情况：结构体中声明了却没有任何代码读取的字段。
var skipped = map[string]string{
	"proxies.vless.ws-headers": "v1.19.33 中声明于 VlessOption，但没有代码读取；请求头写在 ws-opts.headers",
}

// Coverage 是 coverage 子命令的输出。
type Coverage struct {
	Version  string           `json:"version"`
	Total    int              `json:"total"`
	Counts   map[string]int   `json:"counts"`
	Sections []SectionCount   `json:"sections"`
	Gaps     map[string][]Gap `json:"gaps"` // passthrough 与 missing 的路径
}

// SectionCount 是按顶层键（proxies、listeners 再细分到类型）统计的覆盖情况。
type SectionCount struct {
	Section string         `json:"section"`
	Counts  map[string]int `json:"counts"`
}

// Gap 是一条未类型化收录的路径。
type Gap struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Source string `json:"source"`
}

// coverage 把 scan 结果逐条放到目录中解析。路径的分段规则与 scan 一致：
// 列表与 map 的元素层不占路径段，按 type 分派的位置（proxies、listeners、proxy-groups）用判别值占一段。
func coverage(res Result, cat *catalog.Catalog) Coverage {
	cv := Coverage{Version: res.Version, Counts: map[string]int{}, Gaps: map[string][]Gap{}}
	bySection := map[string]map[string]int{}
	for _, f := range res.Fields {
		st := classify(cat.Root, strings.Split(f.Path, "."), f.Type)
		if _, ok := skipped[f.Path]; ok && st == covMissing {
			st = covSkipped
		}
		cv.Total++
		cv.Counts[st]++
		sec := section(f.Path)
		if bySection[sec] == nil {
			bySection[sec] = map[string]int{}
		}
		bySection[sec][st]++
		if st == covPassthrough || st == covMissing {
			cv.Gaps[st] = append(cv.Gaps[st], Gap{Path: f.Path, Type: f.Type, Source: f.Source})
		}
	}
	for s, c := range bySection {
		cv.Sections = append(cv.Sections, SectionCount{Section: s, Counts: c})
	}
	sort.Slice(cv.Sections, func(i, j int) bool { return cv.Sections[i].Section < cv.Sections[j].Section })
	return cv
}

// section 返回统计用的分节名：proxies 与 listeners 细分到协议类型。
func section(p string) string {
	seg := strings.SplitN(p, ".", 3)
	if len(seg) >= 2 && (seg[0] == "proxies" || seg[0] == "listeners") {
		return seg[0] + "." + seg[1]
	}
	return seg[0]
}

func classify(f *catalog.Field, segs []string, scanType string) string {
	for {
		f = unwrap(f)
		if f == nil {
			return covMissing
		}
		if f.Type == catalog.TAny {
			if len(segs) == 0 && strings.Contains(scanType, "any") {
				return covFree
			}
			return covPassthrough
		}
		if len(segs) == 0 {
			return covCovered
		}
		seg := segs[0]
		switch f.Type {
		case catalog.TOneOf:
			if v := f.Variant(seg); v != nil {
				// 判别值本身占一段：proxies.vmess 对应 vmess 分支。
				segs = segs[1:]
				if len(segs) == 0 {
					return covCovered
				}
				f = f.Child(segs[0], discriminated(f.Discriminator, seg))
				segs = segs[1:]
				continue
			}
			// 源码按公共结构体扫描（如 provider schema）：在任一分支中出现即可。
			f = anyVariantChild(f, seg)
		case catalog.TAnyOf:
			f = anyOfChild(f, seg)
		default:
			f = f.Child(seg, nil)
		}
		segs = segs[1:]
	}
}

// unwrap 穿过列表与 map 的元素层，以及只含一个容器候选的 anyOf（如「字符串或列表」）。
func unwrap(f *catalog.Field) *catalog.Field {
	for f != nil {
		switch f.Type {
		case catalog.TList:
			f = f.Items
		case catalog.TMap:
			f = f.Values
		default:
			return f
		}
	}
	return nil
}

func discriminated(key, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key}, {Kind: yaml.ScalarNode, Value: value},
	}}
}

func anyVariantChild(f *catalog.Field, key string) *catalog.Field {
	for _, v := range f.Variants {
		if ch := f.Child(key, discriminated(f.Discriminator, v.Value)); ch != nil {
			return ch
		}
	}
	return nil
}

// anyOfChild 在 anyOf 的候选中寻找能容纳 key 的对象（或列表/map 中的对象）。
func anyOfChild(f *catalog.Field, key string) *catalog.Field {
	for _, o := range f.Of {
		o = unwrap(o)
		if o == nil {
			continue
		}
		switch o.Type {
		case catalog.TObject:
			if ch := o.Child(key, nil); ch != nil {
				return ch
			}
		case catalog.TOneOf:
			if ch := anyVariantChild(o, key); ch != nil {
				return ch
			}
		case catalog.TAny:
			return o
		}
	}
	return nil
}
