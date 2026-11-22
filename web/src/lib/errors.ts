// 把 API 错误转换为界面文字：已知的错误类型有对应的消息键，其余显示服务端的说明。
import { ApiError } from "./api/client";
import { t } from "./i18n/i18n.svelte";

const known: Record<string, string> = {
  "auth.invalid": "error.auth_invalid",
  "auth.required": "error.auth_required",
  "auth.csrf": "error.csrf",
  "auth.weak_password": "error.weak_password",
  "register.closed": "error.register_closed",
  "register.username": "error.username",
  "register.rate": "error.rate",
  "auth.locked": "error.locked",
  "recovery.failed": "error.recovery_failed",
  "pow.invalid": "error.pow",
  "account.questions": "error.questions",
  "account.wrong_password": "error.wrong_password",
  "mfa.invalid": "error.mfa",
  "passkey.invalid": "error.passkey",
  "passkey.unavailable": "error.passkey_unavailable",
  "quota.exceeded": "error.quota",
  "smtp.unavailable": "error.smtp_unavailable",
  "admin.self": "error.admin_self",
  "mfa.state": "error.mfa_state",
  "mfa.policy_self": "error.mfa_policy_self",
  "mfa.required_by_policy": "error.mfa_required_by_policy",
  "mfa.setup_required": "error.mfa_setup_required",
  "terms.required": "error.terms_required",
  "terms.outdated": "terms.changed",
  "token.limit": "error.token_limit",
  "channel.limit": "error.channel_limit",
  "profile.protected": "error.profile_protected",
  "export.needs_link": "error.export_needs_link",
  "backup.passphrase": "error.backup_passphrase",
  "account.last_admin": "error.last_admin",
  "admin.retention": "error.retention",
  "account.deleted": "error.account_deleted",
  "backup.format": "error.backup_format",
  "backup.weak": "error.backup_weak",
  "channel.unavailable": "channels.unavailable",
  "token.invalid": "error.token_invalid",
  rate_limited: "error.rate",
  "rate_limited.check": "error.rate",
  "source.client_fetch": "source.client_fetch",
  "rule.host_not_allowed": "error.rule_host",
  "short.code": "error.short_code",
  "message.recipient": "error.recipient",
  "message.invalid": "error.message",
  "account.timezone": "error.timezone",
  "account.avatar": "error.avatar",
  "announcement.invalid": "error.announcement",
  "message.key": "error.message_key",
  "account.avatar_too_large": "error.avatar_too_large",
  conflict: "error.conflict",
  exists: "error.exists",
  not_found: "error.not_found",
  not_ready: "error.not_ready",
  internal: "error.internal",
};

export function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    // 邮件发送失败的说明（阶段与服务器返回码）对用户有用，原样附上
    if (e.type === "smtp.failed") return t("error.smtp", { detail: e.detail.replace(/^source: 邮件发送失败：/, "") });
    // 推送失败的原因（已分类，不含地址与令牌）同样原样附上
    if (e.type === "push.failed") return t("error.push", { detail: e.detail.replace(/^source: 推送失败：/, "") });
    const key = known[e.type];
    if (key) return t(key);
    if (e.status === 429) return t("error.rate");
    return e.detail || t("error.http", { status: e.status });
  }
  if (e instanceof TypeError) return t("error.network");
  return e instanceof Error ? e.message : String(e);
}
