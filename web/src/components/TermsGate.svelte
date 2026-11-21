<script lang="ts">
  // 协议待同意（需求 R40）：已登录的普通用户同意的版本低于当前版本时，App 用它代替整个界面。
  // 服务端在同意前拦截其他接口（403 terms.required），所以这里不启动实时通道、统计等后台请求。
  // 拒绝只记入审计并退出登录；下次登录仍会看到协议。不同意时也可以在这里注销账号（需求 R20）。
  import AuthLayout from "./AuthLayout.svelte";
  import DeleteAccount from "./account/DeleteAccount.svelte";
  import Markdown from "./Markdown.svelte";
  import { ApiError } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";
  import { fetchTerms, termsDoc, type Terms } from "../lib/terms";

  let terms = $state<Terms | null>(null);
  let error = $state("");
  let busy = $state(false);
  const load = () =>
    fetchTerms()
      .then((x) => (terms = x))
      .catch((e) => (error = errorText(e)));
  $effect(() => void load());
  const doc = $derived(terms ? termsDoc(terms, i18n.locale) : null);
  const first = $derived((session.terms?.accepted ?? 0) === 0);

  async function answer(accept: boolean) {
    if (!terms) return;
    busy = true;
    error = "";
    try {
      await session.answerTerms(terms.version, accept);
      if (!accept) {
        await session.logout();
        router.go("/login", true);
      }
    } catch (e) {
      // 阅读期间协议又更新了：重新读取，请用户再看一遍
      if (e instanceof ApiError && e.status === 409) {
        error = t("terms.changed");
        await load();
      } else error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<AuthLayout wide title={first ? t("terms.gate_first") : t("terms.gate_updated")} subtitle={first ? t("terms.gate_first_hint") : t("terms.gate_updated_hint")}>
  {#if doc}
    <article class="terms" aria-label={doc.title}>
      {#if doc.title}<h2>{doc.title}</h2>{/if}
      <Markdown blocks={doc.blocks} />
    </article>
  {:else if !error}
    <p class="muted">{t("common.loading")}</p>
  {/if}
  {#if error}<p class="field-error" role="alert">{error}</p>{/if}
  <div class="actions">
    <button class="btn" type="button" disabled={busy} onclick={() => void answer(false)}>{t("terms.decline")}</button>
    <button class="btn primary" type="button" disabled={busy || !doc} onclick={() => void answer(true)}>{t("terms.accept")}</button>
  </div>
  <details class="del">
    <summary class="subtle">{t("terms.delete_instead")}</summary>
    <DeleteAccount compact />
  </details>
</AuthLayout>

<style>
  .terms {
    max-block-size: min(60dvh, 640px);
    overflow-y: auto;
    margin-block: var(--space-4);
    padding: var(--space-4) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg);
    line-height: 1.8;
  }
  .del {
    margin-block-start: var(--space-5);
    font-size: var(--text-sm);
  }
  .del summary {
    cursor: pointer;
    margin-block-end: var(--space-3);
  }
  .terms h2 {
    font-size: var(--text-lg);
    margin-block: 0 var(--space-3);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
