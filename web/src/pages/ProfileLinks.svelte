<script lang="ts">
  import { uniqueName } from "../lib/names";
  // 订阅链接（第 13.2 节）：随时查看与复制、二维码；重置（旧链接立即失效）、备注、最近访问时间与客户端。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import QRCode from "../components/QRCode.svelte";
  import Emoji from "../components/Emoji.svelte";
  import LinkStats, { type LinkStat } from "../components/links/LinkStats.svelte";
  import ImportMenu from "../components/links/ImportMenu.svelte";
  import { api, listAll, type Link, type Profile } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { live, type Live } from "../lib/live";
  import { toasts } from "../lib/toast.svelte";

  let { params }: { params: Record<string, string> } = $props();

  let profile = $state<Profile | null>(null);
  let links = $state<Link[] | null>(null);
  let note = $state("");
  // 默认名称（反馈 #65）：「设备 1」「设备 2」……
  const defaultNote = $derived(uniqueName(t("links.default_note", { n: (links?.length ?? 0) + 1 }), (links ?? []).map((l) => l.note)));
  let qrFor = $state<string | null>(null);
  let error = $state("");

  const base = $derived("/api/v1/profiles/" + params.id + "/links");

  // 订阅统计（反馈 #67）
  let ps = $state<{ createdAt: string; days: number[]; links: LinkStat[] } | null>(null);
  const statOf = (id: string) => ps?.links.find((x) => x.id === id);
  const totalAll = $derived((ps?.links ?? []).reduce((a, b) => a + b.total, 0));
  const firstEver = $derived((ps?.links ?? []).map((x) => x.first).filter((x): x is string => !!x).sort()[0]);
  // 时间线的第二格显示最近一次被订阅（R53）：比首次更能说明配置仍在被使用；首次保留为副标题
  const lastEver = $derived((ps?.links ?? []).map((x) => x.last).filter((x): x is string => !!x).sort().at(-1));
  // 里程碑：1、10、100、1000……
  const nextMilestone = $derived(10 ** Math.max(0, Math.floor(Math.log10(Math.max(1, totalAll))) + 1));
  const DAY = 86_400_000;
  const daysSince = (iso: string) => Math.max(1, Math.floor((Date.now() - new Date(iso).getTime()) / DAY) + 1);

  async function load() {
    try {
      [profile, links, ps] = await Promise.all([
        api.get<Profile>("/api/v1/profiles/" + params.id),
        listAll<Link>(base),
        api.get<{ createdAt: string; days: number[]; links: LinkStat[] }>("/api/v1/profiles/" + params.id + "/stats"),
      ]);
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());

  // 自动刷新（反馈 #73）：请求记录、最近访问会因客户端拉取而变化；页面可见时每 30 秒刷新，导出后稍等即刷新
  let poll: Live | undefined;
  $effect(() => {
    const l = live(async () => {
      [links, ps] = await Promise.all([
        listAll<Link>(base),
        api.get<{ createdAt: string; days: number[]; links: LinkStat[] }>("/api/v1/profiles/" + params.id + "/stats"),
      ]);
    }, 30_000);
    poll = l;
    return l.stop;
  });

  async function create(e: SubmitEvent) {
    e.preventDefault();
    try {
      await api.post<Link>(base, { note: note.trim() || defaultNote });
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

  <!-- 有仪式感的时间线（反馈 #67）：这份配置的诞生、第一次被订阅、累计请求与下一个里程碑 -->
  {#if ps}
    <section class="panel story" aria-label={t("links.story")}>
      <div class="moment">
        <Emoji name="rocket" size={22} />
        <div><span class="muted">{t("links.born")}</span><strong>{i18n.date(ps.createdAt)}</strong><span class="muted">{t("links.days_running", { n: daysSince(ps.createdAt) })}</span></div>
      </div>
      <div class="moment">
        <Emoji name={lastEver ? "party" : "mailbox"} size={22} />
        <div><span class="muted">{t("links.last_sub")}</span><strong title={lastEver ? i18n.date(lastEver) : undefined}>{lastEver ? i18n.relative(lastEver) : t("links.not_yet")}</strong>{#if firstEver}<span class="muted">{t("links.first_request", { when: i18n.date(firstEver) })}</span>{/if}</div>
      </div>
      <div class="moment">
        <Emoji name={totalAll >= 100 ? "trophy" : "sparkles"} size={22} />
        <div><span class="muted">{t("links.total_requests")}</span><strong>{i18n.number(totalAll)}</strong><span class="muted">{t("links.next_milestone", { n: i18n.number(nextMilestone - totalAll), m: i18n.number(nextMilestone) })}</span></div>
      </div>
    </section>
  {/if}

  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  <form class="new panel" onsubmit={create}>
    <input class="input" bind:value={note} placeholder={defaultNote} aria-label={t("links.note")} maxlength="80" />
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
            <!-- 导入到客户端与导出文件（反馈 #72、#74）；导出记为一次请求，稍后自动刷新统计 -->
            <ImportMenu url={l.url} name={profile?.name ?? ""} track={{ link: l.id }} ondownload={() => poll?.soon()} />
            <button class="btn sm ghost icon" type="button" aria-label={t("links.qr")} aria-expanded={qrFor === l.id} onclick={() => (qrFor = qrFor === l.id ? null : l.id)}><Icon name="qr" size={15} /></button>
          </div>
          <p class="meta muted">
            {t("links.created", { when: i18n.date(l.createdAt) })}
            ·
            {#if l.lastAccess}{t("links.last_access", { when: i18n.relative(l.lastAccess), ua: l.lastUa || "—" })}{:else}{t("links.never_used")}{/if}
          </p>
          {#if statOf(l.id)}<LinkStats stat={statOf(l.id)!} days={ps!.days} profileId={params.id!} />{/if}
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
  .page > :global(*) {
    max-inline-size: 960px;
  }
  /* 与其他页面同一左缘（1120 居中），内容本身较窄（960）（反馈 #56） */
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .story {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-4);
    padding: var(--space-4) var(--space-5);
    margin-block-end: var(--space-4);
  }
  .moment {
    display: flex;
    gap: var(--space-3);
    align-items: flex-start;
  }
  .moment > div {
    display: grid;
    font-size: var(--text-sm);
  }
  .moment strong {
    font-size: var(--text-lg);
    font-weight: 620;
    font-variant-numeric: tabular-nums;
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
