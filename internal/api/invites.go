package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 用户名、昵称与人人可邀请（需求 R4、R5、R12）。

// —— 用户名与昵称 ——

func (s *Server) renameUser(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.Svc.Rename(r.Context(), p.User.ID, req.Password, req.Username)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userWithInviter(r.Context(), u))
}

func (s *Server) setNickname(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if _, err := s.Svc.SetNickname(r.Context(), p.User.ID, req.Nickname); err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.Svc.Store.Users().ByID(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.userWithInviter(r.Context(), u))
}

// userWithInviter 是 toUser 加上邀请人的当前用户名（邀请人改过名时以新名为准；已注销时用快照）。
func (s *Server) userWithInviter(ctx context.Context, u *store.User) userDTO {
	d := toUser(u)
	if u.InvitedBy != "" {
		if iv, err := s.Svc.Store.Users().ByID(ctx, u.InvitedBy); err == nil {
			d.InvitedBy = iv.Username
		}
	}
	return d
}

// —— 邀请 ——

type inviteDTO struct {
	ID        string     `json:"id"`
	URL       string     `json:"url,omitempty"` // 只在创建时返回：令牌只以哈希存储
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	MaxUses   int        `json:"maxUses"`
	Uses      int        `json:"uses"`
	Status    string     `json:"status"`              // active | used | expired | revoked
	CreatedBy string     `json:"createdBy,omitempty"` // 管理员视图：发出者的用户名
	UsedBy    []string   `json:"usedBy,omitempty"`    // 用这个邀请注册的人（用户名）
}

func inviteStatus(v *store.Invite, now time.Time) string {
	switch {
	case !v.RevokedAt.IsZero():
		return "revoked"
	case v.Uses >= v.MaxUses:
		return "used"
	case !v.ExpiresAt.IsZero() && now.After(v.ExpiresAt):
		return "expired"
	}
	return "active"
}

func toInvite(v *store.Invite) inviteDTO {
	return inviteDTO{ID: v.ID, Note: v.Note, CreatedAt: v.CreatedAt, ExpiresAt: tp(v.ExpiresAt), UsedAt: tp(v.UsedAt), MaxUses: v.MaxUses, Uses: v.Uses, Status: inviteStatus(v, time.Now())}
}

// usernames 把用户 ID 换成用户名（查不到的跳过：账号已注销）。names 是跨调用的缓存。
func (s *Server) usernames(ctx context.Context, ids []string, names map[string]string) []string {
	var out []string
	for _, id := range ids {
		if n, ok := names[id]; ok {
			out = append(out, n)
			continue
		}
		if u, err := s.Svc.Store.Users().ByID(ctx, id); err == nil {
			names[id] = u.Username
			out = append(out, u.Username)
		}
	}
	return out
}

func (s *Server) inviteDetails(ctx context.Context, v *store.Invite, names map[string]string) inviteDTO {
	d := toInvite(v)
	if v.Uses > 0 {
		if ids, err := s.Svc.Store.Invites().UsedBy(ctx, v.ID); err == nil {
			d.UsedBy = s.usernames(ctx, ids, names)
		}
	}
	return d
}

type inviteeDTO struct {
	ID       string    `json:"id"`
	Avatar   string    `json:"avatar,omitempty"` // 需求 R19
	Username string    `json:"username"`
	Nickname string    `json:"nickname,omitempty"`
	JoinedAt time.Time `json:"joinedAt"`
}

