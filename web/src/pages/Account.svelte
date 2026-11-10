<script lang="ts">
  // 账号：当前用户、界面偏好。安全问题、通行密钥、两步验证、登录设备与个人审计记录在 M5 加入。
  import PageHeader from "../components/PageHeader.svelte";
  import { catalog } from "../lib/catalog.svelte";
  import { i18n, LOCALES, t } from "../lib/i18n/i18n.svelte";
  import { session } from "../lib/session.svelte";
  import { theme, type Theme } from "../lib/theme.svelte";

  async function setLang(l: "en" | "zh-CN") {
    await i18n.set(l);
    if (catalog.meta) await catalog.loadMessages();
  }
</script>

<div class="page">
  <PageHeader title={t("nav.account")} />
  <section class="panel box">
    <dl>
      <dt>{t("field.username")}</dt>
      <dd>{session.user?.username}</dd>
      <dt>{t("account.role")}</dt>
      <dd>{t("role." + (session.user?.role ?? "user"))}</dd>
    </dl>
  </section>
  <section class="panel box">
    <h2>{t("account.preferences")}</h2>
    <div class="grid">
      <div>
        <label class="label" for="ac-lang">{t("shell.language")}</label>
        <select id="ac-lang" class="select" value={i18n.locale} onchange={(e) => void setLang(e.currentTarget.value as "en" | "zh-CN")}>
          {#each LOCALES as l (l.id)}<option value={l.id}>{l.label}</option>{/each}
        </select>
      </div>
      <div>
        <label class="label" for="ac-theme">{t("account.theme")}</label>
        <select id="ac-theme" class="select" value={theme.value} onchange={(e) => theme.apply(e.currentTarget.value as Theme)}>
          <option value="system">{t("theme.system")}</option>
          <option value="light">{t("theme.light")}</option>
          <option value="dark">{t("theme.dark")}</option>
        </select>
      </div>
    </div>
    <p class="hint">{t("account.local_hint")}</p>
  </section>
</div>

<style>
  .page {
    max-inline-size: 720px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  dl {
    display: grid;
    grid-template-columns: 8rem 1fr;
    gap: var(--space-2) var(--space-4);
  }
  dt {
    color: var(--fg-muted);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--space-4);
  }
</style>
