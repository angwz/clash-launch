package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/angwz/clash-launch/internal/auth"
	"github.com/angwz/clash-launch/internal/geo"
	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 账号安全接口（第 14.1 节）：工作量证明、找回、安全问题、修改密码、会话管理。
// 业务规则在 internal/service/account.go；这里只做 HTTP 层的同源检查、限流与 DTO。

type powDTO struct {
	Challenge string `json:"challenge"`
	Bits      int    `json:"bits"`
}

type powProof struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
}

// powChallenge 签发工作量证明挑战。purpose 只能是 register 或 recover。
func (s *Server) powChallenge(w http.ResponseWriter, r *http.Request) {
	purpose := r.URL.Query().Get("purpose")
	if purpose != service.PoWRegister && purpose != service.PoWRecover {
		s.fail(w, badRequest("purpose must be register or recover"))
		return
	}
	ch, bits := s.Svc.PoWChallenge(purpose)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, powDTO{Challenge: ch, Bits: bits})
}

// publicLimited 是未登录的写接口共用的前置检查：同源、按 IP 限速。
func (s *Server) publicLimited(w http.ResponseWriter, r *http.Request, l *limiter) bool {
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "auth.csrf", "cross-origin request refused")
		return false
	}
	if ok, wait := l.allow(s.clientIP(r).String(), time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(wait))
		problem(w, http.StatusTooManyRequests, "rate_limited", "too many attempts")
		return false
	}
	return true
}

// —— 找回 ——

type questionsDTO struct {
	Questions []auth.Question `json:"questions"`
}

// recoveryQuestions 返回找回时显示的问题；用户不存在时返回稳定的诱饵问题（不泄露用户名是否存在）。
func (s *Server) recoveryQuestions(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimited(w, r, s.recoverLimit) {
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, questionsDTO{Questions: s.Svc.RecoveryQuestions(r.Context(), req.Username)})
}

