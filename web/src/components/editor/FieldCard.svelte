<script lang="ts">
  // 字段卡片（第 16.2 节）：标题与摘要、控件与默认值、示例（点击填入）、徽章、官方文档链接、恢复默认。
  // 对象字段是一组嵌套卡片；短控件（开关、分段、数字、短文本）放在标题右侧，长控件放在下方。
  import FieldCard from "./FieldCard.svelte";
  import FieldControl from "./FieldControl.svelte";
  import FieldGuide from "./FieldGuide.svelte";
  import Icon from "../Icon.svelte";
  import Emoji from "../Emoji.svelte";
  import { catalog, type MetaField } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { slide } from "svelte/transition";
  import { dur } from "../../lib/motion";
  import { prefs } from "../../lib/prefs.svelte";

  let {
    field,
    path,
    editor,
    core,
    onlySet = false,
  }: { field: MetaField; path: string[]; editor: EditorState; core: string; onlySet?: boolean } = $props();

  const pathKey = $derived(path.join("."));
  const labelId = $derived("lbl-" + pathKey);
  const set = $derived(editor.isSet(path));
  const raw = $derived(editor.touched(path));
  const isGroup = $derived(field.type === "object" && !!field.fields?.length);
  // 总开关：同一对象下有 bool 类型的 enable 且它没有打开时，其余字段变淡（仍可编辑，只是暂不生效）
  const switchPath = $derived([...path.slice(0, -1), "enable"]);
  const switchField = $derived(path.length > 1 && field.key !== "enable" ? catalog.field(switchPath.join(".")) : undefined);
  const inactive = $derived(
    switchField?.type === "bool" && (editor.isSet(switchPath) ? editor.value(switchPath) : switchField.default) !== true,
  );
  const inline = $derived(
    field.type === "bool" || field.type === "int" || (field.type === "enum" && (field.enum?.length ?? 0) <= 6) || (field.type === "string" && !field.sensitive),
  );
  const newer = $derived(!!field.since && cmpVersion(field.since, core) > 0);
  const effective = $derived(set ? editor.value(path) : field.default);
  const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
  let expanded = $state(false);
  const clickExpand = $derived(prefs.data.fieldClick !== false);
  function onTextClick(e: MouseEvent) {
    if (!clickExpand) return;
    if ((e.target as HTMLElement).closest("a, button, input, select, textarea")) return;
    if (window.getSelection()?.toString()) return;
    expanded = !expanded;
  }
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
  <section class="group" class:inactive id={"f-" + pathKey} data-path={pathKey} title={inactive ? t("field.inactive_hint") : undefined}>
    <button class="group-head" type="button" aria-expanded={groupOpen || onlySet} onclick={() => (groupOpen = !groupOpen)}>
      <Icon name={groupOpen || onlySet ? "chevron-down" : "chevron-right"} size={15} />
      <span class="title" id={labelId}>{catalog.title(field)}</span>
      <code class="key ltr">{field.key}</code>
      {#if setCount}<span class="count">{t("editor.set_count", { n: setCount })}</span>{/if}
      {#if raw}<span class="badge raw" title={t("raw.badge_hint")}><Icon name="bolt" size={12} />{t("raw.badge")}</span>{/if}
    </button>
    {#if catalog.summary(field)}<p class="group-sum">{catalog.summary(field)}</p>{/if}
    {#if groupOpen || onlySet}
      <div class="group-body" transition:slide={{ duration: dur(220) }}>
        {#each field.fields ?? [] as f (f.key)}
          {#if childVisible(f)}
            <FieldCard field={f} path={[...path, f.key]} {editor} {core} {onlySet} />
          {/if}
        {/each}
      </div>
    {/if}
  </section>
{:else}
  <article class="card" class:is-set={set} class:is-raw={raw} class:inactive id={"f-" + pathKey} data-path={pathKey} title={inactive ? t("field.inactive_hint") : undefined}>
    <div class="head">
      <!-- 点击左侧说明区域展开或收起「说明与建议」（反馈 #50）。键盘与读屏器使用下方的按钮，这里只是更大的鼠标目标；
           选中文字、点击链接或徽章中的按钮时不触发。可在账号设置中关闭。 -->
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="text" class:clickable={clickExpand} onclick={onTextClick}>
        <div class="title-row">
          <span class="title" id={labelId}>{catalog.title(field)}</span>
          <code class="key ltr">{field.key}</code>
          {#if field.importance === "core"}<span class="badge core">{t("importance.core")}</span>{/if}
          {#if field.stability === "experimental"}<span class="badge warn">{t("badge.experimental")}</span>{/if}
          {#if field.stability === "deprecated"}<span class="badge err"><Emoji name="prohibited" size={12} />{t("badge.deprecated")}</span>{/if}
          {#if field.since}<span class="badge" class:warn={newer} title={newer ? t("badge.since_newer", { core }) : undefined}>{t("badge.since", { v: field.since })}</span>{/if}
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

    {#if expanded}
      <div transition:slide={{ duration: dur(200) }}>
        <FieldGuide {field} {path} />
      </div>
    {/if}

    <div class="foot">
      <button type="button" class="link guide" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
        <Icon name={expanded ? "chevron-down" : "chevron-right"} size={13} />{expanded ? t("field.less") : t("field.more")}
      </button>
      {#if field.suggest !== undefined}
        <!-- 建议值（反馈 #27）：未采用时点击即写入；已是建议值时只显示勾 -->
        {#if same(effective, field.suggest)}
          <span class="sugg ok" data-basis={field.basis}><Emoji name="trophy" size={12} />{t("guide.suggest." + field.basis)}</span>
        {:else}
          <button type="button" class="sugg" data-basis={field.basis} title={t("guide.apply")} onclick={() => editor.set(path, field.suggest)}>
            {t("guide.suggest." + field.basis)}<span class="mono ltr">{exampleText(field.suggest)}</span>
          </button>
        {/if}
      {/if}
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
      {#if set}
        <!-- 恢复默认独占最右端，与左端的「说明与建议」分开，降低误触（反馈 #34）；官方文档链接只在展开面板里 -->
        <button type="button" class="link reset" onclick={() => editor.reset(path)}><Icon name="refresh" size={13} />{t("field.reset")}</button>
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
  /* 总开关关闭时变淡；悬停或聚焦时恢复一些，便于继续编辑 */
  .card,
  .group {
    transition:
      opacity var(--dur) var(--ease),
      border-color var(--dur-fast) var(--ease);
  }
  .inactive {
    opacity: 0.5;
  }
  .inactive:hover,
  .inactive:focus-within {
    opacity: 0.8;
  }
  /* 已设置标记：竖条从中间展开 */
  .card::before {
    content: "";
    position: absolute;
    inset-block: 10px;
    inset-inline-start: -1px;
    inline-size: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
    transform: scaleY(0);
    transition: transform var(--dur) var(--ease-spring);
  }
  .card.is-set::before {
    transform: scaleY(1);
  }
  :global([dir="rtl"]) .card::before {
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
  .text.clickable {
    cursor: pointer;
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
  /* 建议值：官方依据用强调色，通行做法用中性色 */
  .sugg {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    block-size: 22px;
    padding-inline: var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-full);
    background: var(--bg-hover);
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
  }
  .sugg[data-basis="official"] {
    border-color: transparent;
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  button.sugg:hover {
    border-color: var(--accent);
  }
  .sugg.ok {
    cursor: default;
    background: transparent;
    border-color: transparent;
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
  .link.reset {
    padding-inline-start: var(--space-4);
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
