<script lang="ts">
  // 注销账号（需求 R20）：需要当前密码，启用了两步验证时还要验证码。发起后进入 7 天冷静期，
  // 所有设备立即退出；冷静期内重新登录即撤销。账号页与用户协议页（不同意新协议时）共用。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { router } from "../../lib/router.svelte";
  import { session } from "../../lib/session.svelte";
  import { toasts } from "../../lib/toast.svelte";

  let { compact = false }: { compact?: boolean } = $props();

  let mfa = $state(false);
  let password = $state("");
  let code = $state("");
  let sure = $state(false);
  let busy = $state(false);

  $effect(() => {
    api
      .get<{ enabled: boolean }>("/api/v1/account/totp")
      .then((r) => (mfa = r.enabled))
      .catch(() => undefined);
  });

  async function run(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      const r = await api.post<{ at: string }>("/api/v1/account/delete", { password, code });
      // 服务器已注销全部会话；这里只清本地状态
      await session.logout().catch(() => undefined);
      toasts.push("info", t("deletion.requested", { when: i18n.date(r.at) }));
      router.go("/login", true);
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }
</script>

<form class="grid" onsubmit={run} aria-label={t("deletion.title")}>
  {#if !compact}<p class="muted">{t("deletion.desc")}</p>{/if}
  <p class="hint">{t("deletion.backup_first")}</p>
  <div class="two">
    <div>
      <label class="label" for="del-pw">{t("backup.password")}</label>
      <input id="del-pw" class="input" type="password" autocomplete="current-password" bind:value={password} />
    </div>
    {#if mfa}
      <div>
        <label class="label" for="del-code">{t("deletion.code")}</label>
        <input id="del-code" class="input mono" inputmode="numeric" autocomplete="one-time-code" bind:value={code} />
      </div>
    {/if}
  </div>
  <label class="opt"><input type="checkbox" bind:checked={sure} />{t("deletion.confirm_text")}</label>
  <div class="actions">
    <button class="btn danger" type="submit" disabled={busy || !sure || !password || (mfa && !code)}>{t("deletion.run")}</button>
  </div>
</form>

<style>
  .grid {
    display: grid;
    gap: var(--space-3);
  }
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-3);
  }
  .opt {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .opt input {
    accent-color: var(--err);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
