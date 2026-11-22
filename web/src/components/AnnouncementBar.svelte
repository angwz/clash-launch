<script lang="ts">
  // 站内公告（需求 R32）：页面顶部的横幅，新的在前。正文是纯文字，原样显示。
  // 关闭后记在服务器上，换设备也不再出现；管理员发布、修改或删除时经实时通道即时更新。
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { live } from "../lib/live";
  import { toasts } from "../lib/toast.svelte";
  import Icon from "./Icon.svelte";

  interface Announcement {
    id: string;
    text: string;
    level: "info" | "warn";
  }
  let items = $state<Announcement[]>([]);

  async function load() {
    items = (await api.get<{ items: Announcement[] }>("/api/v1/announcements")).items;
  }
  // 公告的生效与过期按时间发生，没有事件：每 10 分钟也重读一次
  $effect(() => void load().catch(() => undefined));
  $effect(() => live(load, 600_000, ["announce"]).stop);

  async function dismiss(a: Announcement) {
    items = items.filter((x) => x.id !== a.id);
    try {
      await api.post("/api/v1/announcements/" + a.id + "/dismiss");
    } catch (e) {
      toasts.push("err", errorText(e));
      await load().catch(() => undefined);
    }
  }
</script>

{#if items.length}
  <div class="bars" role="region" aria-label={t("announce.region")}>
    {#each items as a (a.id)}
      <div class="bar" data-level={a.level}>
        <Icon name={a.level === "warn" ? "warning" : "info"} size={16} />
        <p>{a.text}</p>
        <button class="btn ghost icon sm" type="button" aria-label={t("announce.dismiss")} title={t("announce.dismiss")} onclick={() => void dismiss(a)}><Icon name="x" size={14} /></button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .bars {
    display: grid;
  }
  .bar {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding-block: var(--space-2);
    padding-inline: var(--space-4);
    border-block-end: 1px solid var(--border);
    background: var(--info-soft);
    color: var(--fg);
    font-size: var(--text-sm);
  }
  .bar[data-level="warn"] {
    background: var(--warn-soft);
  }
  .bar > :global(svg) {
    flex: none;
    margin-block-start: 2px;
    color: var(--info);
  }
  .bar[data-level="warn"] > :global(svg) {
    color: var(--warn);
  }
  p {
    flex: 1;
    min-inline-size: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
