<script lang="ts">
  // 首次设置：用日志中的一次性设置链接（/setup#<令牌>）创建首个管理员。令牌在 URL 片段中，不会发给任何服务器日志。
  import AuthLayout from "../components/AuthLayout.svelte";
  import PasswordFields from "../components/PasswordFields.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  const token = $derived(router.hash.slice(1));
  let username = $state("admin");
  let password = $state("");
  let valid = $state(false);
  let busy = $state(false);
  let error = $state("");

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      await api.post("/api/v1/setup", { token, username, password });
      await session.login(username, password);
      history.replaceState(null, "", "/setup"); // 令牌已用，移出地址栏
      router.go("/welcome", true);
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<AuthLayout title={t("setup.title")} subtitle={t("setup.subtitle")}>
  {#if !token}
    <p class="field-error" role="alert">{t("setup.no_token")}</p>
  {:else}
    <form onsubmit={submit} novalidate>
      <PasswordFields bind:username bind:password bind:valid />
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !valid}>{t("setup.submit")}</button>
    </form>
  {/if}
</AuthLayout>
