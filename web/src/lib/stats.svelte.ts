// 使用统计（反馈 #66、#68）：打开网站、切换页面、改设置项的值、展开说明。
// 事件先在内存中排队，每 15 秒或页面隐藏时一次性上报；去重与计数在服务端完成，这里只做最基本的合并。
// 不上报任何 ID 或内容：页面只报类型（如 profile、help），字段只报路径（如 dns.enable）。
import { api } from "./api/client";

export type StatKind = "visit" | "pv" | "field_change" | "field_expand";

interface Ev {
  kind: StatKind;
  key?: string;
}

class StatsTracker {
  #queue: Ev[] = [];
  #timer: ReturnType<typeof setTimeout> | undefined;
  #started = false;
  /** 设置项热度快照（每周由服务端重算） */
  scores = $state<Record<string, number>>({});

  /** 登录后调用一次：记一次「打开网站」（刷新不算），并取得热度快照。 */
  start(): void {
    if (this.#started) return;
    this.#started = true;
    const nav = performance.getEntriesByType("navigation")[0] as PerformanceNavigationTiming | undefined;
    if (nav?.type !== "reload") this.track("visit");
    addEventListener("visibilitychange", () => document.visibilityState === "hidden" && this.flush(true));
    api
      .get<{ scores: Record<string, number> }>("/api/v1/stats/field-scores")
      .then((r) => (this.scores = r.scores ?? {}))
      .catch((e) => console.warn("stats: field scores", e)); // 取不到时按默认顺序，不影响使用
  }

  track(kind: StatKind, key?: string): void {
    if (!this.#started && kind !== "visit") return;
    this.#queue.push(key ? { kind, key } : { kind });
    if (this.#queue.length >= 40) this.flush();
    else if (!this.#timer) this.#timer = setTimeout(() => this.flush(), 15_000);
  }

  flush(closing = false): void {
    clearTimeout(this.#timer);
    this.#timer = undefined;
    if (!this.#queue.length) return;
    const events = this.#queue.splice(0, 50);
    // 统计失败不打扰用户，只在控制台留下记录
    api.post("/api/v1/stats/events", { events }, { keepalive: closing }).catch((e) => console.warn("stats: report", e));
  }
}

export const stats = new StatsTracker();
