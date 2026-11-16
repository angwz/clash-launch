<script lang="ts">
  // 选中项的详情与无鼠标编辑（routing 第 11.1、11.2、11.3 节）：规则或合集、策略组、出口。
  // 编辑都经 onEdit 交给分流视图，变成草稿；这里不直接改方案文档。
  import { stringify } from "yaml";
  import { catalog } from "../../lib/catalog.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import type { Graph } from "../../lib/routing/layout";
  import type { ViewPlan } from "../../lib/routing/plan";
  import { addMember, GROUP_TYPES, move, PARAM_KEYS, removeMember, setFirst, setParam, setTarget, splitOut, unitOf, type GroupType, type Routing } from "../../lib/routing/draft";
  import { builtinLabel, exitLabel, fmtCount, ruleName, typeLabel } from "../../lib/routing/labels";

  let {
    graph,
    plan,
    selected,
    routing,
    editable,
    onEdit,
    onType,
    onTrace,
    onClose,
  }: {
    graph: Graph;
    plan: ViewPlan;
    selected: string;
    /** 当前草稿或已应用的 routing；跟随上游且还没有改动时为 null（第一次编辑时由视图取得） */
    routing: Routing | null;
    editable: boolean;
    onEdit: (op: (r: Routing) => Routing | null, fail?: string) => void;
    onType: (group: string, type: GroupType) => void;
    onTrace: (host: string) => void;
    onClose: () => void;
  } = $props();

  const kind = $derived(selected.split(":")[0]);
  const id = $derived(selected.slice(selected.indexOf(":") + 1));

  // —— 规则或合集 ——
  const ruleRows = $derived.by(() => {
    if (kind === "rule") return [graph.rules[Number(id)]!].filter(Boolean);
    if (kind === "bundle") {
      const from = Number(id);
      const b = graph.rules[from]?.set?.bundle;
      const out = [graph.rules[from]!];
      for (let i = from + 1; i < graph.rules.length && graph.rules[i]!.set?.bundle === b; i++) out.push(graph.rules[i]!);
      return out.filter(Boolean);
    }
    return [];
  });
  const head = $derived(ruleRows[0]);
  const entry = $derived(head?.entry);
  const ruleEditable = $derived(editable && entry !== undefined && head?.origin !== "raw");
  const isFinalRule = $derived(head?.type === "MATCH");
  const targets = $derived([...graph.groups.map((g) => g.name), "DIRECT", "REJECT"]);
  const desc = $derived(ruleRows.map((r) => r.set?.description).find(Boolean));
  const samples = $derived([...new Set(ruleRows.flatMap((r) => (r.set?.samples ?? []).map((s) => s.host)))].slice(0, 8));

  // 相邻单元：上移、下移
  function neighbor(r: Routing, e: number, dir: -1 | 1): number | null {
    const u = unitOf(r, e);
    const j = dir < 0 ? u[0]! - 1 : u[u.length - 1]! + 1;
    return j >= 0 && j < r.rulesets.length ? j : null;
  }

  // —— 策略组 ——
  const group = $derived(kind === "group" ? graph.groups.find((g) => g.name === id) : undefined);
  const rgroup = $derived(group && routing ? routing.groups.find((g) => g.name === group.name) : undefined);
  const groupEditable = $derived(editable && !!group && group.origin !== "raw");
  const gtype = $derived((group?.type ?? "select") as GroupType);
  const params = $derived(PARAM_KEYS[gtype] ?? []);
  const addable = $derived(
    group ? [...graph.groups.map((g) => g.name).filter((n) => n !== group.name && !group.members.some((m) => m.name === n)), ...["DIRECT", "REJECT"].filter((n) => !group.members.some((m) => m.name === n))] : [],
  );
  let adding = $state("");
  const paramValue = (k: string): string => {
    const v = rgroup ? rgroup[k] : (group?.params as Record<string, unknown> | undefined)?.[k];
    return v === undefined || v === null ? "" : String(v);
  };
  function setP(k: string, raw: string | boolean) {
    const value = typeof raw === "boolean" ? raw : raw.trim() === "" ? undefined : k === "interval" || k === "tolerance" ? Number(raw) : raw.trim();
    if (typeof value === "number" && (!Number.isFinite(value) || value < 0)) return;
    onEdit((r) => setParam(r, group!.name, k, value));
  }
  const yamlOf = $derived(rgroup ? stringify(rgroup, { lineWidth: 0 }) : "");

  // —— 出口 ——
  const exit = $derived(kind === "exit" ? plan.exits.find((e) => e.id === id) : undefined);
</script>

