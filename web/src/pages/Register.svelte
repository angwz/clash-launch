<script lang="ts">
  // 注册（决定 0003）：注册开放时直接注册；关闭时需要一次性邀请链接 /register#<令牌>。
  // 需要 3 个安全问题（找回口令的唯一途径）与本地工作量证明（约 1 秒）。
  import AuthLayout from "../components/AuthLayout.svelte";
  import PasswordFields from "../components/PasswordFields.svelte";
  import QuestionFields, { blankQuestions, questionsValid, toPayload } from "../components/QuestionFields.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { proofOfWork } from "../lib/pow";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  const invite = $derived(router.hash.slice(1));
  let open = $state<boolean | null>(null);
  let username = $state("");
  let password = $state("");
  let valid = $state(false);
  let questions = $state(blankQuestions());
  let busy = $state(false);
  let progress = $state<number | null>(null);
  let error = $state("");

  $effect(() => {
    api
      .get<{ open: boolean }>("/api/v1/register")
      .then((r) => (open = r.open))
      .catch(() => (open = false));
  });

  const allowed = $derived(open === true || invite !== "");

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      progress = 0;
      const pow = await proofOfWork("register", (p) => (progress = p));
      progress = null;
      await api.post("/api/v1/register", { username, password, invite: open ? undefined : invite, ...toPayload(questions), pow });
      await session.login(username, password);
      history.replaceState(null, "", "/register");
      router.go("/welcome", true);
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
      progress = null;
    }
  }
</script>

<AuthLayout title={t("register.title")} subtitle={open === false && invite ? t("register.with_invite") : t("register.subtitle")}>
  {#if open === null}
    <p class="muted">{t("common.loading")}</p>
  {:else if !allowed}
    <p class="closed">{t("register.closed")}</p>
    <p class="alt"><a href="/login">{t("register.back_login")}</a></p>
  {:else}
    <form onsubmit={submit} novalidate>
      <PasswordFields bind:username bind:password bind:valid />
      <QuestionFields bind:questions />
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !valid || !questionsValid(questions)}>
        {#if progress !== null}{t("pow.working", { percent: Math.round(progress * 100) })}{:else}{t("register.submit")}{/if}
      </button>
      <p class="alt">{t("register.have_account")} <a href="/login">{t("register.back_login")}</a></p>
    </form>
  {/if}
</AuthLayout>

<style>
  .closed {
    margin-block-start: var(--space-6);
    padding: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  .alt {
    margin-block-start: var(--space-3);
    text-align: center;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
</style>
