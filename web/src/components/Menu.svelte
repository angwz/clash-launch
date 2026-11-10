<script lang="ts">
  // 下拉菜单：按钮 + 浮层。Esc 关闭并把焦点还给按钮，点击外部关闭，方向键在菜单项间移动。
  import type { Snippet } from "svelte";
  import Icon, { type IconName } from "./Icon.svelte";

  let {
    label,
    icon = "more",
    align = "end",
    text = false,
    items,
  }: { label: string; icon?: IconName; align?: "start" | "end"; text?: boolean; items: Snippet<[() => void]> } = $props();

  let open = $state(false);
  let root: HTMLDivElement | undefined = $state();
  let button: HTMLButtonElement | undefined = $state();
  let panel: HTMLDivElement | undefined = $state();

  function close() {
    open = false;
    button?.focus();
  }

  function toggle() {
    open = !open;
    if (open) queueMicrotask(() => focusItem(0));
  }

  function itemsOf(): HTMLElement[] {
    return panel ? [...panel.querySelectorAll<HTMLElement>("[role^=menuitem]")] : [];
  }

  function focusItem(i: number) {
    const all = itemsOf();
    if (!all.length) return;
    all[(i + all.length) % all.length]?.focus();
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
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="menu" bind:this={root} onkeydown={onKey}>
  <button
    bind:this={button}
    class="btn ghost"
    class:icon={!text}
    type="button"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={text ? undefined : label}
    title={text ? undefined : label}
    onclick={toggle}
  >
    <Icon name={icon} />
    {#if text}<span>{label}</span><Icon name="chevron-down" size={14} />{/if}
  </button>
  {#if open}
    <div class="panel-menu" class:start={align === "start"} role="menu" bind:this={panel}>
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
    position: absolute;
    z-index: 50;
    inset-block-start: calc(100% + 4px);
    inset-inline-end: 0;
    min-inline-size: 200px;
    padding: var(--space-1);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
  }
  .panel-menu.start {
    inset-inline-end: auto;
    inset-inline-start: 0;
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
