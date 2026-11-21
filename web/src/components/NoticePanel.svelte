<script lang="ts">
  // 站内提醒列表（需求 R23、R24）：右上角铃铛打开。每条提醒按种类生成文案与去处；
  // 订阅链接出现新来源时可以直接更换链接令牌（旧链接立即失效，确认后执行）。打开即把可见的提醒标为已读。
  import Icon from "./Icon.svelte";
  import { api, type Link } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { notices, type Notice } from "../lib/notices.svelte";
  import { flagOf, regionName } from "../lib/regions";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { open = $bindable(false) }: { open?: boolean } = $props();
  let panel: HTMLDivElement | undefined = $state();

  $effect(() => {
    if (!open) return;
    void notices.refresh().then(() => {
      const ids = notices.items.filter((n) => !n.read).map((n) => n.id);
      if (ids.length) void notices.markRead(ids);
    });
    panel?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && (open = false);
    const onDown = (e: PointerEvent) => {
      if (panel && !panel.contains(e.target as Node) && !(e.target as Element).closest?.(".notice-btn")) open = false;
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("pointerdown", onDown, true);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("pointerdown", onDown, true);
    };
  });

  function text(n: Notice): string {
    const d = n.detail;
    switch (n.kind) {
      case "source_failing":
        return t("notices.source_failing", { name: d.name ?? "" });
      case "check_failed":
        return t("notices.check_failed", { name: d.name ?? "" });
      case "link_new":
        return d.country
          ? t("notices.link_country", { link: d.link ?? "", where: flagOf(d.country) + " " + regionName(d.country, i18n.intlLocale) })
          : t("notices.link_client", { link: d.link ?? "", client: d.client ?? "" });
    }
  }
  function href(n: Notice): string {
    const d = n.detail;
    if (n.kind === "source_failing" && d.source) return "/sources/" + d.source;
    if (n.kind === "check_failed" && d.profile) return "/profiles/" + d.profile;
    if (n.kind === "link_new" && d.profile) return "/profiles/" + d.profile + "/links";
    return "";
  }
  function go(n: Notice) {
    const h = href(n);
    if (!h) return;
    open = false;
    router.go(h);
  }

  let busy = $state("");
  async function resetLink(n: Notice) {
    const { profile, linkId } = n.detail;
    if (!profile || !linkId || !confirm(t("links.reset_confirm"))) return;
    busy = n.id;
    try {
      const base = "/api/v1/profiles/" + profile + "/links";
      const l = (await api.get<{ items: Link[] }>(base)).items.find((x) => x.id === linkId);
      if (!l) throw new Error(t("notices.link_gone"));
      await api.post(base + "/" + l.id + "/reset", {}, { ifMatch: l.version });
      toasts.push("ok", t("links.reset_done"));
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      busy = "";
    }
  }
</script>

{#if open}
  <div class="panel-pop" role="dialog" aria-label={t("notices.title")} tabindex="-1" bind:this={panel}>
    <header>
      <h2>{t("notices.title")}</h2>
    </header>
    {#if notices.items.length === 0}
      <p class="empty muted">{t("notices.empty")}</p>
    {:else}
      <ul>
        {#each notices.items as n (n.id)}
          <li class:unread={!n.read} data-kind={n.kind}>
            <Icon name={n.kind === "link_new" ? "globe" : "warning"} size={16} />
            <div class="body">
              <p>{text(n)}</p>
              {#if n.detail.error}<p class="err">{n.detail.error}</p>{/if}
              <p class="meta">
                <time datetime={n.createdAt}>{i18n.relative(n.createdAt)}</time>
                {#if href(n)}<button class="link" type="button" onclick={() => go(n)}>{t("notices.view")}</button>{/if}
                {#if n.kind === "link_new" && n.detail.linkId}
                  <button class="link danger" type="button" disabled={busy === n.id} onclick={() => void resetLink(n)}>{t("notices.reset_link")}</button>
                {/if}
              </p>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .panel-pop {
    position: fixed;
    z-index: 55;
    inset-block-start: calc(var(--topbar-h) + 4px);
    inset-inline-end: var(--space-4);
    inline-size: min(380px, calc(100vw - 32px));
    max-block-size: min(70dvh, 560px);
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
    outline: none;
  }
  header {
    position: sticky;
    inset-block-start: 0;
    padding: var(--space-3) var(--space-4);
    border-block-end: 1px solid var(--border);
    background: var(--bg-raised);
  }
  h2 {
    font-size: var(--text-md);
  }
  .empty {
    padding: var(--space-6) var(--space-4);
    text-align: center;
    font-size: var(--text-sm);
  }
  li {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border-block-end: 1px solid var(--border);
    font-size: var(--text-sm);
  }
  li:last-child {
    border-block-end: none;
  }
  li :global(.icon) {
    flex: none;
    margin-block-start: 2px;
    color: var(--warn);
  }
  li[data-kind="link_new"] :global(.icon) {
    color: var(--info);
  }
  li.unread {
    background: var(--accent-soft);
  }
  .body {
    display: grid;
    gap: 2px;
    min-inline-size: 0;
  }
  .err {
    color: var(--fg-muted);
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-fg);
    font: inherit;
    text-decoration: underline;
    text-underline-offset: 2px;
    cursor: pointer;
  }
  .link.danger {
    color: var(--err);
  }
</style>
