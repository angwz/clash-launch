<script lang="ts">
  import NoticePanel from "./NoticePanel.svelte";
  import { notices } from "../lib/notices.svelte";
  import type { Snippet } from "svelte";
  import { onMount } from "svelte";
  import { i18n, LOCALES, t } from "../lib/i18n/i18n.svelte";
  import { catalog } from "../lib/catalog.svelte";
  import { palette } from "../lib/palette.svelte";
  import { router } from "../lib/router.svelte";
  import { session } from "../lib/session.svelte";
  import { shell } from "../lib/shell.svelte";
  import { theme } from "../lib/theme.svelte";
  import Icon, { type IconName } from "./Icon.svelte";
  import Menu from "./Menu.svelte";
  import Emoji, { type EmojiName } from "./Emoji.svelte";
  import { prefs } from "../lib/prefs.svelte";
  import { mainNav } from "../lib/nav";
  import { inbox } from "../lib/inbox.svelte";
  import { chat } from "../lib/chat.svelte";
  // 消息悬浮窗按需加载：第一次打开时才下载，不增加首屏体积
  const loadChat = () => import("./chat/ChatPanel.svelte");
  import { stats } from "../lib/stats.svelte";
  import { errorText } from "../lib/errors";
  import { toasts } from "../lib/toast.svelte";
  import { realtime } from "../lib/realtime.svelte";
  import { api } from "../lib/api/client";
  import { footprintText, type Footprint } from "../lib/footprints";
  import Pulse from "./Pulse.svelte";

  let { children }: { children: Snippet } = $props();

  onMount(() => {
    shell.init();
    stats.start();
    inbox.start(); // 未读消息角标（反馈 #74）
    notices.start(); // 站内提醒（R23、R24）
    realtime.start(); // 实时通道（R22）
    // 新足迹（R10）：取回完整的一条（事件只带类别与区分），弹出一次轻提示；指示灯的庆祝效果在 Pulse 中
    const offFootprint = realtime.on(["footprint"], (kind, e) => {
      api.get<{ items: Footprint[] }>("/api/v1/account/footprints").then(
        (r) => {
          const f = r.items.find((x) => x.kind === kind && (x.subject ?? "") === (e?.state ?? ""));
          if (f) toasts.push("ok", t("footprint.toast", { text: footprintText(f) }), { emoji: "party" });
        },
        () => undefined, // 只是提示：取不到时时间线里仍然能看到
      );
    });
    prefs.load().catch((e) => toasts.push("err", errorText(e)));
    return offFootprint;
  });
  // 关闭动效（R11）：根元素加 data-motion="off"，base.css 据此压掉所有动画与过渡
  $effect(() => {
    if (prefs.data.motion === "off") document.documentElement.dataset.motion = "off";
    else delete document.documentElement.dataset.motion;
  });

  let noticeOpen = $state(false);
  const nav = $derived(mainNav(session.isAdmin, prefs.data.nav).map((n) => ({ ...n, active: n.matches(router.route.name) })));

  const themeIcon = $derived<IconName>(theme.value === "light" ? "sun" : theme.value === "dark" ? "moon" : "monitor");

  async function setLang(l: "en" | "zh-CN") {
    await i18n.set(l);
    if (catalog.meta) await catalog.loadMessages();
  }

  async function logout() {
    inbox.stop();
    notices.stop();
    realtime.stop();
    chat.close();
    await session.logout();
    router.go("/login");
  }

  // 页面浏览统计（反馈 #66）：只报页面类型
  $effect(() => {
    stats.track("pv", router.route.name);
  });

  $effect(() => {
    // 换页时收起手机抽屉
    void router.path;
    shell.drawer = false;
  });

  const p = $derived(shell.profile);
  // 检查结果用 emoji 表达（反馈 #44）；空闲与进行中仍用线性图标
  const checkEmoji = $derived<EmojiName | null>(
    !p ? null : p.check === "passed" ? "check" : p.check === "failed" || p.check === "error" ? "no-entry" : p.check === "unavailable" ? "warning" : null,
  );
  const checkIcon = $derived<IconName>(
    !p ? "info" : p.check === "passed" ? "ok" : p.check === "failed" || p.check === "error" ? "error" : p.check === "unavailable" ? "warning" : "info",
  );
