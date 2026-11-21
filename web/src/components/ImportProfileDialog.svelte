<script lang="ts">
  // 导入配置（需求 R17）：粘贴网上复制的完整 Clash/mihomo 配置，先预览将要创建的内容与每一条去掉的内容，
  // 确认后经已有的接口创建节点源与配置（配额、地址检查、校验照常生效）。
  // 中途失败时删除已经创建的节点源，不留下半个导入。
  import Dialog from "./Dialog.svelte";
  import { api, type Profile, type Schemas, type Source } from "../lib/api/client";
  import { catalog } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  type Diag = Schemas["Diagnostic"];
  interface ImportSource {
    key: string;
    kind: "batch" | "subscription";
    name: string;
    url?: string;
    content?: string;
    ua?: string;
    headers?: Record<string, string>;
    interval?: number;
    processing: Record<string, unknown>;
    mode: "inline" | "provider";
  }
  interface Plan {
    ok: boolean;
    doc?: string;
    sources?: ImportSource[];
    counts?: { proxies?: number; providers?: number; groups?: number; rules?: number; ruleProviders?: number; form?: number; raw?: number };
    diagnostics: Diag[];
  }

  let { open = $bindable(false) }: { open?: boolean } = $props();

  let text = $state("");
  let name = $state("");
  let keepUnknown = $state(false);
  let plan = $state<Plan | null>(null);
  let busy = $state(false);
  let error = $state("");

  $effect(() => {
    if (open) {
      plan = null;
      error = "";
    }
  });
  // 改了输入就要重新预览
  $effect(() => {
    void text;
    void keepUnknown;
    plan = null;
  });

  const notes = $derived<Diag[]>(plan?.diagnostics ?? []);

  async function preview() {
    busy = true;
    error = "";
    try {
      await catalog.load();
      plan = await api.post<Plan>("/api/v1/import/preview", { text, name: name.trim() || t("import.default_name"), keepUnknown });
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  async function run() {
    if (!plan?.ok || !plan.doc) return;
    busy = true;
    error = "";
    const created: string[] = [];
    try {
      let doc = plan.doc;
      for (const s of plan.sources ?? []) {
        const body: Record<string, unknown> = { name: s.name, kind: s.kind, processing: s.processing };
        if (s.kind === "subscription") Object.assign(body, { url: s.url, interval: s.interval ?? 0, ua: s.ua ?? "", headers: s.headers ?? {} });
        else body.content = s.content;
        const src = await api.post<Source>("/api/v1/sources", body);
        created.push(src.id);
        doc = doc.replaceAll("@" + s.key, src.id);
      }
      const p = await api.post<Profile>("/api/v1/profiles", { doc, note: t("import.note") });
      open = false;
      text = "";
      toasts.push("ok", t("import.done", { name: p.name }));
      router.go("/profiles/" + p.id);
    } catch (e) {
      error = errorText(e);
      for (const id of created) await api.del("/api/v1/sources/" + id).catch(() => undefined);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog bind:open title={t("import.title")} wide>
  <div class="body">
    <p class="muted">{t("import.desc")}</p>
    <label class="label" for="imp-text">{t("import.text")}</label>
    <textarea id="imp-text" class="input mono ltr" rows="12" bind:value={text} spellcheck="false" placeholder="mixed-port: 7890&#10;proxies:&#10;  - name: …"></textarea>
    <div class="row">
      <div class="grow">
        <label class="label" for="imp-name">{t("import.name")}</label>
        <input id="imp-name" class="input" bind:value={name} maxlength="80" placeholder={t("import.default_name")} />
      </div>
      <label class="opt"><input type="checkbox" bind:checked={keepUnknown} />{t("import.keep_unknown")}</label>
    </div>
    <p class="hint">{t("import.hint")}</p>
    {#if error}<p class="field-error" role="alert">{error}</p>{/if}

    {#if plan}
      <section class="result" aria-label={t("import.preview")}>
        {#if plan.ok && plan.counts}
          <p><strong>{t("import.will_create")}</strong></p>
          <ul class="counts">
            {#if plan.counts.proxies}<li>{t("import.c_proxies", { n: plan.counts.proxies })}</li>{/if}
            {#if plan.counts.providers}<li>{t("import.c_providers", { n: plan.counts.providers })}</li>{/if}
            {#if plan.counts.groups}<li>{t("import.c_groups", { n: plan.counts.groups })}</li>{/if}
            {#if plan.counts.rules}<li>{t("import.c_rules", { n: plan.counts.rules, sets: plan.counts.ruleProviders ?? 0 })}</li>{/if}
            {#if plan.counts.form}<li>{t("import.c_form", { n: plan.counts.form })}</li>{/if}
            {#if plan.counts.raw}<li>{t("import.c_raw", { n: plan.counts.raw })}</li>{/if}
          </ul>
        {/if}
        {#if notes.length}
          <p><strong>{t("import.notes", { n: notes.length })}</strong></p>
          <ul class="notes">{#each notes as d, i (i)}<li data-severity={d.severity}>{catalog.diagMessage(d.code, d.args)}</li>{/each}</ul>
        {:else if plan.ok}
          <p class="hint">{t("import.no_notes")}</p>
        {/if}
      </section>
    {/if}
  </div>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    {#if plan?.ok}
      <button class="btn primary" type="button" disabled={busy} onclick={() => void run()}>{t("import.confirm")}</button>
    {:else}
      <button class="btn primary" type="button" disabled={busy || !text.trim()} onclick={() => void preview()}>{t("import.preview")}</button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .body {
    display: grid;
    gap: var(--space-2);
  }
  textarea {
    resize: vertical;
    font-size: var(--text-sm);
  }
  .row {
    display: flex;
    align-items: flex-end;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  .grow {
    flex: 1 1 220px;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding-block-end: var(--space-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .opt input {
    accent-color: var(--accent);
  }
  .result {
    display: grid;
    gap: var(--space-2);
    margin-block-start: var(--space-2);
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    font-size: var(--text-sm);
  }
  .counts,
  .notes {
    margin: 0;
    padding-inline-start: 18px;
  }
  .notes {
    color: var(--fg-muted);
  }
  .notes li[data-severity="warning"] {
    color: var(--warn);
  }
  .notes li[data-severity="error"] {
    color: var(--err);
  }
</style>
