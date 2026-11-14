<script lang="ts">
  // 字段说明面板（反馈 #27）：这是什么 · 怎么选 · 各选项的区别 · YAML 写法 · 官方文档。
  // 编辑器的字段卡片展开时使用，百科页直接展示；内容全部来自字段目录，不在前端另写。
  import Icon from "../Icon.svelte";
  import { catalog, type MetaField } from "../../lib/catalog.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { highlightLine } from "../../lib/yaml-highlight";
  import { yamlSnippet } from "../../lib/yaml-snippet";

  let { field, path }: { field: MetaField; path: string[] } = $props();

  const detail = $derived(catalog.detail(field));
  const guide = $derived(catalog.guide(field));
  const options = $derived((field.enum ?? []).map((v) => ({ v, label: catalog.enumLabel(field, v) })));
  // 写法示例的取值：建议值 → 第一个示例 → 默认值；都没有时不展示
  const sample = $derived(field.suggest ?? field.examples?.[0] ?? field.default);
  const snippet = $derived(sample === undefined ? "" : yamlSnippet(path, sample));
  const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
</script>

<div class="guide">
  {#if detail}
    <section>
      <h4>{t("guide.what")}</h4>
      <p>{detail}</p>
    </section>
  {/if}

  {#if guide}
    <section>
      <h4>
        {t("guide.how")}
        <span class="basis" data-basis={field.basis} title={t("guide.basis_hint." + field.basis)}>{t("guide.basis." + field.basis)}</span>
      </h4>
      <p>{guide}</p>
    </section>
  {/if}

  {#if options.length}
    <section>
      <h4>{t("guide.options")}</h4>
      <dl class="opts">
        {#each options as o (o.v)}
          <dt class="mono ltr">
            {o.v}
            {#if same(o.v, field.default)}<span class="tag">{t("field.default_tag")}</span>{/if}
            {#if same(o.v, field.suggest)}<span class="tag sugg" data-basis={field.basis}>{t("guide.suggest." + field.basis)}</span>{/if}
          </dt>
          <dd>{o.label || "—"}</dd>
        {/each}
      </dl>
    </section>
  {/if}

  {#if snippet}
    <section>
      <h4>{t("guide.yaml")}</h4>
      <pre class="code ltr">{#each snippet.split("\n") as line, i (i)}{#each highlightLine(line) as tok, j (j)}<span class={"t-" + tok.k}>{tok.s}</span>{/each}{"\n"}{/each}</pre>
    </section>
  {/if}

  {#if field.doc}
    <a class="doc" href={field.doc} target="_blank" rel="noopener noreferrer" data-external>
      <Icon name="external" size={13} />{t("guide.doc")}
    </a>
  {/if}
</div>

<style>
  .guide {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    font-size: var(--text-sm);
    line-height: 1.65;
  }
  h4 {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-block-end: 2px;
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--fg-subtle);
  }
  p {
    color: var(--fg);
  }
  .basis,
  .tag {
    padding: 0 6px;
    border-radius: var(--radius-full);
    font-size: 11px;
    font-weight: 500;
    letter-spacing: 0;
    line-height: 18px;
    background: var(--bg-hover);
    color: var(--fg-muted);
  }
  /* 官方依据用强调色，通行做法用中性色：两者在视觉上明确区分 */
  [data-basis="official"] {
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .opts {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: 4px var(--space-4);
  }
  .opts dt {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    color: var(--fg);
  }
  .opts dd {
    color: var(--fg-muted);
  }
  .code {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--bg-code);
    font-family: var(--font-mono);
    font-size: 12.5px;
    line-height: 1.6;
    overflow-x: auto;
  }
  .t-key {
    color: var(--syn-key);
  }
  .t-str {
    color: var(--syn-string);
  }
  .t-num {
    color: var(--syn-number);
  }
  .t-bool {
    color: var(--syn-bool);
  }
  .t-pun {
    color: var(--syn-punct);
  }
  .doc {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    justify-self: start;
    color: var(--accent-fg);
  }
  @media (max-width: 767px) {
    .opts {
      grid-template-columns: 1fr;
    }
    .opts dd {
      margin-block-end: var(--space-1);
    }
  }
</style>
