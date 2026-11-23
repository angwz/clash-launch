// 国家与地区的显示（反馈 #74）：名称用浏览器的 Intl.DisplayNames 按当前语言给出，旗帜用区域指示符 emoji。
import { placeName } from "./place-names";

/** ISO 两位代码 → 旗帜 emoji（无效代码返回空字符串） */
export function flagOf(code: string): string {
  if (!/^[A-Za-z]{2}$/.test(code)) return "";
  const base = 0x1f1e6 - 65; // 区域指示符 A 的码位减去 'A'
  return String.fromCodePoint(...[...code.toUpperCase()].map((c) => base + c.charCodeAt(0)));
}

const cache = new Map<string, Intl.DisplayNames | null>();

/** ISO 两位代码 → 当前语言的国家或地区名称；不认识时原样返回代码 */
export function regionName(code: string, locale: string): string {
  if (!code) return "";
  let dn = cache.get(locale);
  if (dn === undefined) {
    try {
      dn = new Intl.DisplayNames([locale], { type: "region" });
    } catch {
      dn = null;
    }
    cache.set(locale, dn);
  }
  try {
    return dn?.of(code.toUpperCase()) ?? code;
  } catch {
    return code;
  }
}

/**
 * 是否为本机或内网地址（回环、私有网段、链路本地、CGNAT、IPv6 唯一本地地址）。
 * 这类地址没有国家归属，分析页显示为「本地网络」，而不是「未知」（反馈 #75）。
 */
export function isLocalIP(ip: string): boolean {
  const v4 = /^(\d+)\.(\d+)\.\d+\.\d+$/.exec(ip);
  if (v4) {
    const a = Number(v4[1]);
    const b = Number(v4[2]);
    return a === 10 || a === 127 || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168) || (a === 169 && b === 254) || (a === 100 && b >= 64 && b <= 127);
  }
  const s = ip.toLowerCase();
  return s === "::1" || s.startsWith("fc") || s.startsWith("fd") || s.startsWith("fe8") || s.startsWith("fe9") || s.startsWith("fea") || s.startsWith("feb") || s.startsWith("::ffff:127.");
}

/** 地点（登录设备 R7、订阅来源 O7）：国家代码与按数据库原文的省、市、运营商。 */
export interface Place {
  cc?: string;
  region?: string;
  city?: string;
  isp?: string;
}

/** 地点的显示文字：国家名按界面语言生成；省、市收录在 place-names 中的按界面语言显示，其余按数据库原文；
 * 运营商按原文。重复的部分只显示一次（例如直辖市的省与市）。 */
export function placeText(p: Place, locale: string, withISP = true): string {
  const loc = (s: string | undefined) => (s ? placeName(s, p.cc, locale) : "");
  const parts = [p.cc ? regionName(p.cc, locale) : "", loc(p.region), loc(p.city), withISP ? p.isp : ""].filter((x): x is string => !!x);
  return parts.filter((x, i) => parts.indexOf(x) === i).join(" · ");
}
