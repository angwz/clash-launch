<script lang="ts">
  // 足迹（需求 R10）：不会重复、日后回看有意义的事，按时间先后排成时间线，按年分组。
  // 「补记」的足迹是上线时由已有数据推算的，时间可能不精确。周年由加入日期推算，不单独存。
  // 管理员另有一列「站点」：上线日、第一位注册用户、用户数。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { footprintText, type Footprint } from "../../lib/footprints";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { live } from "../../lib/live";

  interface Resp {
    items: Footprint[];
    site?: Footprint[];
    joinedAt: string;
  }
  let data = $state<Resp | null>(null);
  let error = $state("");

  async function load() {
    try {
      data = await api.get<Resp>("/api/v1/account/footprints");
      error = "";
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());
  $effect(() => live(load, 300_000, ["footprint"]).stop);

  // 新的在前，按年分组
  function byYear(items: Footprint[]): [string, Footprint[]][] {
    const groups = new Map<string, Footprint[]>();
    for (const f of [...items].reverse()) {
      const y = yearOf(f.at);
      if (!groups.has(y)) groups.set(y, []);
      groups.get(y)!.push(f);
    }
    return [...groups];
  }
  // 只显示日期（按账户时区）：足迹回看的是哪一天，不需要几点几分
  function day(iso: string): string {
    try {
      return new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeZone: i18n.timeZone }).format(new Date(iso));
    } catch {
      return new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium" }).format(new Date(iso));
    }
  }
  // 年份同样按账户时区
  function yearOf(iso: string): string {
    try {
      return new Intl.DateTimeFormat("en", { year: "numeric", timeZone: i18n.timeZone }).format(new Date(iso));
    } catch {
      return String(new Date(iso).getFullYear());
    }
  }
  const days = $derived(data ? Math.max(1, Math.floor((Date.now() - new Date(data.joinedAt).getTime()) / 86_400_000) + 1) : 0);
</script>

<section class="panel box" aria-labelledby="ac-fp">
  <h2 id="ac-fp">{t("footprint.title")}</h2>
  {#if error}
    <p class="err">{error}</p>
  {:else if data}
    <p class="muted">{t("footprint.since", { date: day(data.joinedAt), n: i18n.number(days) })}</p>
    {#each [{ title: "", items: data.items }, ...(data.site ? [{ title: t("footprint.site"), items: data.site }] : [])] as col (col.title)}
      {#if col.title}<h3>{col.title}</h3>{/if}
      {#if col.items.length === 0}
        <p class="muted">{t("footprint.empty")}</p>
      {:else}
        {#each byYear(col.items) as [year, items] (year)}
          <div class="year">
            <span class="y">{year}</span>
            <ol>
              {#each items as f (f.kind + "\u0000" + (f.subject ?? ""))}
                <li>
                  <time datetime={f.at}>{day(f.at)}</time>
                  <span class="text">{footprintText(f)}</span>
                  {#if f.backfilled}<span class="tag" title={t("footprint.backfilled_hint")}>{t("footprint.backfilled")}</span>{/if}
                </li>
              {/each}
            </ol>
          </div>
        {/each}
      {/if}
    {/each}
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  h3 {
    margin-block-start: var(--space-2);
    font-size: var(--text-md);
  }
  .year {
    display: grid;
    grid-template-columns: 3.5em 1fr;
    gap: var(--space-3);
  }
  .y {
    color: var(--fg-subtle);
    font-variant-numeric: tabular-nums;
    font-size: var(--text-sm);
    padding-block-start: 2px;
  }
  ol {
    display: grid;
    gap: var(--space-2);
    padding-inline-start: var(--space-3);
    border-inline-start: 2px solid var(--border);
  }
  li {
    position: relative;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 2px var(--space-3);
  }
  li::before {
    content: "";
    position: absolute;
    inset-inline-start: calc(-1 * var(--space-3) - 5px);
    inset-block-start: 0.45em;
    inline-size: 8px;
    block-size: 8px;
    border-radius: 50%;
    background: var(--accent);
  }
  time {
    min-inline-size: 7em;
    color: var(--fg-muted);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }
  .text {
    flex: 1;
    min-inline-size: 12em;
  }
  .tag {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding-inline: 4px;
  }
  .err {
    color: var(--err);
  }
</style>
