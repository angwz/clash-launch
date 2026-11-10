<script lang="ts">
  import AuthLayout from "../components/AuthLayout.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  let username = $state("");
  let password = $state("");
  let busy = $state(false);
  let error = $state("");
  let canRegister = $state(false);

  $effect(() => {
    api
      .get<{ open: boolean }>("/api/v1/register")
      .then((r) => (canRegister = r.open))
      .catch(() => (canRegister = false));
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      await session.login(username, password);
      router.go(new URLSearchParams(router.search).get("next") || "/", true);
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<AuthLayout title={t("login.title")} subtitle={t("login.subtitle")}>
  <form onsubmit={submit} novalidate>
    <div>
      <label class="label" for="u">{t("field.username")}</label>
      <input id="u" class="input" bind:value={username} autocomplete="username" required autocapitalize="off" spellcheck="false" />
    </div>
    <div>
      <label class="label" for="p">{t("field.password")}</label>
      <input id="p" class="input" type="password" bind:value={password} autocomplete="current-password" required />
    </div>
    <p class="forgot"><a href="/recover">{t("login.forgot")}</a></p>
    {#if error}<p class="field-error" role="alert">{error}</p>{/if}
    <button class="btn primary" type="submit" disabled={busy || !username || !password}>{busy ? t("login.busy") : t("login.submit")}</button>
    {#if canRegister}
      <p class="alt">{t("login.no_account")} <a href="/register">{t("login.register")}</a></p>
    {/if}
  </form>
</AuthLayout>

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
</style>
