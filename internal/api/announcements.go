package api

import (
	"net/http"
	"time"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 站内公告（需求 R32）。

type announcementDTO struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Level     string     `json:"level"`
	StartsAt  time.Time  `json:"startsAt"`
	EndsAt    *time.Time `json:"endsAt,omitempty"`
	CreatedBy string     `json:"createdBy,omitempty"` // 只在管理页返回
	CreatedAt time.Time  `json:"createdAt"`
}

func toAnnouncement(a store.Announcement, admin bool) announcementDTO {
	d := announcementDTO{ID: a.ID, Text: a.Text, Level: a.Level, StartsAt: a.StartsAt, EndsAt: tp(a.EndsAt), CreatedAt: a.CreatedAt}
	if admin {
		d.CreatedBy = a.CreatedBy
	}
	return d
}

type announcementReq struct {
	Text     string     `json:"text"`
	Level    string     `json:"level"`
	StartsAt *time.Time `json:"startsAt"`
	EndsAt   *time.Time `json:"endsAt"`
}

func (q announcementReq) input() service.AnnouncementInput {
	in := service.AnnouncementInput{Text: q.Text, Level: q.Level}
	if q.StartsAt != nil {
		in.StartsAt = *q.StartsAt
	}
	if q.EndsAt != nil {
		in.EndsAt = *q.EndsAt
	}
	return in
}

// activeAnnouncements 返回此刻生效、本人没有关闭的公告。
func (s *Server) activeAnnouncements(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.Svc.Store.Announcements().Active(r.Context(), p.User.ID, time.Now())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]announcementDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAnnouncement(a, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) dismissAnnouncement(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Announcements().Dismiss(r.Context(), p.User.ID, r.PathValue("id"), time.Now()); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminAnnouncements(w http.ResponseWriter, r *http.Request, _ *principal) {
	list, err := s.Svc.Store.Announcements().All(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]announcementDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAnnouncement(a, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request, p *principal) {
	var req announcementReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	a, err := s.Svc.CreateAnnouncement(r.Context(), p.User, req.input())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAnnouncement(*a, true))
}

func (s *Server) updateAnnouncement(w http.ResponseWriter, r *http.Request, p *principal) {
	var req announcementReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	a, err := s.Svc.UpdateAnnouncement(r.Context(), p.User, r.PathValue("id"), req.input())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAnnouncement(*a, true))
}

func (s *Server) deleteAnnouncement(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteAnnouncement(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
