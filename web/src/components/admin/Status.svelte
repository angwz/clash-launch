<script lang="ts">
  // 系统状态：版本、内核、目录、主密钥、数据库、可选功能是否可用。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Status {
    version: string;
    commit: string;
    built: string;
    kernel: { available: boolean; version: string };
    catalog: { pack: string; coreMax: string; digest: string };
    keys: { current: string; count: number };
    users: number;
    database: string;
    publicURL: string;
    features: { passkeys: boolean; smtp: boolean };
    auditDays: number;
  }
  let st = $state<Status | null>(null);
  $effect(() => {
    api
      .get<Status>("/api/v1/admin/status")
      .then((x) => (st = x))
      .catch((e) => toasts.push("err", errorText(e)));
  });
  const yes = (b: boolean) => (b ? t("admin.yes") : t("admin.no"));
</script>

{#if st}
  <dl class="status">
    <dt>{t("admin.st_version")}</dt>
    <dd>
      <span class="mono">{st.version}</span>
      {#if st.commit}· <a class="mono" href={"https://github.com/angwz/clash-launch/commit/" + st.commit} target="_blank" rel="noopener noreferrer">{st.commit.slice(0, 7)}</a>{/if}
      {#if st.built}· {t("admin.st_built", { d: new Date(st.built).toLocaleString() })}{/if}
      <span class="hint ver-hint">{t("admin.st_version_hint")}</span>
    </dd>
    <dt>{t("admin.st_kernel")}</dt>
    <dd>{st.kernel.available ? "mihomo " + st.kernel.version : t("admin.st_kernel_off")}</dd>
    <dt>{t("admin.st_catalog")}</dt>
    <dd class="mono">{st.catalog.pack} ≤ {st.catalog.coreMax} · {st.catalog.digest}</dd>
    <dt>{t("admin.st_keys")}</dt>
    <dd><span class="mono">{st.keys.current}</span> · {t("admin.st_key_count", { n: st.keys.count })}</dd>
    <dt>{t("admin.st_db")}</dt>
    <dd class="mono">{st.database}</dd>
    <dt>{t("admin.st_users")}</dt>
    <dd>{st.users}</dd>
    <dt>{t("admin.st_public")}</dt>
    <dd class="mono ltr">{st.publicURL || "—"}</dd>
    <dt>{t("admin.st_features")}</dt>
    <dd>{t("admin.st_feature", { name: t("passkey.section"), value: yes(st.features.passkeys) })} · {t("admin.st_feature", { name: t("alerts.section"), value: yes(st.features.smtp) })}</dd>
    <dt>{t("admin.st_audit")}</dt>
    <dd>{st.auditDays ? t("admin.st_audit_days", { n: st.auditDays }) : t("admin.st_audit_forever")}</dd>
  </dl>
  <p class="hint">{t("admin.st_backup")}</p>
{/if}

<style>
  .status {
    display: grid;
    grid-template-columns: 9rem 1fr;
    gap: var(--space-2) var(--space-4);
    font-size: var(--text-sm);
  }
  dt {
    color: var(--fg-muted);
  }
  .ver-hint {
    display: block;
    font-size: var(--text-xs);
  }
  dd {
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }
</style>
