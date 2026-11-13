<script lang="ts">
  // 方案内的有序源列表（第 16.1 节「源列表」）：顺序即节点顺序，可拖动或用按钮排序；
  // 左侧数字只表示位置（即节点顺序）。订阅编号（GROUPID，来自后端预览，不在前端推算）
  // 只在规则源用到 !!GROUPID 或选择器 #N 时（预览出现 group.groupid_shift）才作为标签显示。
  // 每个源可以选择写法：写入配置（服务器解析节点）或交给客户端（只写 proxy-providers），也可以停用。
  import Icon from "../Icon.svelte";
  import type { Source } from "../../lib/api/client";
  import { catalog } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import type { SourceRef } from "../../lib/profile-doc";

  let { editor }: { editor: EditorState } = $props();

  const refs = $derived(editor.sourceRefs);
  const byId = $derived(new Map(editor.sources.map((s) => [s.id, s])));
  const numbering = $derived(new Map((editor.preview?.numbering ?? []).map((n) => [n.ref, n.groupId])));
  const usesGroupId = $derived((editor.preview?.diagnostics ?? []).some((d) => d.code === "group.groupid_shift"));
  const available = $derived(editor.sources.filter((s) => !refs.some((r) => r.ref === s.id)));
  let adding = $state("");
  let dragFrom = $state<number | null>(null);
  let dropAt = $state<number | null>(null);

  function update(i: number, patch: Partial<SourceRef>) {
    editor.setSources(refs.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  }

  function move(from: number, to: number) {
    if (to < 0 || to >= refs.length || from === to) return;
    const list = [...refs];
    const [x] = list.splice(from, 1);
    list.splice(to, 0, x!);
    editor.setSources(list);
    queueMicrotask(() => document.getElementById("src-row-" + to)?.querySelector<HTMLElement>(".handle")?.focus());
  }

  function remove(i: number) {
    editor.setSources(refs.filter((_, j) => j !== i));
  }

  function add() {
    if (!adding) return;
    editor.setSources([...refs, { ref: adding }]);
    adding = "";
  }

  // 规则源的 preset 形如「预设/变体」（见 lib/rules.ts）；名称与预设相同时不重复显示
  function presetLabel(preset: string | undefined): string {
    if (!preset) return "";
    const [pid, vid] = preset.split("/");
    const p = catalog.meta?.ruleSources.find((x) => x.id === pid);
    const v = p?.variants.find((x) => x.id === vid);
    return v ? " · " + catalog.label(v.label) : "";
  }

  function kindIcon(s: Source | undefined) {
    return s?.kind === "subscription" ? "link" : "server";
  }

  function onHandleKey(e: KeyboardEvent, i: number) {
    if (e.altKey && e.key === "ArrowUp") {
      e.preventDefault();
      move(i, i - 1);
    } else if (e.altKey && e.key === "ArrowDown") {
      e.preventDefault();
      move(i, i + 1);
    }
  }
</script>

<section class="panel-sec" aria-labelledby="sec-sources">
  <header class="head">
    <div>
      <h2 id="sec-sources">{t("editor.sources_title")}</h2>
      <p class="muted sub">{t("editor.sources_sub")}</p>
    </div>
    <a class="btn sm" href="/sources"><Icon name="server" size={14} />{t("editor.manage_sources")}</a>
  </header>

  {#if refs.length === 0}
    <p class="empty muted">{t("editor.no_sources")}</p>
  {:else}
    <ol class="list" aria-describedby="src-help">
      {#each refs as r, i (r.ref)}
        {@const s = byId.get(r.ref)}
        {@const gid = numbering.get(r.ref)}
        <li
          id={"src-row-" + i}
          class="row"
          class:disabled={r.disabled}
          class:drop-before={dropAt === i && dragFrom !== null && dragFrom > i}
          class:drop-after={dropAt === i && dragFrom !== null && dragFrom < i}
          draggable="true"
          ondragstart={(e) => { dragFrom = i; e.dataTransfer?.setData("text/plain", String(i)); }}
          ondragover={(e) => { e.preventDefault(); dropAt = i; }}
          ondragleave={() => { if (dropAt === i) dropAt = null; }}
          ondrop={(e) => { e.preventDefault(); if (dragFrom !== null) move(dragFrom, i); dragFrom = dropAt = null; }}
          ondragend={() => (dragFrom = dropAt = null)}
        >
          <button class="handle" type="button" aria-label={t("editor.reorder", { name: s?.name ?? r.ref })} onkeydown={(e) => onHandleKey(e, i)}>
            <Icon name="grip" size={16} />
          </button>
          <span class="gid mono" aria-hidden="true">{i + 1}</span>
          <span class="kind" title={s ? t("source.kind." + s.kind) : ""}><Icon name={kindIcon(s)} size={15} /></span>
          <span class="name">
            {#if s}<a href={"/sources/" + s.id}>{s.name}</a>{#if usesGroupId && gid !== undefined}<span class="chip mono" title={t("editor.groupid_hint")}>GROUPID {gid}</span>{/if}{:else}<span class="missing">{t("editor.source_missing", { ref: r.ref })}</span>{/if}
          </span>
          <div class="mode" role="radiogroup" aria-label={t("editor.mode")}>
            <button type="button" role="radio" aria-checked={r.mode !== "provider"} class:on={r.mode !== "provider"} onclick={() => update(i, { mode: undefined })}>{t("mode.inline")}</button>
            <button type="button" role="radio" aria-checked={r.mode === "provider"} class:on={r.mode === "provider"} onclick={() => update(i, { mode: "provider" })}>{t("mode.provider")}</button>
          </div>
          <div class="actions">
            <button class="btn ghost icon sm" type="button" aria-label={t("editor.move_up")} disabled={i === 0} onclick={() => move(i, i - 1)}><Icon name="arrow-up" size={14} /></button>
            <button class="btn ghost icon sm" type="button" aria-label={t("editor.move_down")} disabled={i === refs.length - 1} onclick={() => move(i, i + 1)}><Icon name="arrow-down" size={14} /></button>
            <button class="btn ghost icon sm" type="button" aria-label={r.disabled ? t("editor.enable") : t("editor.disable")} aria-pressed={!r.disabled} title={r.disabled ? t("editor.enable") : t("editor.disable")} onclick={() => update(i, { disabled: !r.disabled })}>
              <Icon name={r.disabled ? "eye-off" : "eye"} size={14} />
            </button>
            <button class="btn ghost icon sm danger" type="button" aria-label={t("common.remove")} onclick={() => remove(i)}><Icon name="x" size={14} /></button>
          </div>
        </li>
      {/each}
    </ol>
    <p id="src-help" class="hint">{t("editor.sources_help")}</p>
  {/if}

  <div class="add">
    <select class="select" bind:value={adding} aria-label={t("editor.add_source")} disabled={!available.length}>
      <option value="">{available.length ? t("editor.add_source") : t("editor.no_more_sources")}</option>
      {#each available as s (s.id)}<option value={s.id}>{s.name} · {t("source.kind." + s.kind)}</option>{/each}
    </select>
    <button class="btn" type="button" disabled={!adding} onclick={add}><Icon name="plus" size={14} />{t("common.add")}</button>
  </div>
</section>

<section class="panel-sec" aria-labelledby="sec-rules-ref">
  <header class="head">
    <div>
      <h2 id="sec-rules-ref">{t("editor.rules_title")}</h2>
      <p class="muted sub">{t("editor.rules_sub")}</p>
    </div>
    <a class="btn sm" href="/rules"><Icon name="rules" size={14} />{t("editor.manage_rules")}</a>
  </header>
  <select class="select" value={editor.rulesRef ?? ""} onchange={(e) => editor.setRules(e.currentTarget.value || undefined)} aria-label={t("editor.rules_title")}>
    <option value="">{t("editor.no_rules")}</option>
    {#each editor.ruleSources as rs (rs.id)}
      <option value={rs.id}>{rs.name}{presetLabel(rs.preset)}</option>
    {/each}
  </select>
  {#if editor.preview}
    <p class="hint">{t("editor.nodes_count", { n: editor.preview.nodes })}</p>
  {/if}
</section>

<style>
  .panel-sec {
    display: grid;
    gap: var(--space-3);
  }
  .panel-sec + .panel-sec {
    margin-block-start: var(--space-8);
  }
  .head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  h2 {
    font-size: var(--text-xl);
  }
  .sub {
    font-size: var(--text-sm);
    margin-block-start: 2px;
  }
  .list {
    display: grid;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    overflow: hidden;
  }
  .row {
    display: grid;
    grid-template-columns: auto 2.5rem auto minmax(0, 1fr) auto auto;
    align-items: center;
    gap: var(--space-2);
    min-block-size: 46px;
    padding-inline: var(--space-2) var(--space-3);
    border-block-end: 1px solid var(--border);
  }
  .row:last-child {
    border-block-end: 0;
  }
  .row.disabled .name,
  .row.disabled .gid,
  .row.disabled .kind {
    opacity: 0.45;
    text-decoration: line-through;
  }
  .row.drop-before {
    box-shadow: inset 0 2px 0 var(--accent);
  }
  .row.drop-after {
    box-shadow: inset 0 -2px 0 var(--accent);
  }
  .handle {
    display: grid;
    place-items: center;
    inline-size: 24px;
    block-size: 30px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--fg-subtle);
    cursor: grab;
  }
  .handle:hover {
    background: var(--bg-hover);
  }
  .chip {
    margin-inline-start: var(--space-2);
    padding: 1px 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: var(--text-xs);
  }
  .gid {
    display: inline-grid;
    place-items: center;
    block-size: 22px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-variant-numeric: tabular-nums;
  }
  .kind {
    display: grid;
    color: var(--fg-subtle);
  }
  .name {
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 540;
  }
  .name a {
    color: var(--fg);
  }
  .missing {
    color: var(--err);
  }
  .mode {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
  }
  .mode button {
    block-size: 24px;
    padding-inline: var(--space-2);
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
    white-space: nowrap;
  }
  .mode button.on {
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-sm);
  }
  .actions {
    display: inline-flex;
    gap: 2px;
  }
  .empty {
    padding: var(--space-6);
    text-align: center;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-lg);
  }
  .add {
    display: flex;
    gap: var(--space-2);
  }
  .add .select {
    flex: 1;
  }
  @media (max-width: 700px) {
    .row {
      grid-template-columns: auto 2.5rem auto minmax(0, 1fr);
      padding-block: var(--space-2);
    }
    .mode,
    .actions {
      grid-column: 2 / -1;
    }
  }
</style>
