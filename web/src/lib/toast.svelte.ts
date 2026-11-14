// 轻量通知：保存成功、复制、错误。错误需要手动关闭，其余 4 秒后消失；带操作（如「撤销」）的停留 8 秒。
// key 相同的通知互相替换而不是堆叠（例如连续的自动保存只保留最新一条）。
export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  id: number;
  kind: "ok" | "err" | "info";
  text: string;
  action?: ToastAction;
  key?: string;
}

class Toasts {
  items = $state<Toast[]>([]);
  #n = 0;
  #timers = new Map<number, ReturnType<typeof setTimeout>>();

  push(kind: Toast["kind"], text: string, opts: { action?: ToastAction; key?: string } = {}): number {
    if (opts.key) for (const x of this.items.filter((x) => x.key === opts.key)) this.dismiss(x.id);
    // 同样的错误不重复堆叠
    if (kind === "err" && this.items.some((x) => x.kind === "err" && x.text === text)) return 0;
    const id = ++this.#n;
    this.items = [...this.items, { id, kind, text, ...opts }];
    if (kind !== "err") this.#timers.set(id, setTimeout(() => this.dismiss(id), opts.action ? 8000 : 4000));
    return id;
  }

  dismiss(id: number): void {
    clearTimeout(this.#timers.get(id));
    this.#timers.delete(id);
    this.items = this.items.filter((x) => x.id !== id);
  }
}

export const toasts = new Toasts();
