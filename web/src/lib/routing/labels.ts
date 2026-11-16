// 分流页的显示文字：规则名、组类型、出口名。只做显示，不改变任何语义。
import { i18n, t } from "../i18n/i18n.svelte";
import { flagOf } from "../regions";
import { catalog } from "../catalog.svelte";
import type { Rule } from "./layout";
import type { ExitRow } from "./plan";

/** 组类型（只有 mihomo 官方的四种，决定 0007 #15）。 */
export const GROUP_TYPES = ["select", "url-test", "fallback", "load-balance"] as const;

export function typeLabel(type: string): string {
  return (GROUP_TYPES as readonly string[]).includes(type) ? t("routing.type." + type) : type;
}

/** 紧凑视图里的单字类型标签（选、速、备、均）。 */
export function typeChar(type: string): string {
  return (GROUP_TYPES as readonly string[]).includes(type) ? t("routing.type_char." + type) : type.slice(0, 1);
}

/** 规则在地图上的名字：规则集用 provider 名；其他规则用「类型,条件」。 */
export function ruleName(r: Rule): string {
  if (r.set) return r.set.name;
  if (r.type === "MATCH") return t("routing.match");
  const parts = r.raw.split(",");
  // 去掉目标与参数，只留类型与条件；逻辑规则的条件里有逗号，按原文去掉末尾的目标
  if (r.flags.includes("logic") || r.type === "SUB-RULE") return r.raw.slice(0, r.raw.length - r.target.length - 1);
  return parts.slice(0, 2).join(",");
}

// 内置出口的显示名：键为 routing.exit.<后缀>
const BUILTIN_LABEL: Record<string, string> = { DIRECT: "direct", REJECT: "reject", "REJECT-DROP": "reject_drop", PASS: "pass", COMPATIBLE: "compatible" };

export function exitLabel(e: ExitRow): string {
  if (e.kind === "builtin") return builtinLabel(e.label);
  if (e.kind === "region" && e.label === "?") return t("routing.exit.unknown_region");
  if (e.kind === "region") return `${flagOf(e.label)} ${catalog.label(e.names) || shortRegion(e.label)}`;
  if (e.kind === "source" && e.label === "?") return t("routing.exit.unknown_source");
  return e.label;
}

export function builtinLabel(name: string): string {
  return BUILTIN_LABEL[name] ? t("routing.exit." + BUILTIN_LABEL[name]) : name;
}

/** 条数：1.2k、35k。 */
export function fmtCount(n: number): string {
  if (!n) return "";
  return n >= 10000 ? Math.round(n / 1000) + "k" : n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n);
}

// 出口列很窄：地区名用简短形式（香港、澳门，而不是「中国香港特别行政区」）
const shortNames = new Map<string, Intl.DisplayNames | null>();
function shortRegion(code: string): string {
  const loc = i18n.intlLocale;
  let dn = shortNames.get(loc);
  if (dn === undefined) {
    try {
      dn = new Intl.DisplayNames([loc], { type: "region", style: "short" });
    } catch {
      dn = null;
    }
    shortNames.set(loc, dn);
  }
  try {
    return dn?.of(code.toUpperCase()) ?? code;
  } catch {
    return code;
  }
}

/** 折叠段里显示的规则集名：同一类的 X、X-ipcidr、X-classic 只显示一次 X（domainrouter_v2 的发布约定）。 */
export function setBases(rules: readonly Rule[]): string[] {
  const out: string[] = [];
  for (const r of rules) {
    const n = (r.set?.name ?? ruleName(r)).replace(/-(ipcidr|classic)$/, "");
    if (!out.includes(n)) out.push(n);
  }
  return out;
}

/** 规则目标的显示名：内置出口用中文名，组名原样。 */
export function targetLabel(name: string): string {
  return builtinLabel(name);
}
