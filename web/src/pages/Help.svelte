<script lang="ts">
  // 帮助（反馈 #30）：「教程」是随前端打包的 Markdown（src/help/<语言>/*.md），
  // 「百科」按分区浏览字段目录，复用编辑器的「说明与建议」面板（反馈 #27）。
  // 状态放在查询参数里（?tab=wiki&f=dns.enhanced-mode / ?topic=02-nodes），便于分享与从别处链接进来。
  import { onMount, tick } from "svelte";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import FieldGuide from "../components/editor/FieldGuide.svelte";
  import { catalog, type MetaField } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { parse, type Block, type Inline } from "../lib/md";

  // 教程按语言懒加载，文件名前的序号决定顺序
  const docs = import.meta.glob<string>("../help/*/*.md", { query: "?raw", import: "default" });

  const q = new URLSearchParams(location.search);
  let tab = $state<"guide" | "wiki">(q.get("tab") === "wiki" ? "wiki" : "guide");
  let topic = $state(q.get("topic") ?? "");
  let focus = $state(q.get("f") ?? "");
  let search = $state("");
  let error = $state("");

  let topics = $state<{ id: string; title: string; blocks: Block[] }[]>([]);
  const current = $derived(topics.find((x) => x.id === topic) ?? topics[0]);

  $effect(() => {
    const lang = i18n.apiLang === "zh-CN" ? "zh-CN" : "en";
    const files = Object.keys(docs)
      .filter((p) => p.includes("/" + lang + "/"))
      .sort();
    void Promise.all(files.map(async (p) => {
      const blocks = parse(await docs[p]!());
      const h = blocks.find((b) => b.k === "h" && b.level === 1);
      const title = h && h.k === "h" ? h.text.map((x) => x.s).join("") : p;
      return { id: p.replace(/^.*\/|\.md$/g, ""), title, blocks: blocks.filter((b) => b !== h) };
    })).then((x) => (topics = x)).catch((e) => (error = errorText(e)));
  });

  onMount(() => {
    catalog.load().catch((e) => (error = errorText(e)));
  });

  function setQuery() {
    const p = new URLSearchParams();
    if (tab === "wiki") {
      p.set("tab", "wiki");
      if (focus) p.set("f", focus);
    } else if (topic) p.set("topic", topic);
    const s = p.toString();
    history.replaceState(null, "", location.pathname + (s ? "?" + s : ""));
  }

  function openTopic(id: string) {
    topic = id;
    setQuery();
    document.getElementById("help-main")?.scrollTo({ top: 0 });
    window.scrollTo({ top: 0 });
  }

  // —— 百科 ——
  interface Entry {
    field: MetaField;
    path: string[];
    text: string; // 搜索用：标题、键、摘要、说明、建议
  }
  const sections = $derived(
    (catalog.meta?.pack.nav ?? [])
      .filter((s) => !s.generated)
      .map((s) => {
        const entries: Entry[] = [];
        // 对象展开为子字段；oneOf、列表等复合结构只列出自身
        const walk = (f: MetaField, path: string[]) => {
          if (f.type === "object" && f.fields?.length) {
            for (const c of f.fields) walk(c, [...path, c.key]);
            return;
          }
          void catalog.messagesLang;
          const text = [catalog.title(f), path.join("."), catalog.summary(f), catalog.detail(f), catalog.guide(f)].join(" ").toLowerCase();
          entries.push({ field: f, path, text });
        };
        for (const k of s.keys) {
          const f = catalog.top(k);
          if (f) walk(f, [k]);
        }
        return { id: s.id, title: catalog.navTitle(s), entries };
      }),
  );
  const needle = $derived(search.trim().toLowerCase());
  const visible = $derived(
    sections
      .map((s) => ({ ...s, entries: needle ? s.entries.filter((e) => e.text.includes(needle)) : s.entries }))
      .filter((s) => s.entries.length),
  );
  const total = $derived(visible.reduce((n, s) => n + s.entries.length, 0));

  // 深链接：?s= 滚动到某个分区（编辑器「高级」中的「本分区文档」，反馈 #48）
  const sectionLink = q.get("s") ?? "";
  $effect(() => {
    if (tab !== "wiki" || !sectionLink || focus || !sections.length) return;
    void tick().then(() => document.getElementById("s-" + sectionLink)?.scrollIntoView({ block: "start" }));
  });

  // 深链接：打开并滚动到 ?f= 指定的字段
  $effect(() => {
    if (tab !== "wiki" || !focus || !sections.length) return;
    void tick().then(() => document.getElementById("w-" + focus)?.scrollIntoView({ block: "start" }));
  });

  function toggle(id: string, open: boolean) {
    if (open) {
      focus = id;
      setQuery();
    } else if (focus === id) {
      focus = "";
      setQuery();
    }
  }

  const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
  const valueText = (v: unknown) => (Array.isArray(v) ? v.join(", ") : typeof v === "object" ? JSON.stringify(v) : String(v));
