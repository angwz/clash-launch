<script lang="ts">
  // 短链接（反馈 #74、#75）：把自己的配置链接变成好记的短地址 域名/<短码>。
  // 在消息里授权给好友时，会为对方单独生成一个新的短链接（这里标出「授权给谁」，可以收回）；
  // 别人授权给我的短链接列在页面下方，只能复制与导入。访问短链接时直接返回配置内容，
  // 请求同时计入原链接与短链接，在「订阅分析」中可以单独查看。可以停用、设置过期时间、一键导入到客户端。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import QRCode from "../components/QRCode.svelte";
  import DayBars from "../components/stats/DayBars.svelte";
  import ImportMenu from "../components/links/ImportMenu.svelte";
  import ShareDialog, { type ShareTarget } from "../components/ShareDialog.svelte";
  import { api, listAll, type Link, type Profile } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { live, type Live } from "../lib/live";
  import { toasts } from "../lib/toast.svelte";

  interface Target {
    profileId: string;
    profileName: string;
    note: string;
  }
  interface Short {
    id: string;
    code: string;
    url: string;
    linkId: string;
    target: Target;
    note: string;
    disabled: boolean;
    expiresAt?: string;
    createdAt: string;
    total: number;
    series: number[];
    last?: string;
    version: number;
    grantee?: string;
    grantId?: string;
    granted?: boolean;
  }
  interface Received {
    grantId: string;
    code: string;
    url: string;
    from: string;
    name: string;
    createdAt: string;
  }
  interface Option {
    id: string;
    label: string;
  }

  let items = $state<Short[] | null>(null);
  let shareTarget = $state<ShareTarget | null>(null);
  let received = $state<Received[]>([]);
  let groups = $state<{ profile: string; links: Option[] }[]>([]);
  let error = $state("");
  let qrFor = $state<string | null>(null);
  let expanded = $state<string | null>(null); // 展开详情的短链接（默认都折叠）
  let form = $state({ linkId: "", code: "", note: "", expiry: "0" });
  let busy = $state(false);

  // 近 30 天（UTC）：与接口返回的 series 对齐
  const days = Array.from({ length: 30 }, (_, i) => {
    const d = new Date(Date.now() - (29 - i) * 86_400_000);
    return d.getUTCFullYear() * 10000 + (d.getUTCMonth() + 1) * 100 + d.getUTCDate();
  });
  const origin = location.origin;

  async function loadList() {
    const r = await api.get<{ items: Short[]; received: Received[] }>("/api/v1/short-links");
    items = r.items;
    received = r.received;
  }

  async function loadTargets() {
    const ps = await listAll<Profile>("/api/v1/profiles");
    const gs = await Promise.all(
      ps.map(async (p) => ({
        profile: p.name,
        links: (await listAll<Link>("/api/v1/profiles/" + p.id + "/links")).map((l) => ({ id: l.id, label: l.note || t("links.untitled") })),
      })),
    );
    groups = gs.filter((g) => g.links.length);
    if (!form.linkId) form.linkId = groups[0]?.links[0]?.id ?? "";
  }

  $effect(() => {
    Promise.all([loadList(), loadTargets()]).catch((e) => (error = errorText(e)));
  });
  let poll: Live | undefined;
  $effect(() => {
    const l = live(loadList, 30_000);
    poll = l;
    return l.stop;
  });

  async function create(e: SubmitEvent) {
    e.preventDefault();
    if (!form.linkId) return;
    busy = true;
    try {
      const days = Number(form.expiry);
      await api.post<Short>("/api/v1/short-links", {
        linkId: form.linkId,
        code: form.code.trim(),
        note: form.note.trim(),
        ...(days ? { expiresAt: new Date(Date.now() + days * 86_400_000).toISOString() } : {}),
      });
      form = { ...form, code: "", note: "" };
      toasts.push("ok", t("short.created"));
      await loadList();
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function patch(s: Short, body: Record<string, unknown>) {
    try {
      await api.patch("/api/v1/short-links/" + s.id, { ...body, version: s.version });
      await loadList();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function remove(s: Short) {
    if (!confirm(t("short.delete_confirm", { code: s.code }))) return;
    try {
      await api.del("/api/v1/short-links/" + s.id);
      await loadList();
    } catch (e) {
      toasts.push("err", errorText(e));
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

  async function revokeGrant(id: string, name: string, mine: boolean) {
    if (!confirm(mine ? t("messages.revoke_confirm", { name }) : t("messages.remove_confirm", { name }))) return;
    try {
      await api.post("/api/v1/grants/" + id + "/revoke", {});
      toasts.push("ok", mine ? t("messages.revoked") : t("messages.removed"));
      await loadList();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  const expired = (s: Short) => !!s.expiresAt && new Date(s.expiresAt).getTime() <= Date.now();
  const statusOf = (s: Short) => (s.disabled ? "disabled" : expired(s) ? "expired" : "active");
</script>

<ShareDialog bind:target={shareTarget} />

<div class="page">
  <PageHeader title={t("nav.short_links")} subtitle={t("short.subtitle")} />
  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  <form class="new panel" onsubmit={create}>
    <div class="row">
      <label class="field grow">
        <span class="label">{t("short.target")}</span>
        <select class="select" bind:value={form.linkId} required disabled={!groups.length}>
          {#each groups as g (g.profile)}
            <optgroup label={g.profile}>
              {#each g.links as l (l.id)}<option value={l.id}>{g.profile} · {l.label}</option>{/each}
            </optgroup>
          {/each}
        </select>
      </label>
      <label class="field code">
        <span class="label">{t("short.code")}</span>
        <span class="prefix-input">
          <span class="prefix ltr">{origin}/</span>
          <input class="input mono ltr" bind:value={form.code} placeholder={t("short.code_random")} maxlength="32" autocomplete="off" spellcheck="false" />
        </span>
      </label>
    </div>
    <div class="row">
      <label class="field grow">
        <span class="label">{t("links.note")}</span>
        <input class="input" bind:value={form.note} maxlength="100" placeholder={t("short.note_placeholder")} />
      </label>
      <label class="field">
        <span class="label">{t("short.expiry")}</span>
        <select class="select" bind:value={form.expiry}>
          <option value="0">{t("short.expiry_never")}</option>
          {#each [1, 7, 30, 90] as d (d)}<option value={String(d)}>{t("short.expiry_days", { n: d })}</option>{/each}
        </select>
      </label>
      <button class="btn primary submit" type="submit" disabled={busy || !form.linkId}><Icon name="plus" size={15} />{t("short.create")}</button>
    </div>
    {#if !groups.length && items}<p class="hint">{t("short.no_links")}</p>{/if}
    <p class="hint">{t("short.safety")}</p>
  </form>

  {#if items && !items.length}
    <p class="empty muted">{t("short.empty")}</p>
  {:else if items}
    <ul class="list">
      {#each items as s (s.id)}
        <li class="item panel" class:off={statusOf(s) !== "active"}>
          <!-- 默认折叠（反馈 #78）：只显示短码、状态、指向、请求次数与上次请求时间；趋势、二维码与管理操作在「详情」里 -->
          <div class="head">
            <div class="title">
              <code class="code-big ltr">/{s.code}</code>
              <span class="tag" data-state={statusOf(s)}>{t("short.state." + statusOf(s))}</span>
              {#if s.grantee}<span class="tag friend"><Icon name="shield" size={11} />{s.granted ? t("short.granted_to", { name: s.grantee }) : t("short.revoked_from", { name: s.grantee })}</span>{/if}
            </div>
            <div class="actions">
              <button class="btn sm" type="button" onclick={() => void copy(s.url)}><Icon name="copy" size={14} />{t("common.copy")}</button>
              <ImportMenu url={s.url} name={s.target.profileName} track={{ short: s.id }} ondownload={() => poll?.soon()} />
              <button class="btn sm ghost" type="button" aria-expanded={expanded === s.id} onclick={() => (expanded = expanded === s.id ? null : s.id)}>
                {t("short.details")}<Icon name={expanded === s.id ? "chevron-up" : "chevron-down"} size={14} />
              </button>
            </div>
          </div>
          <p class="meta">
            <a href={"/profiles/" + s.target.profileId + "/links"}>{s.target.profileName || "—"} · {s.target.note || t("links.untitled")}</a>
            <span class="muted">· {t("short.n_requests", { n: i18n.number(s.total) })}</span>
            <span class="muted">· {s.last ? t("short.last_request", { when: i18n.relative(s.last) }) : t("short.never_requested")}</span>
          </p>
          {#if expanded === s.id}
            <div class="detail">
              <p class="meta">
                {#if s.note}<span class="muted">{s.note} ·</span>{/if}
                <span class="muted">{t("links.created", { when: i18n.date(s.createdAt) })}</span>
                {#if s.last}<span class="muted">· {t("short.last_request_at", { when: i18n.date(s.last) })}</span>{/if}
                {#if s.expiresAt}<span class="muted">· {expired(s) ? t("short.expired_at", { when: i18n.date(s.expiresAt) }) : t("short.expires_at", { when: i18n.date(s.expiresAt) })}</span>{/if}
                <a class="linkish" href={"/analytics?short=" + s.id}>{t("short.analyze")}<Icon name="chevron-right" size={13} /></a>
              </p>
              <DayBars {days} values={s.series} label={t("links.requests_daily")} height={56} />
              {#if qrFor === s.id}
                <div class="qr-box"><QRCode text={s.url} label={t("links.qr")} /><p class="hint">{t("links.qr_hint")}</p></div>
              {/if}
              <div class="foot">
                <button class="btn sm ghost" type="button" aria-expanded={qrFor === s.id} onclick={() => (qrFor = qrFor === s.id ? null : s.id)}><Icon name="qr" size={14} />{t("links.qr")}</button>
                {#if s.grantee}
                  {#if s.granted && s.grantId}<button class="btn sm ghost" type="button" onclick={() => void revokeGrant(s.grantId!, "/" + s.code, true)}>{t("messages.revoke")}</button>{/if}
                {:else}
                  <button class="btn sm ghost" type="button" title={t("messages.grant_short_hint")} onclick={() => (shareTarget = { kind: "short", id: s.id, name: "/" + s.code })}><Icon name="shield" size={14} />{t("share.grant_friend")}</button>
                  <button class="btn sm ghost" type="button" onclick={() => void patch(s, { disabled: !s.disabled })}>{s.disabled ? t("short.enable") : t("short.disable")}</button>
                {/if}
                {#if s.expiresAt}<button class="btn sm ghost" type="button" onclick={() => void patch(s, { clearExpiry: true })}>{t("short.never_expire")}</button>{/if}
                <button class="btn sm ghost danger" type="button" onclick={() => void remove(s)}><Icon name="trash" size={14} />{t("common.delete")}</button>
              </div>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {:else if !error}
    <p class="muted">{t("common.loading")}</p>
  {/if}

  {#if received.length}
    <h2 class="sub-h"><Icon name="shield" size={15} />{t("short.received")}</h2>
    <p class="hint">{t("short.received_hint")}</p>
    <ul class="list">
      {#each received as g (g.grantId)}
        <li class="item panel">
          <div class="head">
            <div class="title">
              <code class="code-big ltr">/{g.code}</code>
              <span class="muted">{t("sources.granted_from", { name: g.from, when: i18n.date(g.createdAt) })}{#if g.name} · {g.name}{/if}</span>
            </div>
            <div class="actions">
              <button class="btn sm" type="button" onclick={() => void copy(g.url)}><Icon name="copy" size={14} />{t("common.copy")}</button>
              <ImportMenu url={g.url} name={g.name || g.from} track={{ code: g.code }} />
              <button class="btn sm ghost" type="button" onclick={() => void revokeGrant(g.grantId, "/" + g.code, false)}>{t("messages.remove")}</button>
            </div>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .page > :global(*) {
    max-inline-size: 960px;
  }
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .new {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
    margin-block-end: var(--space-5);
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    align-items: end;
  }
  .field {
    display: grid;
    gap: var(--space-1);
  }
  .grow {
    flex: 1 1 260px;
    min-inline-size: 0;
  }
  .code {
    flex: 1 1 300px;
  }
  .prefix-input {
    display: flex;
    align-items: stretch;
  }
  .prefix {
    display: inline-flex;
    align-items: center;
    padding-inline: var(--space-2);
    border: 1px solid var(--border);
    border-inline-end: 0;
    border-start-start-radius: var(--radius-md);
    border-end-start-radius: var(--radius-md);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: nowrap;
  }
  .prefix-input .input {
    border-start-start-radius: 0;
    border-end-start-radius: 0;
  }
  .submit {
    align-self: end;
  }
  .list {
    display: grid;
    gap: var(--space-3);
  }
  .item {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-4) var(--space-5);
  }
  .item.off {
    background: var(--bg-sunken);
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-inline-size: 0;
  }
  .code-big {
    font-size: var(--text-lg);
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .tag {
    padding-inline: 7px;
    border-radius: var(--radius-full);
    font-size: var(--text-xs);
    background: color-mix(in srgb, var(--ok) 14%, transparent);
    color: var(--ok);
  }
  .tag[data-state="disabled"],
  .tag[data-state="expired"] {
    background: var(--bg-hover);
    color: var(--fg-muted);
  }
  .actions,
  .foot {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .foot {
    justify-content: flex-end;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 0 var(--space-2);
    font-size: var(--text-sm);
  }
  .linkish {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .detail {
    display: grid;
    gap: var(--space-2);
    padding-block-start: var(--space-2);
    border-block-start: 1px solid var(--border);
  }
  .qr-box {
    display: grid;
    justify-items: start;
    gap: var(--space-2);
  }
  .sub-h {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-block: var(--space-6) var(--space-1);
    font-size: var(--text-lg);
  }
  .tag.friend {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .empty {
    padding: var(--space-6) 0;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
