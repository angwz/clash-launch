<script lang="ts">
  // 管理（决定 0003）：注册开关（默认关闭）、邀请规则与全站的邀请（需求 R12：人人可邀请，见 InvitesPanel）。
  import TermsEditor from "../components/admin/TermsEditor.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import AuditLog from "../components/AuditLog.svelte";
  import Status from "../components/admin/Status.svelte";
  import StatsPanel from "../components/admin/StatsPanel.svelte";
  import Users from "../components/admin/Users.svelte";
  import JobsPanel from "../components/admin/JobsPanel.svelte";
  import RetentionPanel from "../components/admin/RetentionPanel.svelte";
  import AnnouncementsPanel from "../components/admin/AnnouncementsPanel.svelte";
  import { api } from "../lib/api/client";
  import InvitesPanel from "../components/InvitesPanel.svelte";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import { toasts } from "../lib/toast.svelte";

  let open = $state<boolean | null>(null);
  let error = $state("");

  // 邀请规则（需求 R12）：普通用户最近 30 天的名额与注册时长门槛
  let policy = $state<{ perMonth: number; minAgeDays: number } | null>(null);
  async function savePolicy(e: SubmitEvent) {
    e.preventDefault();
    if (!policy) return;
    try {
      policy = await api.put<{ perMonth: number; minAgeDays: number }>("/api/v1/admin/invite-policy", { perMonth: Number(policy.perMonth), minAgeDays: Number(policy.minAgeDays) });
      toasts.push("ok", t("invites.policy_saved"));
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function load() {
    try {
      const [r, pol] = await Promise.all([api.get<{ open: boolean }>("/api/v1/admin/registration"), api.get<{ perMonth: number; minAgeDays: number }>("/api/v1/admin/invite-policy")]);
      open = r.open;
      policy = pol;
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => {
    if (session.isAdmin) void load();
  });

  // 节点写法的全局默认值（反馈 #46）
  let nodeMode = $state<"inline" | "provider" | null>(null);
  async function loadNodeMode() {
    try {
      nodeMode = (await api.get<{ default: "inline" | "provider" }>("/api/v1/node-mode")).default;
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => {
    if (session.isAdmin) void loadNodeMode();
  });
  async function setNodeMode(m: "inline" | "provider") {
    try {
      nodeMode = (await api.put<{ default: "inline" | "provider" }>("/api/v1/admin/node-mode", { default: m })).default;
      toasts.push("ok", t("admin.node_mode_saved"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  // 两步验证策略（R41）
  let mfaPolicy = $state<"off" | "admins" | "all" | null>(null);
  $effect(() => {
    if (!session.isAdmin) return;
    api
      .get<{ policy: "off" | "admins" | "all" }>("/api/v1/admin/mfa-policy")
      .then((r) => (mfaPolicy = r.policy))
      .catch((e) => (error = errorText(e)));
  });
  async function setMfaPolicy(p: "off" | "admins" | "all") {
    try {
      mfaPolicy = (await api.put<{ policy: "off" | "admins" | "all" }>("/api/v1/admin/mfa-policy", { policy: p })).policy;
      toasts.push("ok", t("admin.mfa_policy_saved"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  // 是否向普通用户显示在线人数（R11）
  let showOnline = $state<boolean | null>(null);
  $effect(() => {
    if (!session.isAdmin) return;
    api
      .get<{ showOnline: boolean }>("/api/v1/admin/pulse")
      .then((r) => (showOnline = r.showOnline))
      .catch((e) => (error = errorText(e)));
  });
  async function toggleOnline() {
    try {
      const r = await api.put<{ showOnline: boolean }>("/api/v1/admin/pulse", { showOnline: !showOnline });
      showOnline = r.showOnline;
      toasts.push("ok", r.showOnline ? t("admin.pulse_online_on") : t("admin.pulse_online_off"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function toggle() {
    try {
      const r = await api.put<{ open: boolean }>("/api/v1/admin/registration", { open: !open });
      open = r.open;
      toasts.push("ok", r.open ? t("admin.reg_opened") : t("admin.reg_closed"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

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

    <StatsPanel />

    <AnnouncementsPanel />

    <JobsPanel />

    <RetentionPanel />

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

    <section class="panel box" aria-labelledby="a-mode">
      <div class="row">
        <div>
          <h2 id="a-mode">{t("admin.node_mode")}</h2>
          <p class="muted">{t("admin.node_mode_desc")}</p>
        </div>
        {#if nodeMode}
          <div class="seg" role="radiogroup" aria-labelledby="a-mode">
            {#each ["inline", "provider"] as const as m (m)}
              <button type="button" role="radio" aria-checked={nodeMode === m} class:on={nodeMode === m} onclick={() => void setNodeMode(m)}>{t("mode." + m)}</button>
            {/each}
          </div>
        {/if}
      </div>
      <p class="hint">{t("mode.explain")}</p>
    </section>

    <TermsEditor />

    <section class="panel box" aria-labelledby="a-mfa">
      <div class="row">
        <div>
          <h2 id="a-mfa">{t("admin.mfa_policy")}</h2>
          <p class="muted">{t("admin.mfa_policy_desc")}</p>
        </div>
        {#if mfaPolicy}
          <div class="seg" role="radiogroup" aria-labelledby="a-mfa">
            {#each ["off", "admins", "all"] as const as m (m)}
              <button type="button" role="radio" aria-checked={mfaPolicy === m} class:on={mfaPolicy === m} onclick={() => void setMfaPolicy(m)}>{t("admin.mfa_policy_" + m)}</button>
            {/each}
          </div>
        {/if}
      </div>
    </section>

    <section class="panel box" aria-labelledby="a-online">
      <div class="row">
        <div>
          <h2 id="a-online">{t("admin.pulse_online")}</h2>
          <p class="muted">{t("admin.pulse_online_desc")}</p>
        </div>
        {#if showOnline !== null}
          <button class="switch" type="button" role="switch" aria-checked={showOnline} aria-labelledby="a-online" onclick={() => void toggleOnline()}><span class="knob"></span></button>
        {/if}
      </div>
    </section>

    <section class="panel box" aria-labelledby="a-ipol">
      <h2 id="a-ipol">{t("invites.policy_title")}</h2>
      <p class="muted">{t("invites.policy_desc")}</p>
      {#if policy}
        <form class="new" onsubmit={savePolicy}>
          <label class="pol">{t("invites.policy_per_month")}<input class="input num" type="number" min="0" max="1000" bind:value={policy.perMonth} /></label>
          <label class="pol">{t("invites.policy_min_age")}<input class="input num" type="number" min="0" max="3650" bind:value={policy.minAgeDays} /></label>
          <button class="btn" type="submit">{t("common.save")}</button>
        </form>
      {/if}
    </section>

    <InvitesPanel admin />

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
    /* 隐式列的最小宽度是内容宽度，宽表格会撑破页面；限定为 minmax(0, 1fr) 让表格在自身容器内滚动 */
    grid-template-columns: minmax(0, 1fr);
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
  .seg {
    display: inline-flex;
    flex: none;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg);
  }
  .seg button {
    padding-block: 4px;
    padding-inline: var(--space-3);
    border: 0;
    border-radius: calc(var(--radius-md) - 2px);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
    white-space: nowrap;
  }
  .seg button.on {
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-sm);
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
  .pol {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .num {
    inline-size: 90px;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
