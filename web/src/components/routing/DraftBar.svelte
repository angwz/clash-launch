<script lang="ts">
  // 草稿栏（routing 第 11.2 节）：列出尚未应用的改动；「应用」写入方案文档的 routing 段，之后照常自动保存与检查。
  import { t } from "../../lib/i18n/i18n.svelte";
  import type { Change } from "../../lib/routing/draft";

  let { changes, own, error, onCancel, onApply }: { changes: Change[]; own: boolean; error: string; onCancel: () => void; onApply: () => void } = $props();
</script>

<div class="draft" role="region" aria-label={t("routing.draft.title")}>
  <div class="sum">
    <strong>{t("routing.draft.count", { n: changes.length })}</strong>
    {#if !own}<span class="hint">{t("routing.draft.start_own")}</span>{/if}
    <ul>
      {#each changes.slice(0, 8) as c, i (i)}<li>{t("routing.change." + c.key, c.params)}</li>{/each}
      {#if changes.length > 8}<li>{t("routing.draft.more", { n: changes.length - 8 })}</li>{/if}
    </ul>
    {#if error}<p class="err" role="alert">{error}</p>{/if}
  </div>
  <div class="actions">
    <button class="btn" type="button" onclick={onCancel}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" onclick={onApply}>{t("routing.draft.apply")}</button>
  </div>
</div>

<style>
  .draft {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    align-items: flex-start;
    padding: var(--space-3) var(--space-4);
    border-block-start: 1px solid var(--border);
    background: var(--warn-soft);
  }
  .sum {
    flex: 1 1 260px;
    min-inline-size: 0;
    font-size: var(--text-sm);
    color: var(--fg);
    display: grid;
    gap: 4px;
  }
  .hint {
    font-size: var(--text-xs);
    color: var(--fg-muted);
  }
  ul {
    margin: 0;
    padding-inline-start: 18px;
    font-size: var(--text-xs);
    color: var(--fg-muted);
  }
  .err {
    color: var(--err);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
