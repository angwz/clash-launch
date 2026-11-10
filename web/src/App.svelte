<script lang="ts">
  import type { Component } from "svelte";
  import { onMount } from "svelte";
  import { catalog } from "./lib/catalog.svelte";
  import { i18n, LOCALES, t } from "./lib/i18n/i18n.svelte";
  import { palette } from "./lib/palette.svelte";
  import { PUBLIC, router, type RouteName } from "./lib/router.svelte";
  import { session } from "./lib/session.svelte";
  import { theme } from "./lib/theme.svelte";
  import AppShell from "./components/AppShell.svelte";
  import CommandPalette from "./components/CommandPalette.svelte";
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
    profile: () => import("./pages/ProfileEditor.svelte"),
    "profile-links": () => import("./pages/ProfileLinks.svelte"),
    "profile-history": () => import("./pages/ProfileHistory.svelte"),
    sources: () => import("./pages/Sources.svelte"),
    source: () => import("./pages/SourceDetail.svelte"),
    rules: () => import("./pages/Rules.svelte"),
    rule: () => import("./pages/RuleDetail.svelte"),
    account: () => import("./pages/Account.svelte"),
    admin: () => import("./pages/Admin.svelte"),
    notfound: () => import("./pages/NotFound.svelte"),
  };

  let Page = $state<PageComponent | null>(null);
  let loadedFor = $state("");
  let booted = $state(false);
  let bootError = $state("");

  onMount(() => {
    theme.init();
    void boot();
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (session.user) (palette.open ? palette.close() : palette.show());
      }
    };
    window.addEventListener("keydown", onKey);
    const off = palette.register("global", () => [
      { id: "page.home", kind: "page", title: t("nav.profiles"), icon: "layers", run: () => router.go("/") },
      { id: "page.sources", kind: "page", title: t("nav.sources"), icon: "server", run: () => router.go("/sources") },
      { id: "page.rules", kind: "page", title: t("nav.rules"), icon: "rules", run: () => router.go("/rules") },
      { id: "page.account", kind: "page", title: t("nav.account"), icon: "user", run: () => router.go("/account") },
      ...(session.isAdmin ? [{ id: "page.admin", kind: "page" as const, title: t("nav.admin"), icon: "shield" as const, run: () => router.go("/admin") }] : []),
      { id: "cmd.theme.light", kind: "command", title: t("palette.theme", { theme: t("theme.light") }), icon: "sun", run: () => theme.apply("light") },
      { id: "cmd.theme.dark", kind: "command", title: t("palette.theme", { theme: t("theme.dark") }), icon: "moon", run: () => theme.apply("dark") },
      { id: "cmd.theme.system", kind: "command", title: t("palette.theme", { theme: t("theme.system") }), icon: "monitor", run: () => theme.apply("system") },
      ...LOCALES.map((l) => ({ id: "cmd.lang." + l.id, kind: "command" as const, title: t("palette.language", { language: l.label }), icon: "globe" as const, run: () => void setLang(l.id) })),
    ]);
    return () => {
      window.removeEventListener("keydown", onKey);
      off();
    };
  });

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
      if (loadedFor === key) Page = m.default;
    });
  });

  const bare = $derived(PUBLIC.has(router.route.name) || !session.user);
</script>

{#if bootError}
  <div class="boot-error" role="alert">
    <p>Clash, Launch! — {bootError}</p>
  </div>
{:else if booted}
  {#if bare}
    {#if Page}<Page params={router.route.params} />{/if}
  {:else}
    <AppShell>
      {#if Page}
        {#key router.route.params.id}
          <Page params={router.route.params} />
        {/key}
      {/if}
    </AppShell>
    <CommandPalette />
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
