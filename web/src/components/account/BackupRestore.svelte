<script lang="ts">
  // 个人备份与还原（需求 R21）：下载加密的备份文件；从备份还原（先比较数量，确认后整体覆盖）；
  // 7 天内撤销最近一次还原。都需要当前密码。备份口令只用于加密文件，服务器不保存，忘记就无法还原。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Summary {
    sources: number;
    ruleSources: number;
    profiles: number;
    revisions: number;
    templates: number;
    links: number;
    shortLinks: number;
    channels: number;
    smtp: boolean;
    createdAt?: string;
    server?: string;
  }
  interface Report {
    newTokens: string[];
    newCodes: string[];
    revoked: number;
    undoUntil?: string;
  }
  const ROWS = ["sources", "ruleSources", "profiles", "revisions", "templates", "links", "shortLinks", "channels"] as const;

  // 备份
  let bPassword = $state("");
  let bPass = $state("");
  let bPass2 = $state("");
  let bBusy = $state(false);

  async function backup(e: SubmitEvent) {
    e.preventDefault();
    bBusy = true;
    try {
      const text = await api.post<string>("/api/v1/account/backup", { password: bPassword, passphrase: bPass });
      const url = URL.createObjectURL(new Blob([text], { type: "application/octet-stream" }));
      const a = document.createElement("a");
      a.href = url;
      a.download = "clash-launch-" + new Date().toISOString().slice(0, 10).replaceAll("-", "") + ".backup";
      a.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      bPassword = bPass = bPass2 = "";
      toasts.push("ok", t("backup.done"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      bBusy = false;
    }
  }

  // 还原
  let file = $state("");
  let fileName = $state("");
  let rPass = $state("");
  let rPassword = $state("");
  let confirmed = $state(false);
  let preview = $state<{ current: Summary; backup: Summary } | null>(null);
  let report = $state<Report | null>(null);
  let rBusy = $state(false);

  async function pick(e: Event) {
    const f = (e.currentTarget as HTMLInputElement).files?.[0];
    preview = null;
    report = null;
    if (!f) return;
    fileName = f.name;
    file = await f.text();
  }

  async function check() {
    rBusy = true;
    try {
      preview = await api.post("/api/v1/account/restore/preview", { file, passphrase: rPass });
      confirmed = false;
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      rBusy = false;
    }
  }

  async function restore() {
    rBusy = true;
    try {
      report = await api.post<Report>("/api/v1/account/restore", { file, passphrase: rPass, password: rPassword });
      preview = null;
      file = fileName = rPass = rPassword = "";
      toasts.push("ok", t("restore.done"));
      await loadUndo();
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      rBusy = false;
    }
  }

  // 撤销
  let undo = $state<{ available: boolean; expiresAt?: string } | null>(null);
  let uPassword = $state("");
  async function loadUndo() {
    undo = await api.get("/api/v1/account/restore/undo");
  }
  $effect(() => {
    loadUndo().catch(() => undefined);
  });
  async function doUndo() {
    rBusy = true;
    try {
      await api.post("/api/v1/account/restore/undo", { password: uPassword });
      uPassword = "";
      report = null;
      toasts.push("ok", t("restore.undone"));
      await loadUndo();
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      rBusy = false;
    }
  }
</script>

<section class="panel box" aria-labelledby="ac-bk">
  <div>
    <h2 id="ac-bk">{t("backup.section")}</h2>
    <p class="muted">{t("backup.desc")}</p>
  </div>

  <form class="grid" onsubmit={backup} aria-label={t("backup.download")}>
    <h3>{t("backup.download")}</h3>
    <div class="three">
      <div>
        <label class="label" for="bk-pw">{t("backup.password")}</label>
        <input id="bk-pw" class="input" type="password" autocomplete="current-password" bind:value={bPassword} />
      </div>
      <div>
        <label class="label" for="bk-pass">{t("backup.passphrase")}</label>
        <input id="bk-pass" class="input" type="password" autocomplete="new-password" bind:value={bPass} />
      </div>
      <div>
        <label class="label" for="bk-pass2">{t("backup.passphrase_again")}</label>
        <input id="bk-pass2" class="input" type="password" autocomplete="new-password" bind:value={bPass2} />
      </div>
    </div>
    <p class="hint">{t("backup.passphrase_hint")}</p>
    {#if bPass2 && bPass !== bPass2}<p class="field-error">{t("backup.mismatch")}</p>{/if}
    <div class="actions">
      <button class="btn primary" type="submit" disabled={bBusy || !bPassword || bPass.length < 10 || bPass !== bPass2}>{t("backup.download")}</button>
    </div>
  </form>

  <div class="grid">
    <h3>{t("restore.title")}</h3>
    <p class="hint">{t("restore.desc")}</p>
    <div class="two">
      <div>
        <label class="label" for="rs-file">{t("restore.file")}</label>
        <input id="rs-file" class="input" type="file" accept=".backup,application/octet-stream,application/json" onchange={(e) => void pick(e)} />
      </div>
      <div>
        <label class="label" for="rs-pass">{t("backup.passphrase")}</label>
        <input id="rs-pass" class="input" type="password" autocomplete="off" bind:value={rPass} />
      </div>
    </div>
    {#if !preview}
      <div class="actions">
        <button class="btn" type="button" disabled={rBusy || !file || !rPass} onclick={() => void check()}>{t("restore.check")}</button>
      </div>
    {:else}
      <table class="cmp">
        <thead><tr><th></th><th>{t("restore.now")}</th><th>{t("restore.in_backup")}</th></tr></thead>
        <tbody>
          {#each ROWS as k (k)}
            <tr><th scope="row">{t("restore.row_" + k)}</th><td>{preview.current[k]}</td><td>{preview.backup[k]}</td></tr>
          {/each}
          <tr><th scope="row">{t("restore.row_smtp")}</th><td>{preview.current.smtp ? "✓" : "—"}</td><td>{preview.backup.smtp ? "✓" : "—"}</td></tr>
        </tbody>
      </table>
      {#if preview.backup.createdAt}<p class="hint">{t("restore.made_at", { when: i18n.date(preview.backup.createdAt), server: preview.backup.server || "—" })}</p>{/if}
      <label class="opt"><input type="checkbox" bind:checked={confirmed} />{t("restore.confirm_text")}</label>
      <div class="two">
        <div>
          <label class="label" for="rs-pw">{t("backup.password")}</label>
          <input id="rs-pw" class="input" type="password" autocomplete="current-password" bind:value={rPassword} />
        </div>
      </div>
      <div class="actions">
        <button class="btn ghost" type="button" onclick={() => (preview = null)}>{t("common.cancel")}</button>
        <button class="btn danger" type="button" disabled={rBusy || !confirmed || !rPassword} onclick={() => void restore()}>{t("restore.run")}</button>
      </div>
    {/if}
    {#if report}
      <div class="report" role="status">
        <p><strong>{t("restore.done")}</strong></p>
        {#if report.newTokens.length}<p class="warn">{t("restore.new_tokens", { list: report.newTokens.join("、") })}</p>{/if}
        {#if report.newCodes.length}<p class="warn">{t("restore.new_codes", { list: report.newCodes.join("、") })}</p>{/if}
        {#if report.revoked}<p>{t("restore.revoked", { n: report.revoked })}</p>{/if}
      </div>
    {/if}
  </div>

  {#if undo?.available}
    <div class="grid undo">
      <h3>{t("restore.undo_title")}</h3>
      <p class="hint">{t("restore.undo_desc", { when: i18n.date(undo.expiresAt ?? "") })}</p>
      <div class="two">
        <div>
          <label class="label" for="un-pw">{t("backup.password")}</label>
          <input id="un-pw" class="input" type="password" autocomplete="current-password" bind:value={uPassword} />
        </div>
      </div>
      <div class="actions">
        <button class="btn" type="button" disabled={rBusy || !uPassword} onclick={() => void doUndo()}>{t("restore.undo")}</button>
      </div>
    </div>
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-5);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  h3 {
    font-size: var(--text-md);
  }
  .grid {
    display: grid;
    gap: var(--space-3);
  }
  .two,
  .three {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-3);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .cmp {
    inline-size: 100%;
    max-inline-size: 420px;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  .cmp th,
  .cmp td {
    padding-block: 4px;
    padding-inline: var(--space-2);
    border-block-end: 1px solid var(--border);
    text-align: start;
    font-variant-numeric: tabular-nums;
  }
  .cmp thead th {
    color: var(--fg-muted);
    font-weight: 500;
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .opt input {
    accent-color: var(--accent);
  }
  .report {
    display: grid;
    gap: var(--space-1);
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    font-size: var(--text-sm);
  }
  .warn {
    color: var(--warn);
  }
  .undo {
    padding-block-start: var(--space-4);
    border-block-start: 1px solid var(--border);
  }
</style>
