<script lang="ts">
  // 账号与安全（第 14.1 节）：密码、安全问题、登录设备与审计。个人界面偏好在「设置」页（反馈 #57）。
  // 通行密钥、两步验证与提醒邮件在各自的分节中（随 M5 后续步骤加入）。
  import { passwordOk } from "../lib/password";
  import PageHeader from "../components/PageHeader.svelte";
  import QuestionFields, { blankQuestions, questionsValid, toPayload, type QuestionInput } from "../components/QuestionFields.svelte";
  import Icon from "../components/Icon.svelte";
  import TwoFactor from "../components/account/TwoFactor.svelte";
  import Passkeys from "../components/account/Passkeys.svelte";
  import Alerts from "../components/account/Alerts.svelte";
  import AuditLog from "../components/AuditLog.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import { toasts } from "../lib/toast.svelte";

  interface SessionInfo {
    id: string;
    device: string;
    ua: string;
    ip: string;
    createdAt: string;
    lastSeen: string;
    current: boolean;
  }

  let sessions = $state<SessionInfo[]>([]);
  let quota = $state<{ limits: { sources: number; profiles: number; links: number; storage: number }; usage: { sources: number; profiles: number; links: number; bytes: number } } | null>(null);
  let myQuestions = $state<{ builtin: number; text?: string }[]>([]);

  // 修改密码
  let pw = $state({ current: "", next: "", confirm: "", logoutOthers: true });
  // 修改安全问题
  let editingQs = $state(false);
  let qs = $state<QuestionInput[]>(blankQuestions());
  let qsPassword = $state("");

  async function load() {
    try {
      const [s, q] = await Promise.all([
        api.get<{ items: SessionInfo[] }>("/api/v1/account/sessions"),
        api.get<{ questions: { builtin: number; text?: string }[] }>("/api/v1/account/questions"),
      ]);
      sessions = s.items;
      quota = await api.get("/api/v1/account/quota");
      myQuestions = q.questions;
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  $effect(() => void load());

  async function changePassword(e: SubmitEvent) {
    e.preventDefault();
    try {
      await api.put("/api/v1/account/password", { current: pw.current, password: pw.next, logoutOthers: pw.logoutOthers });
      pw = { current: "", next: "", confirm: "", logoutOthers: true };
      toasts.push("ok", t("account.password_changed"));
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function saveQuestions(e: SubmitEvent) {
    e.preventDefault();
    try {
      await api.put("/api/v1/account/questions", { password: qsPassword, ...toPayload(qs) });
      editingQs = false;
      qsPassword = "";
      qs = blankQuestions();
      toasts.push("ok", t("account.questions_saved"));
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function revoke(s: SessionInfo) {
    try {
      await api.del("/api/v1/account/sessions/" + s.id);
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  const pwOk = $derived(pw.current !== "" && passwordOk(pw.next) && pw.next === pw.confirm);
  const qLabel = (q: { builtin: number; text?: string }) => (q.builtin >= 0 ? t("question.builtin." + q.builtin) : (q.text ?? ""));
</script>

<div class="page">
  <PageHeader title={t("nav.account")} subtitle={session.user ? session.user.username + " · " + t("role." + session.user.role) : ""} />

  <section class="panel box" aria-labelledby="ac-pw">
    <h2 id="ac-pw">{t("account.password")}</h2>
    <form class="grid" onsubmit={changePassword}>
      <div>
        <label class="label" for="ac-cur">{t("account.current_password")}</label>
        <input id="ac-cur" class="input" type="password" bind:value={pw.current} autocomplete="current-password" />
      </div>
      <div>
        <label class="label" for="ac-new">{t("recover.new_password")}</label>
        <input id="ac-new" class="input" type="password" bind:value={pw.next} autocomplete="new-password" />
      </div>
      <div>
        <label class="label" for="ac-cf">{t("field.password_confirm")}</label>
        <input id="ac-cf" class="input" type="password" bind:value={pw.confirm} autocomplete="new-password" aria-invalid={pw.confirm !== "" && pw.confirm !== pw.next} />
      </div>
      <label class="check"><input type="checkbox" bind:checked={pw.logoutOthers} />{t("account.logout_others")}</label>
      <div class="actions"><button class="btn primary" type="submit" disabled={!pwOk}>{t("account.change_password")}</button></div>
    </form>
  </section>

  <section class="panel box" aria-labelledby="ac-q">
    <div class="row">
      <div>
        <h2 id="ac-q">{t("questions.title")}</h2>
        <p class="muted">{myQuestions.length ? t("account.questions_desc") : t("account.questions_missing")}</p>
      </div>
      {#if !editingQs}<button class="btn" type="button" onclick={() => (editingQs = true)}>{myQuestions.length ? t("account.questions_change") : t("account.questions_set")}</button>{/if}
    </div>
    {#if !editingQs && myQuestions.length}
      <ol class="qlist">{#each myQuestions as q, i (i)}<li>{qLabel(q)}</li>{/each}</ol>
    {/if}
    {#if editingQs}
      <form class="grid" onsubmit={saveQuestions}>
        <QuestionFields bind:questions={qs} />
        <div>
          <label class="label" for="ac-qpw">{t("account.current_password")}</label>
          <input id="ac-qpw" class="input" type="password" bind:value={qsPassword} autocomplete="current-password" />
        </div>
        <div class="actions">
          <button class="btn ghost" type="button" onclick={() => (editingQs = false)}>{t("common.cancel")}</button>
          <button class="btn primary" type="submit" disabled={!qsPassword || !questionsValid(qs)}>{t("common.save")}</button>
        </div>
      </form>
    {/if}
  </section>

  <Passkeys />

  <TwoFactor />

  <Alerts />

  <section class="panel box" aria-labelledby="ac-s">
    <h2 id="ac-s">{t("account.sessions")}</h2>
    <ul class="sessions">
      {#each sessions as s (s.id)}
        <li>
          <Icon name="monitor" size={18} />
          <div class="sinfo">
            <strong>{s.device}{#if s.current}<span class="cur">{t("account.this_device")}</span>{/if}</strong>
            <span class="muted mono ltr">{s.ip || "—"}</span>
            <span class="muted">{t("account.last_active", { when: i18n.relative(s.lastSeen) })}</span>
          </div>
          {#if !s.current}<button class="btn sm ghost danger" type="button" onclick={() => void revoke(s)}>{t("account.revoke")}</button>{/if}
        </li>
      {/each}
    </ul>
  </section>

  {#if quota}
    <section class="panel box" aria-labelledby="ac-quota">
      <h2 id="ac-quota">{t("account.quota")}</h2>
      <ul class="meters">
        {#each [[t("admin.q_sources"), quota.usage.sources, quota.limits.sources], [t("admin.q_profiles"), quota.usage.profiles, quota.limits.profiles], [t("admin.q_links"), quota.usage.links, quota.limits.links]] as [name, used, limit] (name)}
          <li><span>{name}</span><span class="mono">{used} / {Number(limit) < 0 ? "∞" : limit}</span></li>
        {/each}
        <li><span>{t("admin.q_storage")}</span><span class="mono">{(quota.usage.bytes / (1 << 20)).toFixed(1)} / {quota.limits.storage < 0 ? "∞" : (quota.limits.storage / (1 << 20)).toFixed(0)} MiB</span></li>
      </ul>
    </section>
  {/if}

  <section class="panel box" aria-labelledby="ac-audit">
    <h2 id="ac-audit">{t("account.audit")}</h2>
    <AuditLog scope="me" />
  </section>

</div>

<style>
  .page > :global(*) {
    max-inline-size: 760px;
  }
  /* 与其他页面同一左缘（1120 居中），内容本身较窄（760）（反馈 #56） */
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .box {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .grid {
    display: grid;
    gap: var(--space-3);
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-4);
  }
  .check {
    display: inline-flex;
    gap: var(--space-2);
    align-items: center;
    font-size: var(--text-sm);
  }
  .check input {
    accent-color: var(--accent);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .qlist {
    display: grid;
    gap: var(--space-1);
    padding-inline-start: var(--space-5);
    list-style: decimal;
    color: var(--fg-muted);
  }
  .meters {
    display: grid;
    gap: var(--space-1);
    font-size: var(--text-sm);
  }
  .meters li {
    display: flex;
    justify-content: space-between;
    padding-block: var(--space-1);
    border-block-end: 1px solid var(--border);
  }
  .sessions li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding-block: var(--space-3);
    border-block-end: 1px solid var(--border);
  }
  .sessions li:last-child {
    border-block-end: 0;
  }
  .sessions :global(.icon) {
    color: var(--fg-subtle);
  }
  .sinfo {
    display: grid;
    flex: 1;
    font-size: var(--text-sm);
  }
  .cur {
    margin-inline-start: var(--space-2);
    padding-inline: 6px;
    border-radius: var(--radius-full);
    background: var(--ok-soft);
    color: var(--ok);
    font-size: var(--text-xs);
    font-weight: 560;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
