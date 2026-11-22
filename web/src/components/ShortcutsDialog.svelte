<script lang="ts" module>
  // 快捷键一览（需求 R45）：按 ? 打开（不在输入框中时），也可从命令面板打开。
  // 这里只列出界面中真实存在的快捷键；新增快捷键时同步这张表。
  class ShortcutsState {
    open = $state(false);
  }
  export const shortcuts = new ShortcutsState();
</script>

<script lang="ts">
  import { t } from "../lib/i18n/i18n.svelte";
  import Dialog from "./Dialog.svelte";

  // macOS 上显示 ⌘，其他系统显示 Ctrl；两种按法在代码里都接受（ctrlKey || metaKey）
  const mac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
  const mod = mac ? "⌘" : "Ctrl";
  const alt = mac ? "⌥" : "Alt";

  // 每一项：按键（每个元素是一个键，| 分隔「或」）、说明；seq 表示依次按下而不是同时按下。
  // 用 $derived 直接调用 t()：切换语言时跟着更新
  type Row = { keys: string[]; label: string; seq?: boolean };
  const groups = $derived<{ title: string; rows: Row[] }[]>([
    { title: t("shortcuts.g.global"), rows: [
      { keys: [mod, "K"], label: t("shortcuts.palette") },
      { keys: ["?"], label: t("shortcuts.this") },
      { keys: ["Esc"], label: t("shortcuts.close") },
    ] },
    { title: t("shortcuts.g.palette"), rows: [
      { keys: ["↑|↓"], label: t("shortcuts.move") },
      { keys: ["Enter"], label: t("shortcuts.open") },
      { keys: [">"], label: t("shortcuts.commands") },
    ] },
    { title: t("shortcuts.g.editor"), rows: [
      { keys: [mod, "S"], label: t("shortcuts.save") },
      { keys: ["Esc"], label: t("shortcuts.preview") },
      { keys: [alt, "↑|↓"], label: t("shortcuts.source_order") },
    ] },
    { title: t("shortcuts.g.sort"), rows: [
      { keys: ["↑|↓|←|→"], label: t("shortcuts.sort") },
    ] },
    { title: t("shortcuts.g.code"), rows: [
      { keys: [mod, "F"], label: t("shortcuts.find") },
      { keys: [mod, "Z"], label: t("shortcuts.undo") },
      { keys: [mod, "Shift", "Z"], label: t("shortcuts.redo") },
      { keys: ["Tab"], label: t("shortcuts.indent") },
      { keys: ["Esc", "Tab"], label: t("shortcuts.leave"), seq: true },
    ] },
    { title: t("shortcuts.g.explain"), rows: [
      { keys: ["←|→"], label: t("shortcuts.steps") },
    ] },
    { title: t("shortcuts.g.chat"), rows: [
      { keys: ["Enter"], label: t("shortcuts.send") },
      { keys: ["Shift", "Enter"], label: t("shortcuts.newline") },
    ] },
  ]);
</script>

<Dialog bind:open={shortcuts.open} title={t("shortcuts.title")} wide>
  <div class="cols">
    {#each groups as g (g.title)}
      <section>
        <h3>{g.title}</h3>
        <dl>
          {#each g.rows as r (r.label)}
            <div class="row">
              <dt>{r.label}</dt>
              <dd>
                {#each r.keys as k, i (i)}
                  {#if i > 0}<span class="plus">{r.seq ? t("shortcuts.then") : "+"}</span>{/if}
                  {#each k.split("|") as part, j (j)}{#if j > 0}<span class="plus">/</span>{/if}<kbd class="kbd">{part}</kbd>{/each}
                {/each}
              </dd>
            </div>
          {/each}
        </dl>
      </section>
    {/each}
  </div>
</Dialog>

<style>
  .cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
    gap: var(--space-5) var(--space-6);
  }
  h3 {
    margin-block-end: var(--space-2);
    font-size: var(--text-sm);
    color: var(--fg-muted);
  }
  dl {
    display: grid;
    gap: var(--space-2);
  }
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  dd {
    display: flex;
    flex: none;
    align-items: center;
    gap: 3px;
  }
  .plus {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
</style>
