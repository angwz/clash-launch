// 主题：system（默认，跟随系统）| light | dark。选择保存在本机；读不到存储时按 system 处理。
export type Theme = "system" | "light" | "dark";
const KEY = "cl.theme";

class ThemeState {
  value = $state<Theme>("system");

  init(): void {
    let saved: string | null = null;
    try {
      saved = localStorage.getItem(KEY);
    } catch {
      /* 存储不可用 */
    }
    const forced = new URLSearchParams(location.search).get("theme");
    this.apply(forced === "light" || forced === "dark" ? forced : saved === "light" || saved === "dark" ? saved : "system", false);
  }

  apply(v: Theme, persist = true): void {
    this.value = v;
    const root = document.documentElement;
    if (v === "system") delete root.dataset.theme;
    else root.dataset.theme = v;
    if (persist) {
      try {
        localStorage.setItem(KEY, v);
      } catch {
        /* 只影响下次打开 */
      }
    }
  }

  cycle(): void {
    this.apply(this.value === "system" ? "light" : this.value === "light" ? "dark" : "system");
  }
}

export const theme = new ThemeState();
