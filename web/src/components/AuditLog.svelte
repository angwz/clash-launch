<script lang="ts" module>
  // 已翻译的审计动作（与后端写入的动作名一致；未列出的动作原样显示）。i18n.test.ts 的 dynamic 中登记了同一列表。
  export const AUDIT_ACTIONS = [
    "account.password", "account.password_reset_by_admin", "account.questions", "admin.create", "admin.invite.create", "admin.invite.delete",
    "admin.registration", "admin.user.disable", "admin.user.enable", "admin.user.password", "admin.user.quota", "link.create", "link.delete",
    "link.reset", "login.failed", "login.ok", "mfa.disable", "mfa.enable", "mfa.failed", "mfa.recovery_code", "mfa.recovery_codes",
    "passkey.add", "passkey.delete", "passkey.login_failed", "profile.create", "profile.delete", "profile.update", "recovery.failed",
    "recovery.ok", "register", "session.revoke", "smtp.delete", "smtp.update", "smtp.verified",
  ];
</script>

<script lang="ts">
  // 审计记录：scope="me" 为本人的记录；scope="all" 为管理员查看全部（附账号名）。按页加载。
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { toasts } from "../lib/toast.svelte";

  interface Entry {
    id: string;
    at: string;
    action: string;
    detail: Record<string, string>;
    user?: string;
  }

  let { scope }: { scope: "me" | "all" } = $props();
  let items = $state<Entry[]>([]);
  let next = $state<string | undefined>();
  let loading = $state(false);

  const path = $derived(scope === "me" ? "/api/v1/account/audit" : "/api/v1/admin/audit");

  async function more() {
    loading = true;
    try {
      const r = await api.get<{ items: Entry[]; next?: string }>(path, { query: { limit: 30, cursor: next } });
      items = [...items, ...r.items];
      next = r.items.length === 30 ? r.next : undefined;
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      loading = false;
    }
  }
  $effect(() => {
    void path;
    items = [];
    next = undefined;
    void more();
  });

  const label = (a: string) => (AUDIT_ACTIONS.includes(a) ? t("audit.action." + a) : a);
  const tone = (a: string) => (a.endsWith("failed") ? "err" : a.startsWith("admin.") ? "admin" : "");
  const detailText = (d: Record<string, string>) =>
    Object.entries(d)
      .map(([k, v]) => k + "=" + v)
      .join(" · ");
</script>

<div class="audit">
  {#if items.length === 0 && !loading}
    <p class="muted empty">{t("audit.empty")}</p>
  {:else}
    <table class="table">
      <thead>
        <tr>
          <th>{t("audit.time")}</th>
          {#if scope === "all"}<th>{t("audit.user")}</th>{/if}
          <th>{t("audit.action")}</th>
          <th>{t("audit.detail")}</th>
        </tr>
      </thead>
      <tbody>
        {#each items as e (e.id)}
          <tr>
            <td class="when" title={i18n.date(e.at, "long")}>{i18n.date(e.at)}</td>
            {#if scope === "all"}<td>{e.user || "—"}</td>{/if}
            <td><span class="act" data-tone={tone(e.action)}>{label(e.action)}</span></td>
            <td class="mono ltr detail">{detailText(e.detail)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if next}
      <button class="btn sm ghost more" type="button" disabled={loading} onclick={() => void more()}>{t("audit.more")}</button>
    {/if}
  {/if}
</div>

<style>
  .audit {
    overflow-x: auto;
  }
  .when {
    white-space: nowrap;
    color: var(--fg-muted);
  }
  .act[data-tone="err"] {
    color: var(--err);
  }
  .act[data-tone="admin"] {
    color: var(--accent-fg);
  }
  .detail {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    word-break: break-all;
  }
  .empty {
    padding: var(--space-4);
  }
  .more {
    margin-block-start: var(--space-2);
  }
</style>
