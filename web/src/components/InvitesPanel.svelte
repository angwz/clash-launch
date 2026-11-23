<script lang="ts">
  // 邀请（需求 R12）：账户页显示「我的邀请」，管理页显示全站的邀请（多一列发出者，撤销即删除）。
  // 发出邀请总是走 /api/v1/invites：管理员不受名额限制，普通用户受名额、注册时长与邀请权约束。
  import { copyAndTell } from "../lib/clipboard";
  import Avatar from "./Avatar.svelte";
  import Icon from "./Icon.svelte";
  import QRCode from "./QRCode.svelte";
  import StatusBadge from "./StatusBadge.svelte";
  import { api, listAll, type Invite } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { admin = false }: { admin?: boolean } = $props();

  interface Quota {
    unlimited: boolean;
    limit: number;
    used: number;
    allowed: boolean;
    reason?: "banned" | "too_new" | "quota" | "disabled";
    availableAt?: string;
  }
  interface Invitee {
    username: string;
    nickname?: string;
    id?: string;
    avatar?: string;
    joinedAt: string;
  }

  let invites = $state<Invite[]>([]);
  let invitees = $state<Invitee[]>([]);
  let quota = $state<Quota | null>(null);
  let hours = $state(168);
  let uses = $state(1);
  let note = $state("");
  let created = $state<Invite | null>(null);
  let showQR = $state(false);
  let busy = $state(false);

  const durations = [1, 24, 72, 168, 720, 2160, 0];
  const useChoices = [1, 5, 10, 20, 50, 100];

  async function load() {
    try {
      if (admin) {
        invites = await listAll<Invite>("/api/v1/admin/invites");
      } else {
        const r = await api.get<{ quota: Quota; items: Invite[]; invitees: Invitee[] }>("/api/v1/invites");
        quota = r.quota;
        invites = r.items;
        invitees = r.invitees;
      }
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  $effect(() => {
    void load();
  });

  async function create(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      created = await api.post<Invite>("/api/v1/invites", { hours, maxUses: uses, note: note.trim() });
      showQR = false;
      note = "";
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function revoke(inv: Invite) {
    try {
      if (admin) await api.del("/api/v1/admin/invites/" + inv.id);
      else await api.post(`/api/v1/invites/${inv.id}/revoke`, {});
      if (created?.id === inv.id) created = null;
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function copy(url: string) {
    await copyAndTell(url);
  }

  const durLabel = (h: number) => (h === 0 ? t("admin.never") : h < 24 ? t("admin.hours", { n: h }) : t("admin.days", { n: h / 24 }));
  const tone = (s: Invite["status"]) => (s === "active" ? "ok" : s === "used" ? "muted" : "warn") as "ok" | "muted" | "warn";
  const canCreate = $derived(admin || !!quota?.allowed);
  const quotaText = $derived.by(() => {
    if (!quota) return "";
    if (quota.unlimited) return t("invites.quota_unlimited");
    if (quota.reason === "banned") return t("invites.reason_banned");
    if (quota.reason === "disabled") return t("invites.reason_disabled");
    if (quota.reason === "too_new") return t("invites.reason_too_new", { when: quota.availableAt ? i18n.date(quota.availableAt) : "" });
    return t("invites.quota", { used: quota.used, limit: quota.limit });
  });
</script>

<section class="panel box" aria-labelledby={admin ? "inv-all" : "inv-mine"}>
  <h2 id={admin ? "inv-all" : "inv-mine"}>{admin ? t("invites.all_title") : t("invites.title")}</h2>
  <p class="muted">{admin ? t("invites.all_desc") : t("invites.desc")}</p>
  {#if !admin && session.user?.invitedBy}<p class="by">{t("invites.invited_by", { name: session.user.invitedBy })}</p>{/if}
  {#if quotaText}<p class="quota" class:off={!canCreate}>{quotaText}</p>{/if}

  <form class="new" onsubmit={create}>
    <label class="visually-hidden" for={admin ? "ia-h" : "im-h"}>{t("admin.validity")}</label>
    <select id={admin ? "ia-h" : "im-h"} class="select" bind:value={hours} disabled={!canCreate}>
      {#each durations as h (h)}<option value={h}>{durLabel(h)}</option>{/each}
    </select>
    <label class="visually-hidden" for={admin ? "ia-u" : "im-u"}>{t("invites.uses")}</label>
    <select id={admin ? "ia-u" : "im-u"} class="select" bind:value={uses} disabled={!canCreate}>
      {#each useChoices as n (n)}<option value={n}>{n === 1 ? t("invites.uses_once") : t("invites.uses_n", { n })}</option>{/each}
    </select>
    <input class="input" bind:value={note} placeholder={t("admin.invite_note")} aria-label={t("admin.invite_note")} maxlength="200" disabled={!canCreate} />
    <button class="btn primary" type="submit" disabled={!canCreate || busy}><Icon name="plus" size={15} />{t("admin.new_invite")}</button>
  </form>

  {#if created?.url}
    <div class="created" role="status">
      <p><strong>{t("admin.invite_ready")}</strong> {t("admin.invite_once")}</p>
      <div class="url-row">
        <code class="url ltr">{created.url}</code>
        <button class="btn sm" type="button" onclick={() => void copy(created!.url!)}><Icon name="copy" size={14} />{t("common.copy")}</button>
        <button class="btn sm ghost" type="button" aria-pressed={showQR} onclick={() => (showQR = !showQR)}><Icon name="qr" size={14} />{t("invites.qr")}</button>
      </div>
      {#if showQR}<QRCode text={created.url} label={t("invites.qr")} />{/if}
    </div>
  {/if}

  {#if invites.length}
    <div class="table-wrap">
      <table class="table flush">
        <thead>
          <tr>
            {#if admin}<th>{t("invites.col_by")}</th>{/if}
            <th>{t("admin.col_note")}</th>
            <th>{t("admin.col_status")}</th>
            <th>{t("invites.col_joined")}</th>
            <th>{t("admin.col_expires")}</th>
            <th>{t("admin.col_created")}</th>
            <th><span class="visually-hidden">{t("common.actions")}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each invites as inv (inv.id)}
            <tr>
              {#if admin}<td>{inv.createdBy ?? "—"}</td>{/if}
              <td>{inv.note || "—"}</td>
              <td><StatusBadge tone={tone(inv.status)} label={t("invite." + inv.status)} />{#if inv.maxUses > 1}<span class="muted uses">{inv.uses}/{inv.maxUses}</span>{/if}</td>
              <td>{inv.usedBy?.join("、") || "—"}</td>
              <td>{inv.expiresAt ? i18n.date(inv.expiresAt) : t("admin.never")}</td>
              <td class="muted">{i18n.relative(inv.createdAt)}</td>
              <td class="act">
                {#if inv.status === "active"}
                  <button class="btn sm ghost danger" type="button" onclick={() => void revoke(inv)}>{t("admin.revoke")}</button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if !admin && invitees.length}
    <h3>{t("invites.invitees", { n: invitees.length })}</h3>
    <ul class="invitees">
      {#each invitees as p (p.username)}
        <li><Avatar user={p} size={24} /><strong>{p.nickname || p.username}</strong>{#if p.nickname}<span class="muted">@{p.username}</span>{/if}<span class="muted when">{i18n.date(p.joinedAt)}</span></li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  /* 外框的内边距与标题字号在组件内定义：页面的 .box 是页面作用域的样式，不会作用到这里（反馈 #113） */
  .box {
    /* grid 子项默认最小宽度为内容宽度：设为 0，表格才会在外框内横向滚动而不是撑宽页面 */
    grid-template-columns: minmax(0, 1fr);
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .box > .muted {
    font-size: var(--text-sm);
  }
  .by,
  .quota {
    font-size: var(--text-sm);
  }
  .quota.off {
    color: var(--warn);
  }
  .new {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .new .select {
    inline-size: auto;
  }
  .new .input {
    flex: 1 1 200px;
  }
  .created {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--ok-soft);
    font-size: var(--text-sm);
  }
  .url-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .url {
    min-inline-size: 0;
    padding: 5px var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    font-size: var(--text-xs);
    flex: 1 1 240px;
    overflow-wrap: anywhere;
  }
  .table-wrap {
    /* 表头里的 visually-hidden 是绝对定位的：滚动框必须是它的包含块，否则会跑出滚动框、撑宽页面 */
    position: relative;
    overflow-x: auto;
  }
  .uses {
    margin-inline-start: var(--space-1);
    font-size: var(--text-xs);
  }
  .act {
    text-align: end;
  }
  h3 {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .invitees {
    display: grid;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-sm);
  }
  .invitees li {
    display: flex;
    gap: var(--space-2);
  }
  .invitees .when {
    margin-inline-start: auto;
  }
</style>
