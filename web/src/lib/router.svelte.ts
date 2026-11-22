// 极简路由（决定 0004）：History API + 路径模式匹配，不引入路由库。
// 前端路径避开后端保留的 /api、/s、/p、/u（订阅、托管地址与短链接）；未知路径由后端回退到 index.html。

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
  | "terms"
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
  | "settings"
  | "analytics"
  | "short-links"
  | "messages"
  | "message-thread"
  | "notfound";

const table: [RouteName, string][] = [
  ["home", "/"],
  ["welcome", "/welcome"],
  ["login", "/login"],
  ["setup", "/setup"],
  ["register", "/register"],
  ["recover", "/recover"],
  ["terms", "/terms"],
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
  ["settings", "/settings"],
  ["analytics", "/analytics"],
  ["short-links", "/short-links"],
  ["messages", "/messages"],
  ["message-thread", "/messages/:username"],
];

/** 公开页面：未登录也可以访问。 */
export const PUBLIC: ReadonlySet<RouteName> = new Set(["login", "setup", "register", "recover", "terms", "notfound"]);

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

// 历史记录（需求 R3）：每条记录的 state 带序号 i 与来源地址 from。
// - 序号：浏览器返回或前进时知道走了几步；离开检查拒绝时据此撤销这次跳转；滚动位置按序号保存。
// - from：保存或删除后「回到列表」时，若上一条正是列表就后退一步，不再新增记录（返回不会回到刚离开的详情或已删除的页面）。
interface Entry {
  i: number;
  from?: string;
}

const SCROLL_KEY = "cl.scroll";

/** 页面的滚动容器：桌面上是外壳的 <main>，手机上是整个文档。两者都记，恢复时各自还原。 */
function scrollers(): (Element | null)[] {
  return [document.getElementById("main"), document.scrollingElement];
}

class Router {
  path = $state(location.pathname);
  search = $state(location.search);
  hash = $state(location.hash);
  route = $derived(match(this.path));

  #index = 0;
  #ignore = false; // 撤销跳转时触发的 popstate 不再处理
  #scroll: Record<number, number[]> = {};

  constructor() {
    try {
      this.#scroll = JSON.parse(sessionStorage.getItem(SCROLL_KEY) ?? "{}") as Record<number, number[]>;
    } catch {
      this.#scroll = {};
    }
    // 由我们自己还原滚动位置：内容是异步加载的，浏览器在数据到达前恢复只会停在顶部
    history.scrollRestoration = "manual";
    const st = history.state as Entry | null;
    if (typeof st?.i === "number") this.#index = st.i;
    else history.replaceState({ ...(history.state ?? {}), i: 0 }, "");
    window.addEventListener("popstate", (e) => this.#pop(e.state as Entry | null));
    // 同一页面内只改片段（如 /register → /register#令牌）不会触发 popstate
    window.addEventListener("hashchange", () => this.#sync());
    // 刷新或关闭前记下当前位置，刷新后同样回到原处
    window.addEventListener("pagehide", () => this.#remember());
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
    queueMicrotask(() => this.#restore());
  }

  /**
   * 离开当前页面前的检查：配置编辑器借此立即保存，节点源、规则源有未保存的修改时请用户确认。
   * 返回 false 时取消跳转；浏览器的返回与前进同样经过它（拒绝时撤销这次返回）。
   */
  guard: (() => boolean) | null = null;

  go(to: string, replace = false): void {
    const here = location.pathname + location.search + location.hash;
    if (to === here) return;
    const samePage = to.split("?")[0]!.split("#")[0] === location.pathname;
    if (!samePage && this.guard && !this.guard()) return;
    this.#remember();
    if (replace) {
      history.replaceState({ ...(history.state as Entry), i: this.#index }, "", to);
    } else {
      this.#index++;
      history.pushState({ i: this.#index, from: here } satisfies Entry, "", to);
      // 新的分支：丢弃序号更大的旧记录的滚动位置
      for (const k of Object.keys(this.#scroll)) if (Number(k) >= this.#index) delete this.#scroll[Number(k)];
    }
    this.#sync();
    if (!samePage) for (const el of scrollers()) el?.scrollTo({ top: 0 });
  }

  /** 只更新查询参数，不新增历史记录（筛选、展开的条目等随手调整的状态）。 */
  replace(search: string): void {
    const s = search && !search.startsWith("?") ? "?" + search : search;
    history.replaceState(history.state, "", location.pathname + s + location.hash);
    this.#sync();
  }

  /**
   * 完成一件事后回到 to（如保存或删除后回到列表）：上一条记录正是 to 时后退一步，否则替换当前记录。
   * 两种情况都不会留下刚离开的页面，返回时不会再回到它。
   */
  leave(to: string): void {
    if (this.guard && !this.guard()) return;
    const g = this.guard;
    this.guard = null; // 已经确认过：后退触发的 popstate 不再询问
    if ((history.state as Entry | null)?.from === to) {
      this.#remember();
      history.back();
      return;
    }
    this.guard = g;
    this.go(to, true);
  }

  #pop(st: Entry | null): void {
    if (this.#ignore) {
      this.#ignore = false;
      return;
    }
    const target = typeof st?.i === "number" ? st.i : 0;
    const delta = target - this.#index;
    const samePage = location.pathname === this.path;
    if (!samePage && this.guard && !this.guard()) {
      // 撤销：地址栏已经变了，走回原来的记录
      this.#ignore = true;
      history.go(-delta);
      return;
    }
    this.#scroll[this.#index] = this.#positions();
    this.#index = target;
    this.#sync();
    this.#restore();
  }

  #positions(): number[] {
    return scrollers().map((el) => el?.scrollTop ?? 0);
  }

  #remember(): void {
    this.#scroll[this.#index] = this.#positions();
    try {
      sessionStorage.setItem(SCROLL_KEY, JSON.stringify(this.#scroll));
    } catch {
      // 隐私模式等不能写入：只影响刷新后的滚动位置
    }
  }

  /** 还原滚动位置：页面内容异步到达，在约 2 秒内反复尝试，直到滚得到或用户自己动了。 */
  #restore(): void {
    const want = this.#scroll[this.#index];
    if (!want || want.every((y) => y === 0)) return;
    const started = performance.now();
    let touched = false;
    const stop = () => (touched = true);
    window.addEventListener("wheel", stop, { once: true, passive: true });
    window.addEventListener("touchstart", stop, { once: true, passive: true });
    window.addEventListener("keydown", stop, { once: true });
    const step = () => {
      if (touched) return;
      const els = scrollers();
      let done = true;
      els.forEach((el, k) => {
        const y = want[k] ?? 0;
        if (!el || y === 0) return;
        el.scrollTop = y;
        if (Math.abs(el.scrollTop - y) > 2) done = false;
      });
      if (!done && performance.now() - started < 2000) requestAnimationFrame(step);
      else {
        window.removeEventListener("wheel", stop);
        window.removeEventListener("touchstart", stop);
        window.removeEventListener("keydown", stop);
      }
    };
    requestAnimationFrame(step);
  }

  #sync(): void {
    this.path = location.pathname;
    this.search = location.search;
    this.hash = location.hash;
  }
}

export const router = new Router();
