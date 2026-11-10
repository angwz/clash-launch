<script lang="ts">
  // 首次注册：还没有任何账号时，第一个访问者在这里创建管理员（反馈 #17：不再需要日志中的一次性令牌）。
  // 已有账号时只给出登录入口。
  import AuthLayout from "../components/AuthLayout.svelte";
  import PasswordFields from "../components/PasswordFields.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  let needed = $state<boolean | null>(null);
  let username = $state("admin");
  let password = $state("");
  let valid = $state(false);
  let busy = $state(false);
  let error = $state("");

  $effect(() => {
    api
      .get<{ needed: boolean }>("/api/v1/setup")
      .then((r) => (needed = r.needed))
      .catch((err) => (error = errorText(err)));
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      await api.post("/api/v1/setup", { username, password });
      await session.login(username, password);
      router.go("/welcome", true);
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<AuthLayout title={t("setup.title")} subtitle={t("setup.subtitle")}>
  {#if needed === false}
    <p class="muted">{t("setup.done")}</p>
    <a class="btn primary" href="/login">{t("setup.to_login")}</a>
  {:else if needed}
    <form onsubmit={submit} novalidate>
      <PasswordFields bind:username bind:password bind:valid />
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !valid}>{t("setup.submit")}</button>
    </form>
  {:else if error}
    <p class="field-error" role="alert">{error}</p>
  {/if}
</AuthLayout>
