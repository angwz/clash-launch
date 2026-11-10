// 把 API 错误转换为界面文字：已知的错误类型有对应的消息键，其余显示服务端的说明。
import { ApiError } from "./api/client";
import { t } from "./i18n/i18n.svelte";

const known: Record<string, string> = {
  "auth.invalid": "error.auth_invalid",
  "auth.required": "error.auth_required",
  "auth.csrf": "error.csrf",
  "auth.weak_password": "error.weak_password",
  "setup.invalid": "error.setup_invalid",
  "register.closed": "error.register_closed",
  "register.username": "error.username",
  "register.rate": "error.rate",
  conflict: "error.conflict",
  exists: "error.exists",
  not_found: "error.not_found",
  not_ready: "error.not_ready",
  internal: "error.internal",
};

export function errorText(e: unknown): string {
  if (e instanceof ApiError) {
    const key = known[e.type];
    if (key) return t(key);
    if (e.status === 429) return t("error.rate");
    return e.detail || t("error.http", { status: e.status });
  }
  if (e instanceof TypeError) return t("error.network");
  return e instanceof Error ? e.message : String(e);
}
