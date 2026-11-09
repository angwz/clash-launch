package ruleset

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/node"
)

// Selector 是 [custom_groups.select] 表：按节点的属性选择节点（第 6.4 节）。
// 同一个键内的多个值是「或」，不同键之间是「且」；exclude 中任一条件命中即排除。
type Selector struct {
	Region   []string
	Source   []string // 源名称，或 #编号（即 GROUPID）
	Protocol []string
	Tags     []string
	Name     *regexp2.Regexp
	NameText string
	RateMin  *float64
	RateMax  *float64
	Exclude  *Selector
	Sort     string // source（默认）| name | rate
}

var selectorKeys = []string{"region", "source", "protocol", "tags", "name", "rate", "exclude", "sort"}

func parseSelector(m map[string]any, group, where string, out *diag.List) *Selector {
	s, ok := selectorFrom(m, group, where, out, true)
	if !ok {
		return nil
	}
	return s
}

func selectorFrom(m map[string]any, group, where string, out *diag.List, top bool) (*Selector, bool) {
	s := &Selector{}
	bad := func(reason string) (*Selector, bool) {
		out.Add(diag.SelectInvalid, where, "name", group, "reason", reason)
		return nil, false
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		if !slices.Contains(selectorKeys, k) || (!top && (k == "exclude" || k == "sort" || k == "rate")) {
			return bad("unknown key " + k)
		}
		switch k {
		case "region", "source", "protocol", "tags":
			list, ok := stringList(v)
			if !ok {
				if str, isStr := v.(string); isStr {
					list, ok = []string{str}, true
				}
			}
			if !ok {
				return bad(k + " must be a string or an array of strings")
			}
			switch k {
			case "region":
				for i := range list {
					list[i] = strings.ToUpper(list[i])
				}
				s.Region = list
			case "source":
				s.Source = list
			case "protocol":
				s.Protocol = list
			case "tags":
				s.Tags = list
			}
		case "name":
			str, ok := v.(string)
			if !ok {
				return bad("name must be a regular expression string")
			}
			re, err := node.Compile(str)
			if err != nil {
				return bad("name: " + err.Error())
			}
			s.Name, s.NameText = re, str
		case "rate":
			r, ok := v.(map[string]any)
			if !ok {
				return bad("rate must be a table such as { max = 1.5 }")
			}
			for rk, rv := range r {
				f, ok := number(rv)
				if !ok || (rk != "min" && rk != "max") {
					return bad("rate accepts numeric min and max")
				}
				if rk == "min" {
					s.RateMin = &f
				} else {
					s.RateMax = &f
				}
			}
		case "exclude":
			em, ok := v.(map[string]any)
			if !ok {
				return bad("exclude must be a table")
			}
			ex, ok := selectorFrom(em, group, where, out, false)
			if !ok {
				return nil, false
			}
			s.Exclude = ex
		case "sort":
			str, _ := v.(string)
			if str != "source" && str != "name" && str != "rate" {
				return bad("sort must be source, name or rate")
			}
			s.Sort = str
		}
	}
	return s, true
}

func number(v any) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// sourceMatch 判断 source 条件：源名称精确匹配，或 #N 匹配订阅编号。
func sourceMatch(want []string, n *node.Node) bool {
	for _, w := range want {
		if strings.HasPrefix(w, "#") {
			if id, err := strconv.Atoi(w[1:]); err == nil && n.GroupID >= 0 && n.GroupID == id {
				return true
			}
			continue
		}
		if w == n.SourceName {
			return true
		}
	}
	return false
}

// matches 判断节点是否满足全部条件（不含 exclude）。
func (s *Selector) matches(n *node.Node, where string, out *diag.List) bool {
	if len(s.Region) > 0 && !slices.Contains(s.Region, n.Region.Code) {
		return false
	}
	if len(s.Source) > 0 && !sourceMatch(s.Source, n) {
		return false
	}
	if len(s.Protocol) > 0 && !slices.Contains(s.Protocol, n.Type) {
		return false
	}
	if len(s.Tags) > 0 && !slices.ContainsFunc(s.Tags, func(t string) bool { return slices.Contains(n.Tags, t) }) {
		return false
	}
	if s.Name != nil && !node.Match(s.Name, n.Name, where, out) {
		return false
	}
	if s.RateMin != nil && n.Rate < *s.RateMin {
		return false
	}
	if s.RateMax != nil && n.Rate > *s.RateMax {
		return false
	}
	return true
}

// excluded 判断 exclude 中是否有任一条件命中。
func (s *Selector) excluded(n *node.Node, where string, out *diag.List) bool {
	e := s.Exclude
	if e == nil {
		return false
	}
	return (len(e.Region) > 0 && slices.Contains(e.Region, n.Region.Code)) ||
		(len(e.Source) > 0 && sourceMatch(e.Source, n)) ||
		(len(e.Protocol) > 0 && slices.Contains(e.Protocol, n.Type)) ||
		(len(e.Tags) > 0 && slices.ContainsFunc(e.Tags, func(t string) bool { return slices.Contains(n.Tags, t) })) ||
		(e.Name != nil && node.Match(e.Name, n.Name, where, out))
}

// Pick 返回满足选择器的节点，按 sort 排列（source 即保持源顺序）。
func (s *Selector) Pick(nodes []*node.Node, where string, out *diag.List) []*node.Node {
	var res []*node.Node
	for _, n := range nodes {
		if s.matches(n, where, out) && !s.excluded(n, where, out) {
			res = append(res, n)
		}
	}
	switch s.Sort {
	case "name":
		sort.SliceStable(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	case "rate":
		sort.SliceStable(res, func(i, j int) bool { return res[i].Rate < res[j].Rate })
	}
	return res
}

// String 用于诊断。
func (s *Selector) String() string {
	var parts []string
	add := func(k string, v []string) {
		if len(v) > 0 {
			parts = append(parts, fmt.Sprintf("%s=%s", k, strings.Join(v, "|")))
		}
	}
	add("region", s.Region)
	add("source", s.Source)
	add("protocol", s.Protocol)
	add("tags", s.Tags)
	if s.NameText != "" {
		parts = append(parts, "name="+s.NameText)
	}
	return strings.Join(parts, " ")
}
