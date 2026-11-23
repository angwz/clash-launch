<script lang="ts">
  // 下拉菜单：按钮 + 浮层。Esc 关闭并把焦点还给按钮，点击外部关闭，方向键在菜单项间移动。
  import { flushSync, type Snippet } from "svelte";
  import Icon, { type IconName } from "./Icon.svelte";

  let {
    label,
    icon = "more",
    align = "end",
    text = false,
    buttonClass = "btn ghost",
    items,
    trigger,
  }: { label: string; icon?: IconName; align?: "start" | "end"; text?: boolean; buttonClass?: string; items: Snippet<[() => void]>; trigger?: Snippet } = $props();

  let open = $state(false);
  let root: HTMLDivElement | undefined = $state();
  let button: HTMLButtonElement | undefined = $state();
  let panel: HTMLDivElement | undefined = $state();

  function close() {
    open = false;
    button?.focus();
  }

  // 浮层用固定定位，按按钮的位置摆放：放在可横向滚动的表格等裁剪容器里也不会被截断（反馈 #113）；
  // 下方空间不够时向上展开
  // 祖先元素的 transform、contain 等会让固定定位改以该祖先为参照；打开后量一次实际位置与期望位置的差，之后按差值修正
  let pos = $state<{ top: number; left?: number; right?: number } | null>(null);
  let shift = { x: 0, y: 0 };
  let want = { top: 0, x: 0 };
  function place() {
    if (!button) return;
    const r = button.getBoundingClientRect();
    const h = panel?.offsetHeight ?? 0;
    const below = r.bottom + 4;
    const top = h && below + h > window.innerHeight - 8 && r.top - 4 - h > 8 ? r.top - 4 - h : below;
    want = { top, x: align === "start" ? r.left : r.right };
    pos = align === "start" ? { top: top - shift.y, left: r.left - shift.x } : { top: top - shift.y, right: window.innerWidth - r.right + shift.x };
  }
  function calibrate() {
    flushSync(); // 位置的修改先写进 DOM，再量
    if (!panel) return;
    const a = panel.getBoundingClientRect();
    shift = { x: shift.x + (align === "start" ? a.left : a.right) - want.x, y: shift.y + a.top - want.top };
    place();
    flushSync();
  }

  function toggle() {
    open = !open;
    if (open) {
      shift = { x: 0, y: 0 };
      place();
      queueMicrotask(() => {
        flushSync();
        place(); // 浮层渲染后知道高度，再决定向上还是向下
        calibrate();
        focusItem(0);
      });
    }
  }

  function itemsOf(): HTMLElement[] {
    return panel ? [...panel.querySelectorAll<HTMLElement>("[role^=menuitem]")] : [];
  }

  function focusItem(i: number) {
    const all = itemsOf();
    if (!all.length) return;
    // 不滚动：浮层是固定定位的，聚焦时让浏览器滚动只会把页面带走
    all[(i + all.length) % all.length]?.focus({ preventScroll: true });
  }

  function onKey(e: KeyboardEvent) {
    if (!open) return;
    const all = itemsOf();
    const at = all.indexOf(document.activeElement as HTMLElement);
    if (e.key === "Escape") {
      e.preventDefault();
      close();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      focusItem(at + 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      focusItem(at - 1);
    } else if (e.key === "Tab") {
      open = false;
    }
  }

  $effect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (root && !root.contains(e.target as Node)) open = false;
    };
    // 页面滚动或窗口变化时关闭（浮层是固定定位的，不跟随内容移动）；浮层内部的滚动不算
    const onMove = (e: Event) => {
      if (e.target instanceof Node && panel?.contains(e.target)) return;
      open = false;
    };
    document.addEventListener("pointerdown", onDown);
    window.addEventListener("scroll", onMove, true);
    window.addEventListener("resize", onMove);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      window.removeEventListener("scroll", onMove, true);
      window.removeEventListener("resize", onMove);
    };
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="menu" bind:this={root} onkeydown={onKey}>
  <button
    bind:this={button}
    class={buttonClass}
    class:icon={!text}
    type="button"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={text ? undefined : label}
    title={text ? undefined : label}
    onclick={toggle}
  >
    {#if trigger}{@render trigger()}{:else}<Icon name={icon} />{/if}
    {#if text}<span>{label}</span><Icon name="chevron-down" size={14} />{/if}
  </button>
  {#if open}
    <div class="panel-menu" role="menu" bind:this={panel} style:top={pos ? pos.top + "px" : undefined} style:left={pos?.left !== undefined ? pos.left + "px" : undefined} style:right={pos?.right !== undefined ? pos.right + "px" : undefined}>
      {@render items(close)}
    </div>
  {/if}
</div>

<style>
  .menu {
    position: relative;
    display: inline-flex;
  }
  .panel-menu {
    position: fixed;
    z-index: 50;
    min-inline-size: 200px;
    padding: var(--space-1);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
  }
  .panel-menu :global(a),
  .panel-menu :global(button) {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    inline-size: 100%;
    block-size: 32px;
    padding-inline: var(--space-3);
    border: 0;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--fg);
    font: inherit;
    font-size: var(--text-md);
    text-align: start;
    cursor: pointer;
    white-space: nowrap;
  }
  .panel-menu :global(a:hover),
  .panel-menu :global(button:hover),
  .panel-menu :global(a:focus-visible),
  .panel-menu :global(button:focus-visible) {
    background: var(--bg-hover);
    text-decoration: none;
    outline: none;
  }
  .panel-menu :global(.danger) {
    color: var(--err);
  }
  .panel-menu :global(hr) {
    border: 0;
    border-block-start: 1px solid var(--border);
    margin-block: var(--space-1);
  }
</style>