</script>

{#snippet inl(text: Inline[])}{#each text as x, i (i)}{#if x.k === "code"}<code>{x.s}</code>{:else if x.k === "bold"}<strong>{x.s}</strong>{:else if x.k === "link"}<a href={x.href} target={x.href.startsWith("/") ? undefined : "_blank"} rel={x.href.startsWith("/") ? undefined : "noopener noreferrer"}>{x.s}</a>{:else}{x.s}{/if}{/each}{/snippet}

<div class="page">
  <PageHeader title={t("help.title")} subtitle={t("help.subtitle")} />

  <div class="tabs" role="tablist" aria-label={t("help.title")}>
    <button role="tab" type="button" aria-selected={tab === "guide"} onclick={() => ((tab = "guide"), setQuery())}><Icon name="file" size={15} />{t("help.tab_guide")}</button>
    <button role="tab" type="button" aria-selected={tab === "wiki"} onclick={() => ((tab = "wiki"), setQuery())}><Icon name="rules" size={15} />{t("help.tab_wiki")}</button>
  </div>

  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  {#if tab === "guide"}
    <div class="layout">
      <nav class="toc" aria-label={t("help.tab_guide")}>
        <ol>
          {#each topics as x (x.id)}
            <li><button type="button" aria-current={current?.id === x.id ? "page" : undefined} onclick={() => openTopic(x.id)}>{x.title}</button></li>
          {/each}
        </ol>
      </nav>
      {#if current}
        <article class="doc" id="help-main">
          <h2 class="doc-title">{current.title}</h2>
          {#each current.blocks as b, i (i)}
            {#if b.k === "h"}
              {#if b.level <= 2}<h3 id={b.id}>{@render inl(b.text)}</h3>{:else}<h4 id={b.id}>{@render inl(b.text)}</h4>{/if}
            {:else if b.k === "p"}
              <p>{@render inl(b.text)}</p>
            {:else if b.k === "ul"}
              <ul>{#each b.items as it, j (j)}<li>{@render inl(it)}</li>{/each}</ul>
            {:else if b.k === "ol"}
              <ol>{#each b.items as it, j (j)}<li>{@render inl(it)}</li>{/each}</ol>
            {:else if b.k === "code"}
              <pre class="code ltr"><code>{b.text}</code></pre>
            {:else if b.k === "tip"}
              <aside class="tip"><Icon name="info" size={15} /><p>{@render inl(b.text)}</p></aside>
            {/if}
          {/each}
          {#if topics.length > 1}
            {@const idx = topics.findIndex((x) => x.id === current.id)}
            <footer class="pager">
              {#if idx > 0}<button class="btn ghost sm" type="button" onclick={() => openTopic(topics[idx - 1]!.id)}>← {topics[idx - 1]!.title}</button>{/if}
              <span class="spacer"></span>
              {#if idx < topics.length - 1}<button class="btn ghost sm" type="button" onclick={() => openTopic(topics[idx + 1]!.id)}>{topics[idx + 1]!.title} →</button>{/if}
            </footer>
          {/if}
        </article>
      {/if}
    </div>
  {:else}
    <div class="layout">
      <nav class="toc" aria-label={t("help.tab_wiki")}>
        <ol>
          {#each visible as s (s.id)}
            <li><a href={"#s-" + s.id}>{s.title}<span class="n">{s.entries.length}</span></a></li>
          {/each}
        </ol>
      </nav>
      <div class="wiki">
        <label class="search">
          <Icon name="search" size={15} />
          <input class="input" type="search" bind:value={search} placeholder={t("help.search")} aria-label={t("help.search")} />
          <span class="muted count">{t("help.count", { n: total })}</span>
        </label>
        {#if catalog.meta}
          {#each visible as s (s.id)}
            <section class="sec" id={"s-" + s.id} aria-labelledby={"sh-" + s.id}>
              <h2 id={"sh-" + s.id}>{s.title}</h2>
              {#each s.entries as e (e.path.join("."))}
                {@const id = e.path.join(".")}
                <details class="entry" id={"w-" + id} open={focus === id || undefined} ontoggle={(ev) => toggle(id, (ev.currentTarget as HTMLDetailsElement).open)}>
                  <summary>
                    <span class="title">{catalog.title(e.field)}</span>
                    <code class="key ltr">{id}</code>
                    {#if e.field.suggest !== undefined}
                      <span class="sugg" data-basis={e.field.basis}>{t("guide.suggest." + e.field.basis)} <span class="mono ltr">{valueText(e.field.suggest)}</span></span>
                    {:else if e.field.basis}
                      <span class="sugg" data-basis={e.field.basis}>{t("guide.basis." + e.field.basis)}</span>
                    {/if}
                    {#if e.field.default !== undefined && !same(e.field.default, e.field.suggest)}<span class="dflt muted">{t("field.default_is", { value: valueText(e.field.default) })}</span>{/if}
                    <span class="sum muted">{catalog.summary(e.field)}</span>
                  </summary>
                  <FieldGuide field={e.field} path={e.path} />
                </details>
              {/each}
            </section>
          {:else}
            <p class="muted empty">{t("help.no_match")}</p>
          {/each}
        {:else}
          <p class="muted" aria-busy="true">{t("common.loading")}</p>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }
  .tabs {
    display: flex;
    gap: var(--space-1);
    margin-block: var(--space-5) var(--space-6);
    border-block-end: 1px solid var(--border);
  }
  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 0;
    border-block-end: 2px solid transparent;
    margin-block-end: -1px;
    background: none;
    color: var(--fg-muted);
    font: inherit;
    cursor: pointer;
  }
  .tabs button[aria-selected="true"] {
    border-color: var(--accent);
    color: var(--fg);
    font-weight: 560;
  }
  .layout {
    display: grid;
    grid-template-columns: 200px minmax(0, 1fr);
    gap: var(--space-8);
    align-items: start;
  }
  .toc {
    position: sticky;
    inset-block-start: var(--space-4);
  }
  .toc ol {
    display: grid;
    gap: 1px;
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .toc button,
  .toc a {
    display: flex;
    justify-content: space-between;
    inline-size: 100%;
    padding: 6px var(--space-2);
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  .toc button:hover,
  .toc a:hover {
    background: var(--bg-hover);
    color: var(--fg);
    text-decoration: none;
  }
  .toc [aria-current="page"] {
    background: var(--bg-active);
    color: var(--fg);
    font-weight: 560;
  }
  .n {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    font-variant-numeric: tabular-nums;
  }

  /* —— 教程正文：限制行宽，层次靠字号与留白 —— */
  .doc {
    max-inline-size: 720px;
    line-height: 1.8;
  }
  .doc-title {
    font-size: var(--text-2xl, 1.6rem);
    margin-block-end: var(--space-4);
  }
  .doc h3 {
    font-size: var(--text-lg);
    margin-block: var(--space-8) var(--space-2);
  }
  .doc h4 {
    font-size: var(--text-md);
    margin-block: var(--space-5) var(--space-1);
  }
  .doc p,
  .doc ul,
  .doc ol {
    margin-block: var(--space-2);
  }
  .doc ul,
  .doc ol {
    padding-inline-start: 1.4em;
  }
  .doc li {
    margin-block: 2px;
  }
  .doc :global(code) {
    padding: 1px 5px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    font-family: var(--font-mono);
    font-size: 0.9em;
  }
  .code {
    margin-block: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-code);
    font-size: 12.5px;
    line-height: 1.6;
    overflow-x: auto;
  }
  .code code {
    padding: 0;
    background: none;
  }
  .tip {
    display: flex;
    gap: var(--space-2);
    margin-block: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-inline-start: 3px solid var(--accent);
    border-radius: var(--radius-md);
    background: var(--accent-soft);
  }
  .tip :global(.icon) {
    flex: none;
    margin-block-start: 5px;
    color: var(--accent-fg);
  }
  .tip p {
    margin: 0;
  }
  .pager {
    display: flex;
    gap: var(--space-3);
    margin-block-start: var(--space-10);
    padding-block-start: var(--space-4);
    border-block-start: 1px solid var(--border);
  }
  .spacer {
    flex: 1;
  }

  /* —— 百科 —— */
  .wiki {
    display: grid;
    gap: var(--space-6);
    min-inline-size: 0;
  }
  .search {
    position: sticky;
    inset-block-start: 0;
    z-index: 2;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: var(--space-2);
    background: var(--bg);
  }
  .search .input {
    flex: 1;
  }
  .count {
    font-size: var(--text-sm);
    white-space: nowrap;
  }
  .sec {
    display: grid;
    gap: var(--space-2);
    scroll-margin-block-start: var(--space-12, 4rem);
  }
  .sec h2 {
    font-size: var(--text-xl);
    margin-block-end: var(--space-1);
  }
  .entry {
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    scroll-margin-block-start: var(--space-12, 4rem);
  }
  .entry[open] {
    box-shadow: var(--shadow-sm);
  }
  .entry > :global(.guide) {
    margin: 0 var(--space-3) var(--space-3);
  }
  summary {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px var(--space-2);
    padding: var(--space-3) var(--space-4);
    cursor: pointer;
    list-style: none;
  }
  summary::-webkit-details-marker {
    display: none;
  }
  .title {
    font-weight: 600;
  }
  .key {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .sugg {
    padding: 0 8px;
    border-radius: var(--radius-full);
    background: var(--bg-hover);
    color: var(--fg-muted);
    font-size: var(--text-xs);
    line-height: 20px;
  }
  .sugg[data-basis="official"] {
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .dflt {
    font-size: var(--text-xs);
  }
  .sum {
    flex-basis: 100%;
    font-size: var(--text-sm);
  }
  .empty {
    padding: var(--space-8) 0;
    text-align: center;
  }

  @media (max-width: 767px) {
    .page {
      padding: var(--space-4) var(--space-4) var(--space-8);
    }
    .layout {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--space-4);
    }
    .toc {
      position: static;
      min-inline-size: 0;
    }
    .doc,
    .wiki {
      min-inline-size: 0;
      overflow-wrap: anywhere;
    }
    .toc ol {
      display: flex;
      gap: var(--space-1);
      overflow-x: auto;
      padding-block-end: var(--space-1);
    }
    .toc li {
      flex: none;
    }
    .toc button,
    .toc a {
      gap: var(--space-2);
      white-space: nowrap;
    }
  }
</style>
