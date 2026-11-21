<script lang="ts" module>
  // 已翻译的审计动作（与后端写入的动作名一致；未列出的动作原样显示）。i18n.test.ts 的 dynamic 中登记了同一列表。
  export const AUDIT_ACTIONS = [
    "account.password", "account.password_reset_by_admin", "account.questions", "admin.create", "admin.invite.create", "admin.invite.delete",
    "admin.node_mode", "admin.registration", "template.create", "template.delete", "admin.user.disable", "admin.user.enable", "admin.user.password", "admin.user.quota", "link.create", "link.delete",
    "link.reset", "login.failed", "login.ok", "mfa.disable", "mfa.enable", "mfa.failed", "mfa.recovery_code", "mfa.recovery_codes",
    "passkey.add", "passkey.delete", "passkey.login_failed", "profile.create", "profile.delete", "profile.update", "recovery.failed",
    "recovery.ok", "register", "session.revoke", "smtp.delete", "smtp.update", "smtp.verified", "channel.create", "channel.update", "channel.delete", "channel.verified", "profile.protect", "profile.unprotect", "profile.export", "account.backup", "account.restore", "account.restore_undo", "account.delete_request", "account.delete_cancel", "account.deleted", "admin.user.delete", "grant.create", "grant.revoke", "message.source_added", "message.template_added",
  ];
</script>

<script lang="ts">
  // 审计记录：scope="me" 为本人的记录；scope="all" 为管理员查看全部（附账号名）。
  // 按页码分页（反馈 #75）：上一页、下一页与跳到首页、末页，显示「第 n / m 页 · 共 k 条」。
  import { untrack } from "svelte";
  import Icon from "./Icon.svelte";
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
  const SIZE = 10; // 每页 10 条（需求 R6）
  let items = $state<Entry[]>([]);
  let pageN = $state(1);
  let total = $state(0);
  let more = $state(false); // 总数只数到上限（R14d）：显示为「10,000+」
  let loading = $state(false);
  const pages = $derived(Math.max(1, Math.ceil(total / SIZE)));

  const path = $derived(scope === "me" ? "/api/v1/account/audit" : "/api/v1/admin/audit");

  async function go(n: number) {
    loading = true;
    try {
      const r = await api.get<{ items: Entry[]; total: number; more?: boolean; page: number }>(path, { query: { pageNo: n, limit: SIZE } });
      items = r.items;
      total = r.total;
      more = !!r.more;
      pageN = r.page;
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      loading = false;
    }
  }
  // 只随 path 重新加载（go() 读写的状态放在 untrack 中，避免加载后再次触发本 effect）
  $effect(() => {
    void path;
    untrack(() => void go(1));
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
    <table class="table flush">
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
    {#if pages > 1}
      <nav class="pager" aria-label={t("audit.pager")}>
        <button class="btn sm ghost icon" type="button" disabled={loading || pageN <= 1} onclick={() => void go(1)} aria-label={t("audit.first")} title={t("audit.first")}><Icon name="chevron-up" size={14} /></button>
        <button class="btn sm" type="button" disabled={loading || pageN <= 1} onclick={() => void go(pageN - 1)}>{t("audit.prev")}</button>
        <span class="muted" aria-live="polite">{t("audit.page_of", { n: pageN, m: pages, total: i18n.number(total) + (more ? "+" : "") })}</span>
        <button class="btn sm" type="button" disabled={loading || pageN >= pages} onclick={() => void go(pageN + 1)}>{t("audit.next")}</button>
        <button class="btn sm ghost icon" type="button" disabled={loading || pageN >= pages} onclick={() => void go(pages)} aria-label={t("audit.last")} title={t("audit.last")}><Icon name="chevron-down" size={14} /></button>
      </nav>
    {/if}
  {/if}
</div>

<style>
  .audit {
    overflow-x: auto;
    /* 让表内绝对定位的无障碍标签以此为包含块，否则会逃出滚动容器、撑宽页面（反馈 #41） */
    position: relative;
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
  .pager {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    margin-block-start: var(--space-3);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }
</style>
