<script lang="ts">
  // 按天的柱状图（反馈 #66）：单一序列、单一色相（强调色），柱顶 2px 圆角、柱间留缝，
  // 底线与刻度用最弱的边框色；最高值与首尾日期直接标注，不在每根柱上写数字。
  // 每一天有覆盖整列的悬停区域，提示框给出日期与数值；同样的数据在页面上以文字给出合计，读屏器读 aria-label。
  import { i18n, t } from "../../lib/i18n/i18n.svelte";

  let { days, values, label, height = 96 }: { days: number[]; values: number[]; label: string; height?: number } = $props();

  const W = 600; // viewBox 宽度；实际宽度随容器缩放
  const top = 14; // 为最高值标注留出的空间
  const bottom = 16; // 为日期标注留出的空间
  const max = $derived(Math.max(1, ...values));
  const n = $derived(values.length);
  const step = $derived(W / Math.max(1, n));
  const bw = $derived(Math.max(1, step - 2)); // 柱间 2px 缝
  const plotH = $derived(height - top - bottom);
  const peak = $derived(values.indexOf(Math.max(...values)));
  let hover = $state(-1);

  const dateOf = (d: number) => new Date(Date.UTC(Math.floor(d / 10000), Math.floor((d % 10000) / 100) - 1, d % 100));
  const fmt = (d: number) => dateOf(d).toLocaleDateString(i18n.intlLocale, { month: "numeric", day: "numeric", timeZone: "UTC" });
  const total = $derived(values.reduce((a, b) => a + b, 0));

  // 柱：从底线向上，顶部 2px 圆角（只圆顶部，底部贴住底线）
  function bar(i: number): string {
    const h = values[i]! > 0 ? Math.max(2, (values[i]! / max) * plotH) : 0;
    if (!h) return "";
    const x = i * step + 1;
    const y = top + plotH - h;
    const r = Math.min(2, bw / 2, h);
    return `M${x},${top + plotH}V${y + r}Q${x},${y} ${x + r},${y}H${x + bw - r}Q${x + bw},${y} ${x + bw},${y + r}V${top + plotH}Z`;
  }
</script>

<figure class="daybars">
  <svg viewBox={`0 0 ${W} ${height}`} preserveAspectRatio="none" role="img" aria-label={t("stats.chart_aria", { label, days: n, total })} onpointerleave={() => (hover = -1)}>
    <line class="base" x1="0" x2={W} y1={top + plotH + 0.5} y2={top + plotH + 0.5} />
    {#each values as _, i (i)}
      <path class="bar" class:dim={hover >= 0 && hover !== i} d={bar(i)} />
      <!-- 悬停区域覆盖整列，比柱本身大，细柱也容易指到 -->
      <rect class="hit" x={i * step} y="0" width={step} height={height} onpointerenter={() => (hover = i)} role="presentation" />
    {/each}
  </svg>
  <div class="labels" aria-hidden="true">
    <span>{n ? fmt(days[0]!) : ""}</span>
    <span>{n ? fmt(days[n - 1]!) : ""}</span>
  </div>
  {#if peak >= 0 && values[peak]! > 0}
    <span class="peak" style:inset-inline-start={`${((peak + 0.5) / n) * 100}%`} aria-hidden="true">{values[peak]}</span>
  {/if}
  {#if hover >= 0}
    <div class="tip" style:inset-inline-start={`${((hover + 0.5) / n) * 100}%`} role="status">
      <span class="muted">{fmt(days[hover]!)}</span>
      <strong>{values[hover]}</strong>
    </div>
  {/if}
</figure>

<style>
  .daybars {
    position: relative;
    margin: 0;
  }
  svg {
    display: block;
    inline-size: 100%;
    block-size: 96px;
  }
  .base {
    stroke: var(--border);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }
  .bar {
    fill: var(--accent);
    transition: opacity var(--dur-fast) var(--ease);
  }
  .bar.dim {
    opacity: 0.45;
  }
  .hit {
    fill: transparent;
  }
  .labels {
    display: flex;
    justify-content: space-between;
    margin-block-start: -14px;
    color: var(--fg-subtle);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    pointer-events: none;
  }
  .peak {
    position: absolute;
    inset-block-start: -2px;
    transform: translateX(-50%);
    color: var(--fg-muted);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
    pointer-events: none;
  }
  .tip {
    position: absolute;
    inset-block-start: -30px;
    transform: translateX(-50%);
    display: flex;
    gap: 6px;
    padding-block: 3px;
    padding-inline: 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-sm);
    font-size: var(--text-xs);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
    pointer-events: none;
  }
  :global([dir="rtl"]) .peak,
  :global([dir="rtl"]) .tip {
    transform: translateX(50%);
  }
</style>
