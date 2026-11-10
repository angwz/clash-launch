// 命令面板（第 16.3 节）的状态与命令来源。
// 全局命令（页面跳转、主题、语言）由 App 注册；页面可以注册额外的来源（如方案编辑器的字段搜索与原始编辑命令）。
// 以 > 开头进入命令模式：只列出命令（kind: "command"），「原始编辑」只在这里出现（第 16.4 节）。
import type { IconName } from "../components/Icon.svelte";

export interface PaletteItem {
  id: string;
  kind: "page" | "field" | "command";
  title: string;
  /** 次要文字：字段键、所在分节等 */
  hint?: string;
  /** 参与搜索但不显示的文字（字段摘要与详细说明全文） */
  text?: string;
  icon?: IconName;
  run: () => void;
}

export type Provider = () => PaletteItem[];

class PaletteState {
  open = $state(false);
  query = $state("");
  #providers = new Map<string, Provider>();

  register(id: string, p: Provider): () => void {
    this.#providers.set(id, p);
    return () => this.#providers.delete(id);
  }

  show(query = ""): void {
    this.query = query;
    this.open = true;
  }

  close(): void {
    this.open = false;
  }

  /** 后注册的来源（当前页面）排在前面：页面相关的命令比全局命令更常用。 */
  items(): PaletteItem[] {
    return [...this.#providers.values()].reverse().flatMap((p) => p());
  }
}

export const palette = new PaletteState();

/** 搜索：不区分大小写；每个词都要出现在标题、提示或全文中；标题命中的排在前面。 */
export function search(items: PaletteItem[], q: string, limit = 60): PaletteItem[] {
  const commandMode = q.startsWith(">");
  const words = (commandMode ? q.slice(1) : q).toLowerCase().split(/\s+/).filter(Boolean);
  const pool = items.filter((x) => (commandMode ? x.kind === "command" : x.kind !== "command" || !x.id.startsWith("raw")));
  if (!words.length) return pool.slice(0, limit);
  const scored: [number, PaletteItem][] = [];
  for (const it of pool) {
    const title = it.title.toLowerCase();
    const hint = (it.hint ?? "").toLowerCase();
    const text = (it.text ?? "").toLowerCase();
    let score = 0;
    let ok = true;
    for (const w of words) {
      if (title.startsWith(w)) score += 6;
      else if (title.includes(w)) score += 4;
      else if (hint.includes(w)) score += 3;
      else if (text.includes(w)) score += 1;
      else {
        ok = false;
        break;
      }
    }
    if (ok) scored.push([score, it]);
  }
  return scored
    .sort((a, b) => b[0] - a[0])
    .slice(0, limit)
    .map((x) => x[1]);
}
