// 可撤销的删除（需求 R33、R35）：点删除后条目立即从列表隐藏，底部提示条保留 10 秒「撤销」，
// 到时才真正调用删除接口。后端的删除路径（R2 的授权收回与通知）因此不变，也不需要「已标记删除」的状态。
// 页面关闭或刷新时，未到期的删除立即以 keepalive 请求发出，不会因离开页面而丢失；
// 浏览器崩溃等无法发出时，条目保留在服务器上（宁可不删，不误删）。
import { api } from "./api/client";
import { errorText } from "./errors";
import { t } from "./i18n/i18n.svelte";
import { toasts } from "./toast.svelte";

/** 撤销窗口。 */
export const UNDO_MS = 10_000;

interface Batch {
  paths: string[]; // 每项一个 DELETE 路径
  ids: string[];
  timer: ReturnType<typeof setTimeout>;
  toast: number;
  done?: () => void;
}

class Pending {
  /** 等待删除、应在列表中隐藏的 ID。 */
  hidden = $state<ReadonlySet<string>>(new Set());
  #batches = new Set<Batch>();

  constructor() {
    if (typeof window !== "undefined") window.addEventListener("pagehide", () => this.flush());
  }

  has(id: string): boolean {
    return this.hidden.has(id);
  }

  /**
   * 安排删除一组条目。label 显示在提示条上；done 在全部请求结束后调用（通常是重新加载列表）。
   * items 的每项给出 ID 与它的 DELETE 路径。
   */
  remove(items: { id: string; path: string }[], label: string, done?: () => void): void {
    if (!items.length) return;
    const b: Batch = { paths: items.map((x) => x.path), ids: items.map((x) => x.id), timer: 0 as unknown as ReturnType<typeof setTimeout>, toast: 0, done };
    this.#hide(b.ids, true);
    b.toast = toasts.push("info", label, { action: { label: t("common.undo"), run: () => this.#undo(b) }, duration: UNDO_MS });
    b.timer = setTimeout(() => void this.#commit(b), UNDO_MS);
    this.#batches.add(b);
  }

  #hide(ids: string[], on: boolean) {
    const s = new Set(this.hidden);
    for (const id of ids) {
      if (on) s.add(id);
      else s.delete(id);
    }
    this.hidden = s;
  }

  #undo(b: Batch) {
    if (!this.#batches.delete(b)) return;
    clearTimeout(b.timer);
    this.#hide(b.ids, false);
    b.done?.();
  }

  async #commit(b: Batch) {
    if (!this.#batches.delete(b)) return;
    toasts.dismiss(b.toast);
    // 逐个删除；已经不存在的（404）视为成功。删掉的 ID 继续隐藏：页面上可能还有删除前取到的列表，
    // 这样它们不会重新出现。失败的条目重新显示，并报告第一条原因
    let failed: unknown = null;
    const back: string[] = [];
    for (const [i, p] of b.paths.entries()) {
      try {
        await api.del(p);
      } catch (e) {
        if ((e as { status?: number }).status === 404) continue;
        failed ??= e;
        back.push(b.ids[i]!);
      }
    }
    if (back.length) this.#hide(back, false);
    if (failed) toasts.push("err", errorText(failed));
    b.done?.();
  }

  /** 立即发出所有未到期的删除（页面关闭时）。 */
  flush(): void {
    for (const b of [...this.#batches]) {
      this.#batches.delete(b);
      clearTimeout(b.timer);
      for (const p of b.paths) void api.del(p, { keepalive: true }).catch(() => undefined);
    }
  }
}

export const pending = new Pending();
