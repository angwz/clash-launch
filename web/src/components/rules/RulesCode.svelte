<script lang="ts">
  // 规则源文本的代码编辑器（O2）：CodeMirror 按需加载；加载完成前与失败时退回普通文本框，内容不受影响。
  import type { LineDiagnostic, RulesEditor, RulesLang } from "../../lib/cm-rules";

  let {
    value = $bindable(""),
    lang,
    label,
    placeholder,
    diagnostics = [],
    id,
    fill = false,
  }: { value?: string; lang: RulesLang; label: string; placeholder: string; diagnostics?: LineDiagnostic[]; id: string; fill?: boolean } = $props();

  let host = $state<HTMLDivElement>();
  let ed = $state<RulesEditor | null>(null);
  let failed = $state(false);

  $effect(() => {
    if (!host) return;
    let alive = true;
    let made: RulesEditor | null = null;
    import("../../lib/cm-rules")
      .then(({ createRulesEditor }) => {
        if (!alive || !host) return;
        made = createRulesEditor({ parent: host, doc: value, lang, label, placeholder, fill, onChange: (t) => (value = t) });
        if (fill) made.view.focus();
        ed = made;
      })
      .catch(() => (failed = true));
    return () => {
      alive = false;
      made?.destroy();
      ed = null;
    };
  });

  // 外部改动（如重新加载）同步进编辑器；编辑器自己触发的改动内容相同，不会重复写入
  $effect(() => {
    const v = value;
    if (ed && ed.view.state.doc.toString() !== v) ed.view.dispatch({ changes: { from: 0, to: ed.view.state.doc.length, insert: v } });
  });
  $effect(() => {
    ed?.diagnose(diagnostics);
  });

  /** 滚动到某一行（诊断列表点击时调用）。 */
  export function reveal(line: number) {
    ed?.reveal(line);
  }
</script>

{#if failed}
  <textarea {id} class="textarea mono ltr" rows="12" bind:value spellcheck="false" {placeholder} aria-label={label}></textarea>
{:else}
  <div class="host" class:fill bind:this={host} {id}></div>
{/if}

<style>
  .host {
    min-block-size: 12em;
  }
  .host.fill {
    block-size: 100%;
  }
</style>
