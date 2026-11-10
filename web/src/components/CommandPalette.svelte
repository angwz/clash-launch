<script lang="ts">
  // 命令面板（第 16.3 节）：Ctrl/⌘+K。搜索字段（含摘要与详细说明全文）、跳转页面、切换主题与语言。
  // 输入 > 进入命令模式；「原始编辑」只在命令模式中出现（第 16.4 节）。
  import { t } from "../lib/i18n/i18n.svelte";
  import { palette, search, type PaletteItem } from "../lib/palette.svelte";
  import Icon from "./Icon.svelte";

  let el: HTMLDialogElement | undefined = $state();
  let input: HTMLInputElement | undefined = $state();
  let active = $state(0);

  const results = $derived(palette.open ? search(palette.items(), palette.query) : []);
  const commandMode = $derived(palette.query.startsWith(">"));

  $effect(() => {
    if (!el) return;
    if (palette.open && !el.open) {
      el.showModal();
      active = 0;
      queueMicrotask(() => input?.select());
    } else if (!palette.open && el.open) el.close();
  });

  $effect(() => {
    void palette.query;
    active = 0;
  });

  function run(it: PaletteItem | undefined) {
    if (!it) return;
    palette.close();
    it.run();
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      active = Math.min(active + 1, results.length - 1);
      scrollActive();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      active = Math.max(active - 1, 0);
      scrollActive();
    } else if (e.key === "Enter") {
      e.preventDefault();
      run(results[active]);
    }
  }

  function scrollActive() {
    queueMicrotask(() => el?.querySelector(`[data-i="${active}"]`)?.scrollIntoView({ block: "nearest" }));
  }

  const kindLabel = (k: PaletteItem["kind"]) => t("palette.kind." + k);
</script>

<!-- cancel（Esc）同步触发，close 事件是异步排队的：在 cancel 中更新状态，避免紧接着的 Ctrl+K 读到旧状态 -->
<dialog bind:this={el} class="palette" oncancel={() => palette.close()} onclose={() => palette.close()} aria-label={t("palette.title")}>
  <div class="bar">
    <Icon name={commandMode ? "command" : "search"} size={17} />
    <input
      bind:this={input}
      bind:value={palette.query}
      onkeydown={onKey}
      placeholder={t("palette.placeholder")}
      aria-label={t("palette.title")}
      role="combobox"
      aria-expanded="true"
      aria-controls="palette-list"
      aria-activedescendant={results.length ? "pi-" + active : undefined}
      autocomplete="off"
      spellcheck="false"
    />
    <span class="kbd">Esc</span>
  </div>
  <ul id="palette-list" role="listbox">
    {#each results as it, i (it.id)}
      <li id={"pi-" + i} role="option" aria-selected={i === active} data-i={i}>
        <button type="button" class:active={i === active} onclick={() => run(it)} onpointermove={() => (active = i)} tabindex="-1">
          <span class="ico"><Icon name={it.icon ?? (it.kind === "field" ? "settings" : it.kind === "page" ? "arrow-right" : "command")} size={15} /></span>
          <span class="title">{it.title}</span>
          {#if it.hint}<span class="hint mono ltr">{it.hint}</span>{/if}
          <span class="kind">{kindLabel(it.kind)}</span>
        </button>
      </li>
    {:else}
      <li class="empty">{t("palette.empty")}</li>
    {/each}
  </ul>
  <div class="foot">
    <span><span class="kbd">↑</span><span class="kbd">↓</span> {t("palette.move")}</span>
    <span><span class="kbd">Enter</span> {t("palette.select")}</span>
    <span><span class="kbd">&gt;</span> {t("palette.commands")}</span>
  </div>
</dialog>

<style>
  .palette {
    inline-size: min(640px, calc(100vw - 2 * var(--space-4)));
    margin-block-start: 12vh;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-lg);
  }
  .palette::backdrop {
    background: var(--scrim);
  }
  .bar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding-inline: var(--space-4);
    border-block-end: 1px solid var(--border);
    color: var(--fg-subtle);
  }
  input {
    flex: 1;
    block-size: 50px;
    border: 0;
    background: transparent;
    color: var(--fg);
    font: inherit;
    font-size: var(--text-lg);
    outline: none;
  }
  ul {
    max-block-size: min(420px, 55dvh);
    overflow-y: auto;
    padding: var(--space-1);
  }
  li button {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    inline-size: 100%;
    min-block-size: 38px;
    padding-inline: var(--space-3);
    border: 0;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--fg);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  li button.active {
    background: var(--bg-active);
  }
  .ico {
    display: grid;
    place-items: center;
    color: var(--fg-subtle);
  }
  .title {
    flex: none;
    max-inline-size: 60%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hint {
    flex: 1;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .kind {
    margin-inline-start: auto;
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .empty {
    padding: var(--space-6);
    text-align: center;
    color: var(--fg-muted);
  }
  .foot {
    display: flex;
    gap: var(--space-4);
    padding-block: var(--space-2);
    padding-inline: var(--space-4);
    border-block-start: 1px solid var(--border);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .foot .kbd {
    margin-inline-end: 2px;
  }
</style>