<section class="detail" aria-labelledby="detail-h">
  <header>
    <h3 id="detail-h">
      {#if head}{kind === "bundle" ? head.set?.bundle : ruleName(head)}{#if kind === "bundle"} <span class="btag">{t("routing.bundle")}</span>{/if}
      {:else if group}{group.name} <span class="gtype">{typeLabel(group.type)}</span>
      {:else if exit}{exitLabel(exit)}{/if}
    </h3>
    <button class="btn ghost sm" type="button" onclick={onClose}>{t("common.close")}</button>
  </header>

  {#if head}
    {#if desc}
      <p class="d">{catalog.label(desc.text)}</p>
      <p class="meta"><a href={desc.repo} target="_blank" rel="noopener noreferrer">{desc.repo.replace(/^https:\/\//, "")}</a> · {desc.license} · {t("routing.detail.checked", { date: desc.checked })}</p>
    {/if}
    <ul class="members">
      {#each ruleRows as r (r.index)}
        <li>
          <span class="mono">{r.index + 1}</span>
          <span class="grow">
            <span class="mono raw">{r.raw}</span>
            {#if r.set}<span class="meta">{r.set.behavior}{#if r.set.count !== undefined} · {t("routing.detail.entries", { n: fmtCount(r.set.count) })}{/if}{#if r.set.url} · <a href={r.set.url} target="_blank" rel="noopener noreferrer">{t("routing.detail.file")}</a>{/if}</span>{/if}
          </span>
          {#if kind === "bundle" && ruleEditable && r.entry !== undefined}
            <button class="btn ghost sm" type="button" onclick={() => onEdit((x) => splitOut(x, r.entry!))}>{t("routing.detail.split")}</button>
          {/if}
        </li>
      {/each}
    </ul>
    {#if samples.length}
      <div class="samples">
        <span class="lbl">{t("routing.detail.samples")}</span>
        {#each samples as s (s)}<button class="chip" type="button" onclick={() => onTrace(s)}>{s}</button>{/each}
      </div>
    {/if}
    {#if ruleEditable}
      <div class="edit">
        <label class="field">
          <span>{t("routing.detail.target")}</span>
          <select class="select" value={head.target} onchange={(e) => onEdit((r) => setTarget(r, entry!, e.currentTarget.value))}>
            {#each targets as n (n)}<option value={n}>{builtinLabel(n)}</option>{/each}
          </select>
        </label>
        {#if !isFinalRule}
          <div class="inline">
            <button class="btn sm" type="button" onclick={() => onEdit((r) => { const j = neighbor(r, entry!, -1); return j === null ? null : move(r, entry!, j, false); }, t("routing.err.move"))}>{t("routing.detail.up")}</button>
            <button class="btn sm" type="button" onclick={() => onEdit((r) => { const j = neighbor(r, entry!, 1); return j === null ? null : move(r, entry!, j, true); }, t("routing.err.move"))}>{t("routing.detail.down")}</button>
          </div>
        {/if}
      </div>
    {:else}
      <p class="note">{head.origin === "raw" ? t("routing.detail.raw_readonly") : t("routing.detail.readonly")}</p>
    {/if}
  {:else if group}
    <div class="seg" role="radiogroup" aria-label={t("routing.detail.type")}>
      {#each GROUP_TYPES as ty (ty)}
        <button type="button" role="radio" aria-checked={gtype === ty} class:on={gtype === ty} disabled={!groupEditable} onclick={() => onType(group.name, ty)}>{typeLabel(ty)}</button>
      {/each}
    </div>
    <div class="fit">
      <p>{t("routing.type_info." + gtype + "_tip")}</p>
      <p><b>{t("routing.detail.fit")}</b>{t("routing.type_info." + gtype + "_fit")}</p>
      <p><b>{t("routing.detail.avoid")}</b>{t("routing.type_info." + gtype + "_avoid")}</p>
    </div>
    {#each group.lints.filter((l) => l !== "group.unused") as l (l)}<p class="lint">{t("routing.lint." + l.slice(6))}</p>{/each}
    {#if params.length}
      <div class="edit">
        {#each params as k (k)}
          <label class="field">
            <span>{t("routing.param." + k)}</span>
            {#if k === "lazy"}
              <input type="checkbox" checked={paramValue(k) !== "false"} disabled={!groupEditable} onchange={(e) => setP(k, e.currentTarget.checked)} />
            {:else if k === "strategy"}
              <select class="select" value={paramValue(k) || "consistent-hashing"} disabled={!groupEditable} onchange={(e) => setP(k, e.currentTarget.value)}>
                {#each ["consistent-hashing", "round-robin", "sticky-sessions"] as s (s)}<option value={s}>{t("routing.strategy." + s)}</option>{/each}
              </select>
            {:else}
              <input class="input" type={k === "url" ? "url" : "number"} min="0" value={paramValue(k)} placeholder={k === "tolerance" ? "0" : ""} disabled={!groupEditable} onchange={(e) => setP(k, e.currentTarget.value)} />
            {/if}
          </label>
        {/each}
      </div>
    {/if}
    <h4>{t("routing.detail.members", { n: group.members.length })}</h4>
    <ol class="members">
      {#each group.members as m, i (m.name + i)}
        <li>
          <span class="mono">{i + 1}</span>
          <span class="grow">{builtinLabel(m.name)}{#if i === 0} <span class="tag">{t("routing.detail.default")}</span>{/if}</span>
          {#if groupEditable && m.entry !== undefined}
            {#if i > 0}<button class="btn ghost sm" type="button" onclick={() => onEdit((r) => setFirst(r, group.name, m.entry!))}>{t("routing.detail.first")}</button>{/if}
            <button class="btn ghost sm" type="button" onclick={() => onEdit((r) => removeMember(r, group.name, m.entry!))}>{t("common.remove")}</button>
          {:else if m.entry === undefined && m.kind !== "provider"}
            <span class="meta">{t("routing.detail.from_filter")}</span>
          {/if}
        </li>
      {/each}
    </ol>
    {#if groupEditable}
      <div class="inline">
        <select class="select" bind:value={adding} aria-label={t("routing.detail.add")}>
          <option value="">{t("routing.detail.add")}</option>
          {#each addable as n (n)}<option value={n}>{builtinLabel(n)}</option>{/each}
        </select>
        <button class="btn sm" type="button" disabled={!adding} onclick={() => { onEdit((r) => addMember(r, group.name, adding), t("routing.err.cycle")); adding = ""; }}>{t("common.add")}</button>
      </div>
    {:else}
      <p class="note">{group.origin === "raw" ? t("routing.detail.raw_readonly") : t("routing.detail.readonly")}</p>
    {/if}
    {#if yamlOf}<pre class="yaml">{yamlOf}</pre>{/if}
  {:else if exit}
    {#if exit.kind === "provider"}<p class="d">{t("routing.trace.exit_provider_d")}{#if exit.filter} · {exit.filter}{/if}</p>{/if}
    {#if exit.via}<p class="d">{t("routing.via", { via: exit.via })}</p>{/if}
    {#if exit.members.length > 1}
      <ul class="members">{#each exit.members as n (n)}<li><span class="grow">{n}</span></li>{/each}</ul>
    {/if}
    {#if editable}<p class="meta">{t("routing.detail.exit_hint")}</p>{/if}
  {/if}
</section>

<style>
  .detail {
    display: grid;
    gap: var(--space-3);
    align-content: start;
    min-inline-size: 0;
  }
  header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-2);
  }
  h3 {
    font-size: var(--text-md);
    font-weight: 620;
    overflow-wrap: anywhere;
  }
  h4 {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .d {
    font-size: var(--text-sm);
    color: var(--fg-muted);
  }
  .meta {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .members {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 6px;
  }
  .members li {
    display: flex;
    gap: 8px;
    align-items: center;
    padding: 6px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
  }
  .grow {
    flex: 1;
    min-inline-size: 0;
    display: grid;
    gap: 2px;
    overflow-wrap: anywhere;
  }
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .mono.raw {
    color: var(--fg-muted);
  }
  .samples {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    align-items: center;
  }
  .lbl {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    margin-inline-end: 4px;
  }
  .chip {
    font: inherit;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: 1px 8px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--fg-muted);
    cursor: pointer;
  }
  .edit {
    display: grid;
    gap: 8px;
    padding-block-start: var(--space-3);
    border-block-start: 1px dashed var(--border);
  }
  .field {
    display: grid;
    grid-template-columns: 7em minmax(0, 1fr);
    gap: 8px;
    align-items: center;
    font-size: var(--text-sm);
  }
  .field > span {
    color: var(--fg-subtle);
  }
  .inline {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    align-items: center;
  }
  .seg {
    display: inline-flex;
    flex-wrap: wrap;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    overflow: hidden;
    justify-self: start;
  }
  .seg button {
    font: inherit;
    font-size: var(--text-xs);
    padding: 5px 10px;
    border: 0;
    background: var(--bg);
    color: var(--fg-muted);
    cursor: pointer;
  }
  .seg button + button {
    border-inline-start: 1px solid var(--border-strong);
  }
  .seg button.on {
    background: var(--accent);
    color: var(--fg-on-accent);
  }
  .fit {
    display: grid;
    gap: 4px;
    font-size: var(--text-sm);
    padding: 8px 10px;
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
  }
  .fit b {
    font-weight: 600;
    margin-inline-end: 4px;
  }
  .lint {
    font-size: var(--text-sm);
    padding: 6px 10px;
    border-radius: var(--radius-md);
    background: var(--warn-soft);
    color: var(--warn);
  }
  .note {
    font-size: var(--text-sm);
    color: var(--fg-subtle);
  }
  .tag,
  .btag,
  .gtype {
    font-size: var(--text-xs);
    font-weight: 400;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  .btag {
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .yaml {
    margin: 0;
    padding: 8px 10px;
    border-radius: var(--radius-md);
    background: var(--bg-code);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow-x: auto;
  }
</style>
