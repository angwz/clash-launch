// 站内提醒（需求 R23、R24）：右上角铃铛的列表与未读数。新提醒经实时通道的 notice 事件立即出现，
// 连不上时每 5 分钟刷新一次（页面不可见时暂停）。
import { api } from "./api/client";
import { live } from "./live";

export interface Notice {
  id: string;
  kind: "source_failing" | "check_failed" | "link_new";
  detail: Record<string, string>;
  createdAt: string;
  read: boolean;
}

class Notices {
  items = $state<Notice[]>([]);
  unread = $state(0);
  #stop: (() => void) | undefined;

  async refresh(): Promise<void> {
    const r = await api.get<{ items: Notice[]; unread: number }>("/api/v1/notifications");
    this.items = r.items;
    this.unread = r.unread;
  }

  /** 标为已读；不给 ids 时全部标为已读。先改本地状态，失败时重新读取。 */
  async markRead(ids?: string[]): Promise<void> {
    const set = ids ? new Set(ids) : null;
    this.items = this.items.map((n) => (!set || set.has(n.id) ? { ...n, read: true } : n));
    this.unread = this.items.filter((n) => !n.read).length;
    try {
      await api.post("/api/v1/notifications/read", { ids: ids ?? [] });
    } catch {
      await this.refresh().catch(() => undefined);
    }
  }

  start(): void {
    if (this.#stop) return;
    void this.refresh().catch(() => undefined);
    this.#stop = live(() => this.refresh(), 300_000, ["notice"]).stop;
  }

  stop(): void {
    this.#stop?.();
    this.#stop = undefined;
    this.items = [];
    this.unread = 0;
  }
}

export const notices = new Notices();