// myInvites 返回我的邀请名额、发出的邀请（含谁用它注册）与邀请来的人。
func (s *Server) myInvites(w http.ResponseWriter, r *http.Request, p *principal) {
	ctx := r.Context()
	q, err := s.Svc.QuotaFor(ctx, p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	invs, err := s.Svc.Store.Invites().ByCreator(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	names := map[string]string{}
	items := make([]inviteDTO, 0, len(invs))
	for i := range invs {
		items = append(items, s.inviteDetails(ctx, &invs[i], names))
	}
	us, err := s.Svc.Store.Users().Invitees(ctx, p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	invitees := make([]inviteeDTO, 0, len(us))
	for _, u := range us {
		invitees = append(invitees, inviteeDTO{ID: u.ID, Avatar: u.Avatar, Username: u.Username, Nickname: u.Nickname, JoinedAt: u.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"quota": q, "items": items, "invitees": invitees})
}

type createInviteReq struct {
	Hours   int    `json:"hours"`
	Note    string `json:"note"`
	MaxUses int    `json:"maxUses"`
}

// createInvite 发出邀请（管理员与普通用户共用）；hours 为有效期（小时），0 表示不过期，最长 90 天；
// maxUses 为可用次数（默认 1，最多 100）。
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, p *principal) {
	var req createInviteReq
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if req.Hours < 0 || req.Hours > 90*24 {
		s.fail(w, badRequest("hours must be between 0 (never expires) and 2160"))
		return
	}
	tok, inv, err := s.Svc.NewInvite(r.Context(), p.User, service.InviteInput{TTL: time.Duration(req.Hours) * time.Hour, Note: truncate(strings.TrimSpace(req.Note), 200), MaxUses: req.MaxUses})
	if err != nil {
		s.fail(w, err)
		return
	}
	d := toInvite(inv)
	d.URL = s.baseURL(r) + "/register#" + tok // 令牌放在片段中：不会出现在反向代理的访问日志里
	writeJSON(w, http.StatusCreated, d)
}

// revokeMyInvite 作废自己发出的邀请。
func (s *Server) revokeMyInvite(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Invites().Revoke(r.Context(), r.PathValue("id"), p.User.ID, time.Now()); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "invite.revoke", map[string]string{"invite": r.PathValue("id")})
	w.WriteHeader(http.StatusNoContent)
}

// inviter 是公开接口：注册页凭邀请令牌显示「由谁邀请」。令牌放在请求体里，不进访问日志；
// 无效的令牌与不存在一样返回 404，并计入失败次数限制，不能用来逐个试探。
func (s *Server) inviter(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.Svc.InviterOf(r.Context(), strings.TrimSpace(req.Token))
	if err != nil {
		if !s.publicLimited(w, r, s.missLimit) {
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": u.Username, "nickname": u.Nickname})
}

// —— 管理员 ——

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request, _ *principal) {
	ctx := r.Context()
	items, err := s.Svc.Store.Invites().List(ctx, page(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	names := map[string]string{}
	out := list[inviteDTO]{Items: []inviteDTO{}}
	for i := range items {
		d := s.inviteDetails(ctx, &items[i], names)
		if by := s.usernames(ctx, []string{items[i].CreatedBy}, names); len(by) == 1 {
			d.CreatedBy = by[0]
		}
		out.Items = append(out.Items, d)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request, p *principal) {
	if err := s.Svc.Store.Invites().Delete(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), p.User.ID, "admin.invite.delete", map[string]string{"invite": r.PathValue("id")})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getInvitePolicy(w http.ResponseWriter, r *http.Request, _ *principal) {
	pol, err := s.Svc.GetInvitePolicy(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pol)
}

func (s *Server) putInvitePolicy(w http.ResponseWriter, r *http.Request, p *principal) {
	var req service.InvitePolicy
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.SetInvitePolicy(r.Context(), p.User.ID, req); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// setInviteBan 收回或恢复某个用户的邀请权；收回时同时作废其全部未用完的邀请。
func (s *Server) setInviteBan(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Banned bool `json:"banned"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	if err := s.Svc.Store.Users().SetInviteBanned(ctx, id, req.Banned); err != nil {
		s.fail(w, err)
		return
	}
	if req.Banned {
		invs, err := s.Svc.Store.Invites().ByCreator(ctx, id)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, v := range invs {
			if service.InviteValid(&v, time.Now()) {
				if err := s.Svc.Store.Invites().Revoke(ctx, v.ID, "", time.Now()); err != nil {
					s.fail(w, err)
					return
				}
			}
		}
	}
	action := "admin.invite.allow"
	if req.Banned {
		action = "admin.invite.ban"
	}
	s.audit(ctx, p.User.ID, action, map[string]string{"user": id})
	w.WriteHeader(http.StatusNoContent)
}
