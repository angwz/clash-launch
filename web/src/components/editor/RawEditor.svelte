<script lang="ts">
  // 后备隐藏能源：原始编辑器（第 16.4 节）。入口只有命令面板的命令模式与分节的 ⋯ 菜单。
  // 编辑内容实时写入文档的 raw（原文保存），右侧预览随之更新；首次进入时显示一次说明。
  import { onMount } from "svelte";
  import type { EditorView } from "@codemirror/view";
  import Icon from "../Icon.svelte";
  import { catalog } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";

  let { editor, focusKey, onClose }: { editor: EditorState; focusKey?: string; onClose: () => void } = $props();

  const INTRO = "cl.raw.intro";
  let host: HTMLDivElement | undefined = $state();
  let view: EditorView | undefined;
  let intro = $state(false);
  let loading = $state(true);

  onMount(() => {
    try {
      intro = localStorage.getItem(INTRO) !== "1";
    } catch {
      intro = true;
    }
    let disposed = false;
    void import("../../lib/cm").then(({ createRawEditor, revealKey }) => {
      if (disposed || !host) return;
      view = createRawEditor({
        parent: host,
        doc: editor.raw,
        sections: catalog.meta?.sections ?? [],
        title: (f) => catalog.title(f),
        summary: (f) => catalog.summary(f),
        tags: [
          { tag: "!append", info: t("raw.tag.append") },
          { tag: "!prepend", info: t("raw.tag.prepend") },
          { tag: "!replace", info: t("raw.tag.replace") },
          { tag: "!delete", info: t("raw.tag.delete") },
          { tag: "!patch-by:", info: t("raw.tag.patch_by") },
        ],
        placeholder: t("raw.placeholder"),
        label: t("raw.title"),
        onChange: (text) => editor.setRaw(text),
      });
      loading = false;
      if (focusKey) revealKey(view, focusKey);
      else view.focus();
    });
    return () => {
      disposed = true;
      view?.destroy();
    };
  });

  function dismissIntro() {
    intro = false;
    try {
      localStorage.setItem(INTRO, "1");
    } catch {
      /* 下次还会显示 */
    }
  }
</script>

<section class="raw" aria-labelledby="raw-title">
  <header class="head">
    <div class="title">
      <span class="bolt"><Icon name="bolt" size={15} /></span>
      <div>
        <h2 id="raw-title">{t("raw.title")}</h2>
        <p class="muted sub">{t("raw.subtitle")}</p>
      </div>
    </div>
    <button class="btn" type="button" onclick={onClose}><Icon name="x" size={14} />{t("raw.close")}</button>
  </header>

  {#if intro}
    <div class="intro" role="note">
      <p>{t("raw.intro")}</p>
      <ul class="tags">
        <li><code>!append</code> {t("raw.tag.append")}</li>
        <li><code>!prepend</code> {t("raw.tag.prepend")}</li>
        <li><code>!replace</code> {t("raw.tag.replace")}</li>
        <li><code>!delete</code> {t("raw.tag.delete")}</li>
        <li><code>!patch-by:name</code> {t("raw.tag.patch_by")}</li>
      </ul>
      <button class="btn sm" type="button" onclick={dismissIntro}>{t("raw.intro_ok")}</button>
    </div>
  {/if}

  <div class="host ltr" bind:this={host} aria-busy={loading}>
    {#if loading}<p class="muted loading">{t("common.loading")}</p>{/if}
  </div>
</section>

<style>
  .raw {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    block-size: 100%;
    min-block-size: 480px;
  }
  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .title {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .bolt {
    display: grid;
    place-items: center;
    inline-size: 30px;
    block-size: 30px;
    border-radius: var(--radius-md);
    background: var(--raw-soft);
    color: var(--raw);
  }
  h2 {
    font-size: var(--text-xl);
  }
  .sub {
    font-size: var(--text-sm);
  }
  .intro {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border: 1px solid color-mix(in srgb, var(--raw) 30%, var(--border));
    border-radius: var(--radius-lg);
    background: var(--raw-soft);
    font-size: var(--text-sm);
  }
  .tags {
    display: grid;
    gap: 2px;
    color: var(--fg-muted);
  }
  .tags code {
    color: var(--syn-tag);
    margin-inline-end: var(--space-1);
  }
  .intro .btn {
    justify-self: start;
  }
  .host {
    flex: 1;
    min-block-size: 360px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
    background: var(--bg-code);
  }
  .host :global(.cm-editor) {
    block-size: 100%;
  }
  .loading {
    padding: var(--space-4);
  }
</style>