// recover 用安全问题重设密码；需要工作量证明。成功后该账号的全部会话失效。
func (s *Server) recover(w http.ResponseWriter, r *http.Request) {
	if !s.publicLimited(w, r, s.recoverLimit) {
		return
	}
	var req struct {
		Username string   `json:"username"`
		Answers  []string `json:"answers"`
		Password string   `json:"password"`
		PoW      powProof `json:"pow"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.CheckPoW(service.PoWRecover, req.PoW.Challenge, req.PoW.Nonce); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.Recover(r.Context(), req.Username, req.Answers, req.Password, s.clientIP(r).String()); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 安全问题 ——

func (s *Server) myQuestions(w http.ResponseWriter, r *http.Request, p *principal) {
	qs, err := s.Svc.MyQuestions(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if qs == nil {
		qs = []auth.Question{}
	}
	writeJSON(w, http.StatusOK, questionsDTO{Questions: qs})
}

func (s *Server) setQuestions(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Password  string          `json:"password"`
		Questions []auth.Question `json:"questions"`
		Answers   []string        `json:"answers"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Svc.SetQuestions(r.Context(), p.User.ID, req.Password, req.Questions, req.Answers); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 密码 ——

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		Current      string `json:"current"`
		Password     string `json:"password"`
		LogoutOthers bool   `json:"logoutOthers"`
		// MessageKey 是用新密码重新包裹的消息私钥（R46）；没有消息密钥时省略
		MessageKey *struct {
			KeyID   string          `json:"keyId"`
			Wrapped json.RawMessage `json:"wrapped"`
		} `json:"messageKey"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	var rewrap *service.KeyRewrap
	if req.MessageKey != nil {
		rewrap = &service.KeyRewrap{KeyID: req.MessageKey.KeyID, Wrapped: req.MessageKey.Wrapped}
	}
	if err := s.Svc.ChangePassword(r.Context(), p.User.ID, req.Current, req.Password, req.LogoutOthers, p.Session.IDHash, rewrap); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// —— 会话 ——

// sessionDTO 是一个登录设备。ID 是会话哈希的前 16 位：足以在本人的会话中定位，
// 不能用来冒充会话（会话令牌原文不可从哈希反推）。
type sessionDTO struct {
	ID        string     `json:"id"`
	Device    string     `json:"device"`
	UA        string     `json:"ua"`
	IP        string     `json:"ip"`
	Location  *geo.Place `json:"location,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	LastSeen  time.Time  `json:"lastSeen"`
	// ExpiresAt 是自然到期时间（登录满 SessionMax）；闲置满 SessionIdle 也会失效，见 IdleUntil
	ExpiresAt time.Time  `json:"expiresAt"`
	IdleUntil time.Time  `json:"idleUntil"`
	LogoutAt  *time.Time `json:"logoutAt,omitempty"`
	Current   bool       `json:"current"`
}

func (s *Server) toSession(x *store.Session, current string) sessionDTO {
	return sessionDTO{ID: x.IDHash[:16], Device: auth.DeviceName(x.UA), UA: x.UA, IP: x.IPPrefix, Location: parsePlace(x.Location), CreatedAt: x.CreatedAt, LastSeen: x.LastSeen,
		ExpiresAt: x.ExpiresAt, IdleUntil: x.LastSeen.Add(s.Cfg.SessionIdle), LogoutAt: tp(x.LogoutAt), Current: x.IDHash == current}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, p *principal) {
	list, err := s.Svc.Store.Sessions().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]sessionDTO, 0, len(list))
	for i := range list {
		out = append(out, s.toSession(&list[i], p.Session.IDHash))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// minLogoutDelay 是定时退出最早的时间：再早就等于立即注销，应当用「注销」。
const minLogoutDelay = time.Minute

// scheduleLogout 设定或取消定时退出（需求 R8）。路径中的 id 为 others 时作用于除当前设备外的全部设备。
// 请求 {"at": 时间} 设定，{"at": null} 取消。定时只能早于该设备的自然到期时间（只能提前，不能延后）；
// 作用于全部其他设备时，自然到期更早的设备保持不变，不算错误。
func (s *Server) scheduleLogout(w http.ResponseWriter, r *http.Request, p *principal) {
	var req struct {
		At *time.Time `json:"at"`
	}
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	now := time.Now()
	if req.At != nil && req.At.Before(now.Add(minLogoutDelay)) {
		problem(w, http.StatusUnprocessableEntity, "session.logout_too_soon", "the time must be at least a minute from now")
		return
	}
	list, err := s.Svc.Store.Sessions().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	id := r.PathValue("id")
	var targets []store.Session
	for _, x := range list {
		if (id == "others" && x.IDHash != p.Session.IDHash) || (len(id) == 16 && strings.HasPrefix(x.IDHash, id)) {
			targets = append(targets, x)
		}
	}
	if id != "others" && len(targets) == 0 {
		problem(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	n := 0
	for _, x := range targets {
		var at time.Time
		if req.At != nil {
			at = req.At.UTC()
			if !at.Before(x.ExpiresAt) {
				if id != "others" {
					problem(w, http.StatusUnprocessableEntity, "session.logout_after_expiry", "the device signs out on its own before this time")
					return
				}
				continue
			}
		}
		if err := s.Svc.Store.Sessions().SetLogoutAt(r.Context(), p.User.ID, x.IDHash, at); err != nil {
			s.fail(w, err)
			return
		}
		n++
		action := "session.schedule_logout"
		if req.At == nil {
			action = "session.cancel_logout"
		}
		detail := map[string]string{"device": auth.DeviceName(x.UA)}
		if req.At != nil {
			detail["at"] = at.Format(time.RFC3339)
		}
		s.audit(r.Context(), p.User.ID, action, detail)
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request, p *principal) {
	id := r.PathValue("id")
	list, err := s.Svc.Store.Sessions().List(r.Context(), p.User.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, x := range list {
		if len(id) == 16 && strings.HasPrefix(x.IDHash, id) {
			if err := s.Svc.Store.Sessions().Delete(r.Context(), p.User.ID, x.IDHash); err != nil {
				s.fail(w, err)
				return
			}
			s.audit(r.Context(), p.User.ID, "session.revoke", map[string]string{"device": auth.DeviceName(x.UA)})
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	problem(w, http.StatusNotFound, "not_found", "resource not found")
}

// placeJSON 把登录地点编码后存入会话（需求 R7）：只在登录时用完整地址查询一次，地址本身不保存。
func placeJSON(p geo.Place) string {
	if p.Empty() {
		return ""
	}
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// parsePlace 是 placeJSON 的逆操作；旧会话或无法解析的内容返回 nil。
func parsePlace(s string) *geo.Place {
	if s == "" {
		return nil
	}
	var p geo.Place
	if json.Unmarshal([]byte(s), &p) != nil || p.Empty() {
		return nil
	}
	return &p
}
