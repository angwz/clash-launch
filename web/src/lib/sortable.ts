// 拖动排序（反馈 #73）：Svelte action，挂在列表或网格的容器上。
//
// 约定：每一项带 data-sort-index（当前位置），项内的拖动手柄带 data-sort-handle。
// - 指针（鼠标、触屏、触控笔）按住手柄拖动：经过其他项时立即调用 onmove，列表实时重排；
//   松手时调用 ondone 保存。只有手柄能开始拖动，卡片上的按钮、下拉框照常使用。
// - 键盘：手柄获得焦点后，方向键上/左前移、下/右后移，每次移动后立即保存，焦点跟随该项。
// 不使用 HTML5 拖放：它在触屏上不可用，且网格布局中的落点判断不稳定。

export interface SortableOptions {
  onmove: (from: number, to: number) => void;
  ondone: () => void;
}

export function sortable(node: HTMLElement, opts: SortableOptions) {
  let o = opts;
  let from = -1;
  let pointer = -1;
  let moved = false;

  const indexOf = (el: Element | null): number => {
    const item = el?.closest<HTMLElement>("[data-sort-index]");
    if (!item || !node.contains(item)) return -1;
    return Number(item.dataset.sortIndex);
  };
  const itemAt = (i: number) => node.querySelector<HTMLElement>(`[data-sort-index="${i}"]`);

  function down(e: PointerEvent) {
    const handle = (e.target as Element).closest("[data-sort-handle]");
    if (!handle || !node.contains(handle) || e.button !== 0) return;
    from = indexOf(handle);
    if (from < 0) return;
    e.preventDefault();
    pointer = e.pointerId;
    moved = false;
    node.setPointerCapture(e.pointerId);
    itemAt(from)?.classList.add("sorting");
    node.classList.add("sort-active");
  }

  function move(e: PointerEvent) {
    if (e.pointerId !== pointer || from < 0) return;
    const to = indexOf(document.elementFromPoint(e.clientX, e.clientY));
    if (to < 0 || to === from) return;
    itemAt(from)?.classList.remove("sorting");
    o.onmove(from, to);
    from = to;
    moved = true;
    // 重排后同一位置是被拖动的项；等 DOM 更新后再标记
    queueMicrotask(() => itemAt(from)?.classList.add("sorting"));
  }

  function up(e: PointerEvent) {
    if (e.pointerId !== pointer) return;
    itemAt(from)?.classList.remove("sorting");
    node.classList.remove("sort-active");
    if (node.hasPointerCapture(e.pointerId)) node.releasePointerCapture(e.pointerId);
    pointer = -1;
    from = -1;
    if (moved) o.ondone();
  }

  function key(e: KeyboardEvent) {
    const handle = (e.target as Element).closest<HTMLElement>("[data-sort-handle]");
    if (!handle || !node.contains(handle)) return;
    const i = indexOf(handle);
    const n = node.querySelectorAll("[data-sort-index]").length;
    const d = e.key === "ArrowUp" || e.key === "ArrowLeft" ? -1 : e.key === "ArrowDown" || e.key === "ArrowRight" ? 1 : 0;
    if (!d || i < 0 || i + d < 0 || i + d >= n) return;
    e.preventDefault();
    o.onmove(i, i + d);
    o.ondone();
    // 等界面重排后把焦点放回被移动项的手柄
    requestAnimationFrame(() => itemAt(i + d)?.querySelector<HTMLElement>("[data-sort-handle]")?.focus());
  }

  node.addEventListener("pointerdown", down);
  node.addEventListener("pointermove", move);
  node.addEventListener("pointerup", up);
  node.addEventListener("pointercancel", up);
  node.addEventListener("keydown", key);
  return {
    update(next: SortableOptions) {
      o = next;
    },
    destroy() {
      node.removeEventListener("pointerdown", down);
      node.removeEventListener("pointermove", move);
      node.removeEventListener("pointerup", up);
      node.removeEventListener("pointercancel", up);
      node.removeEventListener("keydown", key);
    },
  };
}

/** 把 from 位置的元素移到 to，返回新数组。纯函数。 */
export function moveItem<T>(list: readonly T[], from: number, to: number): T[] {
  const out = [...list];
  const [x] = out.splice(from, 1);
  out.splice(to, 0, x as T);
  return out;
}
