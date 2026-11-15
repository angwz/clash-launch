// 主菜单的定义（侧栏与「设置 → 菜单顺序」共用，避免两处不一致）。顺序按用户偏好排列（反馈 #37）。
import type { IconName } from "../components/Icon.svelte";
import { t } from "./i18n/i18n.svelte";
import { ordered } from "./prefs.svelte";

export interface NavItem {
  id: string;
  href: string;
  label: string;
  icon: IconName;
  /** 当前路由属于这一项 */
  matches: (route: string) => boolean;
  /** 角标（未读数）；0 不显示 */
  badge?: number;
}

export function mainNav(isAdmin: boolean, order: readonly string[] | undefined, unread = 0): NavItem[] {
  const items: NavItem[] = [
    { id: "profiles", href: "/", label: t("nav.profiles"), icon: "layers", matches: (r) => ["home", "profile", "profile-links", "profile-history", "welcome"].includes(r) },
    { id: "sources", href: "/sources", label: t("nav.sources"), icon: "server", matches: (r) => r.startsWith("source") },
    { id: "rules", href: "/rules", label: t("nav.rules"), icon: "rules", matches: (r) => r.startsWith("rule") },
    { id: "short-links", href: "/short-links", label: t("nav.short_links"), icon: "shrink", matches: (r) => r === "short-links" },
    { id: "analytics", href: "/analytics", label: t("nav.analytics"), icon: "chart", matches: (r) => r === "analytics" },
    { id: "messages", href: "/messages", label: t("nav.messages"), icon: "message", matches: (r) => r.startsWith("message"), badge: unread },
    { id: "help", href: "/help", label: t("nav.help"), icon: "file", matches: (r) => r === "help" },
  ];
  if (isAdmin) items.push({ id: "admin", href: "/admin", label: t("nav.admin"), icon: "shield", matches: (r) => r === "admin" });
  return ordered(items, (n) => n.id, order);
}
