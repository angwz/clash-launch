<script lang="ts">
  // 模态对话框：原生 <dialog>（焦点约束与 Esc 由浏览器处理）。关闭时把焦点还给打开前的元素。
  import type { Snippet } from "svelte";
  import { t } from "../lib/i18n/i18n.svelte";
  import Icon from "./Icon.svelte";

  let {
    open = $bindable(false),
    title,
    wide = false,
    children,
    footer,
  }: { open?: boolean; title: string; wide?: boolean; children: Snippet; footer?: Snippet } = $props();

  let el: HTMLDialogElement | undefined = $state();
  let opener: Element | null = null;

  $effect(() => {
    if (!el) return;
    if (open && !el.open) {
      opener = document.activeElement;
      el.showModal();
    } else if (!open && el.open) {
      el.close();
    }
  });

  function onClose() {
    open = false;
    if (opener instanceof HTMLElement) opener.focus();
  }
</script>

<dialog bind:this={el} class:wide onclose={onClose} aria-labelledby="dlg-title">
  <header>
    <h2 id="dlg-title">{title}</h2>
    <button class="btn ghost icon sm" type="button" aria-label={t("common.close")} onclick={() => (open = false)}>
      <Icon name="x" size={15} />
    </button>
  </header>
  <div class="body">
    {@render children()}
  </div>
  {#if footer}
    <footer>{@render footer()}</footer>
  {/if}
</dialog>

<style>
  dialog {
    inline-size: min(480px, calc(100vw - 2 * var(--space-4)));
    max-block-size: calc(100dvh - 2 * var(--space-8));
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-lg);
  }
  dialog.wide {
    inline-size: min(720px, calc(100vw - 2 * var(--space-4)));
  }
  dialog::backdrop {
    background: var(--scrim);
  }
  dialog[open] {
    display: flex;
    flex-direction: column;
    animation: pop var(--dur) var(--ease);
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding-block: var(--space-4) var(--space-2);
    padding-inline: var(--space-5) var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
    font-weight: 620;
  }
  .body {
    padding-block: var(--space-2) var(--space-4);
    padding-inline: var(--space-5);
    overflow: auto;
  }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding-block: var(--space-3) var(--space-4);
    padding-inline: var(--space-5);
    border-block-start: 1px solid var(--border);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(4px) scale(0.99);
    }
  }
</style>
