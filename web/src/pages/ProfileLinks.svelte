<script lang="ts">
  // 订阅链接（第 13.2 节）：随时查看与复制、二维码；重置（旧链接立即失效）、备注、最近访问时间与客户端。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import QRCode from "../components/QRCode.svelte";
  import { api, listAll, type Link, type Profile } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { params }: { params: Record<string, string> } = $props();

  let profile = $state<Profile | null>(null);
  let links = $state<Link[] | null>(null);
  let note = $state("");
  let qrFor = $state<string | null>(null);
  let error = $state("");

  const base = $derived("/api/v1/profiles/" + params.id + "/links");

  async function load() {
    try {
      [profile, links] = await Promise.all([api.get<Profile>("/api/v1/profiles/" + params.id), listAll<Link>(base)]);
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());

  async function create(e: SubmitEvent) {
    e.preventDefault();
    try {
      await api.post<Link>(base, { note: note.trim() });
      note = "";
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function copy(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      toasts.push("ok", t("common.copied"));
    } catch {
      toasts.push("err", t("common.copy_failed"));
    }
  }

  async function reset(l: Link) {
    if (!confirm(t("links.reset_confirm"))) return;
    try {
      await api.post(base + "/" + l.id + "/reset", {}, { ifMatch: l.version });
      toasts.push("ok", t("links.reset_done"));
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function remove(l: Link) {
    if (!confirm(t("links.delete_confirm"))) return;
    try {
      await api.del(base + "/" + l.id);
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function rename(l: Link, v: string) {
    if (v === l.note) return;
    try {
      await api.patch(base + "/" + l.id, { note: v }, { ifMatch: l.version });
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
</script>

<div class="page">
  <PageHeader title={t("links.title")} subtitle={profile ? t("links.subtitle", { name: profile.name }) : ""}>
    {#snippet actions()}
      <a class="btn" href={"/profiles/" + params.id}><Icon name="settings" size={15} />{t("links.edit_profile")}</a>
    {/snippet}
  </PageHeader>

  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  <form class="new panel" onsubmit={create}>
    <input class="input" bind:value={note} placeholder={t("links.note_placeholder")} aria-label={t("links.note")} maxlength="80" />
    <button class="btn primary" type="submit"><Icon name="plus" size={15} />{t("links.new")}</button>
  </form>

  {#if links && links.length === 0}
    <p class="empty muted">{t("links.empty")}</p>
  {/if}

  <ul class="list">
    {#each links ?? [] as l (l.id)}
      <li class="panel item">
        <div class="main">
          <input class="note" value={l.note} placeholder={t("links.untitled")} aria-label={t("links.note")} onchange={(e) => void rename(l, e.currentTarget.value.trim())} />
          <div class="url-row">
            <code class="url ltr">{l.url}</code>
            <button class="btn sm" type="button" onclick={() => void copy(l.url)}><Icon name="copy" size={14} />{t("common.copy")}</button>
            <button class="btn sm ghost icon" type="button" aria-label={t("links.qr")} aria-expanded={qrFor === l.id} onclick={() => (qrFor = qrFor === l.id ? null : l.id)}><Icon name="qr" size={15} /></button>
          </div>
          <p class="meta muted">
            {t("links.created", { when: i18n.date(l.createdAt) })}
            ·
            {#if l.lastAccess}{t("links.last_access", { when: i18n.relative(l.lastAccess), ua: l.lastUa || "—" })}{:else}{t("links.never_used")}{/if}
          </p>
          {#if qrFor === l.id}
            <div class="qr-box"><QRCode text={l.url} label={t("links.qr")} /><p class="hint">{t("links.qr_hint")}</p></div>
          {/if}
        </div>
        <div class="side">
          <button class="btn sm" type="button" onclick={() => void reset(l)}><Icon name="refresh" size={14} />{t("links.reset")}</button>
          <button class="btn sm ghost danger" type="button" onclick={() => void remove(l)}><Icon name="trash" size={14} />{t("common.delete")}</button>
        </div>
      </li>
    {/each}
  </ul>
</div>

<style>
  .page {
    max-inline-size: 960px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .new {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-3);
    margin-block-end: var(--space-4);
  }
  .list {
    display: grid;
    gap: var(--space-3);
  }
  .item {
    display: flex;
    gap: var(--space-4);
    justify-content: space-between;
    padding: var(--space-4) var(--space-5);
  }
  .main {
    display: grid;
    gap: var(--space-2);
    min-inline-size: 0;
    flex: 1;
  }
  .note {
    padding: 0;
    border: 0;
    border-block-end: 1px dashed transparent;
    background: none;
    color: var(--fg);
    font: inherit;
    font-weight: 600;
    inline-size: 100%;
  }
  .note:hover,
  .note:focus {
    border-block-end-color: var(--border-strong);
    outline: none;
  }
  .url-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-inline-size: 0;
  }
  .url {
    flex: 1;
    min-inline-size: 0;
    padding: 5px var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    font-size: var(--text-xs);
  }
  .qr-box {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding-block-start: var(--space-2);
  }
  .side {
    display: grid;
    gap: var(--space-2);
    align-content: start;
  }
  .empty {
    padding: var(--space-6);
    text-align: center;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
    .item {
      flex-direction: column;
    }
    .side {
      grid-auto-flow: column;
      justify-content: start;
    }
  }
</style>
