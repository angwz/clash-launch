<script lang="ts">
  // 用户管理：只显示元数据与用量（看不到任何人的内容）。停用 / 启用、重置口令、调整配额。
  import Dialog from "../Dialog.svelte";
  import StatusBadge from "../StatusBadge.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
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

  async function load() {
    const r = await api.get<{ items: U[]; defaults: Limits }>("/api/v1/admin/users", { query: { limit: 200 } });
    users = r.items;
    defaults = r.defaults;
  }
  $effect(() => {
    load().catch((e) => toasts.push("err", errorText(e)));
  });

  const mb = (n: number) => (n / (1 << 20)).toLocaleString(i18n.intlLocale, { maximumFractionDigits: 1 }) + " MiB";

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

<div class="wrap">
  <table class="table">
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
            <strong>{u.username}</strong>
            {#if u.role === "admin"}<span class="role">{t("role.admin")}</span>{/if}
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
              <button class="btn sm ghost" class:danger={!u.disabled} type="button" onclick={() => void toggle(u)}>{u.disabled ? t("admin.enable") : t("admin.disable")}</button>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
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
      <input id="q-sources" class="input mono ltr" inputmode="numeric" bind:value={qf.sources} placeholder={t("admin.q_default", { value: defaults?.sources ?? "" })} />
    </div>
    <div>
      <label class="label" for="q-profiles">{t("admin.q_profiles")}</label>
      <input id="q-profiles" class="input mono ltr" inputmode="numeric" bind:value={qf.profiles} placeholder={t("admin.q_default", { value: defaults?.profiles ?? "" })} />
    </div>
    <div>
      <label class="label" for="q-links">{t("admin.q_links")}</label>
      <input id="q-links" class="input mono ltr" inputmode="numeric" bind:value={qf.links} placeholder={t("admin.q_default", { value: defaults?.links ?? "" })} />
    </div>
    <div>
      <label class="label" for="q-minInterval">{t("admin.q_interval")}</label>
      <input id="q-minInterval" class="input mono ltr" inputmode="numeric" bind:value={qf.minInterval} placeholder={t("admin.q_default", { value: defaults ? defaults.minInterval / 60 : "" })} />
    </div>
    <div>
      <label class="label" for="q-storage">{t("admin.q_storage")}</label>
      <input id="q-storage" class="input mono ltr" inputmode="numeric" bind:value={qf.storage} placeholder={t("admin.q_default", { value: defaults ? defaults.storage / (1 << 20) : "" })} />
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
  .role {
    margin-inline-start: var(--space-2);
    padding-inline: 6px;
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-xs);
  }
  .usage {
    font-size: var(--text-sm);
    color: var(--fg-muted);
    white-space: nowrap;
  }
  .act {
    text-align: end;
    white-space: nowrap;
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
