<script lang="ts">
  import type { Component } from "svelte";
  import { onMount } from "svelte";
  import { catalog } from "./lib/catalog.svelte";
  import { i18n, LOCALES, t } from "./lib/i18n/i18n.svelte";
  import { palette } from "./lib/palette.svelte";
  import { registerPaletteData } from "./lib/palette-data.svelte";
  import { PUBLIC, router, type RouteName } from "./lib/router.svelte";
  import { session } from "./lib/session.svelte";
  import { theme } from "./lib/theme.svelte";
  import AppShell from "./components/AppShell.svelte";
  import CommandPalette from "./components/CommandPalette.svelte";
  import ShortcutsDialog, { shortcuts } from "./components/ShortcutsDialog.svelte";
  import Toasts from "./components/Toasts.svelte";

  // 页面按路由懒加载：首屏只包含外壳与当前页（第 15.4 节）。
  // 所有页面都以 { params } 渲染；不需要路由参数的页面不声明它，因此这里放宽为任意属性的组件。
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  type PageComponent = Component<any>;
  type Loader = () => Promise<{ default: PageComponent }>;
  const pages: Record<RouteName, Loader> = {
    home: () => import("./pages/Profiles.svelte"),
    welcome: () => import("./pages/Welcome.svelte"),
    login: () => import("./pages/Login.svelte"),
    setup: () => import("./pages/Setup.svelte"),
    register: () => import("./pages/Register.svelte"),
    recover: () => import("./pages/Recover.svelte"),
    terms: () => import("./pages/Terms.svelte"),
    profile: () => import("./pages/ProfileEditor.svelte"),
    "profile-links": () => import("./pages/ProfileLinks.svelte"),
    "profile-history": () => import("./pages/ProfileHistory.svelte"),
    sources: () => import("./pages/Sources.svelte"),
    source: () => import("./pages/SourceDetail.svelte"),
    rules: () => import("./pages/Rules.svelte"),
    rule: () => import("./pages/RuleDetail.svelte"),
    account: () => import("./pages/Account.svelte"),
    admin: () => import("./pages/Admin.svelte"),
    help: () => import("./pages/Help.svelte"),
    settings: () => import("./pages/Settings.svelte"),
    analytics: () => import("./pages/Analytics.svelte"),
    "short-links": () => import("./pages/ShortLinks.svelte"),
    messages: () => import("./pages/MessagesRedirect.svelte"),
    "message-thread": () => import("./pages/MessagesRedirect.svelte"),
    notfound: () => import("./pages/NotFound.svelte"),
  };

  let Page = $state<PageComponent | null>(null);
  let loadedFor = $state("");
  // 已加载的组件属于哪个路由：新页面异步加载期间不再用新路由的参数渲染旧页面（否则旧页面会以缺少 id 的参数重新挂载并发出请求）
  let pageFor = $state("");
  const showPage = $derived(Page !== null && pageFor === router.route.name);
  // 传给页面的参数只在路由属于该页面时更新：离开时旧页面在销毁前不会读到新路由的参数（如缺少 id）
  let pageParams = $state<Record<string, string>>({});
  $effect.pre(() => {
    if (router.route.name === pageFor) pageParams = router.route.params;
  });
  let booted = $state(false);
  let bootError = $state("");

  onMount(() => {
    theme.init();
    void boot();
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (session.user) (palette.open ? palette.close() : palette.show());
        return;
      }
      // ?：快捷键一览（需求 R45）。在输入框、代码编辑器中照常输入问号；已有对话框打开时不叠加
      if (e.key === "?" && !e.ctrlKey && !e.metaKey && !e.altKey && session.user && !editable(e.target) && !document.querySelector("dialog[open]")) {
        e.preventDefault();
        shortcuts.open = true;
      }
    };
    window.addEventListener("keydown", onKey);
    // 数据先注册：后注册的排在前面，同分时页面与命令在数据之前
    const offData = registerPaletteData();
    const off = palette.register("global", () => [
      { id: "page.home", kind: "page", title: t("nav.profiles"), icon: "layers", run: () => router.go("/") },
      { id: "page.sources", kind: "page", title: t("nav.sources"), icon: "server", run: () => router.go("/sources") },
      { id: "page.rules", kind: "page", title: t("nav.rules"), icon: "rules", run: () => router.go("/rules") },
      { id: "page.help", kind: "page", title: t("nav.help"), icon: "file", run: () => router.go("/help") },
      { id: "page.account", kind: "page", title: t("nav.account"), icon: "user", run: () => router.go("/account") },
      { id: "page.settings", kind: "page", title: t("nav.settings"), icon: "settings", run: () => router.go("/settings") },
      ...(session.isAdmin ? [{ id: "page.admin", kind: "page" as const, title: t("nav.admin"), icon: "shield" as const, run: () => router.go("/admin") }] : []),
      { id: "cmd.shortcuts", kind: "command", title: t("palette.shortcuts"), hint: "?", icon: "command", run: () => (shortcuts.open = true) },
      { id: "cmd.theme.light", kind: "command", title: t("palette.theme", { theme: t("theme.light") }), icon: "sun", run: () => theme.apply("light") },
      { id: "cmd.theme.dark", kind: "command", title: t("palette.theme", { theme: t("theme.dark") }), icon: "moon", run: () => theme.apply("dark") },
      { id: "cmd.theme.system", kind: "command", title: t("palette.theme", { theme: t("theme.system") }), icon: "monitor", run: () => theme.apply("system") },
      ...LOCALES.map((l) => ({ id: "cmd.lang." + l.id, kind: "command" as const, title: t("palette.language", { language: l.label }), icon: "globe" as const, run: () => void setLang(l.id) })),
    ]);
    return () => {
      window.removeEventListener("keydown", onKey);
      off();
      offData();
    };
  });

  function editable(t: EventTarget | null): boolean {
    return t instanceof HTMLElement && (t.isContentEditable || t.closest("input, textarea, select, [contenteditable]") !== null);
  }

  async function boot() {
    try {
      await i18n.set(i18n.negotiate(), false);
      await session.load();
      if (session.user?.lang && !new URLSearchParams(location.search).has("lang")) {
        const want = i18n.negotiate(session.user.lang);
        if (want !== i18n.locale) await i18n.set(want, false);
      }
      booted = true;
    } catch (e) {
      bootError = e instanceof Error ? e.message : String(e);
    }
  }

  async function setLang(l: "en" | "zh-CN") {
    await i18n.set(l);
    if (catalog.meta) await catalog.loadMessages();
  }

  // 路由守卫：未登录访问受保护页面时转到登录页，登录后回到原处。
  $effect(() => {
    if (!booted) return;
    const name = router.route.name;
    if (!session.user && !PUBLIC.has(name)) {
      router.go("/login?next=" + encodeURIComponent(router.path + router.search), true);
      return;
    }
    if (session.user && name === "login") {
      router.go(new URLSearchParams(router.search).get("next") || "/", true);
      return;
    }
    const key = name;
    if (loadedFor === key) return;
    loadedFor = key;
    Page = null;
    void pages[name]().then((m) => {
      if (loadedFor === key) {
        Page = m.default;
        pageFor = key;
      }
    });
  });

  const bare = $derived(PUBLIC.has(router.route.name) || !session.user);
  // 用户协议有新版本且尚未同意（需求 R40）：除了阅读协议本身，先显示同意页；服务端同时拦截其他接口
  const gated = $derived(!!session.user && !!session.terms?.pending && router.route.name !== "terms");
  // 两步验证策略要求而尚未启用（需求 R41）：先同意协议，再设置两步验证
  const mfaGated = $derived(!!session.user && !gated && session.mfaSetup && router.route.name !== "terms");
</script>

{#if bootError}
  <div class="boot-error" role="alert">
    <p>Clash, Launch! — {bootError}</p>
  </div>
{:else if booted}
  {#if gated}
    <!-- 同意页与两步验证引导页很少出现，按需加载，不计入首屏 -->
    {#await import("./components/TermsGate.svelte") then m}<m.default />{/await}
  {:else if mfaGated}
    {#await import("./components/MfaGate.svelte") then m}<m.default />{/await}
  {:else if bare}
    {#if showPage && Page}<Page params={pageParams} />{/if}
  {:else}
    <AppShell>
      {#if showPage && Page}
        {#key pageParams.id}
          <Page params={pageParams} />
        {/key}
      {/if}
    </AppShell>
    <CommandPalette />
    <ShortcutsDialog />
  {/if}
  <Toasts />
{/if}

<style>
  .boot-error {
    display: grid;
    place-items: center;
    min-block-size: 100dvh;
    color: var(--err);
    padding: var(--space-6);
  }
</style>
