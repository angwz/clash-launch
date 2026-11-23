// 复制到剪贴板（反馈 #114）。
//
// navigator.clipboard 只在安全上下文（HTTPS 或 localhost）中存在：用 http://IP 访问时它是 undefined，
// 直接调用会抛错，界面就只能提示「无法写入剪贴板」。这里先用标准接口，不可用或被拒绝时退回
// document.execCommand("copy")：它在用户点击触发的同步调用中、非安全上下文里也能工作（已标为过时，
// 但各浏览器仍支持，且没有替代品）。两者都失败时返回 false，由调用方提示手动复制。
import { t } from "./i18n/i18n.svelte";
import { toasts } from "./toast.svelte";

export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 权限被拒绝等：继续尝试退回方式
    }
  }
  return legacyCopy(text);
}

function legacyCopy(text: string): boolean {
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  // 放在视口外，不影响布局与滚动；不设样式属性以符合 CSP（style-src 'self'），用 CSSOM 设置
  ta.style.position = "fixed";
  ta.style.insetBlockStart = "-1000px";
  ta.style.opacity = "0";
  const prev = document.activeElement as HTMLElement | null;
  // 模态对话框打开时页面其余部分是 inert 的：放进当前对话框里才能选中
  const host = prev?.closest?.("dialog[open]") ?? document.body;
  host.appendChild(ta);
  ta.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  ta.remove();
  prev?.focus?.({ preventScroll: true });
  return ok;
}

/** 复制并提示结果。失败的提示几秒后自动消失：它不是需要处理的错误，用户手动复制即可（反馈 #114：原来一直停留）。 */
export async function copyAndTell(text: string): Promise<boolean> {
  const ok = await copyText(text);
  if (ok) toasts.push("ok", t("common.copied"));
  else toasts.push("err", t("common.copy_failed"), { duration: 6000 });
  return ok;
}
