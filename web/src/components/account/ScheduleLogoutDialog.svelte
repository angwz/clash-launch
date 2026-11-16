<script lang="ts">
  // 定时退出（需求 R8）：为一台设备（或除当前设备外的全部设备）选一个退出时间。
  // 只能早于设备自然失效的时间：晚于它的选项不可选，并注明自然失效的时间；闲置失效会随使用后延，只作提示。
  import Dialog from "../Dialog.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { session } from "../../lib/session.svelte";
  import { toasts } from "../../lib/toast.svelte";

  let {
    open = $bindable(false),
    target,
    expiresAt,
    onDone,
  }: {
    open?: boolean;
    /** 设备 ID，或 "others" 表示除当前设备外的全部设备 */
    target: { id: string; device: string; current?: boolean } | null;
    /** 自然失效的时间（全部其他设备时取最晚的那台：早于它的选项至少对一台设备有意义） */
    expiresAt: string;
    onDone: () => void;
  } = $props();

  const HOUR = 3600_000;
  const PRESETS = [
    { key: "1h", ms: HOUR },
    { key: "1d", ms: 24 * HOUR },
    { key: "3d", ms: 72 * HOUR },
    { key: "7d", ms: 168 * HOUR },
  ] as const;

  let choice = $state<string>("1h");
  let custom = $state("");
  let busy = $state(false);
  let error = $state("");
  let now = $state(Date.now());

  $effect(() => {
    if (!open) return;
    now = Date.now();
    error = "";
    custom = "";
    choice = PRESETS.find((p) => now + p.ms < limit)?.key ?? "custom";
  });

  const limit = $derived(new Date(expiresAt).getTime());

  // datetime-local 的值是本地时间、不带时区：按浏览器时区解释
  const toLocalInput = (ms: number) => {
    if (!Number.isFinite(ms)) return undefined; // 对话框未打开时没有到期时间
    const d = new Date(ms - new Date(ms).getTimezoneOffset() * 60_000);
    return d.toISOString().slice(0, 16);
  };

  const chosenAt = $derived.by(() => {
    if (choice === "custom") return custom ? new Date(custom).getTime() : NaN;
    return now + (PRESETS.find((p) => p.key === choice)?.ms ?? 0);
  });
  const valid = $derived(Number.isFinite(chosenAt) && chosenAt > now + 60_000 && chosenAt < limit);

  async function confirm() {
    if (!target || !valid || busy) return;
    busy = true;
    error = "";
    try {
      const at = new Date(chosenAt).toISOString();
      await api.put(`/api/v1/account/sessions/${target.id}/logout-at`, { at });
      if (target.current) session.logoutAt = at;
      open = false;
      toasts.push("ok", t("account.logout_scheduled", { when: i18n.date(at, "long") }));
      onDone();
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog bind:open title={target?.id === "others" ? t("account.logout_others_title") : t("account.logout_title", { device: target?.device ?? "" })}>
  <div class="body">
    <fieldset>
      <legend class="label">{t("account.logout_when")}</legend>
      {#each PRESETS as p (p.key)}
        <label class="opt" class:off={now + p.ms >= limit}>
          <input type="radio" name="logout-at" value={p.key} bind:group={choice} disabled={now + p.ms >= limit} />
          <span>{t("account.logout_in_" + p.key)}</span>
          <span class="muted">{i18n.date(new Date(now + p.ms).toISOString())}</span>
        </label>
      {/each}
      <label class="opt">
        <input type="radio" name="logout-at" value="custom" bind:group={choice} />
        <span>{t("account.logout_custom")}</span>
        <input class="input" type="datetime-local" bind:value={custom} min={toLocalInput(now + 60_000)} max={toLocalInput(limit - 60_000)} onfocus={() => (choice = "custom")} aria-label={t("account.logout_custom")} />
      </label>
    </fieldset>
    <p class="note muted">{target?.id === "others" ? t("account.logout_limit_others", { when: expiresAt ? i18n.date(expiresAt, "long") : "" }) : t("account.logout_limit", { when: expiresAt ? i18n.date(expiresAt, "long") : "" })}</p>
    {#if choice === "custom" && custom && !valid}<p class="field-error" role="alert">{t("account.logout_out_of_range")}</p>{/if}
    {#if error}<p class="field-error" role="alert">{error}</p>{/if}
  </div>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" disabled={!valid || busy} onclick={() => void confirm()}>{t("account.logout_confirm")}</button>
  {/snippet}
</Dialog>

<style>
  .body {
    display: grid;
    gap: var(--space-3);
  }
  fieldset {
    display: grid;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .opt {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-block-size: 32px;
    font-size: var(--text-sm);
  }
  .opt.off {
    opacity: 0.5;
  }
  .opt .muted {
    margin-inline-start: auto;
    font-size: var(--text-xs);
  }
  .opt .input {
    margin-inline-start: auto;
    inline-size: auto;
  }
  .note {
    font-size: var(--text-xs);
  }
</style>
