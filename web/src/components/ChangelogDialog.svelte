<script lang="ts" module>
  // 更新日志（需求 R27）：有新版本时从顶栏打开，也可从命令面板打开。
  // 日志是随前端发布的静态文件（public/changelog/<语言>.md），每次打开都向服务器重新读取（no-cache）：
  // 旧页面提示新版本时，读到的是服务器上新版本的日志。
  class ChangelogState {
    open = $state(false);
  }
  export const changelog = new ChangelogState();
</script>

<script lang="ts">
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { parse, type Block } from "../lib/md";
  import { realtime } from "../lib/realtime.svelte";
  import Dialog from "./Dialog.svelte";
  import Markdown from "./Markdown.svelte";

  let blocks = $state<Block[] | null>(null);
  let error = $state(false);
  const newVersion = $derived(!!realtime.pageVersion && !!realtime.serverVersion && realtime.pageVersion !== realtime.serverVersion);

  $effect(() => {
    if (!changelog.open) return;
    const lang = i18n.locale === "ar-XB" ? "en" : i18n.locale;
    blocks = null;
    error = false;
    fetch("/changelog/" + lang + ".md", { cache: "no-cache" })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(String(r.status)))))
      .then((s) => (blocks = parse(s).filter((b) => !(b.k === "h" && b.level === 1)))) // 一级标题与对话框标题重复
      .catch(() => (error = true));
  });
</script>

<Dialog bind:open={changelog.open} title={t("changelog.title")} wide>
  {#if newVersion}
    <p class="new">{t("changelog.new_version", { version: realtime.serverVersion ?? "" })}</p>
  {/if}
  {#if error}
    <p class="muted">{t("changelog.unavailable")}</p>
  {:else if blocks}
    <div class="log"><Markdown {blocks} /></div>
  {:else}
    <p class="muted">{t("common.loading")}</p>
  {/if}
  {#snippet footer()}
    {#if newVersion}<button class="btn primary" type="button" onclick={() => location.reload()}>{t("changelog.reload")}</button>{/if}
    <button class="btn" type="button" onclick={() => (changelog.open = false)}>{t("common.close")}</button>
  {/snippet}
</Dialog>

<style>
  .new {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--info-soft);
    color: var(--fg);
  }
  .log {
    max-block-size: 60dvh;
    overflow-y: auto;
  }
</style>
