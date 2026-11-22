<script lang="ts">
  // 访客试用（需求 R44）：不用登录，在一份只读的示例配置上试用编辑器与分流可视化。
  // 示例环境在服务器上与正式数据完全隔离；这里的修改只在页面里，不会保存，刷新即恢复。
  import { onDestroy } from "svelte";
  import { api, demoMode } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import ProfileEditor from "./ProfileEditor.svelte";
  import Icon from "../components/Icon.svelte";
  import Dialog from "../components/Dialog.svelte";
  import Markdown from "../components/Markdown.svelte";
  import { fetchTerms, termsDoc } from "../lib/terms";
  import { i18n } from "../lib/i18n/i18n.svelte";
  import type { Block } from "../lib/md";

  // 设计理念（R40 用户协议的第一节）：试用的人在注册前就能读到网站怎样对待他们的数据
  let ideaOpen = $state(false);
  let idea = $state<{ title: string; blocks: Block[] } | null>(null);
  fetchTerms().then(
    (x) => {
      const blocks = termsDoc(x, i18n.locale).blocks;
      const start = blocks.findIndex((b) => b.k === "h" && b.level === 2);
      if (start < 0) return;
      const end = blocks.findIndex((b, i) => i > start && b.k === "h" && b.level <= 2);
      const h = blocks[start]!;
      idea = { title: h.k === "h" ? h.text.map((x) => x.s).join("") : "", blocks: blocks.slice(start + 1, end < 0 ? undefined : end) };
    },
    () => undefined, // 读不到协议时不显示这个入口
  );

  // 先于编辑器开启：编辑器挂载时的第一批请求就应发往示例环境
  demoMode.on = true;
  onDestroy(() => (demoMode.on = false));

  let id = $state("");
  let error = $state("");
  api.get<{ profileId: string }>("/api/v1/demo").then(
    (r) => (id = r.profileId),
    (e) => (error = errorText(e)),
  );
</script>

<div class="try">
  <header class="bar">
    <a class="brand" href="/"><img src="/favicon.svg" alt="" width="22" height="22" />Clash, Launch!</a>
    <p class="note"><Icon name="info" size={14} />{t("try.note")}</p>
    <div class="actions">
      {#if idea}<button class="btn sm ghost" type="button" onclick={() => (ideaOpen = true)}>{idea.title}</button>{/if}
      {#if session.user}
        <a class="btn sm primary" href="/">{t("try.back")}</a>
      {:else}
        <a class="btn sm" href="/login">{t("try.login")}</a>
        <a class="btn sm primary" href="/register">{t("try.register")}</a>
      {/if}
    </div>
  </header>
  {#if idea}
    <Dialog bind:open={ideaOpen} title={idea.title}><Markdown blocks={idea.blocks} /></Dialog>
  {/if}
  <div class="body">
    {#if error}
      <p class="muted pad">{t("try.unavailable")}</p>
    {:else if id}
      <ProfileEditor params={{ id }} demo />
    {:else}
      <p class="muted pad">{t("common.loading")}</p>
    {/if}
  </div>
</div>

<style>
  .try {
    display: flex;
    flex-direction: column;
    block-size: 100dvh;
    background: var(--bg);
  }
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-4);
    padding-block: var(--space-2);
    padding-inline: var(--space-4);
    border-block-end: 1px solid var(--border);
    background: var(--bg-raised);
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--fg);
    font-weight: 650;
    text-decoration: none;
  }
  .note {
    display: inline-flex;
    flex: 1;
    align-items: center;
    gap: var(--space-1);
    min-inline-size: 12em;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
  .body {
    flex: 1;
    min-block-size: 0;
    overflow: auto;
  }
  .pad {
    padding: var(--space-6);
  }
</style>
