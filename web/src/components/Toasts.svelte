<script lang="ts">
  import { t } from "../lib/i18n/i18n.svelte";
  import { toasts } from "../lib/toast.svelte";
  import Emoji from "./Emoji.svelte";
  import Icon from "./Icon.svelte";
</script>

<div class="toasts" aria-live="polite" aria-atomic="false">
  {#each toasts.items as x (x.id)}
    <div class="toast" data-kind={x.kind} role={x.kind === "err" ? "alert" : "status"}>
      {#if x.emoji || x.kind !== "info"}<Emoji name={x.emoji ?? (x.kind === "ok" ? "check" : "cross")} size={18} />{:else}<Icon name="info" />{/if}
      <span class="text">{x.text}</span>
      {#if x.action}
        <button class="btn ghost sm action" type="button" onclick={() => { toasts.dismiss(x.id); x.action!.run(); }}>{x.action.label}</button>
      {/if}
      <button class="btn ghost icon sm" type="button" aria-label={t("common.close")} onclick={() => toasts.dismiss(x.id)}>
        <Icon name="x" size={14} />
      </button>
    </div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    z-index: 90;
    inset-block-end: var(--space-4);
    inset-inline-end: var(--space-4);
    display: grid;
    gap: var(--space-2);
    max-inline-size: min(420px, calc(100vw - 2 * var(--space-4)));
  }
  .toast {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: var(--space-2);
    padding-inline: var(--space-3) var(--space-1);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
    font-size: var(--text-md);
    animation: rise var(--dur) var(--ease);
  }
  .action {
    color: var(--accent-fg);
    font-weight: 600;
  }
  .toast[data-kind="ok"] > :global(.icon) {
    color: var(--ok);
  }
  .toast[data-kind="err"] > :global(.icon) {
    color: var(--err);
  }
  .toast[data-kind="info"] > :global(.icon) {
    color: var(--info);
  }
  .text {
    flex: 1;
  }
  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>
