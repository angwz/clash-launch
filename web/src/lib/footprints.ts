// 足迹（需求 R10）的文字。每类一句话，带上回看时有意义的细节（邀请人、客户端、地区、阈值……）。
import { i18n, t } from "./i18n/i18n.svelte";
import { regionName } from "./regions";

export interface Footprint {
  kind: string;
  subject?: string;
  at: string;
  detail: Record<string, string>;
  backfilled?: boolean;
}

export function footprintText(f: Pick<Footprint, "kind" | "subject" | "detail">): string {
  const d = f.detail ?? {};
  const n = Number(f.subject);
  switch (f.kind) {
    case "joined":
      return d.inviter ? t("footprint.joined_by", { name: d.inviter }) : t("footprint.joined");
    case "first_source":
      return d.name ? t("footprint.first_source_named", { name: d.name }) : t("footprint.first_source");
    case "client":
      return t("footprint.client", { client: f.subject ?? "" });
    case "pulls":
      return t("footprint.pulls", { n: i18n.number(n) });
    case "streak":
      return t("footprint.streak", { n });
    case "pull_country":
      return t("footprint.pull_country", { place: regionName(f.subject ?? "", i18n.locale) });
    case "login_country":
      return t("footprint.login_country", { place: regionName(f.subject ?? "", i18n.locale) });
    case "rename":
      return t("footprint.rename", { from: d.from ?? "", to: d.to ?? "" });
    case "nickname":
      return d.nickname ? t("footprint.nickname", { name: d.nickname }) : t("footprint.nickname_clear");
    case "first_contact":
      return t("footprint.first_contact", { name: d.user ?? "" });
    case "first_message":
      return t("footprint.first_message", { name: d.to ?? "" });
    case "first_grant":
      return t("footprint.first_grant", { name: d.to ?? "" });
    case "first_grant_got":
      return t("footprint.first_grant_got", { name: d.from ?? "" });
    case "site_first":
      return t("footprint.site_first", { name: d.user ?? "" });
    case "site_users":
      return t("footprint.site_users", { n: i18n.number(n) });
    default:
      // 其余是不带细节的「第一次」：first_login、first_mfa、first_passkey、first_profile、first_link、first_check、first_routing、first_pull、site_start
      return t("footprint.kind." + f.kind);
  }
}
