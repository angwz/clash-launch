// 未读消息数（反馈 #74）：侧栏「消息」上的角标。登录后每分钟刷新一次（页面不可见时暂停），
// 打开会话或收到新消息时由消息页直接更新。
import { api } from "./api/client";
import { live } from "./live";

class Inbox {
  unread = $state(0);
  #stop: (() => void) | undefined;

  async refresh(): Promise<void> {
    this.unread = (await api.get<{ count: number }>("/api/v1/messages/unread")).count;
  }

  start(): void {
    if (this.#stop) return;
    void this.refresh().catch(() => undefined);
    this.#stop = live(() => this.refresh(), 60_000).stop;
  }

  stop(): void {
    this.#stop?.();
    this.#stop = undefined;
    this.unread = 0;
  }
}

export const inbox = new Inbox();
