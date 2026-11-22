<script lang="ts">
  // 选择条件编辑器（O2，规范第 6.4 节）：把 groups.toml 中某个组的 [custom_groups.select] 做成可点选的标签。
  // 每次点选都直接改写 groups.toml 原文（只改动改过的键，见 lib/toml-select.ts），
  // 上方的代码编辑器与下方的预览随之更新；右侧列出该组在预览中的节点成员。
  import { flagOf, regionName } from "../../lib/regions";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { LIST_KEYS, SORTS, listGroups, readSelect, writeSelect, type Conditions, type ListKey, type Select } from "../../lib/toml-select";

  interface Facet {
    value: string;
    count: number;
  }
  interface Group {
    name: string;
    members: string[];
  }
  let {
    value = $bindable(""),
    groups,
    facets,
  }: { value?: string; groups: Group[]; facets: Record<ListKey, Facet[]> | undefined } = $props();

  const list = $derived(listGroups(value));
  let index = $state(0);
  // 组被删除或文本被整体替换后，选中的序号落回范围内
  const at = $derived(Math.min(index, Math.max(list.length - 1, 0)));
  const current = $derived(list[at]);
  const sel = $derived.by((): Select | null => {
    if (!current || current.readonly) return null;
    try {
      return readSelect(value, at);
    } catch {
      return null;
    }
  });

  function apply(change: (s: Select) => void) {
    if (!sel) return;
    const next: Select = structuredClone($state.snapshot(sel));
    change(next);
    value = writeSelect(value, at, next);
  }
  const target = (s: Select, ex: boolean): Conditions => (ex ? s.exclude : s);
  function add(key: ListKey, v: string, ex = false) {
    const x = v.trim();
    if (!x) return;
    apply((s) => {
      const c = target(s, ex);
      if (!c[key].includes(x)) c[key] = [...c[key], x];
    });
  }
  const remove = (key: ListKey, v: string, ex = false) => apply((s) => (target(s, ex)[key] = target(s, ex)[key].filter((y) => y !== v)));
  const setName = (v: string, ex = false) => apply((s) => (target(s, ex).name = v.trim()));
  const num = (v: string) => (v.trim() === "" || !Number.isFinite(Number(v)) ? null : Number(v));

  const label = (key: ListKey, v: string) => (key === "region" ? flagOf(v) + " " + regionName(v, i18n.intlLocale) : v);
  const suggest = (key: ListKey, chosen: string[]) => (facets?.[key] ?? []).filter((f) => !chosen.includes(f.value)).slice(0, 24);
  let drafts = $state<Record<string, string>>({});
  function onEnter(e: KeyboardEvent, id: string, key: ListKey, ex: boolean) {
    if (e.key !== "Enter") return;
    e.preventDefault();
    add(key, drafts[id] ?? "", ex);
    drafts[id] = "";
  }

  // 该组在预览中的节点成员（去掉其他组与内置出口）
  const BUILTIN = new Set(["DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"]);
  const names = $derived(new Set(groups.map((g) => g.name)));
  const nodes = $derived((groups.find((g) => g.name === current?.name)?.members ?? []).filter((m) => !names.has(m) && !BUILTIN.has(m)));
  const SHOW = 40;
</script>

