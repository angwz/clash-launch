// 默认名称（反馈 #65）：需要命名的地方都给出统一格式的默认名，以灰色占位显示，留空即采用。
// 做法参考常见实践：按类型或来源命名（订阅取域名、配置取所用的模板或客户端预设），
// 与已有名称重复时追加「 2」「 3」……（与 macOS Finder「untitled folder 2」的做法一致）。

/** 返回不与 existing 重复的名称：base、base 2、base 3…… */
export function uniqueName(base: string, existing: Iterable<string>): string {
  const taken = new Set(Array.from(existing, (s) => s.trim()));
  if (!taken.has(base)) return base;
  for (let i = 2; ; i++) {
    const n = `${base} ${i}`;
    if (!taken.has(n)) return n;
  }
}

/** 地址的域名；无法解析时返回 null。 */
export function hostOf(url: string): string | null {
  try {
    const h = new URL(url.trim()).hostname;
    return h || null;
  } catch {
    return null;
  }
}
