<script lang="ts">
  // 节点源列表与新建（第 16.1 节「节点源」）：订阅地址、粘贴分享链接或节点（单条 / 批量）、文件内容。
  import Dialog from "../components/Dialog.svelte";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import { api, listAll, type Source, type SourceInput } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let items = $state<Source[] | null>(null);
  let error = $state("");
  let open = $state(false);
  let busy = $state(false);
  let form = $state<{ kind: SourceInput["kind"]; name: string; url: string; content: string }>({ kind: "subscription", name: "", url: "", content: "" });

  async function load() {
    try {
      items = await listAll<Source>("/api/v1/sources");
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());

  function status(s: Source): { tone: "ok" | "err" | "warn" | "muted"; label: string } {
    if (s.kind !== "subscription") return { tone: "muted", label: t("source.static") };
    const snap = s.snapshot;
    if (!snap?.fetchedAt && !snap?.lastError) return { tone: "muted", label: t("source.never") };
    if (snap.failingSince) return { tone: "warn", label: t("source.failing", { since: i18n.relative(snap.failingSince) }) };
    if (snap.lastError) return { tone: "err", label: t("source.error") };
    return { tone: "ok", label: t("source.fetched", { when: i18n.relative(snap.fetchedAt) }) };
  }

  function host(u?: string): string {
    if (!u) return "";
    try {
      return new URL(u).host;
    } catch {
      return "";
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      const body: SourceInput = { kind: form.kind, name: form.name.trim() };
      if (form.kind === "subscription") body.url = form.url.trim();
      else body.content = form.content;
      const s = await api.post<Source>("/api/v1/sources", body);
      open = false;
      toasts.push("ok", t("source.created"));
      router.go("/sources/" + s.id);
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function refresh(s: Source) {
    try {
      await api.post("/api/v1/sources/" + s.id + "/refresh");
      toasts.push("ok", t("source.refresh_queued"));
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  // 后端只区分订阅与手动（单条、批量、文件都按内容自动识别），界面也只呈现这两类。
  const kindIcon = (k: string) => (k === "subscription" ? "link" : "server") as "link" | "server";
</script>

<div class="page">
  <PageHeader title={t("sources.title")} subtitle={t("sources.subtitle")}>
    {#snippet actions()}
      <button class="btn primary" type="button" onclick={() => { form = { kind: "subscription", name: "", url: "", content: "" }; open = true; }}>
        <Icon name="plus" size={15} />{t("sources.new")}
      </button>
    {/snippet}
  </PageHeader>

  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  {#if items === null}
    <div class="skeleton" aria-busy="true"></div>
  {:else if items.length === 0}
    <div class="empty panel">
      <p>{t("sources.empty")}</p>
    </div>
  {:else}
    <div class="panel table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>{t("sources.col_name")}</th>
            <th>{t("sources.col_kind")}</th>
            <th>{t("sources.col_status")}</th>
            <th><span class="visually-hidden">{t("common.actions")}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each items as s (s.id)}
            {@const st = status(s)}
            <tr>
              <td>
                <a class="name" href={"/sources/" + s.id}>{s.name}</a>
                {#if host(s.url)}<span class="host mono ltr">{host(s.url)}</span>{/if}
              </td>
              <td><span class="kind"><Icon name={kindIcon(s.kind)} size={14} />{t("source.kind." + s.kind)}</span></td>
              <td><StatusBadge tone={st.tone} label={st.label} /></td>
              <td class="act">
                {#if s.kind === "subscription"}
                  <button class="btn ghost icon sm" type="button" aria-label={t("source.refresh")} title={t("source.refresh")} onclick={() => void refresh(s)}><Icon name="refresh" size={14} /></button>
                {/if}
                <a class="btn ghost icon sm" href={"/sources/" + s.id} aria-label={t("common.open")}><Icon name="chevron-right" size={14} /></a>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<Dialog bind:open title={t("sources.new")} wide>
  <form id="new-source" class="form" onsubmit={create}>
    <div class="kinds" role="radiogroup" aria-label={t("sources.col_kind")}>
      {#each ["subscription", "batch"] as k (k)}
        <label class="kind-opt" class:on={form.kind === k}>
          <input type="radio" name="kind" value={k} bind:group={form.kind} />
          <Icon name={kindIcon(k)} size={16} />
          <span class="kt">{t("source.kind." + k)}</span>
          <span class="kd">{t("source.kind_hint." + k)}</span>
        </label>
      {/each}
    </div>
    <div>
      <label class="label" for="ns-name">{t("sources.col_name")}</label>
      <input id="ns-name" class="input" bind:value={form.name} required maxlength="80" />
    </div>
    {#if form.kind === "subscription"}
      <div>
        <label class="label" for="ns-url">{t("source.url")}</label>
        <input id="ns-url" class="input mono ltr" type="url" bind:value={form.url} required placeholder="https://sub.example.com/…" autocomplete="off" spellcheck="false" />
        <p class="hint">{t("source.url_hint")}</p>
      </div>
    {:else}
      <div>
        <label class="label" for="ns-content">{t("source.content")}</label>
        <textarea id="ns-content" class="textarea mono ltr" rows="8" bind:value={form.content} required placeholder={t("source.content_placeholder")} spellcheck="false"></textarea>
        <p class="hint">{t("source.content_hint")}</p>
      </div>
    {/if}
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="new-source" disabled={busy || !form.name.trim() || (form.kind === "subscription" ? !form.url.trim() : !form.content.trim())}>{t("common.create")}</button>
  {/snippet}
</Dialog>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .table-wrap {
    overflow-x: auto;
    /* 让表内绝对定位的无障碍标签以此为包含块，否则会逃出滚动容器、撑宽页面（反馈 #41） */
    position: relative;
  }
  .name {
    display: block;
    color: var(--fg);
    font-weight: 560;
  }
  .host {
    display: block;
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .kind {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--fg-muted);
    white-space: nowrap;
  }
  .act {
    text-align: end;
    white-space: nowrap;
  }
  .empty {
    padding: var(--space-8);
    text-align: center;
    color: var(--fg-muted);
  }
  .skeleton {
    block-size: 160px;
    border-radius: var(--radius-lg);
    background: var(--bg-sunken);
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .kinds {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-2);
  }
  .kind-opt {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 2px var(--space-2);
    align-items: center;
    padding: var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .kind-opt.on {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
    background: var(--accent-soft);
  }
  .kind-opt:has(input:focus-visible) {
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
  .kind-opt input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .kt {
    font-weight: 560;
  }
  .kd {
    grid-column: 1 / -1;
    font-size: var(--text-xs);
    color: var(--fg-muted);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
