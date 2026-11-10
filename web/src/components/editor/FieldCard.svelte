<script lang="ts">
  // 字段卡片（第 16.2 节）：标题与摘要、控件与默认值、示例（点击填入）、徽章、官方文档链接、恢复默认。
  // 对象字段是一组嵌套卡片；短控件（开关、分段、数字、短文本）放在标题右侧，长控件放在下方。
  import FieldCard from "./FieldCard.svelte";
  import FieldControl from "./FieldControl.svelte";
  import Icon from "../Icon.svelte";
  import { catalog, type MetaField } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";

  let {
    field,
    path,
    editor,
    core,
    managed,
    onlySet = false,
  }: { field: MetaField; path: string[]; editor: EditorState; core: string; managed: string[]; onlySet?: boolean } = $props();

  const pathKey = $derived(path.join("."));
  const labelId = $derived("lbl-" + pathKey);
  const set = $derived(editor.isSet(path));
  const raw = $derived(editor.touched(path));
  const isManaged = $derived(managed.includes(pathKey));
  const isGroup = $derived(field.type === "object" && !!field.fields?.length);
  const inline = $derived(
    field.type === "bool" || field.type === "int" || (field.type === "enum" && (field.enum?.length ?? 0) <= 6) || (field.type === "string" && !field.sensitive),
  );
  const newer = $derived(!!field.since && cmpVersion(field.since, core) > 0);
  const detail = $derived(catalog.detail(field));
  let expanded = $state(false);
  let groupOpen = $state(false);
  // 定位到组内字段时展开该组
  $effect(() => {
    if (isGroup && editor.focus.startsWith(pathKey + ".")) groupOpen = true;
  });

  function cmpVersion(a: string, b: string): number {
    const pa = a.split(".").map(Number);
    const pb = b.split(".").map(Number);
    for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
      const d = (pa[i] ?? 0) - (pb[i] ?? 0);
      if (d) return d;
    }
    return 0;
  }

  function example(v: unknown) {
    editor.set(path, v);
  }

  // 示例的显示：标量列表用逗号分隔，映射与对象列表用紧凑的 JSON
  function exampleText(v: unknown): string {
    if (Array.isArray(v) && v.every((x) => typeof x !== "object")) return v.join(", ");
    return typeof v === "object" ? JSON.stringify(v) : String(v);
  }

  // 组内已设置的字段数（用于折叠时的提示）
  const setCount = $derived(isGroup ? editor.isSet(path) ? Object.keys((editor.value(path) as object) ?? {}).length : 0 : 0);
  const childVisible = (f: MetaField) => !onlySet || editor.isSet([...path, f.key]) || editor.touched([...path, f.key]);
</script>

