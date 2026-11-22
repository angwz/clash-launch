// 默认名称（反馈 #65）：需要命名的地方都给出统一格式的默认名，以灰色占位显示，留空即采用。
// 做法参考常见实践：按类型或来源命名（订阅取域名、配置取所用的模板或客户端预设），
// 与已有名称重复时追加「 2」「 3」……（与 macOS Finder「untitled folder 2」的做法一致）。

/** 返回不与 existing 重复的名称：base、base 2、base 3…… */
/** 已注销账号的用户名前缀（需求 R20，与服务端 DeletedPrefix 一致）。 */
export const DELETED_PREFIX = "deleted-";

/** 用户在界面上的名字：已注销的账号显示为 fallback（如「已注销的用户」），否则昵称优先。 */
export function shownName(u: { username: string; nickname?: string }, deletedLabel: string): string {
  if (u.username.startsWith(DELETED_PREFIX)) return deletedLabel;
  return u.nickname || u.username;
}

/** 写在一句话里的名字（O12）：有昵称时为「昵称 @用户名」（昵称可以重名，总带上用户名），否则就是用户名。 */
export function nameWithAt(username: string, nickname?: string): string {
  return nickname ? nickname + " @" + username : username;
}

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
