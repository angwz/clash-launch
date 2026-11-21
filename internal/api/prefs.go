package api

import (
	"encoding/json"
	"net/http"
	"unicode/utf8"
)

// prefsDTO 是用户的界面偏好（反馈 #37）。只接受已知的键，未知键与超限的值拒绝而不是丢弃。
//   - nav：主菜单项的顺序（路由 ID）；
//   - sections：配置编辑器中内核设置分节的顺序（目录分节 ID）；
//   - fieldClick：「说明与建议」的点击范围（反馈 #50、#55）；
//   - rules：分流规则（预设与自己的规则源）的顺序（反馈 #73）。
//
// 列表只表示顺序：界面按它排序，未列出的项保持默认顺序排在后面，因此新增菜单项不需要迁移偏好。
type prefsDTO struct {
	Nav      []string `json:"nav,omitempty"`
	Sections []string `json:"sections,omitempty"`
	// FieldClick：「说明与建议」的点击范围 button | text | card；省略表示默认（card）。
	FieldClick fieldClick `json:"fieldClick,omitempty"`
	// AutoCheck：自动保存后自动做内核检查；省略表示默认（开）。
	AutoCheck *bool `json:"autoCheck,omitempty"`
	// Rules：分流规则页的顺序（反馈 #73），预设 ID 与用户规则源 ID 混排。
	Rules []string `json:"rules,omitempty"`
	// 站内消息（反馈 #74）：Inbox 为 off 时不接收；AutoAdd 收到的节点自动加入节点页（默认开）；Blocked 屏蔽的用户 ID。
	Inbox   string   `json:"inbox,omitempty"`
	AutoAdd *bool    `json:"autoAdd,omitempty"`
	Blocked []string `json:"blocked,omitempty"`
	// RoutingView：分流地图的显示偏好（routing 第 9 节）；省略的字段表示默认。
	RoutingView *routingView `json:"routingView,omitempty"`
	// Motion：off 关闭界面动效（R11），与系统的「减少动态效果」效果相同；省略表示跟随系统。
	Motion string `json:"motion,omitempty"`
}

// fieldClick 兼容早期保存的布尔值（反馈 #50 的第一版）：false 读作 button，true 读作默认。
type fieldClick string

func (f *fieldClick) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "false":
		*f = "button"
		return nil
	case "true", "null":
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*f = fieldClick(s)
	return nil
}

const (
	prefsMaxItems = 64
	prefsMaxRules = 512 // 规则源不计入数量配额，上限放宽
	prefsMaxID    = 64
)

func (p prefsDTO) validate() error {
	switch p.FieldClick {
	case "", "button", "text", "card":
	default:
		return badRequest("fieldClick must be button, text or card")
	}
	if err := p.RoutingView.validate(); err != nil {
		return err
	}
	if p.Motion != "" && p.Motion != "off" {
		return badRequest("motion must be empty or off")
	}
	if p.Inbox != "" && p.Inbox != "off" {
		return badRequest("inbox must be empty or off")
	}
	for i, list := range [][]string{p.Nav, p.Sections, p.Rules, p.Blocked} {
		limit := prefsMaxItems
		if i >= 2 {
			limit = prefsMaxRules
		}
		if len(list) > limit {
			return badRequest("too many items")
		}
		seen := map[string]bool{}
		for _, id := range list {
			if id == "" || utf8.RuneCountInString(id) > prefsMaxID || seen[id] {
				return badRequest("invalid or duplicate item: %q", id)
			}
			seen[id] = true
		}
	}
	return nil
}

func (s *Server) getPrefs(w http.ResponseWriter, r *http.Request, p *principal) {
	b, err := s.Svc.Store.Users().Prefs(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	var out prefsDTO
	if err := json.Unmarshal(b, &out); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) putPrefs(w http.ResponseWriter, r *http.Request, p *principal) {
	var req prefsDTO
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := req.validate(); err != nil {
		s.fail(w, err)
		return
	}
	b, err := json.Marshal(req)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Store.Users().SetPrefs(r.Context(), p.User.ID, b); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// putTimezone 保存用户的时区（反馈 #72）：界面上的时间按它显示；空字符串表示跟随浏览器。
func (s *Server) putTimezone(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Timezone string `json:"timezone"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	tz, err := s.Svc.SetTimezone(r.Context(), p.User.ID, req.Timezone)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"timezone": tz})
}

// routingView 是分流地图的显示偏好：rules auto|all（auto 时智能折叠）、particles all|focus|off、
// candidates 是否总是画出候选成员、exits auto|node|region|source（出口按什么归并）。
type routingView struct {
	Rules      string `json:"rules,omitempty"`
	Particles  string `json:"particles,omitempty"`
	Candidates bool   `json:"candidates,omitempty"`
	Exits      string `json:"exits,omitempty"`
}

func (v *routingView) validate() error {
	if v == nil {
		return nil
	}
	ok := func(x string, allowed ...string) bool {
		if x == "" {
			return true
		}
		for _, a := range allowed {
			if x == a {
				return true
			}
		}
		return false
	}
	if !ok(v.Rules, "auto", "all") || !ok(v.Particles, "all", "focus", "off") || !ok(v.Exits, "auto", "node", "region", "source") {
		return badRequest("invalid routingView")
	}
	return nil
}
