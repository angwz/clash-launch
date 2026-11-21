package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// 站内提醒（需求 R23、R24）。列表最多返回最近 100 条；页面按 kind 与 detail 生成本地化文案和跳转。

const noticeLimit = 100

type noticeDTO struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Detail    map[string]string `json:"detail"`
	CreatedAt time.Time         `json:"createdAt"`
	Read      bool              `json:"read"`
}

func (s *Server) listNotices(w http.ResponseWriter, r *http.Request, p *principal) {
	ns, err := s.Svc.Store.Notifications().List(r.Context(), p.User.ID, noticeLimit)
	if err != nil {
		s.fail(w, err)
		return
	}
	unread, err := s.Svc.Store.Notifications().Unread(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]noticeDTO, 0, len(ns))
	for _, n := range ns {
		d := map[string]string{}
		if err := json.Unmarshal(n.Detail, &d); err != nil {
			d = map[string]string{} // 写入时总是合法的 JSON；解析失败只丢掉细节，不影响列表
		}
		out = append(out, noticeDTO{ID: n.ID, Kind: n.Kind, Detail: d, CreatedAt: n.CreatedAt, Read: !n.ReadAt.IsZero()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "unread": unread})
}

type noticeReadReq struct {
	// IDs 为空表示全部标为已读
	IDs []string `json:"ids"`
}

func (s *Server) readNotices(w http.ResponseWriter, r *http.Request, p *principal) {
	var req noticeReadReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if len(req.IDs) > noticeLimit {
		s.fail(w, badRequest("too many ids"))
		return
	}
	if err := s.Svc.Store.Notifications().MarkRead(r.Context(), p.User.ID, req.IDs, time.Now()); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
