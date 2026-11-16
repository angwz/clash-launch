<script lang="ts">
  // 应用前的对比与可用性检查（反馈 #88）：渲染应用前后的完整配置，列出逐行差异，并对应用后的配置做 mihomo -t。
  // 渲染失败或内核检查不通过时不能应用；没有内核可用时如实说明，仍允许应用（之后的自动检查照常进行）。
  import Dialog from "../Dialog.svelte";
  import YamlView from "../editor/YamlView.svelte";
  import { api, type Preview } from "../../lib/api/client";
  import { catalog } from "../../lib/catalog.svelte";
  import { lineDiff, type DiffLine } from "../../lib/diff";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";

  let { open = $bindable(false), profileId, before, after, onConfirm }: { open?: boolean; profileId: string; before: () => string; after: () => string; onConfirm: () => void } = $props();

  let busy = $state(false);
  let error = $state("");
  let next = $state.raw<Preview | null>(null);
  let diff = $state.raw<{ lines: string[]; ops: string[]; changed: number }>({ lines: [], ops: [], changed: 0 });

  $effect(() => {
    if (open) void run();
  });

  // 只显示改动附近：未变的连续行超过 2×CONTEXT 时折叠成一行说明
  const CONTEXT = 3;
  function compact(d: DiffLine[]): { lines: string[]; ops: string[]; changed: number } {
    const lines: string[] = [];
    const ops: string[] = [];
    const near = d.map((x, i) => x.op !== " " || d.slice(Math.max(0, i - CONTEXT), i + CONTEXT + 1).some((y) => y.op !== " "));
    let skipped = 0;
    d.forEach((x, i) => {
      if (near[i]) {
        if (skipped) {
          lines.push(t("routing.apply.unchanged", { n: skipped }));
          ops.push(" ");
          skipped = 0;
        }
        lines.push(x.text);
        ops.push(x.op);
      } else skipped++;
    });
    if (skipped) {
      lines.push(t("routing.apply.unchanged", { n: skipped }));
      ops.push(" ");
    }
    return { lines, ops, changed: d.filter((x) => x.op !== " ").length };
  }

  async function run() {
    busy = true;
    error = "";
    next = null;
    try {
      const path = `/api/v1/profiles/${profileId}/preview`;
      const [a, b] = await Promise.all([api.post<Preview>(path, { doc: before() }), api.post<Preview>(path, { doc: after() }, { query: { check: true } })]);
      next = b;
      diff = compact(lineDiff(a.yaml, b.yaml));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  const fatal = $derived(next?.fatal ?? "");
  const kernel = $derived(next?.kernel?.status ?? "");
  const errors = $derived((next?.diagnostics ?? []).filter((d) => d.severity === "error"));
  const blocked = $derived(busy || !!error || !next || !!fatal || kernel === "failed" || kernel === "timeout");

  function confirm() {
    if (blocked) return;
    open = false;
    onConfirm();
  }
</script>

<Dialog bind:open title={t("routing.apply.title")} xl>
  <div class="body">
    {#if busy}
      <p class="status muted" aria-busy="true">{t("routing.apply.checking")}</p>
    {:else if error}
      <p class="status bad" role="alert">{error}</p>
    {:else if next}
      {#if fatal}
        <p class="status bad" role="alert">{t("routing.apply.render_failed", { reason: fatal })}</p>
      {:else if kernel === "passed"}
        <p class="status ok">✓ {t("routing.apply.kernel_passed")}</p>
      {:else if kernel === "failed" || kernel === "timeout"}
        <p class="status bad" role="alert">✗ {t("routing.apply.kernel_failed")}{#if next.kernel?.message}：{next.kernel.message}{/if}</p>
      {:else}
        <p class="status note">{t("routing.apply.kernel_unavailable")}</p>
      {/if}
      {#if errors.length}
        <ul class="diags">{#each errors as d, i (i)}<li>{catalog.diagMessage(d.code, d.args)}</li>{/each}</ul>
      {/if}
      <h3>{t("routing.apply.diff_title", { n: diff.changed })}</h3>
      {#if diff.changed}
        <div class="diff"><YamlView lines={diff.lines} ops={diff.ops} /></div>
      {:else}
        <p class="muted">{t("routing.apply.no_diff")}</p>
      {/if}
    {/if}
  </div>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" disabled={blocked} onclick={confirm}>{t("routing.apply.confirm")}</button>
  {/snippet}
</Dialog>

<style>
  .body {
    display: grid;
    gap: var(--space-3);
  }
  .status {
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
  }
  .status.ok {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .status.bad {
    background: var(--err-soft);
    color: var(--err);
  }
  .status.note {
    background: var(--info-soft);
    color: var(--info);
  }
  .diags {
    margin: 0;
    padding-inline-start: 18px;
    font-size: var(--text-sm);
    color: var(--err);
  }
  h3 {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .diff {
    max-block-size: 55vh;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
</style>
