<script lang="ts">
  // 用户管理：只显示元数据与用量（看不到任何人的内容）。停用 / 启用、重置密码、调整配额。
  import Dialog from "../Dialog.svelte";
  import StatusBadge from "../StatusBadge.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import Icon from "../Icon.svelte";
  import { session } from "../../lib/session.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Limits {
    sources: number;
    profiles: number;
    links: number;
    minInterval: number;
    storage: number;
  }
  interface U {
    id: string;
    username: string;
    role: string;
    disabled: boolean;
    createdAt: string;
    mfa: boolean;
    usage: { sources: number; profiles: number; links: number; bytes: number };
    quota: Limits;
    invited: boolean;
    nickname?: string;
    /** 邀请人的用户名（需求 R12） */
    invitedBy?: string;
    inviteBanned: boolean;
    /** 这个用户适用的默认配额（开放注册的用户更小，反馈 #71） */
    defaults: Limits;
  }

  let users = $state<U[]>([]);
  let defaults = $state<Limits | null>(null);
  let resetFor = $state<U | null>(null);
  let resetOpen = $state(false);
  let newPassword = $state("");
  let quotaFor = $state<U | null>(null);
  let quotaOpen = $state(false);
  // 配额表单：空字符串表示使用默认值；分钟与 MiB 便于输入
  let qf = $state({ sources: "", profiles: "", links: "", minInterval: "", storage: "" });

  // 按页码分页并可按用户名搜索（反馈 #76）
  const SIZE = 20;
  let pageN = $state(1);
  let total = $state(0);
  let q = $state("");
  const pages = $derived(Math.max(1, Math.ceil(total / SIZE)));
  async function load(n = pageN) {
    const r = await api.get<{ items: U[]; total: number; page: number; defaults: Limits }>("/api/v1/admin/users", { query: { pageNo: n, limit: SIZE, q: q.trim() || undefined } });
    users = r.items;
    total = r.total;
    pageN = r.page;
    defaults = r.defaults;
  }
  // 搜索：停止输入 250 毫秒后回到第一页
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    void q;
    clearTimeout(timer);
    timer = setTimeout(() => load(1).catch((e) => toasts.push("err", errorText(e))), 250);
  });

  const mb = (n: number) => (n / (1 << 20)).toLocaleString(i18n.intlLocale, { maximumFractionDigits: 1 }) + " MiB";

  // 收回或恢复邀请权（需求 R12）：收回时该用户未用完的邀请随之作废
  async function toggleInvite(u: U) {
    try {
      await api.put(`/api/v1/admin/users/${u.id}/invite-ban`, { banned: !u.inviteBanned });
      toasts.push("ok", u.inviteBanned ? t("invites.ban_lifted", { name: u.username }) : t("invites.banned", { name: u.username }));
      await load(pageN);
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  async function toggle(u: U) {
    try {
      await api.patch("/api/v1/admin/users/" + u.id, { disabled: !u.disabled });
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  function openReset(u: U) {
    resetFor = u;
    newPassword = "";
    resetOpen = true;
  }

  async function reset(e: SubmitEvent) {
    e.preventDefault();
    try {
      await api.post("/api/v1/admin/users/" + resetFor!.id + "/password", { password: newPassword });
      resetOpen = false;
      toasts.push("ok", t("admin.password_reset", { name: resetFor!.username }));
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  function openQuota(u: U) {
    quotaFor = u;
    const v = (n: number, div = 1) => (n < 0 ? "" : String(Math.round(n / div)));
    qf = { sources: v(u.quota.sources), profiles: v(u.quota.profiles), links: v(u.quota.links), minInterval: v(u.quota.minInterval, 60), storage: v(u.quota.storage, 1 << 20) };
    quotaOpen = true;
  }

  async function saveQuota(e: SubmitEvent) {
    e.preventDefault();
    const n = (s: string, mul = 1) => (s.trim() === "" ? -1 : Math.max(0, Math.round(Number(s) * mul)));
    try {
      await api.put("/api/v1/admin/users/" + quotaFor!.id + "/quota", {
        sources: n(qf.sources),
        profiles: n(qf.profiles),
        links: n(qf.links),
        minInterval: n(qf.minInterval, 60),
        storage: n(qf.storage, 1 << 20),
      });
      quotaOpen = false;
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }
</script>

<div class="search">
  <Icon name="search" size={14} />
  <input class="input" type="search" bind:value={q} placeholder={t("admin.user_search")} aria-label={t("admin.user_search")} />
  <span class="muted">{t("admin.user_total", { n: total })}</span>
</div>
<div class="wrap">
  <div class="table-wrap">
  <table class="table flush">
    <thead>
      <tr>
        <th>{t("field.username")}</th>
        <th>{t("admin.col_status")}</th>
        <th>{t("admin.col_usage")}</th>
        <th>{t("admin.col_created")}</th>
        <th><span class="visually-hidden">{t("common.actions")}</span></th>
      </tr>
    </thead>
    <tbody>
      {#each users as u (u.id)}
        <tr>
          <td>
            <strong>{u.username}</strong>{#if u.nickname}<span class="muted nick">{u.nickname}</span>{/if}
            {#if u.role === "admin"}<span class="role">{t("role.admin")}</span>{:else if !u.invited}<span class="role open" title={t("admin.open_user_hint")}>{t("admin.open_user")}</span>{/if}
            {#if u.invitedBy}<div class="muted by">{t("invites.by_user", { name: u.invitedBy })}</div>{/if}
          </td>
          <td>
            {#if u.disabled}<StatusBadge tone="err" label={t("admin.disabled")} />{:else}<StatusBadge tone="ok" label={t("admin.active")} />{/if}
            {#if u.mfa}<StatusBadge tone="info" icon="shield" label="2FA" />{/if}
          </td>
          <td class="usage">{t("admin.usage", { sources: u.usage.sources, profiles: u.usage.profiles, links: u.usage.links })} · {mb(u.usage.bytes)}</td>
          <td class="muted">{i18n.relative(u.createdAt)}</td>
          <td class="act">
            {#if u.id !== session.user?.id}
              <button class="btn sm ghost" type="button" onclick={() => openQuota(u)}>{t("admin.quota")}</button>
              <button class="btn sm ghost" type="button" onclick={() => openReset(u)}>{t("admin.reset_password")}</button>
              {#if u.role !== "admin"}<button class="btn sm ghost" type="button" onclick={() => void toggleInvite(u)}>{u.inviteBanned ? t("invites.allow") : t("invites.ban")}</button>{/if}
              <button class="btn sm ghost" class:danger={!u.disabled} type="button" onclick={() => void toggle(u)}>{u.disabled ? t("admin.enable") : t("admin.disable")}</button>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
  </div>
  {#if pages > 1}
    <nav class="pager" aria-label={t("admin.user_pager")}>
      <button class="btn sm" type="button" disabled={pageN <= 1} onclick={() => void load(pageN - 1)}>{t("audit.prev")}</button>
      <span class="muted">{t("audit.page_of", { n: pageN, m: pages, total: i18n.number(total) })}</span>
      <button class="btn sm" type="button" disabled={pageN >= pages} onclick={() => void load(pageN + 1)}>{t("audit.next")}</button>
    </nav>
  {/if}
</div>

<Dialog bind:open={resetOpen} title={t("admin.reset_password_for", { name: resetFor?.username ?? "" })}>
  <form id="reset-pw" class="form" onsubmit={reset}>
    <p class="hint">{t("admin.reset_hint")}</p>
    <label class="label" for="rp-new">{t("recover.new_password")}</label>
    <input id="rp-new" class="input" type="password" bind:value={newPassword} autocomplete="new-password" />
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (resetOpen = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="reset-pw" disabled={[...newPassword].length < 10}>{t("admin.reset_password")}</button>
  {/snippet}
</Dialog>

<Dialog bind:open={quotaOpen} title={t("admin.quota_for", { name: quotaFor?.username ?? "" })}>
  <form id="quota" class="form grid" onsubmit={saveQuota}>
    <p class="hint full">{t("admin.quota_hint")}</p>
    <div>
      <label class="label" for="q-sources">{t("admin.q_sources")}</label>
      <input id="q-sources" class="input mono ltr" inputmode="numeric" bind:value={qf.sources} placeholder={t("admin.q_default", { value: (quotaFor?.defaults ?? defaults)?.sources ?? "" })} />
    </div>
    <div>
      <label class="label" for="q-profiles">{t("admin.q_profiles")}</label>
      <input id="q-profiles" class="input mono ltr" inputmode="numeric" bind:value={qf.profiles} placeholder={t("admin.q_default", { value: (quotaFor?.defaults ?? defaults)?.profiles ?? "" })} />
    </div>
    <div>
      <label class="label" for="q-links">{t("admin.q_links")}</label>
      <input id="q-links" class="input mono ltr" inputmode="numeric" bind:value={qf.links} placeholder={t("admin.q_default", { value: (quotaFor?.defaults ?? defaults)?.links ?? "" })} />
    </div>
    <div>
      <label class="label" for="q-minInterval">{t("admin.q_interval")}</label>
      <input id="q-minInterval" class="input mono ltr" inputmode="numeric" bind:value={qf.minInterval} placeholder={t("admin.q_default", { value: (quotaFor?.defaults ?? defaults) ? (quotaFor?.defaults ?? defaults)!.minInterval / 60 : "" })} />
    </div>
    <div>
      <label class="label" for="q-storage">{t("admin.q_storage")}</label>
      <input id="q-storage" class="input mono ltr" inputmode="numeric" bind:value={qf.storage} placeholder={t("admin.q_default", { value: (quotaFor?.defaults ?? defaults) ? (quotaFor?.defaults ?? defaults)!.storage / (1 << 20) : "" })} />
    </div>
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (quotaOpen = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="quota">{t("common.save")}</button>
  {/snippet}
</Dialog>

<style>
  .wrap {
    overflow-x: auto;
    /* 让表内绝对定位的无障碍标签以此为包含块，否则会逃出滚动容器、撑宽页面（反馈 #41） */
    position: relative;
  }
  .nick {
    margin-inline-start: var(--space-1);
    font-size: var(--text-xs);
  }
  .by {
    font-size: var(--text-xs);
  }
  .role {
    margin-inline-start: var(--space-2);
    padding-inline: 6px;
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-xs);
  }
  .search {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-block-end: var(--space-3);
    color: var(--fg-muted);
  }
  .search .input {
    max-inline-size: 260px;
  }
  .search .muted {
    font-size: var(--text-sm);
  }
  .pager {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    margin-block-start: var(--space-3);
    font-size: var(--text-sm);
  }
  .role.open {
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  .usage {
    font-size: var(--text-sm);
    color: var(--fg-muted);
    white-space: nowrap;
  }
  .act {
    text-align: end;
  }
  /* 操作按钮在窄屏上换行，不把表格撑出页面（需求 R12 新增了一个按钮） */
  .act :global(.btn) {
    margin-block: 2px;
  }
  .table-wrap {
    /* 表头里的 visually-hidden 是绝对定位的：滚动框必须是它的包含块，否则会跑出滚动框、撑宽页面 */
    position: relative;
    overflow-x: auto;
  }
  .form {
    display: grid;
    gap: var(--space-2);
  }
  .form.grid {
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }
  .full {
    grid-column: 1 / -1;
  }
</style>
