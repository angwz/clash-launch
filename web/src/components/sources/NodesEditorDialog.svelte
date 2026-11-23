<script lang="ts">
  // 节点源内容的大编辑框（反馈 #113、#114）：近乎全屏，YAML 高亮、字段与取值的自动提示、右侧缩略图。
  // 编辑的是草稿：停顿片刻后用服务端的同一套解析（节点预览接口，逐个经 mihomo 校验）检查草稿，
  // 问题标在对应的行上；没有阻断性问题时才能「完成」，把草稿写回页面（保存仍由页面完成）。
  // 关闭而不完成时草稿保留在内存里，再次打开时继续编辑，不会因误按 Esc 丢失。
  import { untrack } from "svelte";
  import Dialog from "../Dialog.svelte";
  import Icon from "../Icon.svelte";
  import StatusBadge from "../StatusBadge.svelte";
  import { api, type Diagnostic, type Node } from "../../lib/api/client";
  import { catalog } from "../../lib/catalog.svelte";
  import type { LineDiagnostic, RulesEditor } from "../../lib/cm-rules";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";

  let {
    open = $bindable(false),
    value = $bindable(""),
    sourceId,
    sourceName,
    processing,
  }: { open?: boolean; value?: string; sourceId: string; sourceName: string; processing: () => object } = $props();

  // 这些诊断说明内容有节点没能加载，完成前必须修正；其余（重复、信息项、按规则过滤）只是提示
  const BLOCKING = new Set(["node.invalid", "node.not_proxy", "node.unrecognized", "node.line_skipped"]);

  let draft = $state("");
  let pendingDraft: string | null = null; // 关闭而未完成时保留的草稿
  let restored = $state(false);
  let host = $state<HTMLDivElement>();
  let ed: RulesEditor | null = null;
  let checking = $state(false);
  let checkError = $state("");
  let nodes = $state<Node[] | null>(null);
  let problems = $state<{ line: number; severity: "error" | "warning" | "info"; message: string; blocking: boolean }[]>([]);
  let syntaxCount = $state(0);
  let checkedText = $state("");

  // 打开：从页面内容或上次保留的草稿开始
  $effect(() => {
    if (!open) return;
    untrack(() => {
      restored = pendingDraft !== null && pendingDraft !== value;
      draft = restored ? pendingDraft! : value;
      pendingDraft = null;
    });
    return () => {
      // 关闭（含 Esc）：没完成的修改留到下次
      if (draft !== value) pendingDraft = draft;
    };
  });

  // 编辑器：对话框打开后按需加载
  $effect(() => {
    if (!open || !host) return;
    let alive = true;
    let made: RulesEditor | null = null;
    void Promise.all([import("../../lib/cm-rules"), import("../../lib/cm-nodes"), catalog.load()]).then(([rules, nodesMod]) => {
      if (!alive || !host) return;
      const extensions = nodesMod.nodeExtensions({
        proxies: catalog.top("proxies"),
        title: (f) => catalog.title(f),
        summary: (f) => catalog.summary(f),
        enumLabel: (f, v) => catalog.enumLabel(f, v),
        variantTitle: (id) => catalog.messages[`catalog.${id}.title`] ?? "",
      });
      made = rules.createRulesEditor({
        parent: host,
        doc: untrack(() => draft),
        lang: "yaml",
        label: t("source.content"),
        placeholder: "",
        fill: true,
        extensions,
        onChange: (text) => {
          draft = text;
          syntaxCount = nodesMod.yamlErrors(text).length;
        },
      });
      syntaxCount = nodesMod.yamlErrors(untrack(() => draft)).length;
      ed = made;
      made.view.focus();
    });
    return () => {
      alive = false;
      made?.view.destroy();
      ed = null;
    };
  });

  // 停顿 500 ms 后检查草稿；只采用最后一次请求的结果
  let seq = 0;
  $effect(() => {
    if (!open) return;
    const text = draft;
    const n = ++seq;
    checking = true;
    const timer = setTimeout(async () => {
      try {
        const r = await api.post<{ nodes: Node[]; diagnostics: Diagnostic[] }>("/api/v1/sources/" + sourceId + "/nodes/preview", { content: text, processing: processing() });
        if (n !== seq) return;
        nodes = r.nodes;
        problems = r.diagnostics.map((d) => ({
          line: lineOf(text, d),
          severity: d.severity,
          message: catalog.diagMessage(d.code, d.args, new Map([[sourceId, sourceName]])),
          blocking: BLOCKING.has(d.code) || d.severity === "error",
        }));
        checkError = "";
        checkedText = text;
      } catch (e) {
        if (n === seq) checkError = errorText(e);
      } finally {
        if (n === seq) checking = false;
      }
    }, 500);
    return () => clearTimeout(timer);
  });

  // 把诊断对应到行：跳过的行有行号；按节点名的诊断找写着这个名字的行；按序号的找第 n 个列表项
  function lineOf(text: string, d: Diagnostic): number {
    const a = d.args ?? {};
    const lines = text.split("\n");
    if (a.line && /^\d+$/.test(a.line)) return Number(a.line);
    if (a.name) {
      const i = lines.findIndex((l) => l.includes(a.name!));
      if (i >= 0) return i + 1;
    }
    if (a.index && /^\d+$/.test(a.index)) {
      let k = -1;
      for (let i = 0; i < lines.length; i++) if (/^\s*- /.test(lines[i]!) && ++k === Number(a.index)) return i + 1;
    }
    return 0;
  }

  // 行内标记
  $effect(() => {
    const list: LineDiagnostic[] = problems.filter((p) => p.line > 0).map((p) => ({ line: p.line, severity: p.severity === "error" || p.blocking ? "error" : p.severity, message: p.message }));
    ed?.diagnose(list);
  });

  const blocking = $derived(problems.filter((p) => p.blocking).length + syntaxCount);
  const notes = $derived(problems.filter((p) => !p.blocking).length);
  const fresh = $derived(!checking && checkedText === draft && !checkError);
  const canFinish = $derived(fresh && blocking === 0 && (!!draft.trim() ? (nodes?.length ?? 0) > 0 : true));

  function finish() {
    if (!canFinish) return;
    value = draft;
    pendingDraft = null;
    open = false;
  }
  function discard() {
    pendingDraft = null;
    draft = value;
    open = false;
  }
