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
