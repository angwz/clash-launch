<script lang="ts">
  // 预览栏（第 16.1 节）：最终 YAML（只读）、与当前版本的差异、诊断列表（点击定位字段）。
  import Icon from "../Icon.svelte";
  import YamlView from "./YamlView.svelte";
  import { yamlRange } from "../../lib/yaml-range";
  import { catalog } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  // onHide：停靠时「收起」，浮层时「关闭」；手机视图由底部切换控制，不传
  let { editor, onLocate, onHide, floating = false }: { editor: EditorState; onLocate: (path: string) => void; onHide?: () => void; floating?: boolean } = $props();

  let tab = $state<"yaml" | "diff" | "diag">("yaml");
  const p = $derived(editor.preview);
  const lines = $derived(p ? p.yaml.split("\n") : []);
  const mark = $derived(editor.hover && lines.length ? yamlRange(lines, editor.hover) : null);
  const changed = $derived(p ? p.diff.filter((d) => d.op !== " ").length : 0);
  const diffLines = $derived(p ? p.diff.map((d) => d.text) : []);
  const diffOps = $derived(p ? p.diff.map((d) => d.op) : []);
  const diags = $derived(p?.diagnostics ?? []);
  // 诊断参数中的源与规则源 ID → 名称
  const names = $derived(new Map<string, string>([...editor.sources.map((s) => [s.id, s.name] as [string, string]), ...editor.ruleSources.map((r) => [r.id, r.name] as [string, string])]));
  function readablePath(path: string): string {
    let out = path;
    for (const [id, name] of names) out = out.split(id).join(name);
    return out;
  }

  async function copy() {
    if (!p) return;
    try {
      await navigator.clipboard.writeText(p.yaml);
      toasts.push("ok", t("common.copied"));
    } catch {
      toasts.push("err", t("common.copy_failed"));
    }
  }

  const sevIcon = (s: string) => (s === "error" ? "error" : s === "warning" ? "warning" : "info") as "error" | "warning" | "info";
</script>