</script>

<Dialog bind:open title={t("source.content_editor", { name: sourceName })} full>
  <div class="wrap">
    {#if restored}<p class="restored" role="status">{t("source.editor_restored")}</p>{/if}
    <div class="host" bind:this={host}></div>
    <div class="status" role="status" aria-live="polite">
      {#if checkError}
        <StatusBadge tone="err" label={t("source.check_failed")} /><span class="muted">{checkError}</span>
      {:else if !fresh}
        <span class="spin"><Icon name="refresh" size={13} /></span><span class="muted">{t("source.check_pending")}</span>
      {:else if blocking}
        <StatusBadge tone="err" label={t("source.check_bad", { n: blocking })} />
      {:else}
        <StatusBadge tone="ok" label={t("source.check_ok", { n: nodes?.length ?? 0 })} />
      {/if}
      {#if fresh && notes}<span class="muted">{t("source.check_notes", { n: notes })}</span>{/if}
      <span class="hint muted">{t("source.editor_hint")}</span>
    </div>
    {#if fresh && problems.length}
      <ul class="problems" aria-label={t("rules.problems")}>
        {#each problems as p, i (i)}
          <li>
            <StatusBadge tone={p.blocking ? "err" : p.severity === "warning" ? "warn" : "info"} label={p.line ? t("source.line_n", { n: p.line }) : "—"} />
            {#if p.line}<button class="msg" type="button" onclick={() => ed?.reveal(p.line)}>{p.message}</button>{:else}<span class="msg">{p.message}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
  {#snippet footer()}
    <button class="btn" type="button" onclick={discard}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" disabled={!canFinish} title={canFinish ? undefined : t("source.finish_blocked")} onclick={finish}>{t("source.content_done")}</button>
  {/snippet}
</Dialog>

<style>
  .wrap {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    block-size: 100%;
  }
  .host {
    flex: 1;
    min-block-size: 0;
  }
  .restored {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-sm);
  }
  .status {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .status .hint {
    margin-inline-start: auto;
    font-size: var(--text-xs);
  }
  .spin {
    display: inline-flex;
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .problems {
    display: grid;
    gap: var(--space-1);
    max-block-size: 9em;
    overflow-y: auto;
    font-size: var(--text-sm);
  }
  .problems li {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }
  .msg {
    border: 0;
    background: none;
    padding: 0;
    color: inherit;
    font: inherit;
    text-align: start;
  }
  button.msg {
    cursor: pointer;
    text-decoration: underline;
    text-decoration-color: var(--border);
  }
</style>
