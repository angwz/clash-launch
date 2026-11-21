<script lang="ts">
  // 用户协议页（需求 R40）：未登录也能访问；注册页与登录后的同意页都链接到这里。
  import AuthLayout from "../components/AuthLayout.svelte";
  import Markdown from "../components/Markdown.svelte";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import { fetchTerms, termsDoc, type Terms } from "../lib/terms";

  let terms = $state<Terms | null>(null);
  let error = $state("");
  $effect(() => {
    fetchTerms()
      .then((x) => (terms = x))
      .catch((e) => (error = errorText(e)));
  });
  const doc = $derived(terms ? termsDoc(terms, i18n.locale) : null);
</script>

<AuthLayout wide title={doc?.title || t("terms.title")} subtitle={doc ? t("terms.version", { n: doc.version }) + (doc.updatedAt ? " · " + t("terms.updated", { when: i18n.date(doc.updatedAt) }) : "") : undefined}>
  {#if error}
    <p class="field-error" role="alert">{error}</p>
  {:else if doc}
    <article class="terms"><Markdown blocks={doc.blocks} /></article>
  {:else}
    <p class="muted">{t("common.loading")}</p>
  {/if}
  <p class="alt"><a href={session.user ? "/" : "/register"}>{session.user ? t("terms.back_home") : t("terms.back_register")}</a></p>
</AuthLayout>

<style>
  .terms {
    line-height: 1.8;
  }
  .alt {
    margin-block-start: var(--space-6);
    text-align: center;
    font-size: var(--text-sm);
  }
</style>
