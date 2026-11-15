<script lang="ts" module>
  export interface Summary {
    nodes: number;
    types: { type: string; count: number }[];
    upload?: number;
    download?: number;
    total?: number;
    expire?: string;
  }
</script>

<script lang="ts">
  // 节点概况（反馈 #76）：可用节点数与协议分布；订阅另显示提供方给出的已用流量与到期时间。
  // 流量用量超过 90% 或 7 天内到期时以警示色显示。
  import { i18n, t } from "../lib/i18n/i18n.svelte";

  let { summary }: { summary: Summary } = $props();

  const used = $derived((summary.upload ?? 0) + (summary.download ?? 0));
  const ratio = $derived(summary.total ? Math.min(1, used / summary.total) : 0);
  const daysLeft = $derived(summary.expire ? Math.ceil((new Date(summary.expire).getTime() - Date.now()) / 86_400_000) : null);
  const gib = (n: number) => (n >= 1 << 30 ? (n / (1 << 30)).toFixed(n >= 100 * (1 << 30) ? 0 : 1) + " GiB" : (n / (1 << 20)).toFixed(0) + " MiB");
</script>

<span class="sum">
  <span class:none={!summary.nodes}>{t("summary.nodes", { n: summary.nodes })}</span>
  {#each summary.types.slice(0, 4) as x (x.type)}<span class="type mono">{x.type} {x.count}</span>{/each}
  {#if summary.types.length > 4}<span class="muted">…</span>{/if}
  {#if summary.total}
    <span class="traffic" class:warn={ratio >= 0.9} title={t("summary.traffic_hint")}>
      <span class="bar"><span style:inline-size={`${ratio * 100}%`}></span></span>
      {t("summary.used", { used: gib(used), total: gib(summary.total) })}
    </span>
  {/if}
  {#if daysLeft !== null}
    <span class:warn={daysLeft <= 7}>{daysLeft > 0 ? t("summary.expires", { date: i18n.date(summary.expire).split(" ")[0], n: daysLeft }) : t("summary.expired")}</span>
  {/if}
</span>

<style>
  .sum {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 2px var(--space-2);
    margin-block-start: 2px;
    color: var(--fg-muted);
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
  }
  .none {
    color: var(--warn);
  }
  .type {
    padding-inline: 5px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
  }
  .traffic {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }
  .bar {
    display: inline-block;
    inline-size: 48px;
    block-size: 4px;
    border-radius: 2px;
    background: var(--bg-sunken);
    overflow: hidden;
  }
  .bar span {
    display: block;
    block-size: 100%;
    background: var(--accent);
  }
  .warn {
    color: var(--warn);
  }
  .warn .bar span {
    background: var(--warn);
  }
</style>
