<script lang="ts">
  // 星期 × 小时热力图（反馈 #74）：单一色相、按数值深浅（顺序色阶），格子之间留 2px 缝；
  // 悬停任意格子显示「星期几 几点 · 多少次」，同一份数据以 aria-label 给出最忙的时段，供读屏器使用。
  import { i18n, t } from "../../lib/i18n/i18n.svelte";

  let { heat }: { heat: number[][] } = $props(); // 7 行（星期一起）× 24 列

  const max = $derived(Math.max(1, ...heat.flat()));
  let hover = $state<[number, number] | null>(null);
  // 星期名称按当前语言：2024-01-01 是星期一
  const dayName = (i: number) => new Intl.DateTimeFormat(i18n.intlLocale, { weekday: "short", timeZone: "UTC" }).format(new Date(Date.UTC(2024, 0, 1 + i)));
  const busiest = $derived.by(() => {
    let best: [number, number, number] = [0, 0, -1];
    heat.forEach((row, d) => row.forEach((n, h) => n > best[2] && (best = [d, h, n])));
    return best;
  });
  // 0 用最浅的底色；其余按比例映射到 15%–100% 的强度，避免小数值几乎看不见
  const level = (n: number) => (n ? 0.15 + 0.85 * (n / max) : 0);
</script>

<figure class="heat" aria-label={busiest[2] > 0 ? t("analytics.heat_aria", { day: dayName(busiest[0]), hour: busiest[1], n: busiest[2] }) : t("stats.none")}>
  <div class="grid" role="presentation" onpointerleave={() => (hover = null)}>
    <span></span>
    {#each Array(24) as _, h (h)}<span class="hour">{h % 3 === 0 ? h : ""}</span>{/each}
    {#each heat as row, d (d)}
      <span class="day">{dayName(d)}</span>
      {#each row as n, h (h)}
        <span class="cell" class:zero={!n} class:on={hover?.[0] === d && hover?.[1] === h} style:--l={level(n)} onpointerenter={() => (hover = [d, h])} role="presentation"></span>
      {/each}
    {/each}
  </div>
  <figcaption class="cap" aria-live="polite">
    {#if hover}
      <span class="muted">{dayName(hover[0])} {String(hover[1]).padStart(2, "0")}:00–{String(hover[1] + 1).padStart(2, "0")}:00</span>
      <strong>{t("analytics.n_requests", { n: heat[hover[0]]![hover[1]]! })}</strong>
    {:else if busiest[2] > 0}
      <span class="muted">{t("analytics.busiest")}</span>
      <strong>{dayName(busiest[0])} {String(busiest[1]).padStart(2, "0")}:00 · {t("analytics.n_requests", { n: busiest[2] })}</strong>
    {/if}
  </figcaption>
</figure>

<style>
  .heat {
    margin: 0;
    display: grid;
    gap: var(--space-2);
  }
  .grid {
    display: grid;
    grid-template-columns: auto repeat(24, minmax(0, 1fr));
    gap: 2px;
    align-items: center;
  }
  .hour,
  .day {
    color: var(--fg-subtle);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .day {
    padding-inline-end: var(--space-2);
    white-space: nowrap;
  }
  /* 固定高度的扁格：宽屏上不会放大成大方块，7 行一眼看全 */
  .cell {
    block-size: 18px;
    border-radius: 3px;
    background: color-mix(in srgb, var(--accent) calc(var(--l) * 100%), var(--bg-sunken));
  }
  .cell.zero {
    background: var(--bg-sunken);
  }
  .cell.on {
    outline: 2px solid var(--fg);
    outline-offset: 1px;
  }
  .cap {
    display: flex;
    gap: var(--space-2);
    min-block-size: 1.4em;
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }
</style>
