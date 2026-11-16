package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/angwz/clash-launch/internal/diag"
	"github.com/angwz/clash-launch/internal/match"
	"github.com/angwz/clash-launch/internal/profile"
	"github.com/angwz/clash-launch/internal/ruleset"
	"github.com/angwz/clash-launch/internal/store"
)

// 上游同步与期望（docs/design/routing.md 第 12 节，决定 0007 第 8 条）。
// 自己管理分流后，规则源的更新不再自动生效：这里按开始管理时保存的 base 做三方比对，逐条交给用户采纳或忽略。

// UpstreamChange 是上游相对 base 的一处变化，以及它与用户自己的改动（mine）的关系。
type UpstreamChange struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`            // group | ruleset
	Op       string `json:"op"`              // add | remove | modify | move
	Key      string `json:"key"`             // 组名，或规则集地址（内联规则为原文）
	Field    string `json:"field,omitempty"` // modify 时变化的字段
	Base     any    `json:"base,omitempty"`
	Upstream any    `json:"upstream,omitempty"`
	Mine     any    `json:"mine,omitempty"`
	// Conflict：用户也改过同一处（三者两两不同），默认保留自己的
	Conflict bool `json:"conflict"`
}

// UpstreamDTO 是 /routing/upstream 的响应：变化列表与上游当前的规范化内容（前端据此采纳）。
type UpstreamDTO struct {
	Changes  []UpstreamChange `json:"changes"`
	Digest   string           `json:"digest"`
	Groups   []map[string]any `json:"groups"`
	Rulesets []map[string]any `json:"rulesets"`
}

// upstreamRouting 把规则源的当前内容转换为 routing 的规范化形式（与 Fork 相同的路径），并给出摘要。
func (s *Service) upstreamRouting(ctx context.Context, owner, ref string) (groups, rulesets []map[string]any, digest string, err error) {
	ri, err := s.rulesInput(ctx, owner, ref)
	if err != nil {
		return nil, nil, "", err
	}
	var l diag.List
	var src *ruleset.Source
	if len(bytes.TrimSpace(ri.INI)) > 0 {
		src = ruleset.ParseINI(ri.INI, ri.Name+".ini", &l)
	} else {
		src = &ruleset.Source{Groups: ruleset.ParseGroups(ri.Groups, "groups.toml", &l), Rulesets: ruleset.ParseRulesets(ri.Rulesets, "rulesets.toml", &l)}
	}
	ruleset.DetectBundles(src.Rulesets)
	g, r := ruleset.ToRouting(src)
	if err := g.Decode(&groups); err != nil {
		return nil, nil, "", fmt.Errorf("解析上游组: %w", err)
	}
	if err := r.Decode(&rulesets); err != nil {
		return nil, nil, "", fmt.Errorf("解析上游规则集: %w", err)
	}
	return canonList(groups), canonList(rulesets), ruleset.Digest(g, r), nil
}

// canonList 经 JSON 往返得到可比较的形式（YAML 与 TOML 的整数类型、映射类型统一）。
func canonList(in []map[string]any) []map[string]any {
	b, _ := json.Marshal(in)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = []map[string]any{}
	}
	return out
}

// RoutingUpstream 计算上游相对开始管理时（base）的变化，以及每处变化与用户改动的关系。只读。
func (s *Service) RoutingUpstream(ctx context.Context, user *store.User, doc []byte) (*UpstreamDTO, error) {
	var l diag.List
	p, err := profile.Parse(doc, &l)
	if err != nil {
		return nil, fmt.Errorf("%w：%w", ErrBadDraft, err)
	}
	if p.Routing == nil || p.Rules == nil {
		return nil, ErrNoRules
	}
	ug, ur, digest, err := s.upstreamRouting(ctx, user.ID, p.Rules.Ref)
	if err != nil {
		return nil, err
	}
	var bg, br []map[string]any
	if p.Routing.Base != nil {
		bg, br = canonList(p.Routing.Base.Groups), canonList(p.Routing.Base.Rulesets)
	}
	mg, mr := canonList(p.Routing.Groups), canonList(p.Routing.Rulesets)
	out := &UpstreamDTO{Digest: digest, Groups: ug, Rulesets: ur, Changes: []UpstreamChange{}}
	if digest == p.Routing.From.Digest && p.Routing.Base != nil {
		return out, nil
	}
	out.Changes = append(diff3("group", "name", bg, ug, mg), diff3("ruleset", "ruleset", br, ur, mr)...)
	out.Changes = append(out.Changes, moves(br, ur, mr)...)
	return out, nil
}

// diff3 比较 base、upstream、mine 三份列表（按 key 字段对应），给出上游的增删与字段级修改。
func diff3(kind, keyField string, base, up, mine []map[string]any) []UpstreamChange {
	index := func(l []map[string]any) (map[string]map[string]any, []string) {
		m := map[string]map[string]any{}
		var order []string
		for _, x := range l {
			k, _ := x[keyField].(string)
			if _, dup := m[k]; !dup {
				order = append(order, k)
			}
			m[k] = x
		}
		return m, order
	}
	b, bo := index(base)
	u, uo := index(up)
	m, _ := index(mine)
	keys := append([]string{}, uo...) // 按上游的顺序报告，上游删除的排在后面
	for _, k := range bo {
		if _, ok := u[k]; !ok {
			keys = append(keys, k)
		}
	}
	var out []UpstreamChange
	add := func(c UpstreamChange) {
		c.Kind = kind
		c.ID = kind + "|" + c.Op + "|" + c.Key + "|" + c.Field
		out = append(out, c)
	}
	for _, k := range keys {
		bx, inB := b[k]
		ux, inU := u[k]
		mx, inM := m[k]
		switch {
		case !inB && inU: // 上游新增
			if inM && reflect.DeepEqual(mx, ux) {
				continue
			}
			add(UpstreamChange{Op: "add", Key: k, Upstream: ux, Mine: nilIfAbsent(mx, inM), Conflict: inM})
		case inB && !inU: // 上游删除
			if !inM {
				continue
			}
			add(UpstreamChange{Op: "remove", Key: k, Base: bx, Mine: mx, Conflict: !reflect.DeepEqual(mx, bx)})
		case inB && inU && !reflect.DeepEqual(bx, ux): // 上游修改：逐字段
			fields := map[string]bool{}
			for f := range bx {
				fields[f] = true
			}
			for f := range ux {
				fields[f] = true
			}
			names := make([]string, 0, len(fields))
			for f := range fields {
				names = append(names, f)
			}
			sort.Strings(names)
			for _, f := range names {
				bv, uv := bx[f], ux[f]
				if reflect.DeepEqual(bv, uv) {
					continue
				}
				var mv any
				if inM {
					mv = mx[f]
				}
				if inM && reflect.DeepEqual(mv, uv) {
					continue // 用户已经改成一样的
				}
				add(UpstreamChange{Op: "modify", Key: k, Field: f, Base: bv, Upstream: uv, Mine: mv, Conflict: !inM || !reflect.DeepEqual(mv, bv)})
			}
		}
	}
	return out
}

func nilIfAbsent(v map[string]any, ok bool) any {
	if !ok {
		return nil
	}
	return v
}

// moves 报告上游调整了顺序的规则集（以 base 与 upstream 共有条目的相对顺序比较，最长递增子序列之外的视为移动）。
// 用户也移动过同一条时标为冲突。
func moves(base, up, mine []map[string]any) []UpstreamChange {
	key := func(x map[string]any) string { k, _ := x["ruleset"].(string); return k }
	pos := func(l []map[string]any) map[string]int {
		m := map[string]int{}
		for i, x := range l {
			m[key(x)] = i
		}
		return m
	}
	bp, mp := pos(base), pos(mine)
	var common []string
	for _, x := range up {
		if _, ok := bp[key(x)]; ok {
			common = append(common, key(x))
		}
	}
	seq := make([]int, len(common))
	for i, k := range common {
		seq[i] = bp[k]
	}
	keep := lis(seq)
	// 用户自己的相对顺序是否与 base 相同（同样只看共有条目）
	var mineSeq []int
	var mineKeys []string
	for _, x := range mine {
		if _, ok := bp[key(x)]; ok {
			mineSeq = append(mineSeq, bp[key(x)])
			mineKeys = append(mineKeys, key(x))
		}
	}
	mineKeep := lis(mineSeq)
	mineMoved := map[string]bool{}
	for i, k := range mineKeys {
		if !mineKeep[i] {
			mineMoved[k] = true
		}
	}
	var out []UpstreamChange
	for i, k := range common {
		if keep[i] {
			continue
		}
		if _, inMine := mp[k]; !inMine {
			continue
		}
		out = append(out, UpstreamChange{ID: "ruleset|move|" + k + "|", Kind: "ruleset", Op: "move", Key: k, Conflict: mineMoved[k]})
	}
	return out
}

// lis 返回最长递增子序列所含的下标。
func lis(seq []int) map[int]bool {
	tails := []int{}
	prev := make([]int, len(seq))
	for i, v := range seq {
		j := sort.Search(len(tails), func(k int) bool { return seq[tails[k]] >= v })
		if j > 0 {
			prev[i] = tails[j-1]
		} else {
			prev[i] = -1
		}
		if j == len(tails) {
			tails = append(tails, i)
		} else {
			tails[j] = i
		}
	}
	keep := map[int]bool{}
	if len(tails) == 0 {
		return keep
	}
	for k := tails[len(tails)-1]; k >= 0; k = prev[k] {
		keep[k] = true
	}
	return keep
}

// —— 期望 ——

// CheckDTO 是一条期望的结果：追踪 host 时路径上（命中的组、沿默认路径的各层组、出口）是否出现 via。
type CheckDTO struct {
	Host string `json:"host"`
	Via  string `json:"via"`
	// OK 为 nil 表示无法判断（规则内容尚未缓存，或命中之前有无法判断的行）
	OK   *bool    `json:"ok,omitempty"`
	Path []string `json:"path"`
}

// routingChecks 用已缓存的规则内容执行期望（不为此等待拉取），并产生诊断。
func (s *Service) routingChecks(in *routingInput, sets map[string]*match.Set, stale map[string]bool, out *diag.List) []CheckDTO {
	if in.prof == nil || in.prof.Routing == nil || len(in.prof.Routing.Checks) == 0 {
		return nil
	}
	names := map[string]bool{}
	for _, g := range in.cfg.ProxyGroups {
		names[g.Name] = true
	}
	for n := range in.proxies {
		names[n] = true
	}
	for _, b := range builtins {
		names[b] = true
	}
	var res []CheckDTO
	for i, c := range in.prof.Routing.Checks {
		where := fmt.Sprintf("routing.checks#%d", i+1)
		dto := CheckDTO{Host: c.Host, Via: c.Via, Path: []string{}}
		if !names[c.Via] {
			out.Add(diag.RoutingCheckUnknown, where, "host", c.Host, "via", c.Via)
			res = append(res, dto)
			continue
		}
		req, err := match.NewRequest(c.Host, "", "")
		if err != nil || s.Trace.Disabled {
			res = append(res, dto)
			continue
		}
		t := in.trace(req, sets, stale)
		uncertain := t.Hit < 0
		for _, st := range t.Steps {
			if st.Result == match.ResultUnknown && st.Reason == match.ReasonNoContent {
				uncertain = true
			}
		}
		if t.Hit >= 0 {
			dto.Path = append(dto.Path, in.lines[t.Hit].Target)
		}
		for _, cs := range t.Chain {
			dto.Path = append(dto.Path, cs.Pick)
		}
		dto.Path = slices.Compact(dto.Path)
		if !uncertain {
			ok := slices.Contains(dto.Path, c.Via) || t.Exit.Name == c.Via
			dto.OK = &ok
			if !ok {
				got := t.Exit.Name
				if len(dto.Path) > 0 {
					got = dto.Path[0]
				}
				out.Add(diag.RoutingCheckFailed, where, "host", c.Host, "via", c.Via, "got", got)
			}
		}
		res = append(res, dto)
	}
	return res
}
