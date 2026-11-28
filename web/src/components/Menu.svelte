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

  // 浮层（反馈 #113、#114）：移到 <body>（菜单在打开的对话框里时移到该对话框）下，用固定定位按按钮的位置摆放。
  // 放在原处时，可滚动表格会截断它，祖先的 transform（页面切换动画等）还会改变固定定位的参照，
  // 滚动后位置失准、点到的是别的元素（e2e 在长列表中复现）。移出后位置只取决于按钮的坐标。
  // 下方空间不够时向上展开；页面滚动或窗口变化时关闭。
  let pos = $state<{ top: number; left?: number; right?: number } | null>(null);
  let anchor: { top: number; left: number } | null = null;
  function place() {
    if (!button) return;
    const r = button.getBoundingClientRect();
    anchor = { top: r.top, left: r.left };
    const h = panel?.offsetHeight ?? 0;
    const below = r.bottom + 4;
    const top = h && below + h > window.innerHeight - 8 && r.top - 4 - h > 8 ? r.top - 4 - h : below;
    pos = align === "start" ? { top, left: r.left } : { top, right: window.innerWidth - r.right };
  }

  /** 把浮层挂到 <body> 或所在的打开的对话框（对话框在顶层，挂到 body 会被它盖住）。 */
  function portal(node: HTMLElement) {
    const host = root?.closest("dialog[open]") ?? document.body;
    host.appendChild(node);
    return { destroy: () => node.remove() };
  }

  function toggle() {
    open = !open;
    if (open) {
      place();
      queueMicrotask(() => {
        flushSync();
        place(); // 浮层渲染后知道高度，再决定向上还是向下
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
      const t = e.target as Node;
      if (!root?.contains(t) && !panel?.contains(t)) open = false;
    };
    // 按钮所在容器滚动导致按钮移位时关闭（浮层是固定定位的，不跟随内容移动）；
    // 浮层内部滚动、无关容器滚动，以及点击按钮前滚动定位的滞后 scroll 事件不会改变按钮坐标，不应误关。
    const onScroll = (e: Event) => {
      if (e.target instanceof Node && panel?.contains(e.target)) return;
      if (button && anchor) {
        const r = button.getBoundingClientRect();
        if (Math.abs(r.top - anchor.top) < 2 && Math.abs(r.left - anchor.left) < 2) return;
      }
      open = false;
    };
    const onResize = () => {
      open = false;
    };
    document.addEventListener("pointerdown", onDown);
    window.addEventListener("scroll", onScroll, true);
    window.addEventListener("resize", onResize);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", onResize);
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
    <div class="panel-menu" role="menu" tabindex="-1" bind:this={panel} use:portal onkeydown={onKey} style:top={pos ? pos.top + "px" : undefined} style:left={pos?.left !== undefined ? pos.left + "px" : undefined} style:right={pos?.right !== undefined ? pos.right + "px" : undefined}>
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
    /* 展开时淡入并轻微下移（反馈 #114：原来没有任何效果） */
    animation: menu-in var(--dur-fast) var(--ease);
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
  @keyframes menu-in {
    from {
      opacity: 0;
      transform: translateY(-4px) scale(0.98);
    }
  }
</style>