{#snippet row(key: ListKey, c: Conditions, ex: boolean)}
  {@const id = (ex ? "x-" : "") + key}
  <fieldset class="row">
    <legend class="label">{t("rules.sel_" + key)}</legend>
    <div class="line">
      {#if c[key].length}
        <ul class="chips" aria-label={t("rules.sel_" + key)}>
          {#each c[key] as v (v)}<li><span class={key === "region" ? "" : "mono"}>{label(key, v)}</span><button type="button" aria-label={t("limits.remove", { item: v })} onclick={() => remove(key, v, ex)}>×</button></li>{/each}
        </ul>
      {:else}
        <span class="muted any">{t("rules.sel_any")}</span>
      {/if}
      <input class="input sm mono" bind:value={drafts[id]} placeholder={t("rules.sel_add")} aria-label={t("rules.sel_add_to", { what: t("rules.sel_" + key) })} onkeydown={(e) => onEnter(e, id, key, ex)} />
    </div>
    {#if suggest(key, c[key]).length}
      <div class="suggest">
        {#each suggest(key, c[key]) as f (f.value)}<button class="btn sm ghost" type="button" onclick={() => add(key, f.value, ex)}>+ {label(key, f.value)} <span class="muted">{f.count}</span></button>{/each}
      </div>
    {/if}
  </fieldset>
{/snippet}

<section class="sel panel" aria-labelledby="sel-h">
  <div class="head">
    <h2 id="sel-h">{t("rules.sel_title")}</h2>
    {#if list.length}
      <label class="pick">
        <span class="muted">{t("rules.sel_group")}</span>
        <select class="select" bind:value={index}>
          {#each list as g, i (i)}<option value={i}>{g.name || t("rules.sel_unnamed", { n: i + 1 })}</option>{/each}
        </select>
      </label>
    {/if}
  </div>
  <p class="hint">{t("rules.sel_hint")}</p>

  {#if !list.length}
    <p class="muted">{t("rules.sel_none")}</p>
  {:else if current?.readonly === "inline"}
    <p class="muted" role="note">{t("rules.sel_inline")}</p>
  {:else if current?.readonly === "complex" || !sel}
    <p class="muted" role="note">{t("rules.sel_complex", { reason: current?.detail ?? "" })}</p>
  {:else}
    <div class="body">
      <div class="conds">
        {#each LIST_KEYS as k (k)}{@render row(k, sel, false)}{/each}
        <fieldset class="row">
          <legend class="label">{t("rules.sel_name")}</legend>
          <input class="input mono" value={sel.name} placeholder="(?i)premium" aria-label={t("rules.sel_name")} onchange={(e) => setName(e.currentTarget.value)} />
        </fieldset>
        <div class="pair">
          <fieldset class="row">
            <legend class="label">{t("rules.sel_rate")}</legend>
            <div class="line">
              <input class="input sm num" type="number" min="0" step="0.1" value={sel.rateMin ?? ""} placeholder={t("rules.sel_rate_min")} aria-label={t("rules.sel_rate_min")} onchange={(e) => apply((s) => (s.rateMin = num(e.currentTarget.value)))} />
              <span class="muted">–</span>
              <input class="input sm num" type="number" min="0" step="0.1" value={sel.rateMax ?? ""} placeholder={t("rules.sel_rate_max")} aria-label={t("rules.sel_rate_max")} onchange={(e) => apply((s) => (s.rateMax = num(e.currentTarget.value)))} />
            </div>
          </fieldset>
          <fieldset class="row">
            <legend class="label">{t("rules.sel_sort")}</legend>
            <select class="select" value={sel.sort} aria-label={t("rules.sel_sort")} onchange={(e) => apply((s) => (s.sort = e.currentTarget.value))}>
              <option value="">{t("rules.sel_sort_default")}</option>
              {#each SORTS as o (o)}<option value={o}>{t("rules.sel_sort_" + o)}</option>{/each}
            </select>
          </fieldset>
        </div>
        <details class="ex" open={LIST_KEYS.some((k) => sel.exclude[k].length) || !!sel.exclude.name}>
          <summary>{t("rules.sel_exclude")}</summary>
          {#each LIST_KEYS as k (k)}{@render row(k, sel.exclude, true)}{/each}
          <fieldset class="row">
            <legend class="label">{t("rules.sel_name")}</legend>
            <input class="input mono" value={sel.exclude.name} placeholder="(?i)test" aria-label={t("rules.sel_exclude_name")} onchange={(e) => setName(e.currentTarget.value, true)} />
          </fieldset>
        </details>
      </div>
      <aside class="hits" aria-label={t("rules.sel_hits")}>
        <h3>{t("rules.sel_hits_n", { n: nodes.length })}</h3>
        <p class="hint">{t("rules.sel_hits_hint")}</p>
        <ul>
          {#each nodes.slice(0, SHOW) as m, i (i)}<li>{m}</li>{/each}
          {#if nodes.length > SHOW}<li class="muted">{t("rules.preview_more", { n: nodes.length - SHOW })}</li>{/if}
        </ul>
      </aside>
    </div>
  {/if}
</section>

<style>
  .sel {
    display: grid;
    gap: var(--space-3);
    margin-block-start: var(--space-6);
    padding: var(--space-4);
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  h2 {
    font-size: var(--text-lg);
  }
  h3 {
    font-size: var(--text-base);
  }
  .pick {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    white-space: nowrap;
  }
  .body {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(200px, 280px);
    gap: var(--space-5);
  }
  .conds {
    display: grid;
    gap: var(--space-3);
    align-content: start;
  }
  fieldset {
    display: grid;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    border: 0;
    min-inline-size: 0;
  }
  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .line .input.sm {
    inline-size: 12em;
  }
  .line .input.num {
    inline-size: 7em;
  }
  .any {
    font-size: var(--text-sm);
  }
  .pair {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-5);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .chips li {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding-block: 2px;
    padding-inline: 10px 4px;
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-sm);
  }
  .chips button {
    inline-size: 22px;
    block-size: 22px;
    border: 0;
    border-radius: 50%;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .chips button:hover {
    background: var(--bg-raised);
  }
  .suggest {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .ex {
    display: grid;
    gap: var(--space-3);
  }
  .ex summary {
    cursor: pointer;
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .ex[open] {
    padding-block-start: var(--space-1);
  }
  .hits {
    display: grid;
    gap: var(--space-2);
    align-content: start;
    padding-inline-start: var(--space-4);
    border-inline-start: 1px solid var(--border);
  }
  .hits ul {
    display: grid;
    gap: 2px;
    font-size: var(--text-sm);
    max-block-size: 28em;
    overflow: auto;
  }
  @media (max-width: 767px) {
    .body {
      grid-template-columns: minmax(0, 1fr);
    }
    .hits {
      padding-inline-start: 0;
      padding-block-start: var(--space-3);
      border-inline-start: 0;
      border-block-start: 1px solid var(--border);
    }
  }
</style>
