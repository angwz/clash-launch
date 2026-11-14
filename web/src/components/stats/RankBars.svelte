<script lang="ts">
  // 排行（反馈 #66）：横向条形，长度与数值成比例，数值以文字写出；单一色相。
  import { i18n } from "../../lib/i18n/i18n.svelte";
  let { items, empty }: { items: { label: string; count: number; mono?: boolean }[]; empty: string } = $props();
  const max = $derived(Math.max(1, ...items.map((x) => x.count)));
</script>

{#if items.length}
  <ol class="rank">
    {#each items as x, i (i)}
      <li>
        <span class="lbl" class:mono={x.mono} title={x.label}>{x.label}</span>
        <span class="track"><span class="fill" style:inline-size={`${(x.count / max) * 100}%`}></span></span>
        <span class="num">{i18n.number(x.count)}</span>
      </li>
    {/each}
  </ol>
{:else}
  <p class="muted empty">{empty}</p>
{/if}

<style>
  .rank {
    display: grid;
    gap: 6px;
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: grid;
    grid-template-columns: minmax(0, 11rem) minmax(0, 1fr) auto;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .lbl {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .track {
    block-size: 8px;
    border-radius: 4px;
    background: var(--bg-sunken);
    overflow: hidden;
  }
  .fill {
    display: block;
    block-size: 100%;
    border-radius: 4px;
    background: var(--accent);
  }
  .num {
    min-inline-size: 3ch;
    color: var(--fg-muted);
    text-align: end;
    font-variant-numeric: tabular-nums;
  }
  .empty {
    font-size: var(--text-sm);
  }
</style>
