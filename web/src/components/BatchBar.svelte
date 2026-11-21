<script lang="ts">
  // 批量操作条（需求 R35）：选中条目后出现在列表上方，显示数量、操作与「取消选择」。
  import type { Snippet } from "svelte";
  import type { Selection } from "../lib/selection.svelte";
  import { t } from "../lib/i18n/i18n.svelte";

  let { sel, children }: { sel: Selection; children: Snippet } = $props();
</script>

{#if sel.size}
  <div class="batch panel" role="toolbar" aria-label={t("batch.toolbar")}>
    <span class="n">{t("batch.selected", { n: sel.size })}</span>
    <div class="acts">{@render children()}</div>
    <button class="btn sm ghost" type="button" onclick={() => sel.clear()}>{t("batch.clear")}</button>
  </div>
{/if}

<style>
  .batch {
    position: sticky;
    z-index: 5;
    inset-block-start: var(--space-2);
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-block-end: var(--space-3);
    padding-block: var(--space-2);
    padding-inline: var(--space-4) var(--space-2);
    background: var(--accent-soft);
    border-color: var(--accent);
  }
  .n {
    font-weight: 560;
    color: var(--accent-fg);
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-inline-end: auto;
  }
</style>
