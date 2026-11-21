<script lang="ts">
  // 渲染 lib/md 解析出的语法树（帮助教程、用户协议）。逐节点渲染、不经过 innerHTML，
  // 管理员改写的协议文本也只会成为文字，不会注入 HTML；链接只认 http(s) 与站内路径（见 md.ts）。
  import Icon from "./Icon.svelte";
  import type { Block, Inline } from "../lib/md";

  let { blocks }: { blocks: Block[] } = $props();
</script>

{#snippet inl(text: Inline[])}{#each text as x, i (i)}{#if x.k === "code"}<code>{x.s}</code>{:else if x.k === "bold"}<strong>{x.s}</strong>{:else if x.k === "link"}<a href={x.href} target={x.href.startsWith("/") ? undefined : "_blank"} rel={x.href.startsWith("/") ? undefined : "noopener noreferrer"}>{x.s}</a>{:else}{x.s}{/if}{/each}{/snippet}

<div class="md">
  {#each blocks as b, i (i)}
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
</div>

<style>
  /* 正文中的链接加下划线：只靠颜色区分时与正文的对比度不足 3:1（WCAG 1.4.1） */
  .md a {
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .md h3 {
    font-size: var(--text-lg);
    margin-block: var(--space-8) var(--space-2);
  }
  .md h3:first-child {
    margin-block-start: 0;
  }
  .md h4 {
    font-size: var(--text-md);
    margin-block: var(--space-5) var(--space-1);
  }
  .md p,
  .md ul,
  .md ol {
    margin-block: var(--space-2);
  }
  /* 列表标记可见并悬挂在正文左缘之外的缩进里：列表文字与段落的对齐关系一目了然（反馈 #56） */
  .md ul,
  .md ol {
    padding-inline-start: 1.4em;
  }
  .md ul {
    list-style: disc;
  }
  .md ol {
    list-style: decimal;
  }
  .md li::marker {
    color: var(--fg-subtle);
  }
  .md li {
    margin-block: 2px;
  }
  .md code {
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
</style>
