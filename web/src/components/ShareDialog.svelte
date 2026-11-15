<script lang="ts">
  // 分享或授权给好友（反馈 #76）：从节点页、模板、短链接页直接发给某个用户，等同于在消息里附上它。
  //   - 节点：分享（复制一份）或授权（只能使用，可收回）；
  //   - 配置模板：只能分享（复制一份）；
  //   - 短链接：只能授权（为对方单独生成新的短链接，可收回，使用情况按好友统计）。
  // 发送后可以直接打开与对方的会话。
  import Dialog from "./Dialog.svelte";
  import Icon from "./Icon.svelte";
  import UserPicker from "./UserPicker.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  export type ShareTarget = { kind: "source" | "template" | "short"; id: string; name: string; mode?: "share" | "grant" };

  let { target = $bindable(null) }: { target: ShareTarget | null } = $props();

  let open = $state(false);
  let to = $state("");
  let text = $state("");
  let mode = $state<"share" | "grant">("share");
  let busy = $state(false);

  $effect(() => {
    if (target) {
      open = true;
      to = "";
      text = "";
      mode = target.kind === "short" ? "grant" : target.kind === "template" ? "share" : (target.mode ?? "share");
    }
  });
  $effect(() => {
    if (!open) target = null;
  });

  async function send() {
    if (!target || !to.trim() || busy) return;
    busy = true;
    try {
      const body: Record<string, string> = { to: to.trim(), text, mode };
      body[target.kind] = target.id;
      await api.post("/api/v1/messages", body);
      const name = to.trim();
      open = false;
      toasts.push("ok", mode === "grant" ? t("share.granted_to", { name }) : t("share.shared_to", { name }), {
        action: { label: t("share.open_chat"), run: () => router.go("/messages/" + encodeURIComponent(name)) },
      });
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      busy = false;
    }
  }
</script>

<Dialog bind:open title={target ? (mode === "grant" ? t("share.title_grant", { name: target.name }) : t("share.title_share", { name: target.name })) : ""}>
  <form id="share-form" class="form" onsubmit={(e) => { e.preventDefault(); void send(); }}>
    <div>
      <label class="label" for="share-to">{t("messages.to")}</label>
      <UserPicker id="share-to" bind:value={to} onselect={(n) => (to = n)} placeholder={t("share.to_placeholder")} />
    </div>
    {#if target?.kind === "source"}
      <fieldset class="modes">
        <legend class="label">{t("share.how")}</legend>
        <label class="choice">
          <input type="radio" name="share-mode" value="share" bind:group={mode} />
          <span><strong>{t("messages.share")}</strong><span class="muted">{t("messages.share_hint")}</span></span>
        </label>
        <label class="choice">
          <input type="radio" name="share-mode" value="grant" bind:group={mode} />
          <span><strong><Icon name="shield" size={13} /> {t("messages.grant")}</strong><span class="muted">{t("messages.grant_hint")}</span></span>
        </label>
      </fieldset>
    {:else if target?.kind === "short"}
      <p class="hint">{t("messages.grant_short_hint")}</p>
    {:else if target?.kind === "template"}
      <p class="hint">{t("share.template_hint")}</p>
    {/if}
    <div>
      <label class="label" for="share-text">{t("share.note")}</label>
      <textarea id="share-text" class="textarea" rows="2" maxlength="2000" bind:value={text} placeholder={t("share.note_placeholder")}></textarea>
    </div>
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="share-form" disabled={busy || !to.trim()}><Icon name="send" size={14} />{t("messages.send")}</button>
  {/snippet}
</Dialog>

<style>
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .modes {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .choice {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
  }
  .choice span {
    display: grid;
  }
  .choice .muted {
    font-size: var(--text-sm);
  }
</style>
