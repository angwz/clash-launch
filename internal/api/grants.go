package api

import (
	"net/http"
	"time"

	"github.com/angwz/clash-launch/internal/store"
)

// 收件人联想、联系人与回收站、授权（反馈 #75）。

// searchUsers 按用户名前缀联想收件人，并标出是否接收消息。
func (s *Server) searchUsers(w http.ResponseWriter, r *http.Request, p *principal) {
	if !s.renderAllowed(w, p, false) { // 与预览共用每分钟的次数上限，避免被用来批量探测账号
		return
	}
	hits, err := s.Svc.SearchUsers(r.Context(), p.User, r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": hits})
}

func (s *Server) deletedThreads(w http.ResponseWriter, r *http.Request, p *principal) {
	ts, err := s.Svc.DeletedThreads(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": ts})
}

// deleteThread 删除与某人的对话：放入回收站保存一年，并收回双方之间的全部授权。
func (s *Server) deleteThread(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.DeleteThread(r.Context(), p.User, r.PathValue("username")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) restoreThread(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.RestoreThread(r.Context(), p.User, r.PathValue("username")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type grantDTO struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"` // source | short
	Peer       string     `json:"peer"` // 对方的用户名
	Name       string     `json:"name"`
	SourceKind string     `json:"sourceKind,omitempty"`
	URL        string     `json:"url,omitempty"` // 短链接授权：为对方生成的短链接地址
	CreatedAt  time.Time  `json:"createdAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

// listGrants 返回我给出的与我收到的授权（含已收回的，供界面显示历史）。
func (s *Server) listGrants(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	given, err := s.Svc.Store.Grants().Given(ctx, p.User.ID, false)
	if err != nil {
		s.fail(w, err)
		return
	}
	received, err := s.Svc.Store.Grants().Received(ctx, p.User.ID, false)
	if err != nil {
		s.fail(w, err)
		return
	}
	names := map[string]string{}
	nameOf := func(id string) string {
		if n, ok := names[id]; ok {
			return n
		}
		if u, err := s.Svc.Store.Users().ByID(ctx, id); err == nil {
			names[id] = u.Username
		}
		return names[id]
	}
	conv := func(gs []store.Grant, peerOf func(store.Grant) string) []grantDTO {
		out := []grantDTO{}
		for _, g := range gs {
			d := grantDTO{ID: g.ID, Kind: g.Kind, Peer: nameOf(peerOf(g)), CreatedAt: g.CreatedAt, RevokedAt: tp(g.RevokedAt)}
			switch g.Kind {
			case "source":
				if src, err := s.Svc.Store.Sources().Get(ctx, g.OwnerID, g.TargetID); err == nil {
					d.Name, d.SourceKind = src.Name, src.Kind
				}
			case "short":
				if sl, err := s.Svc.Store.ShortLinks().Get(ctx, g.OwnerID, g.TargetID); err == nil {
					d.Name, d.URL = sl.Code, s.baseURL(r)+"/"+sl.Code
				}
			}
			out = append(out, d)
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"given":    conv(given, func(g store.Grant) string { return g.GranteeID }),
		"received": conv(received, func(g store.Grant) string { return g.OwnerID }),
	})
}

// revokeGrant 收回授权（授权者），或移除收到的授权（被授权者）。
func (s *Server) revokeGrant(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.RevokeGrant(r.Context(), p.User, r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