{#if isGroup}
  <section class="group" id={"f-" + pathKey} data-path={pathKey}>
    <button class="group-head" type="button" aria-expanded={groupOpen || onlySet} onclick={() => (groupOpen = !groupOpen)}>
      <Icon name={groupOpen || onlySet ? "chevron-down" : "chevron-right"} size={15} />
      <span class="title" id={labelId}>{catalog.title(field)}</span>
      <code class="key ltr">{field.key}</code>
      {#if setCount}<span class="count">{t("editor.set_count", { n: setCount })}</span>{/if}
      {#if raw}<span class="badge raw" title={t("raw.badge_hint")}><Icon name="bolt" size={12} />{t("raw.badge")}</span>{/if}
    </button>
    {#if catalog.summary(field)}<p class="group-sum">{catalog.summary(field)}</p>{/if}
    {#if groupOpen || onlySet}
      <div class="group-body">
        {#each field.fields ?? [] as f (f.key)}
          {#if childVisible(f)}
            <FieldCard field={f} path={[...path, f.key]} {editor} {core} {managed} {onlySet} />
          {/if}
        {/each}
      </div>
    {/if}
  </section>
{:else}
  <article class="card" class:is-set={set} class:is-raw={raw} id={"f-" + pathKey} data-path={pathKey}>
    <div class="head">
      <div class="text">
        <div class="title-row">
          <span class="title" id={labelId}>{catalog.title(field)}</span>
          <code class="key ltr">{field.key}</code>
          {#if field.importance === "core"}<span class="badge core">{t("importance.core")}</span>{/if}
          {#if field.stability === "experimental"}<span class="badge warn">{t("badge.experimental")}</span>{/if}
          {#if field.stability === "deprecated"}<span class="badge err">{t("badge.deprecated")}</span>{/if}
          {#if field.since}<span class="badge" class:warn={newer} title={newer ? t("badge.since_newer", { core }) : undefined}>{t("badge.since", { v: field.since })}</span>{/if}
          {#if isManaged}<span class="badge info" title={t("badge.managed_hint")}>{t("badge.managed")}</span>{/if}
          {#if raw}<span class="badge raw" title={t("raw.badge_hint")}><Icon name="bolt" size={12} />{t("raw.badge")}</span>{/if}
        </div>
        {#if catalog.summary(field)}<p class="summary">{catalog.summary(field)}</p>{/if}
      </div>
      {#if inline}
        <div class="ctl-inline">
          <FieldControl {field} {path} {editor} {labelId} />
        </div>
      {/if}
    </div>

    {#if !inline}
      <div class="ctl-block">
        <FieldControl {field} {path} {editor} {labelId} />
      </div>
    {/if}

    {#if expanded && detail}
      <p class="detail">{detail}</p>
    {/if}

    <div class="foot">
      {#if field.examples?.length}
        <span class="examples">
          <span class="subtle">{t("field.examples")}</span>
          {#each field.examples as ex, i (i)}
            <button type="button" class="chip mono ltr" title={catalog.exampleLabel(field, i) || undefined} onclick={() => example(ex)}>{exampleText(ex)}</button>
          {/each}
        </span>
      {/if}
      <span class="spacer"></span>
      {#if field.replacement}<span class="subtle">{t("badge.replacement", { key: field.replacement })}</span>{/if}
      {#if detail}
        <button type="button" class="link" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>{expanded ? t("field.less") : t("field.more")}</button>
      {/if}
      {#if field.doc}
        <a class="link" href={field.doc} target="_blank" rel="noopener noreferrer" data-external>{t("field.docs")}<Icon name="external" size={12} /></a>
      {/if}
      {#if set}
        <button type="button" class="link" onclick={() => editor.reset(path)}>{t("field.reset")}</button>
      {/if}
    </div>
  </article>
{/if}

<style>
  .card {
    position: relative;
    display: grid;
    gap: var(--space-2);
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    scroll-margin-block: var(--space-6);
    transition: border-color var(--dur-fast) var(--ease);
  }
  .card.is-set::before {
    content: "";
    position: absolute;
    inset-block: 10px;
    inset-inline-start: -1px;
    inline-size: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
  }
  :global([dir="rtl"]) .card.is-set::before {
    border-radius: 3px 0 0 3px;
  }
  .card.is-raw {
    border-color: color-mix(in srgb, var(--raw) 35%, var(--border));
  }
  .card:global(.flash) {
    animation: flash 1.2s var(--ease);
  }
  @keyframes flash {
    0%,
    40% {
      box-shadow: 0 0 0 3px var(--ring);
    }
  }
  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .text {
    min-inline-size: 0;
    display: grid;
    gap: 2px;
  }
  .title-row {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .title {
    font-weight: 580;
  }
  .key {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .summary {
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .detail {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .ctl-inline {
    flex: none;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    max-inline-size: 55%;
    padding-block-start: 1px;
  }
  .ctl-block {
    display: grid;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    block-size: 18px;
    padding-inline: 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: 11px;
    font-weight: 560;
    white-space: nowrap;
  }
  .badge.core {
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .badge.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .badge.err {
    background: var(--err-soft);
    color: var(--err);
  }
  .badge.info {
    background: var(--info-soft);
    color: var(--info);
  }
  .badge.raw {
    background: var(--raw-soft);
    color: var(--raw);
  }
  .foot {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }
  .foot:empty {
    display: none;
  }
  .examples {
    display: inline-flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .chip {
    max-inline-size: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    block-size: 22px;
    padding-inline: var(--space-2);
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--fg-muted);
    font-size: var(--text-xs);
    cursor: pointer;
  }
  .chip:hover {
    border-style: solid;
    color: var(--fg);
  }
  .spacer {
    flex: 1;
  }
  .link {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--fg-muted);
    font: inherit;
    cursor: pointer;
  }
  .link:hover {
    color: var(--accent-fg);
    text-decoration: none;
  }

  .group {
    display: grid;
    gap: var(--space-2);
    padding-block: var(--space-2);
    scroll-margin-block: var(--space-6);
  }
  .group-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 0;
    border: 0;
    background: none;
    color: var(--fg);
    font: inherit;
    font-weight: 600;
    cursor: pointer;
    text-align: start;
  }
  .group-sum {
    margin-inline-start: 23px;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .count {
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--accent-fg);
  }
  .group-body {
    display: grid;
    gap: var(--space-2);
    padding-inline-start: var(--space-4);
    margin-inline-start: 7px;
    border-inline-start: 1px solid var(--border);
  }
  @media (max-width: 600px) {
    .head {
      flex-direction: column;
    }
    .ctl-inline {
      max-inline-size: 100%;
      align-items: flex-start;
    }
  }
</style>
