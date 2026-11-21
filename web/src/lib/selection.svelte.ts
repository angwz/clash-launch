// 列表多选（需求 R35）：节点源、配置、短链接共用。只记 ID；列表变化后用 keep 去掉已不存在的。
export class Selection {
  ids = $state<ReadonlySet<string>>(new Set());

  get size(): number {
    return this.ids.size;
  }
  has(id: string): boolean {
    return this.ids.has(id);
  }
  toggle(id: string, on = !this.ids.has(id)): void {
    const s = new Set(this.ids);
    if (on) s.add(id);
    else s.delete(id);
    this.ids = s;
  }
  /** 全选或全不选 visible 中的条目。 */
  setAll(visible: string[], on: boolean): void {
    this.ids = on ? new Set(visible) : new Set();
  }
  /** 只保留仍在 visible 中的 ID。 */
  keep(visible: string[]): void {
    const v = new Set(visible);
    if ([...this.ids].some((id) => !v.has(id))) this.ids = new Set([...this.ids].filter((id) => v.has(id)));
  }
  clear(): void {
    if (this.ids.size) this.ids = new Set();
  }
}
