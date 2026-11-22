// 命令面板搜数据（需求 R34）：配置、节点源、节点名、规则源、短链接与消息联系人。
// 每次打开命令面板时取一次索引（GET /api/v1/search），按键时在本地匹配；关闭后丢弃，下次打开重新取，
// 不会显示已删除或改名前的数据。没有输入时不列出数据：空白面板只显示页面与常用命令。
import { api } from "./api/client";
import { chat } from "./chat.svelte";
import { t } from "./i18n/i18n.svelte";
import { palette, type PaletteItem } from "./palette.svelte";
import { router } from "./router.svelte";

export interface SearchIndex {
  profiles: { id: string; name: string }[];
  sources: { id: string; name: string; kind: string }[];
  rules: { id: string; name: string; kind: string }[];
  nodes: { source: string; name: string }[];
  shortLinks: { id: string; code: string; note?: string }[];
  friends: { username: string; nickname?: string }[];
  truncated?: boolean;
}

class PaletteData {
  index = $state<SearchIndex | null>(null);
  #loading = false;

  /** 打开时加载；失败只是搜不到数据，页面与命令照常可用。 */
  async load(): Promise<void> {
    if (this.#loading) return;
    this.#loading = true;
    try {
      this.index = await api.get<SearchIndex>("/api/v1/search");
    } catch {
      this.index = null;
    } finally {
      this.#loading = false;
    }
  }

  clear(): void {
    this.index = null;
  }
}

export const paletteData = new PaletteData();

/** 把索引转成命令面板条目。节点排在最后：数量最多，同分时不挤掉配置与节点源。 */
export function dataItems(x: SearchIndex | null): PaletteItem[] {
  if (!x) return [];
  const srcName = new Map(x.sources.map((s) => [s.id, s.name]));
  return [
    ...x.profiles.map((p): PaletteItem => ({ id: "d.p." + p.id, kind: "profile", title: p.name, icon: "layers", run: () => router.go("/profiles/" + p.id) })),
    ...x.sources.map((s): PaletteItem => ({ id: "d.s." + s.id, kind: "source", title: s.name, hint: t("source.kind." + s.kind), icon: s.kind === "subscription" ? "link" : "server", run: () => router.go("/sources/" + s.id) })),
    ...x.rules.map((r): PaletteItem => ({ id: "d.r." + r.id, kind: "rule", title: r.name, icon: "rules", run: () => router.go("/rules/" + r.id) })),
    ...x.shortLinks.map((s): PaletteItem => ({ id: "d.sl." + s.id, kind: "short", title: "/" + s.code, hint: s.note, icon: "qr", run: () => router.go("/short-links?focus=" + encodeURIComponent(s.id)) })),
    ...x.friends.map((f): PaletteItem => ({ id: "d.f." + f.username, kind: "friend", title: f.nickname || f.username, hint: "@" + f.username, icon: "message", run: () => chat.show(f.username) })),
    ...x.nodes.map((n, i): PaletteItem => ({ id: "d.n." + i, kind: "node", title: n.name, hint: srcName.get(n.source), icon: "server", run: () => router.go("/sources/" + n.source + "?node=" + encodeURIComponent(n.name)) })),
  ];
}

/** 由 App 调用一次：注册数据来源，并在命令面板打开时加载索引。 */
export function registerPaletteData(): () => void {
  const off = palette.register("data", () => dataItems(paletteData.index));
  const stop = $effect.root(() => {
    $effect(() => {
      if (palette.open) void paletteData.load();
      else paletteData.clear();
    });
  });
  return () => {
    off();
    stop();
  };
}