<div class="pane">
  <div class="tabs" role="tablist" aria-label={t("preview.title")}>
    <button role="tab" type="button" aria-selected={tab === "yaml"} onclick={() => (tab = "yaml")}>{t("preview.yaml")}</button>
    <button role="tab" type="button" aria-selected={tab === "diff"} onclick={() => (tab = "diff")}>
      {t("preview.diff")}{#if changed}<span class="n">{changed}</span>{/if}
    </button>
    <button role="tab" type="button" aria-selected={tab === "diag"} onclick={() => (tab = "diag")}>
      {t("preview.diagnostics")}{#if diags.length}<span class="n" class:err={editor.counts.errors > 0}>{diags.length}</span>{/if}
    </button>
    <span class="spacer"></span>
    {#if editor.previewing}<span class="busy" aria-live="polite">{t("preview.updating")}</span>{/if}
    {#if tab === "yaml" && p}
      <button
        class="btn ghost sm fmt"
        type="button"
        aria-pressed={editor.proxiesBlock}
        title={t("preview.proxies_block_hint")}
        onclick={() => editor.setProxiesBlock(!editor.proxiesBlock)}
      >{editor.proxiesBlock ? t("preview.proxies_block_on") : t("preview.proxies_block_off")}</button>
    {/if}
    {#if tab === "yaml" && p}
      <button class="btn ghost icon sm" type="button" aria-label={t("common.copy")} title={t("common.copy")} onclick={copy}><Icon name="copy" size={14} /></button>
    {/if}
    {#if onHide}
      <button class="btn ghost icon sm" type="button" aria-label={floating ? t("common.close") : t("preview.hide")} title={floating ? t("common.close") + " (Esc)" : t("preview.hide")} onclick={onHide}><Icon name={floating ? "x" : "chevron-right"} size={14} /></button>
    {/if}
  </div>

  <div class="body" role="tabpanel">
    {#if !p}
      <p class="placeholder muted">{editor.error || t("preview.loading")}</p>
    {:else if p.fatal}
      <div class="fatal" role="alert">
        <Icon name="error" />
        <div>
          <strong>{t("preview.fatal")}</strong>
          <p class="mono ltr">{p.fatal}</p>
        </div>
      </div>
    {:else if tab === "yaml"}
      <YamlView {lines} {mark} />
    {:else if tab === "diff"}
      {#if changed === 0}
        <p class="placeholder muted">{t("preview.no_diff")}</p>
      {:else}
        <YamlView lines={diffLines} ops={diffOps} />
      {/if}
    {:else}
      {#if diags.length === 0}
        <p class="placeholder ok"><Icon name="ok" />{t("preview.no_diagnostics")}</p>
      {:else}
        <ul class="diags">
          {#each diags as d, i (i)}
            <li>
              <button type="button" class="diag" data-sev={d.severity} disabled={!d.path} onclick={() => d.path && onLocate(d.path)}>
                <Icon name={sevIcon(d.severity)} size={15} label={t("severity." + d.severity)} />
                <span class="msg">{catalog.diagMessage(d.code, d.args, names)}</span>
                <span class="meta mono ltr">{d.code}{d.path ? " · " + readablePath(d.path) : ""}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  </div>

  {#if p && p.kernel?.status && editor.check !== "idle"}
    <div class="kernel" data-state={p.kernel.status}>
      <Icon name={p.kernel.status === "passed" ? "ok" : p.kernel.status === "unavailable" ? "warning" : "error"} size={14} />
      <span>{t("kernel." + p.kernel.status, { version: p.kernel.version ?? "" })}</span>
      {#if p.kernel.message && p.kernel.status !== "passed"}<span class="mono ltr kmsg">{p.kernel.message}</span>{/if}
    </div>
  {/if}
</div>

<style>
  .pane {
    display: flex;
    flex-direction: column;
    block-size: 100%;
    min-block-size: 0;
    background: var(--bg-code);
  }
  .tabs {
    display: flex;
    align-items: center;
    gap: 2px;
    padding-inline: var(--space-2);
    block-size: 42px;
    flex: none;
    border-block-end: 1px solid var(--border);
    background: var(--bg-raised);
  }
  .tabs [role="tab"] {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    block-size: 30px;
    padding-inline: var(--space-3);
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 540;
    cursor: pointer;
  }
  .tabs [role="tab"]:hover {
    color: var(--fg);
  }
  .tabs [role="tab"][aria-selected="true"] {
    background: var(--bg-active);
    color: var(--fg);
  }
  .n {
    min-inline-size: 18px;
    padding-inline: 5px;
    border-radius: var(--radius-full);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: 11px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
  .n.err {
    background: var(--err-soft);
    color: var(--err);
  }
  .spacer {
    flex: 1;
  }
  .busy {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    padding-inline: var(--space-2);
  }
  .body {
    flex: 1;
    min-block-size: 0;
    overflow: auto;
    display: flex;
    flex-direction: column;
  }
  .body > :global(.yaml) {
    flex: 1;
    padding-block: var(--space-2);
  }
  .placeholder {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-6);
    font-size: var(--text-sm);
  }
  .placeholder.ok {
    color: var(--ok);
  }
  .fatal {
    display: flex;
    gap: var(--space-3);
    margin: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--err-soft);
    color: var(--err);
  }
  .fatal p {
    margin-block-start: var(--space-1);
    font-size: var(--text-sm);
    word-break: break-word;
  }
  .diags {
    display: grid;
    padding: var(--space-2);
    gap: 2px;
  }
  .diag {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 2px var(--space-2);
    inline-size: 100%;
    padding: var(--space-2) var(--space-3);
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg);
    font: inherit;
    font-size: var(--text-sm);
    text-align: start;
    cursor: pointer;
  }
  .diag:disabled {
    cursor: default;
  }
  .diag:hover:not(:disabled) {
    background: var(--bg-hover);
  }
  .diag :global(.icon) {
    margin-block-start: 2px;
  }
  .diag[data-sev="error"] :global(.icon) {
    color: var(--err);
  }
  .diag[data-sev="warning"] :global(.icon) {
    color: var(--warn);
  }
  .diag[data-sev="info"] :global(.icon) {
    color: var(--info);
  }
  .meta {
    grid-column: 2;
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    word-break: break-all;
  }
  .kernel {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    flex: none;
    padding: var(--space-2) var(--space-4);
    border-block-start: 1px solid var(--border);
    background: var(--bg-raised);
    font-size: var(--text-sm);
  }
  .kernel[data-state="passed"] {
    color: var(--ok);
  }
  .kernel[data-state="failed"],
  .kernel[data-state="timeout"] {
    color: var(--err);
  }
  .kernel[data-state="unavailable"] {
    color: var(--warn);
  }
  .kmsg {
    color: var(--fg-muted);
    font-size: var(--text-xs);
    word-break: break-word;
  }
</style>
