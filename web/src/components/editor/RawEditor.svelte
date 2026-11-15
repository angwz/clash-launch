<script lang="ts">
  // 后备隐藏能源：直接编辑 YAML（反馈 #70）。显示与预览一致的完整配置，用户在上面直接修改；
  // 停止输入约半秒后由服务端换算出「改了哪些地方」，存为原始编辑层（只记差异），
  // 订阅与规则更新后修改继续生效。右侧预览随之更新。
  // 原有的原始编辑本身有错、配置无法生成时，退回为直接编辑原始编辑层，先把错误修好。
  import { onMount } from "svelte";
  import type { EditorView } from "@codemirror/view";
  import Icon from "../Icon.svelte";
  import { api, type Diagnostic } from "../../lib/api/client";
  import { catalog } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  let { editor, focusKey, onClose }: { editor: EditorState; focusKey?: string; onClose: () => void } = $props();

  const INTRO = "cl.raw.intro.full";
  const DELAY = 500;
  let host: HTMLDivElement | undefined = $state();
  let view: EditorView | undefined;
  let intro = $state(false);
  let loading = $state(true);
  let mode = $state<"full" | "fix">("full");
  let problems = $state.raw<Diagnostic[]>([]);
  let invalid = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let pending: AbortController | undefined;
  let unsent: string | null = null;

  // 把完整 YAML 换算成原始编辑层并写回草稿（之后照常预览与自动保存）
  async function absorb(text: string): Promise<void> {
    unsent = null;
    if (!editor.profile) return;
    pending?.abort();
    const ac = new AbortController();
    pending = ac;
    try {
      const res = await api.post<{ ok: boolean; raw: string; diagnostics: Diagnostic[] }>(
        "/api/v1/profiles/" + editor.profile.id + "/absorb",
        { doc: editor.text, yaml: text },
        { signal: ac.signal },
      );
      invalid = !res.ok;
      problems = res.ok ? res.diagnostics.filter((d) => d.code === "layer.unapplied") : res.diagnostics;
      if (res.ok && res.raw !== editor.raw) editor.setRaw(res.raw);
    } catch (e) {
      if ((e as Error).name !== "AbortError") toasts.push("err", errorText(e));
    }
  }

  function changed(text: string) {
    if (mode === "fix") {
      editor.setRaw(text);
      return;
    }
    unsent = text;
    clearTimeout(timer);
    timer = setTimeout(() => void absorb(text), DELAY);
  }

  onMount(() => {
    try {
      intro = localStorage.getItem(INTRO) !== "1";
    } catch {
      intro = true;
    }
    let disposed = false;
    void (async () => {
      // 以最新的预览为起点：有尚未完成的预览时先等它
      if (!editor.preview || editor.previewing) await editor.runPreview(false);
      const { createRawEditor, revealKey } = await import("../../lib/cm");
      if (disposed || !host) return;
      const pv = editor.preview;
      mode = !pv || pv.fatal ? "fix" : "full";
      view = createRawEditor({
        parent: host,
        doc: mode === "full" ? pv!.yaml : editor.raw,
        sections: catalog.meta?.sections ?? [],
        title: (f) => catalog.title(f),
        summary: (f) => catalog.summary(f),
        tags:
          mode === "fix"
            ? [
                { tag: "!append", info: t("raw.tag.append") },
                { tag: "!prepend", info: t("raw.tag.prepend") },
                { tag: "!replace", info: t("raw.tag.replace") },
                { tag: "!delete", info: t("raw.tag.delete") },
                { tag: "!patch-by:", info: t("raw.tag.patch_by") },
              ]
            : [],
        placeholder: t("raw.placeholder"),
        label: t("raw.title"),
        onChange: changed,
      });
      loading = false;
      if (focusKey) revealKey(view, focusKey);
      else view.focus();
    })();
    return () => {
      disposed = true;
      clearTimeout(timer);
      // 离开时还没换算的输入立即提交，不丢修改
      if (unsent !== null) void absorb(unsent);
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

  {#if mode === "fix" && !loading}
    <p class="note warn" role="alert">{t("raw.fix_hint")}</p>
  {:else if intro}
    <div class="intro" role="note">
      <p>{t("raw.intro")}</p>
      <button class="btn sm" type="button" onclick={dismissIntro}>{t("raw.intro_ok")}</button>
    </div>
  {/if}

  {#if problems.length}
    <ul class="note" class:warn={!invalid} class:err={invalid} role="status">
      {#if invalid}<li><strong>{t("raw.invalid")}</strong></li>{/if}
      {#each problems as d, i (i)}<li>{catalog.diagMessage(d.code, d.args)}</li>{/each}
    </ul>
  {/if}

  <div class="host ltr" bind:this={host} aria-busy={loading}>
    {#if loading}<p class="muted loading">{t("preview.loading")}</p>{/if}
  </div>

  {#if mode === "full" && !loading}
    <details class="recorded">
      <summary>{t("raw.recorded")}</summary>
      {#if editor.raw.trim()}
        <pre class="ltr">{editor.raw}</pre>
      {:else}
        <p class="muted">{t("raw.recorded_none")}</p>
      {/if}
      <p class="hint">{t("raw.recorded_hint")}</p>
    </details>
  {/if}
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
  .note {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    padding-inline-start: var(--space-4);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    list-style: none;
  }
  .note.warn {
    background: color-mix(in srgb, var(--warn) 10%, transparent);
    color: var(--fg);
  }
  .note.err {
    background: color-mix(in srgb, var(--err) 10%, transparent);
    color: var(--fg);
  }
  .recorded {
    font-size: var(--text-sm);
  }
  .recorded summary {
    cursor: pointer;
    color: var(--fg-muted);
  }
  .recorded pre {
    margin-block: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-code);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow-x: auto;
  }
</style>
