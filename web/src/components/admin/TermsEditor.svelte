<script lang="ts">
  // 管理页：编辑用户协议（需求 R40）。保存或恢复默认都会生成新版本，普通用户下次使用时需重新同意，
  // 所以保存前先确认。编辑的是原始文本，{{audit_days}}、{{request_days}} 在展示时替换为本站的保留期。
  import Dialog from "../Dialog.svelte";
  import Markdown from "../Markdown.svelte";
  import { api } from "../../lib/api/client";
  import type { Schemas } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { parse } from "../../lib/md";
  import { toasts } from "../../lib/toast.svelte";

  type AdminTerms = Schemas["Terms"] & { defaults?: Record<string, string> };
  const LANGS = ["zh-CN", "en", "ja"] as const;
  const LANG_LABEL: Record<(typeof LANGS)[number], string> = { "zh-CN": "中文", en: "English", ja: "日本語" };

  let cur = $state<AdminTerms | null>(null);
  let texts = $state<Record<string, string>>({});
  let lang = $state<(typeof LANGS)[number]>("zh-CN");
  let preview = $state(false);
  let kind = $state<"save" | "reset">("save");
  let confirmOpen = $state(false);
  const ask = (k: "save" | "reset") => ((kind = k), (confirmOpen = true));
  let busy = $state(false);
  let error = $state("");

  async function load() {
    try {
      cur = await api.get<AdminTerms>("/api/v1/admin/terms");
      texts = { ...cur.texts };
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());

  const dirty = $derived(!!cur && LANGS.some((l) => (texts[l] ?? "") !== (cur!.texts[l] ?? "")));

  async function apply() {
    confirmOpen = false;
    busy = true;
    try {
      const body = kind === "reset" ? { reset: true } : { texts: Object.fromEntries(LANGS.map((l) => [l, texts[l] ?? ""])) };
      const r = await api.put<Schemas["Terms"]>("/api/v1/admin/terms", body);
      toasts.push("ok", t("admin.terms_saved", { n: r.version }));
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      busy = false;
    }
  }
</script>

<section class="panel box" aria-labelledby="a-terms">
  <div class="row">
    <div>
      <h2 id="a-terms">{t("admin.terms")}</h2>
      <p class="muted">
        {#if cur}{t("terms.version", { n: cur.version })} · {cur.custom ? t("admin.terms_custom") : t("admin.terms_default")}{#if cur.updatedAt} · {t("terms.updated", { when: i18n.date(cur.updatedAt) })}{/if}{/if}
      </p>
    </div>
    <a class="btn ghost sm" href="/terms" target="_blank" rel="noopener">{t("admin.terms_open")}</a>
  </div>
  {#if error}<p class="field-error" role="alert">{error}</p>{/if}
  {#if cur}
    <div class="bar">
      <div class="seg" role="tablist" aria-label={t("admin.terms")}>
        {#each LANGS as l (l)}
          <button type="button" role="tab" aria-selected={lang === l} class:on={lang === l} onclick={() => (lang = l)}>{LANG_LABEL[l]}</button>
        {/each}
      </div>
      <label class="pv"><input type="checkbox" bind:checked={preview} />{t("admin.terms_preview")}</label>
    </div>
    {#if preview}
      <div class="preview"><Markdown blocks={parse(texts[lang] ?? "")} /></div>
    {:else}
      <textarea class="textarea mono" rows="16" aria-label={t("admin.terms") + " · " + lang} bind:value={texts[lang]} spellcheck="false"></textarea>
    {/if}
    <p class="hint">{t("admin.terms_hint", { audit: "{{audit_days}}", request: "{{request_days}}" })}</p>
    <div class="actions">
      <button class="btn ghost" type="button" disabled={busy || !cur.custom} onclick={() => ask("reset")}>{t("admin.terms_reset")}</button>
      <button class="btn ghost" type="button" disabled={busy || !dirty} onclick={() => (texts = { ...cur!.texts })}>{t("admin.terms_discard")}</button>
      <button class="btn primary" type="button" disabled={busy || !dirty} onclick={() => ask("save")}>{t("admin.terms_publish")}</button>
    </div>
  {/if}
</section>

<Dialog bind:open={confirmOpen} title={kind === "reset" ? t("admin.terms_reset") : t("admin.terms_publish")}>
  <p>{t("admin.terms_confirm", { n: (cur?.version ?? 0) + 1 })}</p>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (confirmOpen = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" onclick={() => void apply()}>{kind === "reset" ? t("admin.terms_reset") : t("admin.terms_publish")}</button>
  {/snippet}
</Dialog>

<style>
  .row {
    display: flex;
    justify-content: space-between;
    gap: var(--space-4);
    align-items: flex-start;
  }
  .bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-3);
    margin-block: var(--space-4) var(--space-2);
  }
  .pv {
    display: inline-flex;
    gap: var(--space-2);
    align-items: center;
    font-size: var(--text-sm);
  }
  textarea {
    inline-size: 100%;
  }
  .preview {
    max-block-size: 480px;
    overflow-y: auto;
    padding: var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    line-height: 1.8;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
</style>
