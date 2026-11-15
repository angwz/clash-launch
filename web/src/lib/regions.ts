// 国家与地区的显示（反馈 #74）：名称用浏览器的 Intl.DisplayNames 按当前语言给出，旗帜用区域指示符 emoji。

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
