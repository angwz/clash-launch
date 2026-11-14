package api

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/angwz/clash-launch/internal/store"
)

// 配置模板（反馈 #51）：把一份配置的文档存为带名称与说明的模板；新建配置时由前端取模板文档、换上新名称后创建。
// 文档按配置文档校验，存储时与配置一样加密。

type templateDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Note      string    `json:"note"`
	Doc       string    `json:"doc"`
	Default   bool      `json:"default"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type templateReq struct {
	Name    *string `json:"name"`
	Note    *string `json:"note"`
	Doc     *string `json:"doc"`
	Default *bool   `json:"default"`
}

func toTemplate(t *store.Template) templateDTO {
	return templateDTO{ID: t.ID, Name: t.Name, Note: t.Note, Doc: string(t.Doc), Default: t.Default, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

// apply 校验并写入请求中出现的字段。
func (req templateReq) apply(t *store.Template) error {
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || utf8.RuneCountInString(n) > 64 {
			return badRequest("name must be 1-64 characters")
		}
		t.Name = n
	}
	if req.Note != nil {
		if utf8.RuneCountInString(*req.Note) > 500 {
			return badRequest("note must be at most 500 characters")
		}
		t.Note = strings.TrimSpace(*req.Note)
	}
	if req.Doc != nil {
		if _, _, err := parseDoc(*req.Doc); err != nil {
			return badRequest("invalid config document: %v", err)
		}
		t.Doc = []byte(*req.Doc)
	}
	if req.Default != nil {
		t.Default = *req.Default
	}
	return nil
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request, p *principal) {
	items, err := s.Svc.Store.Templates().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := list[templateDTO]{Items: []templateDTO{}}
	for i := range items {
		out.Items = append(out.Items, toTemplate(&items[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request, p *principal) {
	var req templateReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if req.Name == nil || req.Doc == nil {
		s.fail(w, badRequest("name and doc are required"))
		return
	}
	t := &store.Template{}
	if err := req.apply(t); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.quota(r, p, "", int64(len(t.Doc)), 0); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Store.Templates().Create(r.Context(), p.User.ID, t); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "template.create", map[string]string{"template": t.ID})
	writeJSON(w, http.StatusCreated, toTemplate(t))
}

func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request, p *principal) {
	var req templateReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.Svc.Store.Templates().Get(r.Context(), p.User.ID, r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := req.apply(t); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Store.Templates().Update(r.Context(), p.User.ID, t); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toTemplate(t))
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Templates().Delete(r.Context(), p.User.ID, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "template.delete", map[string]string{"template": r.PathValue("id")})
	w.WriteHeader(http.StatusNoContent)
}