</script>

<div class="shell" class:rail={shell.rail}>
  <a class="skip" href="#main">{t("shell.skip")}</a>
  <header class="topbar">
    <button class="btn ghost icon only-narrow" type="button" aria-label={t("shell.menu")} aria-expanded={shell.drawer} onclick={() => (shell.drawer = !shell.drawer)}>
      <Icon name="menu" />
    </button>
    <a class="brand" href="/" aria-label="Clash, Launch!">
      <span class="mark"><Icon name="logo" size={18} /></span>
      <span class="word">Clash, Launch!</span>
    </a>
    <!-- 在线指示灯（R11）：手机上侧栏是抽屉，灯放到顶栏 -->
    <span class="only-narrow pulse-slot"><Pulse id="pulse-top" /></span>

    {#if p}
      <div class="status">
        <button
          class="pill"
          type="button"
          data-state={p.check}
          onclick={() => p.runCheck()}
          disabled={p.check === "running"}
          title={t("check.run")}
        >
          {#if checkEmoji}<Emoji name={checkEmoji} size={14} />{:else}<Icon name={checkIcon} size={14} />{/if}
          <span>{t("check." + p.check)}</span>
        </button>
        {#if p.diagnostics.errors || p.diagnostics.warnings}
          <span class="counts">
            {#if p.diagnostics.errors}<span class="c-err"><Icon name="error" size={13} />{p.diagnostics.errors}</span>{/if}
            {#if p.diagnostics.warnings}<span class="c-warn"><Icon name="warning" size={13} />{p.diagnostics.warnings}</span>{/if}
          </span>
        {/if}
        {#if p.raw}
          <button class="raw-flag" type="button" onclick={() => p.openRaw()} title={t("raw.indicator")} aria-label={t("raw.indicator")}>
            <Icon name="bolt" size={14} />
          </button>
        {/if}
      </div>
    {/if}

    <div class="spacer"></div>

    <button class="search" type="button" onclick={() => palette.show()} aria-keyshortcuts="Control+K Meta+K">
      <Icon name="search" size={15} />
      <span class="search-label">{t("palette.open")}</span>
      <span class="kbd">Ctrl K</span>
    </button>

    <!-- 消息（反馈 #75）：右上角的图标，未读时带角标 -->
    <!-- 点击打开或收起消息悬浮窗（反馈 #79），不离开当前页面 -->
    <button class="btn ghost icon msg-btn" type="button" class:active={chat.open} aria-expanded={chat.open} onclick={() => chat.toggle()} aria-label={inbox.unread ? t("messages.unread_n", { n: inbox.unread }) : t("nav.messages")} title={t("nav.messages")}>
      <Icon name="message" />
      {#if inbox.unread}<span class="dot">{inbox.unread > 99 ? "99+" : inbox.unread}</span>{/if}
    </button>

    <!-- 站内提醒（R23、R24）：节点源失败、配置检查不通过、订阅链接出现新来源 -->
    <button class="btn ghost icon notice-btn" type="button" class:active={noticeOpen} aria-expanded={noticeOpen} onclick={() => (noticeOpen = !noticeOpen)} aria-label={notices.unread ? t("notices.unread_n", { n: notices.unread }) : t("notices.title")} title={t("notices.title")}>
      <Icon name="bell" />
      {#if notices.unread}<span class="dot">{notices.unread > 99 ? "99+" : notices.unread}</span>{/if}
    </button>
    <NoticePanel bind:open={noticeOpen} />

    <Menu label={t("shell.language")} icon="globe">
      {#snippet items(close)}
        {#each LOCALES as l (l.id)}
          <button type="button" role="menuitemradio" aria-checked={i18n.locale === l.id} onclick={() => { void setLang(l.id); close(); }}>
            <span class="check">{#if i18n.locale === l.id}<Icon name="check" size={14} />{/if}</span>{l.label}
          </button>
        {/each}
      {/snippet}
    </Menu>

    <button class="btn ghost icon" type="button" onclick={() => theme.cycle()} aria-label={t("shell.theme", { theme: t("theme." + theme.value) })} title={t("shell.theme", { theme: t("theme." + theme.value) })}>
      <Icon name={themeIcon} />
    </button>

    <Menu label={session.user?.username ?? ""} icon="user" align="end">
      {#snippet items(close)}
        <div class="menu-head">
          <strong>{session.user?.username}</strong>
          <span class="muted">{t("role." + (session.user?.role ?? "user"))}</span>
        </div>
        <a href="/account" role="menuitem" onclick={close}><Icon name="shield" size={15} />{t("nav.account")}</a>
        <a href="/settings" role="menuitem" onclick={close}><Icon name="settings" size={15} />{t("nav.settings")}</a>
        <button type="button" role="menuitem" onclick={() => { close(); void logout(); }}><Icon name="logout" size={15} />{t("shell.logout")}</button>
      {/snippet}
    </Menu>
  </header>

  <nav class="sidebar" class:open={shell.drawer} aria-label={t("shell.nav")}>
    <ul>
      {#each nav as n (n.href)}
        <li>
          <a href={n.href} class:active={n.active} aria-current={n.active ? "page" : undefined} title={shell.rail ? n.label : undefined}>
            <Icon name={n.icon} size={17} />
            <span class="label">{n.label}</span>
            {#if n.badge}<span class="badge" aria-label={t("messages.unread_n", { n: n.badge })}>{n.badge > 99 ? "99+" : n.badge}</span>{/if}
          </a>
        </li>
      {/each}
    </ul>
    <div class="sidebar-foot">
      <!-- 在线指示灯（R11）：左下角，不可点击；悬停或聚焦显示说明 -->
      <span class="pulse-slot"><Pulse id="pulse-side" /></span>
      <!-- 底部两个安静的图标按钮（反馈 #38、#61）：收起侧栏在前，项目入口在后；文字放在提示里 -->
      <button class="foot-btn" type="button" onclick={() => shell.toggleRail()} aria-label={shell.rail ? t("shell.expand") : t("shell.collapse")} title={shell.rail ? t("shell.expand") : t("shell.collapse")}>
        <Icon name="panel" size={16} />
      </button>
      <a class="foot-btn" href="https://github.com/angwz/clash-launch" target="_blank" rel="noopener noreferrer" data-external aria-label="GitHub" title="GitHub">
        <Icon name="github" size={16} />
      </a>
    </div>
  </nav>
  {#if chat.open}
    {#await loadChat() then m}<m.default />{/await}
  {/if}
  {#if shell.drawer}
    <button class="scrim" type="button" aria-label={t("common.close")} onclick={() => (shell.drawer = false)}></button>
  {/if}

  <main id="main" class="main" tabindex="-1">
    {#if session.logoutAt}
      <!-- 当前设备被定时退出（需求 R8）：每个页面顶部提示，链到账户页可以取消 -->
      <p class="logout-banner" role="status"><Icon name="info" size={14} /><span>{t("account.logout_banner", { when: i18n.date(session.logoutAt, "long") })}</span><a href="/account">{t("account.logout_manage")}</a></p>
    {/if}
    {@render children()}
  </main>
</div>

<style>
  .shell {
    display: grid;
    grid-template-columns: var(--sidebar-w) minmax(0, 1fr);
    grid-template-rows: var(--topbar-h) minmax(0, 1fr);
    grid-template-areas: "top top" "side main";
    block-size: 100dvh;
  }
  .shell.rail {
    grid-template-columns: 56px minmax(0, 1fr);
  }

  .skip {
    position: absolute;
    inset-inline-start: var(--space-2);
    inset-block-start: -40px;
    z-index: 100;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-raised);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
  }
  .skip:focus {
    inset-block-start: var(--space-2);
  }

  .topbar {
    grid-area: top;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    /* 起始内边距让 logo 与下方侧栏图标居中对齐（侧栏 8px + 菜单项 12px，图标 17px、logo 26px） */
    padding-inline: calc(var(--space-2) + var(--space-3) - 4px) var(--space-3);
    border-block-end: 1px solid var(--border);
    background: var(--bg-raised);
    min-inline-size: 0;
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--fg);
    font-weight: 650;
    letter-spacing: -0.01em;
    padding-inline-end: var(--space-1);
    white-space: nowrap;
  }
  .brand:hover {
    text-decoration: none;
  }
  .mark {
    display: grid;
    place-items: center;
    inline-size: 26px;
    block-size: 26px;
    border-radius: 7px;
    background: var(--accent);
    color: var(--fg-on-accent);
  }
  .status {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin-inline-start: var(--space-2);
  }
  .pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    block-size: 26px;
    padding-inline: var(--space-2) 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-full);
    background: var(--bg);
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
    white-space: nowrap;
  }
  .pill:hover:not(:disabled) {
    border-color: var(--border-strong);
  }
  .pill[data-state="passed"] {
    color: var(--ok);
    background: var(--ok-soft);
    border-color: transparent;
  }
  .pill[data-state="failed"],
  .pill[data-state="error"] {
    color: var(--err);
    background: var(--err-soft);
    border-color: transparent;
  }
  .pill[data-state="unavailable"] {
    color: var(--warn);
    background: var(--warn-soft);
    border-color: transparent;
  }
  .pill[data-state="running"] {
    cursor: progress;
  }
  .counts {
    display: inline-flex;
    gap: var(--space-2);
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }
  .counts span {
    display: inline-flex;
    align-items: center;
    gap: 3px;
  }
  .c-err {
    color: var(--err);
  }
  .c-warn {
    color: var(--warn);
  }
  .raw-flag {
    display: grid;
    place-items: center;
    inline-size: 24px;
    block-size: 24px;
    border: 0;
    border-radius: var(--radius-full);
    background: var(--raw-soft);
    color: var(--raw);
    cursor: pointer;
  }
  .spacer {
    flex: 1;
  }
  .search {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    block-size: 30px;
    min-inline-size: 220px;
    padding-inline: var(--space-3) var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg);
    color: var(--fg-subtle);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .search:hover {
    border-color: var(--border-strong);
  }
  .search-label {
    flex: 1;
    text-align: start;
  }

  .sidebar {
    grid-area: side;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding: var(--space-3) var(--space-2);
    border-inline-end: 1px solid var(--border);
    background: var(--bg);
    overflow-y: auto;
  }
  .sidebar ul {
    display: grid;
    gap: 2px;
  }
  .sidebar a {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    block-size: 34px;
    padding-inline: var(--space-3);
    border-radius: var(--radius-md);
    color: var(--fg-muted);
    font-weight: 520;
    white-space: nowrap;
  }
  .sidebar a:hover {
    background: var(--bg-hover);
    color: var(--fg);
    text-decoration: none;
  }
  .sidebar a.active {
    background: var(--bg-active);
    color: var(--fg);
  }
  .sidebar a.active :global(.icon) {
    color: var(--accent-fg);
  }
  .rail .sidebar .label {
    display: none;
  }
  /* 右上角的消息图标与未读数（反馈 #75） */
  .msg-btn,
  .notice-btn {
    position: relative;
  }
  .msg-btn.active,
  .notice-btn.active {
    color: var(--accent-fg);
  }
  .dot {
    position: absolute;
    inset-block-start: 1px;
    inset-inline-end: 0;
    min-inline-size: 16px;
    padding-inline: 4px;
    border-radius: var(--radius-full);
    background: var(--accent);
    color: var(--fg-on-accent);
    font-size: 10px;
    font-weight: 650;
    line-height: 16px;
    text-align: center;
    font-variant-numeric: tabular-nums;
    pointer-events: none;
  }
  /* 未读角标（反馈 #74）：收起侧栏时缩成图标右上角的小圆点 */
  .sidebar a {
    position: relative;
  }
  .badge {
    margin-inline-start: auto;
    min-inline-size: 18px;
    padding-inline: 5px;
    border-radius: var(--radius-full);
    background: var(--accent);
    color: var(--fg-on-accent);
    font-size: 11px;
    font-weight: 600;
    line-height: 18px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
  .rail .sidebar .badge {
    position: absolute;
    inset-block-start: 3px;
    inset-inline-end: 6px;
    min-inline-size: 8px;
    block-size: 8px;
    padding: 0;
    font-size: 0;
    line-height: 0;
  }
  .rail .sidebar a {
    justify-content: center;
    padding-inline: 0;
  }
  .sidebar-foot {
    display: flex;
    gap: 2px;
    padding-block-start: var(--space-2);
    padding-inline: var(--space-1);
    border-block-start: 1px solid var(--border);
  }
  .rail .sidebar-foot {
    flex-direction: column;
    align-items: center;
    padding-inline: 0;
  }
  .pulse-slot {
    display: inline-grid;
    place-items: center;
    inline-size: 32px;
    block-size: 32px;
  }
  .sidebar-foot .pulse-slot {
    margin-inline-end: auto;
  }
  .rail .sidebar-foot .pulse-slot {
    margin-inline-end: 0;
  }
  .sidebar .foot-btn {
    display: grid;
    place-items: center;
    inline-size: 32px;
    block-size: 32px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg-subtle);
    cursor: pointer;
  }
  .sidebar .foot-btn:hover {
    background: var(--bg-hover);
    color: var(--fg);
  }

  .main {
    grid-area: main;
    overflow: auto;
    min-inline-size: 0;
    outline: none;
  }
  .logout-banner {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-2) var(--space-4);
    background: var(--info-soft);
    color: var(--info);
    font-size: var(--text-sm);
  }
  .logout-banner a {
    margin-inline-start: auto;
    color: inherit;
  }
  /* 页面切换：新页面挂载时淡入（App.svelte 按路由重新挂载页面，动画随之重放） */
  .main > :global(*) {
    animation: enter var(--dur) var(--ease) both;
  }

  .scrim {
    display: none;
  }
  .only-narrow {
    display: none;
  }

  .menu-head {
    display: grid;
    padding: var(--space-2) var(--space-3);
    border-block-end: 1px solid var(--border);
    margin-block-end: var(--space-1);
    font-size: var(--text-sm);
  }
  .check {
    display: inline-flex;
    inline-size: 16px;
  }

  @media (max-width: 1023px) {
    .search {
      position: relative; /* 约束下面隐藏的文字 */
      min-inline-size: 0;
    }
    .search .kbd {
      display: none;
    }
    /* 窄屏只隐藏文字的外观，读屏仍能读到按钮名称（R30）；与 .visually-hidden 相同 */
    .search-label {
      position: absolute;
      inline-size: 1px;
      block-size: 1px;
      overflow: hidden;
      clip-path: inset(50%);
      white-space: nowrap;
    }
  }

  @media (max-width: 767px) {
    .shell,
    .shell.rail {
      grid-template-columns: minmax(0, 1fr);
      grid-template-areas: "top" "main";
    }
    .only-narrow {
      display: inline-flex;
    }
    .word,
    .counts {
      display: none;
    }
    .status {
      margin-inline-start: 0;
    }
    .sidebar {
      position: fixed;
      z-index: 40;
      inset-block: var(--topbar-h) 0;
      inset-inline-start: 0;
      inline-size: min(280px, 82vw);
      transform: translateX(-100%);
      transition: transform var(--dur) var(--ease);
      box-shadow: var(--shadow-lg);
    }
    :global([dir="rtl"]) .sidebar {
      transform: translateX(100%);
    }
    .sidebar.open,
    :global([dir="rtl"]) .sidebar.open {
      transform: none;
    }
    .rail .sidebar .label {
      display: inline;
    }
    .rail .sidebar a {
      justify-content: flex-start;
      padding-inline: var(--space-3);
    }
    .sidebar-foot {
      display: none;
    }
    .scrim {
      display: block;
      position: fixed;
      inset: var(--topbar-h) 0 0 0;
      z-index: 30;
      border: 0;
      background: var(--scrim);
    }
  }
</style>
