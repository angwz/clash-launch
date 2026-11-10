<script lang="ts">
  // 管理（决定 0003）：注册开关（默认关闭）与一次性邀请链接（可选有效期，用过即作废，可撤销）。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import AuditLog from "../components/AuditLog.svelte";
  import Status from "../components/admin/Status.svelte";
  import Users from "../components/admin/Users.svelte";
  import { api, listAll, type Invite } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import { toasts } from "../lib/toast.svelte";

  let open = $state<boolean | null>(null);
  let invites = $state<Invite[]>([]);
  let hours = $state(72);
  let note = $state("");
  let created = $state<Invite | null>(null);
  let error = $state("");

  const durations = [1, 24, 72, 168, 720, 2160, 0];

  async function load() {
    try {
      const [r, list] = await Promise.all([api.get<{ open: boolean }>("/api/v1/admin/registration"), listAll<Invite>("/api/v1/admin/invites")]);
      open = r.open;
      invites = list;
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => {
    if (session.isAdmin) void load();
  });

  async function toggle() {
    try {
      const r = await api.put<{ open: boolean }>("/api/v1/admin/registration", { open: !open });
      open = r.open;
      toasts.push("ok", r.open ? t("admin.reg_opened") : t("admin.reg_closed"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function createInvite(e: SubmitEvent) {
    e.preventDefault();
    try {
      created = await api.post<Invite>("/api/v1/admin/invites", { hours, note: note.trim() });
      note = "";
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function revoke(inv: Invite) {
    try {
      await api.del("/api/v1/admin/invites/" + inv.id);
      if (created?.id === inv.id) created = null;
      await load();
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

  const durLabel = (h: number) => (h === 0 ? t("admin.never") : h < 24 ? t("admin.hours", { n: h }) : t("admin.days", { n: h / 24 }));
  const tone = (s: Invite["status"]) => (s === "active" ? "ok" : s === "used" ? "muted" : "warn") as "ok" | "muted" | "warn";
</script>

<div class="page">
  <PageHeader title={t("admin.title")} subtitle={t("admin.subtitle")} />
  {#if !session.isAdmin}
    <p class="muted">{t("error.not_found")}</p>
  {:else}
    {#if error}<p class="field-error" role="alert">{error}</p>{/if}

    <section class="panel box" aria-labelledby="a-status">
      <h2 id="a-status">{t("admin.status")}</h2>
      <Status />
    </section>

    <section class="panel box" aria-labelledby="a-users">
      <h2 id="a-users">{t("admin.users")}</h2>
      <Users />
    </section>

    <section class="panel box" aria-labelledby="a-reg">
      <div class="row">
        <div>
          <h2 id="a-reg">{t("admin.registration")}</h2>
          <p class="muted">{open ? t("admin.reg_open_desc") : t("admin.reg_closed_desc")}</p>
        </div>
        {#if open !== null}
          <button class="switch" type="button" role="switch" aria-checked={open} aria-labelledby="a-reg" onclick={() => void toggle()}><span class="knob"></span></button>
        {/if}
      </div>
      <p class="hint">{t("admin.ip_note")}</p>
    </section>

    <section class="panel box" aria-labelledby="a-inv">
      <h2 id="a-inv">{t("admin.invites")}</h2>
      <p class="muted">{t("admin.invites_desc")}</p>
      <form class="new" onsubmit={createInvite}>
        <label class="visually-hidden" for="inv-h">{t("admin.validity")}</label>
        <select id="inv-h" class="select" bind:value={hours}>
          {#each durations as h (h)}<option value={h}>{durLabel(h)}</option>{/each}
        </select>
        <input class="input" bind:value={note} placeholder={t("admin.invite_note")} aria-label={t("admin.invite_note")} maxlength="200" />
        <button class="btn primary" type="submit"><Icon name="plus" size={15} />{t("admin.new_invite")}</button>
      </form>

      {#if created?.url}
        <div class="created" role="status">
          <p><strong>{t("admin.invite_ready")}</strong> {t("admin.invite_once")}</p>
          <div class="url-row">
            <code class="url ltr">{created.url}</code>
            <button class="btn sm" type="button" onclick={() => void copy(created!.url!)}><Icon name="copy" size={14} />{t("common.copy")}</button>
          </div>
        </div>
      {/if}

      {#if invites.length}
        <table class="table">
          <thead>
            <tr>
              <th>{t("admin.col_note")}</th>
              <th>{t("admin.col_status")}</th>
              <th>{t("admin.col_expires")}</th>
              <th>{t("admin.col_created")}</th>
              <th><span class="visually-hidden">{t("common.actions")}</span></th>
            </tr>
          </thead>
          <tbody>
            {#each invites as inv (inv.id)}
              <tr>
                <td>{inv.note || "—"}</td>
                <td><StatusBadge tone={tone(inv.status)} label={t("invite." + inv.status)} /></td>
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
      {/if}
    </section>

    <section class="panel box" aria-labelledby="a-audit">
      <h2 id="a-audit">{t("admin.audit")}</h2>
      <AuditLog scope="all" />
    </section>
  {/if}
</div>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-4);
  }
  .switch {
    position: relative;
    flex: none;
    inline-size: 40px;
    block-size: 22px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-full);
    background: var(--border-strong);
    cursor: pointer;
  }
  .switch[aria-checked="true"] {
    background: var(--accent);
  }
  .knob {
    position: absolute;
    inset-block-start: 3px;
    inset-inline-start: 3px;
    inline-size: 16px;
    block-size: 16px;
    border-radius: 50%;
    background: #fff;
    transition: inset-inline-start var(--dur-fast) var(--ease);
  }
  .switch[aria-checked="true"] .knob {
    inset-inline-start: 21px;
  }
  .new {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .new .select {
    inline-size: auto;
  }
  .new .input {
    flex: 1;
    min-inline-size: 180px;
  }
  .created {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--ok-soft);
    font-size: var(--text-sm);
  }
  .url-row {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .url {
    flex: 1;
    min-inline-size: 0;
    padding: 5px var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .act {
    text-align: end;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
