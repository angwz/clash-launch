<script lang="ts">
  // 一个分节（第 16.2 节）：core 与 recommended 默认展开，optional 与 expert 折叠在「更多」中；
  // 已设置或来自原始编辑的字段总是显示。分节只有一个对象键（如 dns）时直接展开其字段。
  import FieldCard from "./FieldCard.svelte";
  import Icon from "../Icon.svelte";
  import Menu from "../Menu.svelte";
  import { catalog, type MetaField, type NavSection } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";

  let {
    section,
    editor,
    core,
    managed,
    onRaw,
  }: { section: NavSection; editor: EditorState; core: string; managed: string[]; onRaw: (section: NavSection) => void } = $props();

  let onlySet = $state(false);
  let more = $state(false);

  interface Entry {
    field: MetaField;
    path: string[];
  }

  // 单个对象键的分节：展开为其子字段；否则每个顶层键一张卡片（对象键成为一组）。
  const entries = $derived.by<Entry[]>(() => {
    const tops = section.keys.map((k) => catalog.top(k)).filter((f): f is MetaField => !!f);
    if (tops.length === 1 && tops[0]!.type === "object" && tops[0]!.fields?.length) {
      const root = tops[0]!;
      return root.fields!.map((f) => ({ field: f, path: [root.key, f.key] }));
    }
    return tops.map((f) => ({ field: f, path: [f.key] }));
  });

  const prominent = (e: Entry) => e.field.importance === "core" || e.field.importance === "recommended";
  const active = (e: Entry) => editor.isSet(e.path) || editor.touched(e.path);
  const primary = $derived(entries.filter((e) => (onlySet ? active(e) : prominent(e) || active(e))));
  const rest = $derived(onlySet ? [] : entries.filter((e) => !prominent(e) && !active(e)));
  const setCount = $derived(entries.filter(active).length);
  // 定位到折叠在「更多」中的字段时展开
  $effect(() => {
    const f = editor.focus;
    if (f && rest.some((e) => f === e.path.join(".") || f.startsWith(e.path.join(".") + "."))) more = true;
  });
</script>

<section class="section" aria-labelledby={"sec-" + section.id}>
  <header class="head">
    <div>
      <h2 id={"sec-" + section.id}>{catalog.navTitle(section)}</h2>
      <p class="muted sub">
        {t("editor.section_summary", { total: entries.length, set: setCount })}
      </p>
    </div>
    <div class="tools">
      <label class="only-set">
        <input type="checkbox" bind:checked={onlySet} />
        <span>{t("editor.only_set")}</span>
      </label>
      <Menu label={t("editor.section_menu")}>
        {#snippet items(close)}
          <button type="button" role="menuitem" onclick={() => { close(); onRaw(section); }}>
            <Icon name="code" size={15} />{t("raw.title")}
          </button>
          {#if section.keys[0]}
            <a role="menuitem" href={catalog.top(section.keys[0])?.doc} target="_blank" rel="noopener noreferrer" data-external onclick={close}>
              <Icon name="external" size={15} />{t("field.docs")}
            </a>
          {/if}
        {/snippet}
      </Menu>
    </div>
  </header>

  {#if primary.length === 0 && onlySet}
    <p class="empty muted">{t("editor.nothing_set")}</p>
  {/if}

  <div class="cards">
    {#each primary as e (e.path.join("."))}
      <FieldCard field={e.field} path={e.path} {editor} {core} {managed} {onlySet} />
    {/each}
  </div>

  {#if rest.length}
    <button class="more" type="button" aria-expanded={more} onclick={() => (more = !more)}>
      <Icon name={more ? "chevron-down" : "chevron-right"} size={15} />
      {t("editor.more_fields", { n: rest.length })}
    </button>
    {#if more}
      <div class="cards">
        {#each rest as e (e.path.join("."))}
          <FieldCard field={e.field} path={e.path} {editor} {core} {managed} />
        {/each}
      </div>
    {/if}
  {/if}
</section>

<style>
  .section {
    display: grid;
    gap: var(--space-3);
  }
  .head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
    padding-block-end: var(--space-2);
  }
  h2 {
    font-size: var(--text-xl);
  }
  .sub {
    font-size: var(--text-sm);
    margin-block-start: 2px;
  }
  .tools {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .only-set {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--fg-muted);
    cursor: pointer;
  }
  .only-set input {
    accent-color: var(--accent);
  }
  .cards {
    display: grid;
    gap: var(--space-2);
  }
  .more {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    justify-self: start;
    margin-block-start: var(--space-2);
    padding-block: var(--space-2);
    padding-inline: var(--space-1) var(--space-3);
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-weight: 560;
    cursor: pointer;
  }
  .more:hover {
    color: var(--fg);
    background: var(--bg-hover);
  }
  .empty {
    padding: var(--space-6);
    text-align: center;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-lg);
  }
</style>
