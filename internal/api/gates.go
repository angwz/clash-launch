package api

import (
	"net/http"
	"strings"
)

// 已登录请求的附加关卡，在 authed 中依次检查：
//
//  1. 个人访问令牌（R25）只能访问 tokenPaths 列出的资源，read 权限只能读取；
//  2. 用户协议有新版本（R40）时，同意之前只放行 termsExempt；
//  3. 两步验证策略（R41）要求而用户尚未启用时，只放行 mfaExempt（含设置 TOTP 所需的接口）。
//
// 关卡按路由模式（r.Pattern，形如 "GET /api/v1/me"）判断，与路由表一一对应。

// tokenPaths 是令牌可以访问的资源（路径前缀）：节点源、规则源、配置、链接、短链接、模板与统计。
// 账号安全（密码、两步验证、通行密钥、会话、令牌本身）、消息与授权、实时通道、管理接口都不开放：
// 令牌泄露时影响范围限于配置数据，不能借此接管账号。
var tokenPaths = []string{
	"/api/v1/sources",
	"/api/v1/rule-sources",
	"/api/v1/rule-hosts",
	"/api/v1/profiles",
	"/api/v1/templates",
	"/api/v1/short-links",
	"/api/v1/analytics",
	"/api/v1/node-mode",
	"/api/v1/account/quota",
}

// tokenAllowed 报告令牌能否访问这条路由。
func tokenAllowed(pattern, scope string) bool {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		return false
	}
	if scope != "write" && method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if pattern == "GET /api/v1/me" {
		return true
	}
	for _, p := range tokenPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// mfaExempt 是两步验证待设置时仍然可用的接口：读取身份、同意协议、退出登录，以及启用 TOTP 的三步。
var mfaExempt = map[string]bool{
	"GET /api/v1/me":                   true,
	"POST /api/v1/account/terms":       true,
	"DELETE /api/v1/session":           true,
	"GET /api/v1/account/totp":         true,
	"POST /api/v1/account/totp/setup":  true,
	"POST /api/v1/account/totp/enable": true,
	"POST /api/v1/account/delete":      true, // 不愿启用两步验证时可以注销（需求 R20）
}

// gate 依次检查令牌范围、用户协议与两步验证策略；不通过时写出错误并返回 false。
func (s *Server) gate(w http.ResponseWriter, r *http.Request, p *principal) bool {
	if p.Token != nil && !tokenAllowed(r.Pattern, p.Token.Scope) {
		problem(w, http.StatusForbidden, "token.forbidden", "this access token cannot be used for this endpoint")
		return false
	}
	if !termsExempt[r.Pattern] {
		pending, err := s.Svc.TermsPending(r.Context(), p.User)
		if err != nil {
			s.fail(w, err)
			return false
		}
		if pending {
			problem(w, http.StatusForbidden, "terms.required", "accept the latest terms of use to continue")
			return false
		}
	}
	if !mfaExempt[r.Pattern] {
		pending, err := s.Svc.MFASetupPending(r.Context(), p.User)
		if err != nil {
			s.fail(w, err)
			return false
		}
		if pending {
			problem(w, http.StatusForbidden, "mfa.setup_required", "two-step verification is required on this site; set it up to continue")
			return false
		}
	}
	return true
}
