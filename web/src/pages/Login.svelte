<script lang="ts">
  // 登录：口令 →（启用了第二因素时）验证码或恢复码。
  import AuthLayout from "../components/AuthLayout.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { session, type MFAChallenge } from "../lib/session.svelte";
  import { isCancelled, passkeyLogin, passkeysSupported } from "../lib/webauthn";

  const canPasskey = passkeysSupported();

  let username = $state("");
  let password = $state("");
  let code = $state("");
  let challenge = $state<MFAChallenge | null>(null);
  let busy = $state(false);
  let error = $state("");
  let canRegister = $state(false);

  $effect(() => {
    // 还没有任何账号：直接去首次注册（第一个访问者成为管理员）
    api
      .get<{ needed: boolean }>("/api/v1/setup")
      .then((r) => r.needed && router.go("/setup", true))
      .catch(() => {});
    api
      .get<{ open: boolean }>("/api/v1/register")
      .then((r) => (canRegister = r.open))
      .catch(() => (canRegister = false));
  });

  function done() {
    router.go(new URLSearchParams(router.search).get("next") || "/", true);
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      challenge = await session.login(username, password);
      if (!challenge) done();
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }

  // 通行密钥：无 challenge 时为免密码登录；有 challenge 时作为第二步
  async function withPasskey() {
    busy = true;
    error = "";
    try {
      session.adopt(await passkeyLogin(challenge?.ticket ?? ""));
      done();
    } catch (err) {
      if (!isCancelled(err)) error = errorText(err);
    } finally {
      busy = false;
    }
  }

  async function submitCode(e: SubmitEvent) {
    e.preventDefault();
    if (!challenge) return;
    busy = true;
    error = "";
    try {
      await session.completeTOTP(challenge.ticket, code);
      done();
    } catch (err) {
      error = errorText(err);
      code = "";
    } finally {
      busy = false;
    }
  }
</script>

{#if !challenge}
  <AuthLayout title={t("login.title")} subtitle={t("login.subtitle")}>
    <form onsubmit={submit} novalidate>
      <div>
        <label class="label" for="u">{t("field.username")}</label>
        <input id="u" class="input" bind:value={username} autocomplete="username webauthn" required autocapitalize="off" spellcheck="false" />
      </div>
      <div>
        <label class="label" for="p">{t("field.password")}</label>
        <input id="p" class="input" type="password" bind:value={password} autocomplete="current-password" required />
      </div>
      <p class="forgot"><a href="/recover">{t("login.forgot")}</a></p>
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !username || !password}>{busy ? t("login.busy") : t("login.submit")}</button>
      {#if canPasskey}
        <div class="or"><span>{t("login.or")}</span></div>
        <button class="btn" type="button" disabled={busy} onclick={() => void withPasskey()}>{t("passkey.login")}</button>
      {/if}
      {#if canRegister}
        <p class="alt">{t("login.no_account")} <a href="/register">{t("login.register")}</a></p>
      {/if}
    </form>
  </AuthLayout>
{:else}
  <AuthLayout title={t("mfa.title")} subtitle={t("mfa.subtitle")}>
    <form onsubmit={submitCode} novalidate>
      <div>
        <label class="label" for="code">{t("mfa.code")}</label>
        <!-- svelte-ignore a11y_autofocus -->
        <input id="code" class="input mono ltr code" bind:value={code} autocomplete="one-time-code" inputmode="text" maxlength="12" autofocus spellcheck="false" />
        <p class="hint">{t("mfa.code_hint")}</p>
      </div>
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || code.trim().length < 6}>{t("mfa.verify")}</button>
      {#if canPasskey && challenge.mfa.includes("passkey")}
        <button class="btn" type="button" disabled={busy} onclick={() => void withPasskey()}>{t("passkey.use")}</button>
      {/if}
      <p class="alt"><button class="linkish" type="button" onclick={() => { challenge = null; code = ""; error = ""; }}>{t("mfa.back")}</button></p>
    </form>
  </AuthLayout>
{/if}

<style>
  .forgot {
    margin-block-start: calc(-1 * var(--space-2));
    text-align: end;
    font-size: var(--text-sm);
  }
  .alt {
    text-align: center;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .or {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .or::before,
  .or::after {
    content: "";
    flex: 1;
    border-block-start: 1px solid var(--border);
  }
  .code {
    font-size: var(--text-xl);
    letter-spacing: 0.2em;
    text-align: center;
  }
  .linkish {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-fg);
    font: inherit;
    cursor: pointer;
  }
</style>
