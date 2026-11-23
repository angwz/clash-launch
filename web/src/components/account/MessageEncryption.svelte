<script lang="ts">
  // 站内消息加密（需求 R46，决定 0009）：状态、密钥指纹、恢复密钥与重新生成。
  // 指纹是公钥的 SHA-256 前 16 字节：双方可以当面或另找渠道核对，确认对方的公钥没有被调换。
  import { copyAndTell } from "../../lib/clipboard";
  import { api, ApiError } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { messageKeys } from "../../lib/message-keys.svelte";
  import { session } from "../../lib/session.svelte";
  import { toasts } from "../../lib/toast.svelte";
  import { unb64 } from "../../lib/e2e";

  let password = $state("");
  let mode = $state<"" | "recovery" | "unlock" | "reset">("");
  let busy = $state(false);
  let error = $state("");
  let code = $state(""); // 刚生成的恢复密钥：只显示这一次
  let fingerprint = $state("");

  $effect(() => {
    if (session.user) void messageKeys.refresh(session.user.id).catch(() => undefined);
  });
  $effect(() => {
    void messageKeys.keyId;
    if (!messageKeys.keyId) return void (fingerprint = "");
    api.get<{ key: { publicKey: string } | null }>("/api/v1/account/message-key").then(async (r) => {
      if (!r.key) return;
      const d = new Uint8Array(await crypto.subtle.digest("SHA-256", unb64(r.key.publicKey)));
      fingerprint = [...d.slice(0, 16)].map((b) => b.toString(16).padStart(2, "0")).join("").match(/.{4}/g)!.join(" ");
    }, () => undefined);
  });

  async function run() {
    if (!session.user) return;
    busy = true;
    error = "";
    try {
      if (mode === "recovery") code = await messageKeys.makeRecovery(password);
      else if (mode === "reset") await messageKeys.reset(session.user.id, password);
      else await messageKeys.unlock(session.user.id, password);
      if (mode !== "recovery") toasts.push("ok", t("e2e.unlocked"));
      password = "";
      mode = "";
    } catch (e) {
      error = e instanceof ApiError ? errorText(e) : t("e2e.bad_password");
    } finally {
      busy = false;
    }
  }
  async function copy() {
    await copyAndTell(code);
  }
  const stateText = $derived(
    messageKeys.state === "ready" ? t("e2e.state_ready") : messageKeys.state === "locked" ? t("e2e.state_locked") : messageKeys.state === "none" ? t("e2e.state_none") : messageKeys.state === "unsupported" ? t("e2e.off_unsupported") : t("common.loading"),
  );
</script>

<section class="panel box" aria-labelledby="ac-e2e">
  <h2 id="ac-e2e">{t("e2e.title")}</h2>
  <p class="muted">{t("e2e.desc")}</p>
  <p class="state" data-state={messageKeys.state}>{stateText}</p>
  {#if fingerprint}<p class="fp"><span class="muted">{t("e2e.fingerprint")}</span> <code class="mono ltr">{fingerprint}</code></p>{/if}
  {#if messageKeys.state === "ready" || messageKeys.state === "locked"}
    <p class="muted small">{messageKeys.hasRecovery ? t("e2e.recovery_set") : t("e2e.recovery_none")}</p>
  {/if}

  {#if code}
    <div class="code" role="status">
      <p><strong>{t("e2e.recovery_title")}</strong></p>
      <code class="mono ltr big">{code}</code>
      <p class="muted small">{t("e2e.recovery_save")}</p>
      <div class="actions"><button class="btn sm" type="button" onclick={() => void copy()}>{t("common.copy")}</button><button class="btn sm ghost" type="button" onclick={() => (code = "")}>{t("e2e.recovery_done")}</button></div>
    </div>
  {/if}

  <div class="actions">
    {#if messageKeys.state === "ready"}
      <button class="btn sm" type="button" onclick={() => (mode = mode === "recovery" ? "" : "recovery")}>{messageKeys.hasRecovery ? t("e2e.recovery_again") : t("e2e.recovery_make")}</button>
    {:else if messageKeys.state === "locked" || messageKeys.state === "none"}
      <button class="btn sm primary" type="button" onclick={() => (mode = mode === "unlock" ? "" : "unlock")}>{messageKeys.state === "none" ? t("e2e.enable") : t("e2e.unlock")}</button>
    {/if}
    {#if messageKeys.state === "ready" || messageKeys.state === "locked"}
      <button class="btn sm ghost danger" type="button" onclick={() => (mode = mode === "reset" ? "" : "reset")}>{t("e2e.reset")}</button>
    {/if}
  </div>

  {#if mode}
    <form class="confirm" onsubmit={(e) => { e.preventDefault(); void run(); }}>
      {#if mode === "reset"}<p class="warn">{t("e2e.reset_warning")}</p>{/if}
      <label class="label" for="e2e-pw">{t("account.current_password")}</label>
      <input id="e2e-pw" class="input" type="password" bind:value={password} autocomplete="current-password" />
      {#if error}<p class="err">{error}</p>{/if}
      <div class="actions">
        <button class="btn sm ghost" type="button" onclick={() => ((mode = ""), (password = ""), (error = ""))}>{t("common.cancel")}</button>
        <button class="btn sm primary" type="submit" disabled={busy || !password}>{mode === "reset" ? t("e2e.reset_confirm") : mode === "recovery" ? t("e2e.recovery_make") : messageKeys.state === "none" ? t("e2e.enable") : t("e2e.unlock")}</button>
      </div>
    </form>
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .state {
    font-weight: 600;
  }
  .state[data-state="ready"] {
    color: var(--ok);
  }
  .state[data-state="locked"] {
    color: var(--warn);
  }
  .small {
    font-size: var(--text-sm);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .code {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--warn);
    border-radius: var(--radius-md);
    background: var(--warn-soft);
  }
  .big {
    font-size: var(--text-md);
    overflow-wrap: anywhere;
  }
  .confirm {
    display: grid;
    gap: var(--space-2);
    max-inline-size: 420px;
  }
  .warn {
    color: var(--warn);
  }
  .err {
    color: var(--err);
  }
</style>
