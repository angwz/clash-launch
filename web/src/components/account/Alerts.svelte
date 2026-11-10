<script lang="ts">
  // 提醒邮件：用户填写自己的 SMTP，测试邮件送达后启用。新设备登录、口令或安全设置变更时发送提醒。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Settings {
    host: string;
    port: number;
    security: "tls" | "starttls";
    username: string;
    password: string;
    from: string;
    to: string;
    hasPassword?: boolean;
    enabled?: boolean;
    verifiedAt?: string;
  }

  let available = $state(true);
  let saved = $state<Settings | null>(null);
  let form = $state<Settings>({ host: "", port: 465, security: "tls", username: "", password: "", from: "", to: "" });
  let editing = $state(false);
  let busy = $state(false);

  async function load() {
    const r = await api.get<{ available: boolean; settings: Settings | null }>("/api/v1/account/smtp");
    available = r.available;
    saved = r.settings;
    if (saved) form = { ...saved, password: "" };
  }
  $effect(() => {
    load().catch((e) => toasts.push("err", errorText(e)));
  });

  function setPort(p: number) {
    form.port = p;
    form.security = p === 465 ? "tls" : "starttls";
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      await api.put("/api/v1/account/smtp", form);
      editing = false;
      await load();
      toasts.push("ok", t("alerts.saved"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function test() {
    busy = true;
    try {
      await api.post("/api/v1/account/smtp/test");
      await load();
      toasts.push("ok", t("alerts.test_ok"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function remove() {
    try {
      await api.del("/api/v1/account/smtp");
      saved = null;
      form = { host: "", port: 465, security: "tls", username: "", password: "", from: "", to: "" };
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }
</script>

<section class="panel box" aria-labelledby="ac-mail">
  <div class="row">
    <div>
      <h2 id="ac-mail">{t("alerts.section")}</h2>
      <p class="muted">{t("alerts.desc")}</p>
    </div>
    {#if saved && !editing}
      <span class="state" class:on={saved.enabled}>{saved.enabled ? t("alerts.on") : t("alerts.unverified")}</span>
    {/if}
  </div>

  {#if !available}
    <p class="note">{t("alerts.unavailable")}</p>
  {:else if saved && !editing}
    <dl class="summary">
      <dt>{t("alerts.server")}</dt>
      <dd class="mono ltr">{saved.host}:{saved.port} · {saved.security === "tls" ? "TLS" : "STARTTLS"}</dd>
      <dt>{t("alerts.to")}</dt>
      <dd class="mono ltr">{saved.to}</dd>
      {#if saved.verifiedAt}<dt>{t("alerts.verified")}</dt><dd>{i18n.date(saved.verifiedAt)}</dd>{/if}
    </dl>
    <div class="actions start">
      <button class="btn primary" type="button" disabled={busy} onclick={() => void test()}>{t("alerts.send_test")}</button>
      <button class="btn" type="button" onclick={() => (editing = true)}>{t("alerts.edit")}</button>
      <button class="btn ghost danger" type="button" onclick={() => void remove()}>{t("common.delete")}</button>
    </div>
  {:else}
    <form class="grid" onsubmit={save}>
      <div class="two">
        <div>
          <label class="label" for="sm-host">{t("alerts.host")}</label>
          <input id="sm-host" class="input mono ltr" bind:value={form.host} placeholder="smtp.example.com" spellcheck="false" autocomplete="off" />
        </div>
        <div>
          <label class="label" for="sm-port">{t("alerts.port")}</label>
          <select id="sm-port" class="select" value={form.port} onchange={(e) => setPort(Number(e.currentTarget.value))}>
            <option value={465}>465 · TLS</option>
            <option value={587}>587 · STARTTLS</option>
            <option value={25}>25 · STARTTLS</option>
          </select>
        </div>
        <div>
          <label class="label" for="sm-user">{t("alerts.username")}</label>
          <input id="sm-user" class="input mono ltr" bind:value={form.username} autocomplete="off" spellcheck="false" />
        </div>
        <div>
          <label class="label" for="sm-pass">{t("alerts.password")}</label>
          <input id="sm-pass" class="input" type="password" bind:value={form.password} autocomplete="new-password" placeholder={saved?.hasPassword ? t("alerts.password_keep") : ""} />
        </div>
        <div>
          <label class="label" for="sm-from">{t("alerts.from")}</label>
          <input id="sm-from" class="input mono ltr" type="email" bind:value={form.from} spellcheck="false" />
        </div>
        <div>
          <label class="label" for="sm-to">{t("alerts.to")}</label>
          <input id="sm-to" class="input mono ltr" type="email" bind:value={form.to} spellcheck="false" />
        </div>
      </div>
      <p class="hint">{t("alerts.hint")}</p>
      <div class="actions">
        {#if saved}<button class="btn ghost" type="button" onclick={() => (editing = false)}>{t("common.cancel")}</button>{/if}
        <button class="btn primary" type="submit" disabled={busy || !form.host || !form.from || !form.to}>{t("common.save")}</button>
      </div>
    </form>
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-4);
  }
  .state {
    flex: none;
    padding-inline: 8px;
    border-radius: var(--radius-full);
    background: var(--warn-soft);
    color: var(--warn);
    font-size: var(--text-xs);
    font-weight: 560;
    line-height: 22px;
  }
  .state.on {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .note {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .summary {
    display: grid;
    grid-template-columns: 7rem 1fr;
    gap: var(--space-1) var(--space-4);
    font-size: var(--text-sm);
  }
  .summary dt {
    color: var(--fg-muted);
  }
  .grid {
    display: grid;
    gap: var(--space-3);
  }
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-3);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .actions.start {
    justify-content: flex-start;
  }
</style>
