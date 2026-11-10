<script lang="ts">
  // 两步验证（TOTP）：扫码添加 → 输入一个验证码确认 → 显示恢复码（只此一次）。关闭与重新生成恢复码需要口令。
  import Icon from "../Icon.svelte";
  import QRCode from "../QRCode.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  let status = $state<{ enabled: boolean; remainingCodes: number } | null>(null);
  let setup = $state<{ secret: string; uri: string } | null>(null);
  let code = $state("");
  let codes = $state<string[] | null>(null);
  let confirming = $state<"disable" | "codes" | null>(null);
  let password = $state("");

  async function load() {
    status = await api.get("/api/v1/account/totp");
  }
  $effect(() => {
    load().catch((e) => toasts.push("err", errorText(e)));
  });

  async function begin() {
    try {
      setup = await api.post("/api/v1/account/totp/setup");
      code = "";
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function enable(e: SubmitEvent) {
    e.preventDefault();
    try {
      codes = (await api.post<{ recoveryCodes: string[] }>("/api/v1/account/totp/enable", { code })).recoveryCodes;
      setup = null;
      await load();
      toasts.push("ok", t("mfa.enabled"));
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function confirmAction(e: SubmitEvent) {
    e.preventDefault();
    try {
      if (confirming === "disable") {
        await api.post("/api/v1/account/totp/disable", { password });
        toasts.push("ok", t("mfa.disabled"));
        codes = null;
      } else {
        codes = (await api.post<{ recoveryCodes: string[] }>("/api/v1/account/totp/recovery-codes", { password })).recoveryCodes;
      }
      confirming = null;
      password = "";
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function copyCodes() {
    try {
      await navigator.clipboard.writeText((codes ?? []).join("\n"));
      toasts.push("ok", t("common.copied"));
    } catch {
      toasts.push("err", t("common.copy_failed"));
    }
  }

  function downloadCodes() {
    const blob = new Blob([(codes ?? []).join("\n") + "\n"], { type: "text/plain" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "clash-launch-recovery-codes.txt";
    a.click();
    URL.revokeObjectURL(a.href);
  }
</script>

<section class="panel box" aria-labelledby="ac-mfa">
  <div class="row">
    <div>
      <h2 id="ac-mfa">{t("mfa.section")}</h2>
      <p class="muted">
        {#if status?.enabled}{t("mfa.status_on", { n: status.remainingCodes })}{:else}{t("mfa.status_off")}{/if}
      </p>
    </div>
    {#if status && !status.enabled && !setup}
      <button class="btn primary" type="button" onclick={() => void begin()}>{t("mfa.enable")}</button>
    {/if}
  </div>

  {#if setup}
    <div class="setup">
      <QRCode text={setup.uri} label={t("mfa.qr")} size={152} />
      <form class="steps" onsubmit={enable}>
        <p>{t("mfa.scan")}</p>
        <p class="hint">{t("mfa.manual")}</p>
        <code class="secret ltr">{setup.secret.replace(/(.{4})/g, "$1 ").trim()}</code>
        <label class="label" for="mfa-code">{t("mfa.code")}</label>
        <input id="mfa-code" class="input mono ltr" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength="6" />
        <div class="actions">
          <button class="btn ghost" type="button" onclick={() => (setup = null)}>{t("common.cancel")}</button>
          <button class="btn primary" type="submit" disabled={code.trim().length !== 6}>{t("mfa.confirm")}</button>
        </div>
      </form>
    </div>
  {/if}

  {#if codes}
    <div class="codes" role="status">
      <p><strong>{t("mfa.codes_title")}</strong> {t("mfa.codes_hint")}</p>
      <ol class="code-list ltr">{#each codes as c (c)}<li class="mono">{c}</li>{/each}</ol>
      <div class="actions">
        <button class="btn sm" type="button" onclick={() => void copyCodes()}><Icon name="copy" size={14} />{t("common.copy")}</button>
        <button class="btn sm" type="button" onclick={downloadCodes}><Icon name="download" size={14} />{t("mfa.download")}</button>
        <button class="btn sm ghost" type="button" onclick={() => (codes = null)}>{t("mfa.codes_done")}</button>
      </div>
    </div>
  {/if}

  {#if status?.enabled && !codes}
    {#if confirming}
      <form class="confirm" onsubmit={confirmAction}>
        <label class="label" for="mfa-pw">{t("account.current_password")}</label>
        <input id="mfa-pw" class="input" type="password" bind:value={password} autocomplete="current-password" />
        <div class="actions">
          <button class="btn ghost" type="button" onclick={() => (confirming = null)}>{t("common.cancel")}</button>
          <button class="btn" class:danger={confirming === "disable"} type="submit" disabled={!password}>{confirming === "disable" ? t("mfa.disable") : t("mfa.regenerate")}</button>
        </div>
      </form>
    {:else}
      <div class="actions start">
        <button class="btn" type="button" onclick={() => (confirming = "codes")}>{t("mfa.regenerate")}</button>
        <button class="btn ghost danger" type="button" onclick={() => (confirming = "disable")}>{t("mfa.disable")}</button>
      </div>
    {/if}
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-4);
  }
  .setup {
    display: flex;
    gap: var(--space-5);
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .steps {
    display: grid;
    gap: var(--space-2);
    flex: 1;
    min-inline-size: 240px;
  }
  .secret {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    font-size: var(--text-sm);
    word-break: break-all;
  }
  .codes {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--warn-soft);
    font-size: var(--text-sm);
  }
  .code-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap: var(--space-1) var(--space-4);
    list-style: none;
  }
  .confirm {
    display: grid;
    gap: var(--space-2);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .actions.start {
    justify-content: flex-start;
  }
</style>
