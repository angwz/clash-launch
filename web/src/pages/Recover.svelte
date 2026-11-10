<script lang="ts">
  // 找回口令（第 14.1 节）：输入用户名 → 回答安全问题（答对至少 2 个）→ 设置新口令。
  // 成功后该账号的全部会话失效，需要重新登录。需要工作量证明；连续失败后锁定 15 分钟。
  import AuthLayout from "../components/AuthLayout.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { proofOfWork } from "../lib/pow";

  interface Q {
    builtin: number;
    text?: string;
  }

  let step = $state<"name" | "answer" | "done">("name");
  let username = $state("");
  let questions = $state<Q[]>([]);
  let answers = $state(["", "", ""]);
  let password = $state("");
  let confirm = $state("");
  let busy = $state(false);
  let progress = $state<number | null>(null);
  let error = $state("");

  const label = (q: Q) => (q.builtin >= 0 ? t("question.builtin." + q.builtin) : (q.text ?? ""));

  async function loadQuestions(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      questions = (await api.post<{ questions: Q[] }>("/api/v1/recovery/questions", { username })).questions;
      step = "answer";
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    try {
      progress = 0;
      const pow = await proofOfWork("recover", (p) => (progress = p));
      progress = null;
      await api.post("/api/v1/recovery", { username, answers, password, pow });
      step = "done";
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
      progress = null;
    }
  }

  const pwOk = $derived([...password].length >= 10 && password === confirm);
</script>

<AuthLayout title={t("recover.title")} subtitle={step === "done" ? undefined : t("recover.subtitle")}>
  {#if step === "name"}
    <form onsubmit={loadQuestions} novalidate>
      <div>
        <label class="label" for="rc-u">{t("field.username")}</label>
        <input id="rc-u" class="input" bind:value={username} autocomplete="username" autocapitalize="off" spellcheck="false" required />
      </div>
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !username.trim()}>{t("recover.next")}</button>
      <p class="alt"><a href="/login">{t("register.back_login")}</a></p>
    </form>
  {:else if step === "answer"}
    <form onsubmit={submit} novalidate>
      <p class="hint">{t("recover.answer_hint")}</p>
      {#each questions as q, i (i)}
        <div>
          <label class="label" for={"rc-a" + i}>{label(q)}</label>
          <input id={"rc-a" + i} class="input" bind:value={answers[i]} autocomplete="off" />
        </div>
      {/each}
      <div>
        <label class="label" for="rc-p">{t("recover.new_password")}</label>
        <input id="rc-p" class="input" type="password" bind:value={password} autocomplete="new-password" />
        <p class="hint">{t("register.password_hint")}</p>
      </div>
      <div>
        <label class="label" for="rc-c">{t("field.password_confirm")}</label>
        <input id="rc-c" class="input" type="password" bind:value={confirm} autocomplete="new-password" aria-invalid={confirm !== "" && confirm !== password} />
      </div>
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
      <button class="btn primary" type="submit" disabled={busy || !pwOk}>
        {#if progress !== null}{t("pow.working", { percent: Math.round(progress * 100) })}{:else}{t("recover.submit")}{/if}
      </button>
    </form>
  {:else}
    <div class="done" role="status">
      <p>{t("recover.done")}</p>
      <a class="btn primary" href="/login">{t("register.back_login")}</a>
    </div>
  {/if}
</AuthLayout>

<style>
  .alt {
    text-align: center;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .done {
    display: grid;
    gap: var(--space-4);
    margin-block-start: var(--space-6);
  }
</style>
