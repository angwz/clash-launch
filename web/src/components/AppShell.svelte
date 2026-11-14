<script lang="ts">
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
  import OrderDialog from "./OrderDialog.svelte";
  import { ordered, prefs } from "../lib/prefs.svelte";
  import { errorText } from "../lib/errors";
  import { toasts } from "../lib/toast.svelte";

  let { children }: { children: Snippet } = $props();

  onMount(() => {
    shell.init();
    prefs.load().catch((e) => toasts.push("err", errorText(e)));
  });

  // 调整菜单顺序（反馈 #37）：分节列表来自目录，打开时才加载
  let orderOpen = $state(false);
  async function openOrder() {
    try {
      await catalog.load();
      orderOpen = true;
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  const sectionItems = $derived(
    ordered((catalog.meta?.pack.nav ?? []).filter((s) => !s.generated), (s) => s.id, prefs.data.sections).map((s) => ({ id: s.id, label: catalog.navTitle(s) })),
  );

  const defaultNav = $derived<{ id: string; href: string; label: string; icon: IconName; active: boolean }[]>([
    { id: "profiles", href: "/", label: t("nav.profiles"), icon: "layers", active: ["home", "profile", "profile-links", "profile-history", "welcome"].includes(router.route.name) },
    { id: "sources", href: "/sources", label: t("nav.sources"), icon: "server", active: router.route.name.startsWith("source") },
    { id: "rules", href: "/rules", label: t("nav.rules"), icon: "rules", active: router.route.name.startsWith("rule") },
    { id: "help", href: "/help", label: t("nav.help"), icon: "file", active: router.route.name === "help" },
    ...(session.isAdmin ? [{ id: "admin", href: "/admin", label: t("nav.admin"), icon: "shield" as IconName, active: router.route.name === "admin" }] : []),
  ]);
  const nav = $derived(ordered(defaultNav, (n) => n.id, prefs.data.nav));

  const themeIcon = $derived<IconName>(theme.value === "light" ? "sun" : theme.value === "dark" ? "moon" : "monitor");

  async function setLang(l: "en" | "zh-CN") {
    await i18n.set(l);
    if (catalog.meta) await catalog.loadMessages();
  }

  async function logout() {
    await session.logout();
    router.go("/login");
  }

  $effect(() => {
    // 换页时收起手机抽屉
    void router.path;
    shell.drawer = false;
  });

  const p = $derived(shell.profile);
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
          <Icon name={checkIcon} size={14} />
          <span>{t("check." + p.check)}</span>
        </button>
        {#if p.diagnostics.errors || p.diagnostics.warnings}
          <span class="counts">
            {#if p.diagnostics.errors}<span class="c-err"><Icon name="error" size={13} />{p.diagnostics.errors}</span>{/if}
            {#if p.diagnostics.warnings}<span class="c-warn"><Icon name="warning" size={13} />{p.diagnostics.warnings}</span>{/if}
          </span>
        {/if}
        {#if p.preview.available}
          <button class="btn sm preview-btn" type="button" aria-expanded={p.preview.open} onclick={() => p.preview.toggle()}>
            <Icon name="code" size={14} />{t("preview.title")}
          </button>
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
        <a href="/account" role="menuitem" onclick={close}><Icon name="settings" size={15} />{t("nav.account")}</a>
        <button type="button" role="menuitem" onclick={() => { close(); void openOrder(); }}><Icon name="rules" size={15} />{t("order.title")}</button>
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
          </a>
        </li>
      {/each}
    </ul>
    <div class="sidebar-foot">
      <!-- 项目入口（反馈 #38） -->
      <a class="gh" href="https://github.com/angwz/clash-launch" target="_blank" rel="noopener noreferrer" data-external title="GitHub">
        <Icon name="github" size={16} />
        <span class="label">GitHub</span>
      </a>
      <button class="btn ghost sm collapse" type="button" onclick={() => shell.toggleRail()} aria-label={shell.rail ? t("shell.expand") : t("shell.collapse")} title={shell.rail ? t("shell.expand") : t("shell.collapse")}>
        <Icon name="panel" size={15} />
        <span class="label">{t("shell.collapse")}</span>
      </button>
    </div>
  </nav>
  {#if shell.drawer}
    <button class="scrim" type="button" aria-label={t("common.close")} onclick={() => (shell.drawer = false)}></button>
  {/if}

  <OrderDialog bind:open={orderOpen} nav={nav.map((n) => ({ id: n.id, label: n.label }))} sections={sectionItems} />

  <main id="main" class="main" tabindex="-1">
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
    padding-inline: var(--space-3);
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
  .preview-btn[aria-expanded="true"] {
    background: var(--bg-active);
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
  .rail .sidebar a {
    justify-content: center;
    padding-inline: 0;
  }
  .sidebar-foot {
    display: grid;
    gap: 2px;
  }
  .sidebar .gh {
    block-size: 30px;
    color: var(--fg-subtle);
    font-size: var(--text-sm);
  }
  .collapse {
    inline-size: 100%;
    justify-content: flex-start;
    color: var(--fg-subtle);
  }
  .rail .collapse {
    justify-content: center;
  }

  .main {
    grid-area: main;
    overflow: auto;
    min-inline-size: 0;
    outline: none;
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
      min-inline-size: 0;
    }
    .search-label,
    .search .kbd {
      display: none;
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
