<script lang="ts">
  // 删除配置（需求 R1）：配置列表与编辑器共用。删除会让客户端里的订阅失效，
  // 所以先列出会一起消失的东西（订阅链接条数、短链接、修订历史），再由用户确认。
  // 确认后进入可撤销的删除（R33）：10 秒内可以撤销，到期才调用删除接口。
  import Dialog from "./Dialog.svelte";
  import { api } from "../lib/api/client";
  import { t } from "../lib/i18n/i18n.svelte";
  import { pending } from "../lib/pending.svelte";

  let { open = $bindable(false), profile, onDeleted }: { open?: boolean; profile: { id: string; name: string } | null; onDeleted?: () => void } = $props();

  let links = $state<number | null>(null);

  $effect(() => {
    if (!open || !profile) return;
    links = null;
    api
      .get<{ items: unknown[] }>(`/api/v1/profiles/${profile.id}/links`)
      .then((r) => (links = r.items.length))
      .catch(() => (links = null)); // 数不出来也不妨碍删除，只是说明里不写条数
  });

  function confirm() {
    if (!profile) return;
    const p = profile;
    open = false;
    pending.remove([{ id: p.id, path: "/api/v1/profiles/" + p.id }], t("profiles.deleted", { name: p.name }));
    onDeleted?.();
  }
</script>

<Dialog bind:open title={t("profiles.delete_title")}>
  {#if profile}
    <div class="body">
      <p>{t("profiles.delete_lead", { name: profile.name })}</p>
      <ul>
        <li>{links ? t("profiles.delete_links_n", { n: links }) : t("profiles.delete_links")}</li>
        <li>{t("profiles.delete_shorts")}</li>
        <li>{t("profiles.delete_history")}</li>
      </ul>
    </div>
  {/if}
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn danger" type="button" onclick={confirm}>{t("profiles.delete_confirm")}</button>
  {/snippet}
</Dialog>

<style>
  .body {
    display: grid;
    gap: var(--space-2);
  }
  ul {
    margin: 0;
    padding-inline-start: 18px;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
</style>
