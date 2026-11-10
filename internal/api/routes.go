package api

import "net/http"

// Route 是一条路由。路由表是契约测试（TestContract）与 api/openapi.yaml 对照的依据。
type Route struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

func (s *Server) routes() []Route {
	a := s.authed
	return []Route{
		{"GET", "/healthz", s.healthz},
		{"GET", "/readyz", s.readyz},
		{"GET", "/s/{token}", s.subscribe},
		{"GET", "/p/{token}/{source}", s.hosted},

		{"GET", "/api/v1/setup", s.setupStatus},
		{"POST", "/api/v1/setup", s.setupAdmin},
		{"POST", "/api/v1/session", s.login},
		{"DELETE", "/api/v1/session", a(s.logout)},
		{"GET", "/api/v1/me", a(s.me)},
		{"GET", "/api/v1/register", s.registrationState},
		{"POST", "/api/v1/register", s.register},
		{"GET", "/api/v1/pow", s.powChallenge},
		{"POST", "/api/v1/recovery/questions", s.recoveryQuestions},
		{"POST", "/api/v1/recovery", s.recover},

		{"GET", "/api/v1/account/questions", a(s.myQuestions)},
		{"PUT", "/api/v1/account/questions", a(s.setQuestions)},
		{"PUT", "/api/v1/account/password", a(s.changePassword)},
		{"GET", "/api/v1/account/sessions", a(s.listSessions)},
		{"DELETE", "/api/v1/account/sessions/{id}", a(s.deleteSession)},
		{"POST", "/api/v1/session/totp", s.sessionTOTP},
		{"GET", "/api/v1/account/totp", a(s.totpStatus)},
		{"POST", "/api/v1/account/totp/setup", a(s.totpSetup)},
		{"POST", "/api/v1/account/totp/enable", a(s.totpEnable)},
		{"POST", "/api/v1/account/totp/disable", a(s.totpDisable)},
		{"POST", "/api/v1/account/totp/recovery-codes", a(s.totpCodes)},
		{"GET", "/api/v1/account/passkeys", a(s.listPasskeys)},
		{"POST", "/api/v1/account/passkeys/begin", a(s.passkeyRegisterBegin)},
		{"POST", "/api/v1/account/passkeys", a(s.passkeyRegisterFinish)},
		{"PATCH", "/api/v1/account/passkeys/{id}", a(s.renamePasskey)},
		{"POST", "/api/v1/account/passkeys/{id}/delete", a(s.deletePasskey)},
		{"POST", "/api/v1/session/passkey/begin", s.sessionPasskeyBegin},
		{"POST", "/api/v1/session/passkey", s.sessionPasskey},
		{"GET", "/api/v1/account/smtp", a(s.getSMTP)},
		{"PUT", "/api/v1/account/smtp", a(s.putSMTP)},
		{"DELETE", "/api/v1/account/smtp", a(s.deleteSMTP)},
		{"POST", "/api/v1/account/smtp/test", a(s.testSMTP)},
		{"GET", "/api/v1/account/audit", a(s.myAudit)},
		{"GET", "/api/v1/account/quota", a(s.myQuota)},
		{"GET", "/api/v1/admin/users", s.adminOnly(s.adminUsers)},
		{"PATCH", "/api/v1/admin/users/{id}", s.adminOnly(s.adminPatchUser)},
		{"POST", "/api/v1/admin/users/{id}/password", s.adminOnly(s.adminResetPassword)},
		{"PUT", "/api/v1/admin/users/{id}/quota", s.adminOnly(s.adminSetQuota)},
		{"GET", "/api/v1/admin/audit", s.adminOnly(s.adminAudit)},
		{"GET", "/api/v1/admin/status", s.adminOnly(s.adminStatus)},

		{"GET", "/api/v1/admin/registration", s.adminOnly(s.getRegistration)},
		{"PUT", "/api/v1/admin/registration", s.adminOnly(s.putRegistration)},
		{"GET", "/api/v1/admin/invites", s.adminOnly(s.listInvites)},
		{"POST", "/api/v1/admin/invites", s.adminOnly(s.createInvite)},
		{"DELETE", "/api/v1/admin/invites/{id}", s.adminOnly(s.deleteInvite)},

		{"GET", "/api/v1/catalog/{pack}", s.catalogMeta},
		{"GET", "/api/v1/catalog/{pack}/schema", s.catalogSchema},
		{"GET", "/api/v1/catalog/{pack}/messages", s.catalogMessages},
		{"GET", "/api/v1/diagnostics", s.diagnostics},

		{"GET", "/api/v1/sources", a(s.listSources)},
		{"POST", "/api/v1/sources", a(s.createSource)},
		{"GET", "/api/v1/sources/{id}", a(s.getSource)},
		{"PUT", "/api/v1/sources/{id}", a(s.updateSource)},
		{"DELETE", "/api/v1/sources/{id}", a(s.deleteSource)},
		{"POST", "/api/v1/sources/{id}/refresh", a(s.refreshSource)},
		{"GET", "/api/v1/sources/{id}/nodes", a(s.sourceNodes)},

		{"GET", "/api/v1/rule-sources", a(s.listRules)},
		{"POST", "/api/v1/rule-sources", a(s.createRule)},
		{"GET", "/api/v1/rule-sources/{id}", a(s.getRule)},
		{"PUT", "/api/v1/rule-sources/{id}", a(s.updateRule)},
		{"DELETE", "/api/v1/rule-sources/{id}", a(s.deleteRule)},
		{"POST", "/api/v1/rule-sources/{id}/refresh", a(s.refreshRule)},

		{"GET", "/api/v1/profiles", a(s.listProfiles)},
		{"POST", "/api/v1/profiles", a(s.createProfile)},
		{"GET", "/api/v1/profiles/{id}", a(s.getProfile)},
		{"PUT", "/api/v1/profiles/{id}", a(s.updateProfile)},
		{"DELETE", "/api/v1/profiles/{id}", a(s.deleteProfile)},
		{"GET", "/api/v1/profiles/{id}/revisions", a(s.listRevisions)},
		{"POST", "/api/v1/profiles/{id}/rollback", a(s.rollback)},
		{"POST", "/api/v1/profiles/{id}/preview", a(s.preview)},
		{"GET", "/api/v1/profiles/{id}/render", a(s.renderStatus)},
		{"GET", "/api/v1/profiles/{id}/export", a(s.exportProfile)},

		{"GET", "/api/v1/profiles/{id}/links", a(s.listLinks)},
		{"POST", "/api/v1/profiles/{id}/links", a(s.createLink)},
		{"POST", "/api/v1/profiles/{id}/links/{lid}/reset", a(s.resetLink)},
		{"PATCH", "/api/v1/profiles/{id}/links/{lid}", a(s.patchLink)},
		{"DELETE", "/api/v1/profiles/{id}/links/{lid}", a(s.deleteLink)},
	}
}

// Routes 返回路由表（契约测试使用）。
func (s *Server) Routes() []Route { return s.routes() }
