// 轻量通知：保存成功、复制、错误。错误需要手动关闭，其余 4 秒后消失。
export interface Toast {
  id: number;
  kind: "ok" | "err" | "info";
  text: string;
}

class Toasts {
  items = $state<Toast[]>([]);
  #n = 0;

  push(kind: Toast["kind"], text: string): void {
    const id = ++this.#n;
    this.items = [...this.items, { id, kind, text }];
    if (kind !== "err") setTimeout(() => this.dismiss(id), 4000);
  }

  dismiss(id: number): void {
    this.items = this.items.filter((x) => x.id !== id);
  }
}

export const toasts = new Toasts();
