<script lang="ts">
  // 注册（决定 0003）：注册开放时直接注册；关闭时需要一次性邀请链接 /register#<令牌>。
  // 需要 3 个安全问题（找回密码的唯一途径）与本地工作量证明（约 1 秒）。
  import AuthLayout from "../components/AuthLayout.svelte";
  import PasswordFields from "../components/PasswordFields.svelte";
  import QuestionFields, { blankQuestions, questionsValid, toPayload } from "../components/QuestionFields.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { proofOfWork } from "../lib/pow";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";
  import Markdown from "../components/Markdown.svelte";
  import { ApiError } from "../lib/api/client";
  import { fetchTerms, termsDoc, type Terms } from "../lib/terms";

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

  // 用户协议（需求 R40）：勾选同意后才能注册，提交时带上读到的版本；可在本页展开阅读，或在新页面打开 /terms
  let terms = $state<Terms | null>(null);
  let agreed = $state(false);
  const loadTerms = () =>
    fetchTerms()
      .then((x) => (terms = x))
      .catch((e) => (error = errorText(e)));
  $effect(() => void loadTerms());
  const termsView = $derived(terms ? termsDoc(terms, i18n.locale) : null);

  // 由谁邀请（需求 R12）：令牌放在请求体里查询；无效时不显示
  let inviter = $state<{ username: string; nickname?: string } | null>(null);
  $effect(() => {
    inviter = null;
    if (!invite) return;
    api
      .post<{ username: string; nickname?: string }>("/api/v1/register/inviter", { token: invite })
      .then((r) => (inviter = r))
      .catch(() => (inviter = null));
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      progress = 0;
      const pow = await proofOfWork("register", (p) => (progress = p));
      progress = null;
      // 注册开放时也带上邀请令牌：用于记下邀请关系（需求 R12）；无效的令牌在开放时不挡住注册
      await api.post("/api/v1/register", { username, password, invite: invite || undefined, ...toPayload(questions), pow, terms: terms!.version });
      await session.login(username, password);
      history.replaceState(null, "", "/register");
      router.go("/welcome", true);
    } catch (err) {
      // 填写期间协议更新了：取消勾选并重新读取，请用户看过新版本再同意
      if (err instanceof ApiError && err.status === 409) {
        agreed = false;
        error = t("terms.changed");
        await loadTerms();
      } else error = errorText(err);
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
    {#if inviter}<p class="inviter" role="note">{inviter.nickname ? t("register.invited_by_nick", { nick: inviter.nickname, name: inviter.username }) : t("register.invited_by", { name: inviter.username })}</p>{/if}
    <form onsubmit={submit} novalidate>
      <PasswordFields bind:username bind:password bind:valid />
      <QuestionFields bind:questions />
      <label class="agree">
        <input type="checkbox" bind:checked={agreed} disabled={!terms} />
        <span>{t("terms.agree_before")}<a href="/terms" target="_blank" rel="noopener">{termsView?.title || t("terms.title")}</a>{t("terms.agree_after")}</span>
      </label>
      {#if termsView}
        <details class="read">
          <summary>{t("terms.read_here")}</summary>
          <div class="read-body"><Markdown blocks={termsView.blocks} /></div>
        </details>
      {/if}
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !valid || !questionsValid(questions) || !agreed || !terms}>
        {#if progress !== null}{t("pow.working", { percent: Math.round(progress * 100) })}{:else}{t("register.submit")}{/if}
      </button>
      <p class="alt">{t("register.have_account")} <a href="/login">{t("register.back_login")}</a></p>
    </form>
  {/if}
</AuthLayout>

<style>
  .inviter {
    margin: 0 0 var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--info-soft);
    color: var(--info);
    font-size: var(--text-sm);
  }
  .agree {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
    margin-block: var(--space-4) var(--space-2);
    font-size: var(--text-sm);
  }
  .agree input {
    margin-block-start: 3px;
  }
  .read {
    margin-block-end: var(--space-3);
    font-size: var(--text-sm);
  }
  .read summary {
    cursor: pointer;
    color: var(--fg-muted);
  }
  .read-body {
    max-block-size: 320px;
    overflow-y: auto;
    margin-block-start: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg);
    line-height: 1.7;
  }
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
