<script lang="ts">
  // 两步验证策略（需求 R41）：管理员要求而本账号尚未启用时，App 用它代替整个界面。
  // 服务端在启用前只放行设置 TOTP 所需的接口（403 mfa.setup_required）。
  // 设置的恢复码只显示一次：启用后先让用户保存恢复码，点「继续」才进入网站。
  import AuthLayout from "./AuthLayout.svelte";
  import TwoFactor from "./account/TwoFactor.svelte";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";

  let done = $state(false);

  async function signOut() {
    await session.logout();
    router.go("/login", true);
  }
</script>

<AuthLayout wide title={t("mfa_gate.title")} subtitle={t("mfa_gate.hint")}>
  <div class="setup"><TwoFactor onenabled={() => (done = true)} /></div>
  <div class="actions">
    <button class="btn ghost" type="button" onclick={() => void signOut()}>{t("mfa_gate.sign_out")}</button>
    <button class="btn primary" type="button" disabled={!done} onclick={() => void session.load()}>{t("mfa_gate.continue")}</button>
  </div>
</AuthLayout>

<style>
  .setup {
    margin-block: var(--space-4);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
