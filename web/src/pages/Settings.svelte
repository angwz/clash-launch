<script lang="ts">
  // 设置（反馈 #57）：只影响你自己的界面与使用习惯。
  //   设置 —— 个人偏好：语言、主题、编辑器行为、菜单顺序；
  //   账号与安全 —— 登录凭据与设备：密码、安全问题、两步验证、通行密钥、提醒、登录记录；
  //   管理 —— 整个站点（仅管理员）：用户、邀请与注册、默认节点写法、系统状态。
  // 语言与主题存在当前浏览器（登录前也要生效）；其余存在账号中，换设备一致。
  import PageHeader from "../components/PageHeader.svelte";
  import OrderDialog from "../components/OrderDialog.svelte";
  import TimezonePicker from "../components/TimezonePicker.svelte";
  import Icon from "../components/Icon.svelte";
  import { catalog } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { i18n, LOCALES, t } from "../lib/i18n/i18n.svelte";
  import { mainNav } from "../lib/nav";
  import { fieldClickMode, ordered, prefs, type FieldClickMode } from "../lib/prefs.svelte";
  import { session } from "../lib/session.svelte";
  import { theme, type Theme } from "../lib/theme.svelte";
  import { toasts } from "../lib/toast.svelte";

  let orderOpen = $state(false);
  const clickMode = $derived(fieldClickMode(prefs.data.fieldClick));
  const navItems = $derived(mainNav(session.isAdmin, prefs.data.nav).map((n) => ({ id: n.id, label: n.label })));
  const sectionItems = $derived(
    ordered((catalog.meta?.pack.nav ?? []).filter((s) => !s.generated), (s) => s.id, prefs.data.sections).map((s) => ({ id: s.id, label: catalog.navTitle(s) })),
  );

  $effect(() => {
    catalog.load().catch((e) => toasts.push("err", errorText(e)));
  });

  async function setLang(l: "en" | "zh-CN") {
    await i18n.set(l);
    if (catalog.meta) await catalog.loadMessages();
  }

  async function setAutoCheck(on: boolean) {
    try {
      // 开是默认值，不写入偏好
      await prefs.save({ ...prefs.data, autoCheck: on ? undefined : false });
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  // 时区（反馈 #72）：保存在账号中，换设备一致；界面时间与配置文件头时间都按它显示
  let now = $state(new Date().toISOString());
  $effect(() => {
    const id = setInterval(() => (now = new Date().toISOString()), 30_000);
    return () => clearInterval(id);
  });
  async function setTimezone(tz: string) {
    try {
      await session.setTimezone(tz);
      toasts.push("ok", t("settings.tz_saved"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function setClick(m: FieldClickMode) {
    try {
      // card 是默认值，不写入偏好
      await prefs.save({ ...prefs.data, fieldClick: m === "card" ? undefined : m });
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
</script>

<div class="page">
  <PageHeader title={t("nav.settings")} subtitle={t("settings.subtitle")} />

  <section class="panel box" aria-labelledby="st-look">
    <h2 id="st-look">{t("settings.look")}</h2>
    <div class="grid two">
      <div>
        <label class="label" for="st-lang">{t("shell.language")}</label>
        <select id="st-lang" class="select" value={i18n.locale} onchange={(e) => void setLang(e.currentTarget.value as "en" | "zh-CN")}>
          {#each LOCALES as l (l.id)}<option value={l.id}>{l.label}</option>{/each}
        </select>
      </div>
      <div>
        <label class="label" for="st-theme">{t("account.theme")}</label>
        <select id="st-theme" class="select" value={theme.value} onchange={(e) => theme.apply(e.currentTarget.value as Theme)}>
          <option value="system">{t("theme.system")}</option>
          <option value="light">{t("theme.light")}</option>
          <option value="dark">{t("theme.dark")}</option>
        </select>
      </div>
    </div>
    <p class="hint">{t("account.local_hint")}</p>
  </section>

  <section class="panel box" aria-labelledby="st-time">
    <h2 id="st-time">{t("settings.time")}</h2>
    <div class="grid two">
      <div>
        <label class="label" for="st-tz">{t("settings.tz")}</label>
        <TimezonePicker id="st-tz" value={session.user?.timezone ?? ""} onchange={(z) => void setTimezone(z)} />
      </div>
      <div class="tz-now">
        <span class="label">{t("settings.tz_now")}</span>
        <strong>{i18n.date(now, "long")}</strong>
      </div>
    </div>
    <p class="hint">{t("settings.tz_hint")}</p>
  </section>

  <section class="panel box" aria-labelledby="st-editor">
    <h2 id="st-editor">{t("settings.editor")}</h2>
    <fieldset class="choices">
      <legend class="label">{t("settings.field_click")}</legend>
      {#each ["card", "text", "button"] as const as m (m)}
        <label class="choice">
          <input type="radio" name="st-click" value={m} checked={clickMode === m} onchange={() => void setClick(m)} />
          <span>
            <strong>{t("settings.field_click." + m)}</strong>
            <span class="muted">{t("settings.field_click_hint." + m)}</span>
          </span>
        </label>
      {/each}
    </fieldset>
    <label class="choice">
      <input id="st-autocheck" type="checkbox" checked={prefs.data.autoCheck !== false} onchange={(e) => void setAutoCheck(e.currentTarget.checked)} />
      <span>
        <strong>{t("settings.auto_check")}</strong>
        <span class="muted">{t("settings.auto_check_hint")}</span>
      </span>
    </label>
  </section>

  <section class="panel box" aria-labelledby="st-menu">
    <div class="row">
      <div>
        <h2 id="st-menu">{t("order.title")}</h2>
        <p class="muted">{t("order.desc")}</p>
      </div>
      <button class="btn" type="button" onclick={() => (orderOpen = true)}><Icon name="rules" size={15} />{t("settings.order_edit")}</button>
    </div>
  </section>
</div>

<OrderDialog bind:open={orderOpen} nav={navItems} sections={sectionItems} />

<style>
  .page > :global(*) {
    max-inline-size: 760px;
  }
  /* 与其他页面同一左缘（1120 居中），内容本身较窄（760）（反馈 #56） */
  .page {
    max-inline-size: 1120px;
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
  .grid {
    display: grid;
    gap: var(--space-3);
  }
  .grid.two {
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  }
  .tz-now {
    display: grid;
    align-content: start;
  }
  .tz-now strong {
    padding-block: 7px;
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: center;
    gap: var(--space-4);
  }
  .choices {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    border: 0;
  }
  .choice {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    cursor: pointer;
  }
  .choice input {
    margin-block-start: 4px;
  }
  .choice > span {
    display: grid;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
