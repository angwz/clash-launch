package api

import (
	"context"
	"net/http"

	"github.com/angwz/clash-launch/internal/service"
	"github.com/angwz/clash-launch/internal/store"
)

// 用户协议（需求 R40）。
//   - GET /api/v1/terms：公开，注册页与 /terms 页面读取；
//   - PUT /api/v1/admin/terms：管理员改写或恢复默认，生成新版本；
//   - POST /api/v1/account/terms：已登录用户同意或拒绝当前版本。
//
// 普通用户同意的版本低于当前版本时，除了下面 termsExempt 列出的接口，其他已登录接口一律 403 terms.required：
// 页面据此显示协议，同意后才能继续使用。订阅链接不经过会话，不受影响，客户端照常更新。

// termsExempt 是协议待同意时仍然可用的接口：读取身份（页面据此显示协议）、同意或拒绝、退出登录。
var termsExempt = map[string]bool{
	"GET /api/v1/me":              true,
	"POST /api/v1/account/terms":  true,
	"DELETE /api/v1/session":      true,
	"POST /api/v1/account/delete": true, // 不同意新协议时可以选择注销（需求 R20）
	"GET /api/v1/account/totp":    true, // 注销表单据此决定是否要验证码
}

type termsState struct {
	Accepted int  `json:"accepted"` // 用户同意的版本；0 表示尚未同意
	Current  int  `json:"current"`
	Pending  bool `json:"pending"` // 需要先同意才能继续使用
}

func (s *Server) termsState(ctx context.Context, u *store.User) (termsState, error) {
	cur, err := s.Svc.TermsVersion(ctx)
	if err != nil {
		return termsState{}, err
	}
	pending, err := s.Svc.TermsPending(ctx, u)
	if err != nil {
		return termsState{}, err
	}
	return termsState{Accepted: u.TermsVersion, Current: cur, Pending: pending}, nil
}

func (s *Server) getTerms(w http.ResponseWriter, r *http.Request) {
	t, err := s.Svc.CurrentTerms(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type termsEdit struct {
	// Texts 为按语言的新文本；Reset 为 true 时恢复随程序提供的默认文本（忽略 Texts）
	Texts map[string]string `json:"texts"`
	Reset bool              `json:"reset"`
}

func (s *Server) putTerms(w http.ResponseWriter, r *http.Request, p *principal) {
	var req termsEdit
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	texts := req.Texts
	if req.Reset {
		texts = nil
	} else if texts == nil {
		texts = map[string]string{} // 既不恢复默认也没给文本：按空文本校验并拒绝
	}
	t, err := s.Svc.SetTerms(r.Context(), p.User.ID, texts)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// getAdminTerms 返回当前协议的原始文本（占位符未替换）与随程序提供的默认文本，供管理员编辑。
func (s *Server) getAdminTerms(w http.ResponseWriter, r *http.Request, _ *principal) {
	t, err := s.Svc.RawTerms(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminTerms{Terms: t, Defaults: service.DefaultTerms()})
}

type adminTerms struct {
	service.Terms
	Defaults map[string]string `json:"defaults"`
}

type termsAnswer struct {
	Version int  `json:"version"`
	Accept  bool `json:"accept"`
}

func (s *Server) answerTerms(w http.ResponseWriter, r *http.Request, p *principal) {
	var req termsAnswer
	if err := readJSON(r, &req); err != nil {
		s.fail(w, err)
		return
	}
	if !req.Accept {
		// 拒绝只记入审计，账号不变；页面随后退出登录。下次登录仍会看到协议
		s.Svc.DeclineTerms(r.Context(), p.User, req.Version)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.Svc.AcceptTerms(r.Context(), p.User, req.Version); err != nil {
		s.fail(w, err)
		return
	}
	st, err := s.termsState(r.Context(), p.User)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
