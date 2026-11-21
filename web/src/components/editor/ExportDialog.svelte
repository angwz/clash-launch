<script lang="ts">
  // 导出配置（需求 R17）：下载一份客户端可以直接使用的 YAML，与订阅链接同源。可选去掉节点或规则集，
  // 两项都去掉就是可以分享给别人的「骨架」。去掉与改写的内容由服务端逐项列为诊断，这里在下载后列出。
  import Dialog from "../Dialog.svelte";
  import { api, type Schemas } from "../../lib/api/client";
  import { catalog } from "../../lib/catalog.svelte";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";

  type Diag = Schemas["Diagnostic"];
  interface Result {
    yaml: string;
    filename: string;
    diagnostics: Diag[];
    kernel: { status?: string; message?: string; version?: string };
    fatal: string;
  }

  let { open = $bindable(false), profileId }: { open?: boolean; profileId: string } = $props();

  let nodes = $state(true);
  let rules = $state(true);
  let busy = $state(false);
  let error = $state("");
  let result = $state<Result | null>(null);

  $effect(() => {
    if (open) {
      result = null;
      error = "";
    }
  });

  // 只列出与导出有关的诊断（export.*）与内核检查结果；其余诊断与编辑器预览里的相同
  const notes = $derived((result?.diagnostics ?? []).filter((d) => d.code.startsWith("export.")));

  async function run() {
    busy = true;
    error = "";
    result = null;
    try {
      const r = await api.post<Result>("/api/v1/profiles/" + profileId + "/export-config", { nodes, rules });
      result = r;
      if (r.fatal) return;
      const url = URL.createObjectURL(new Blob([r.yaml], { type: "text/yaml;charset=utf-8" }));
      const a = document.createElement("a");
      a.href = url;
      a.download = r.filename;
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog bind:open title={t("export.title")}>
  <div class="body">
    <p class="muted">{t("export.desc")}</p>
    <label class="opt"><input type="checkbox" bind:checked={nodes} /><span><strong>{t("export.nodes")}</strong><span class="hint">{t("export.nodes_hint")}</span></span></label>
    <label class="opt"><input type="checkbox" bind:checked={rules} /><span><strong>{t("export.rules")}</strong><span class="hint">{t("export.rules_hint")}</span></span></label>
    {#if !nodes && !rules}<p class="hint">{t("export.skeleton")}</p>{/if}
    {#if nodes}<p class="hint warn">{t("export.private")}</p>{/if}
    {#if error}<p class="field-error" role="alert">{error}</p>{/if}
    {#if result}
      <div class="result" role="status">
        {#if result.fatal}
          <p class="field-error">{t("export.fatal", { reason: result.fatal })}</p>
        {:else}
          <p>{t("export.done", { file: result.filename })}</p>
          <p class="hint">{t("kernel." + (result.kernel.status ?? "unavailable"), { version: result.kernel.version ?? "" })}{#if result.kernel.status === "failed" && result.kernel.message}：{result.kernel.message}{/if}</p>
        {/if}
        {#if notes.length}
          <ul class="notes">{#each notes as d, i (i)}<li data-severity={d.severity}>{catalog.diagMessage(d.code, d.args)}</li>{/each}</ul>
        {/if}
      </div>
    {/if}
  </div>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.close")}</button>
    <button class="btn primary" type="button" disabled={busy} onclick={() => void run()}>{t("export.download")}</button>
  {/snippet}
</Dialog>

<style>
  .body {
    display: grid;
    gap: var(--space-3);
  }
  .opt {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    cursor: pointer;
  }
  .opt input {
    margin-block-start: 3px;
    accent-color: var(--accent);
  }
  .opt > span {
    display: grid;
    gap: 2px;
  }
  .result {
    display: grid;
    gap: var(--space-2);
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    font-size: var(--text-sm);
  }
  .warn {
    color: var(--warn);
  }
  .notes {
    margin: 0;
    padding-inline-start: 18px;
    color: var(--fg-muted);
  }
  .notes li[data-severity="warning"] {
    color: var(--warn);
  }
</style>
