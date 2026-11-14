package api

import (
	"encoding/json"
	"net/http"
	"unicode/utf8"
)

// prefsDTO 是用户的界面偏好（反馈 #37）。只接受已知的键，未知键与超限的值拒绝而不是丢弃。
//   - nav：主菜单项的顺序（路由 ID）；
//   - sections：配置编辑器中内核设置分节的顺序（目录分节 ID）；
//   - fieldClick：字段卡片说明区域是否可点击展开（反馈 #50）。
//
// 列表只表示顺序：界面按它排序，未列出的项保持默认顺序排在后面，因此新增菜单项不需要迁移偏好。
type prefsDTO struct {
	Nav      []string `json:"nav,omitempty"`
	Sections []string `json:"sections,omitempty"`
	// FieldClick：点击字段卡片左侧说明区域展开「说明与建议」；省略表示默认（开）。
	FieldClick *bool `json:"fieldClick,omitempty"`
}

const (
	prefsMaxItems = 64
	prefsMaxID    = 64
)

func (p prefsDTO) validate() error {
	for _, list := range [][]string{p.Nav, p.Sections} {
		if len(list) > prefsMaxItems {
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
