package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 足迹（需求 R10）：本人的时间线；管理员另附站点的足迹。

type footprintDTO struct {
	Kind       string          `json:"kind"`
	Subject    string          `json:"subject,omitempty"`
	At         time.Time       `json:"at"`
	Detail     json.RawMessage `json:"detail"`
	Backfilled bool            `json:"backfilled,omitempty"`
}

func toFootprints(fs []store.Footprint) []footprintDTO {
	out := make([]footprintDTO, 0, len(fs))
	for _, f := range fs {
		d := json.RawMessage(f.Detail)
		if len(d) == 0 || !json.Valid(d) {
			d = json.RawMessage("{}")
		}
		out = append(out, footprintDTO{Kind: f.Kind, Subject: f.Subject, At: f.At, Detail: d, Backfilled: f.Backfilled})
	}
	return out
}

func (s *Server) myFootprints(w http.ResponseWriter, r *http.Request, p *principal) {
	mine, site, err := s.Svc.Footprints(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := map[string]any{"items": toFootprints(mine), "joinedAt": p.User.CreatedAt}
	if p.User.Role == "admin" {
		out["site"] = toFootprints(site)
	}
	writeJSON(w, http.StatusOK, out)
}
