<script lang="ts">
  // 删除配置（需求 R1）：配置列表与编辑器共用。删除不可撤销，且会让客户端里的订阅立即失效，
  // 所以先列出会一起消失的东西（订阅链接条数、短链接、修订历史），再由用户确认。
  import Dialog from "./Dialog.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { open = $bindable(false), profile, onDeleted }: { open?: boolean; profile: { id: string; name: string } | null; onDeleted: () => void } = $props();

  let links = $state<number | null>(null);
  let busy = $state(false);
  let error = $state("");

  $effect(() => {
    if (!open || !profile) return;
    links = null;
    error = "";
    api
      .get<{ items: unknown[] }>(`/api/v1/profiles/${profile.id}/links`)
      .then((r) => (links = r.items.length))
      .catch(() => (links = null)); // 数不出来也不妨碍删除，只是说明里不写条数
  });

  async function confirm() {
    if (!profile || busy) return;
    busy = true;
    error = "";
    try {
      await api.del("/api/v1/profiles/" + profile.id);
      open = false;
      toasts.push("ok", t("profiles.deleted", { name: profile.name }));
      onDeleted();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
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
      {#if error}<p class="field-error" role="alert">{error}</p>{/if}
    </div>
  {/if}
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn danger" type="button" disabled={busy} onclick={() => void confirm()}>{t("profiles.delete_confirm")}</button>
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
