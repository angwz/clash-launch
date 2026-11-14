// 极简路由（决定 0004）：History API + 路径模式匹配，不引入路由库。
// 前端路径避开后端保留的 /api、/s、/p（订阅与托管地址）；未知路径由后端回退到 index.html。

export interface Match {
  name: RouteName;
  params: Record<string, string>;
}

export type RouteName =
  | "home"
  | "welcome"
  | "login"
  | "setup"
  | "register"
  | "recover"
  | "profile"
  | "profile-links"
  | "profile-history"
  | "sources"
  | "source"
  | "rules"
  | "rule"
  | "account"
  | "admin"
  | "help"
  | "notfound";

const table: [RouteName, string][] = [
  ["home", "/"],
  ["welcome", "/welcome"],
  ["login", "/login"],
  ["setup", "/setup"],
  ["register", "/register"],
  ["recover", "/recover"],
  ["profile", "/profiles/:id"],
  ["profile-links", "/profiles/:id/links"],
  ["profile-history", "/profiles/:id/history"],
  ["sources", "/sources"],
  ["source", "/sources/:id"],
  ["rules", "/rules"],
  ["rule", "/rules/:id"],
  ["account", "/account"],
  ["admin", "/admin"],
  ["help", "/help"],
];

/** 公开页面：未登录也可以访问。 */
export const PUBLIC: ReadonlySet<RouteName> = new Set(["login", "setup", "register", "recover", "notfound"]);

export function match(pathname: string): Match {
  const parts = pathname.replace(/\/+$/, "").split("/").filter(Boolean);
  for (const [name, pattern] of table) {
    const pp = pattern.split("/").filter(Boolean);
    if (pp.length !== parts.length) continue;
    const params: Record<string, string> = {};
    let ok = true;
    for (let i = 0; i < pp.length; i++) {
      const p = pp[i]!;
      const v = parts[i]!;
      if (p.startsWith(":")) params[p.slice(1)] = decodeURIComponent(v);
      else if (p !== v) {
        ok = false;
        break;
      }
    }
    if (ok) return { name, params };
  }
  return { name: "notfound", params: {} };
}

class Router {
  path = $state(location.pathname);
  search = $state(location.search);
  hash = $state(location.hash);
  route = $derived(match(this.path));

  constructor() {
    window.addEventListener("popstate", () => this.#sync());
    // 同一页面内只改片段（如 /register → /register#令牌）不会触发 popstate
    window.addEventListener("hashchange", () => this.#sync());
    // 拦截站内链接的点击：带修饰键、新窗口、下载或外部链接时交给浏览器。
    document.addEventListener("click", (e) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const a = (e.target as Element | null)?.closest?.("a");
      if (!a || a.target || a.hasAttribute("download") || a.dataset.external !== undefined) return;
      const url = new URL(a.href, location.href);
      if (url.origin !== location.origin || /^\/(api|s|p)\//.test(url.pathname)) return;
      e.preventDefault();
      this.go(url.pathname + url.search + url.hash);
    });
  }

  /** 离开当前页面前的确认（如方案有未保存的修改）；返回 false 时取消跳转。 */
  guard: (() => boolean) | null = null;

  go(to: string, replace = false): void {
    if (to === location.pathname + location.search + location.hash) return;
    const samePage = to.split("?")[0]!.split("#")[0] === location.pathname;
    if (!samePage && this.guard && !this.guard()) return;
    if (replace) history.replaceState(null, "", to);
    else history.pushState(null, "", to);
    this.#sync();
    window.scrollTo({ top: 0 });
  }

  #sync(): void {
    this.path = location.pathname;
    this.search = location.search;
    this.hash = location.hash;
  }
}

export const router = new Router();
