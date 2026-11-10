<script lang="ts">
  import type { Snippet } from "svelte";
  import { i18n, LOCALES, t } from "../lib/i18n/i18n.svelte";
  import { theme } from "../lib/theme.svelte";
  import Icon from "./Icon.svelte";

  let { title, subtitle, children }: { title: string; subtitle?: string; children: Snippet } = $props();
</script>

<div class="auth">
  <div class="corner">
    <select class="select lang" aria-label={t("shell.language")} value={i18n.locale} onchange={(e) => void i18n.set(e.currentTarget.value as "en" | "zh-CN")}>
      {#each LOCALES as l (l.id)}<option value={l.id}>{l.label}</option>{/each}
    </select>
    <button class="btn ghost icon" type="button" onclick={() => theme.cycle()} aria-label={t("shell.theme", { theme: t("theme." + theme.value) })}>
      <Icon name={theme.value === "light" ? "sun" : theme.value === "dark" ? "moon" : "monitor"} />
    </button>
  </div>
  <main class="card">
    <div class="brand">
      <span class="mark"><Icon name="logo" size={22} /></span>
      <span>Clash, Launch!</span>
    </div>
    <h1>{title}</h1>
    {#if subtitle}<p class="sub">{subtitle}</p>{/if}
    {@render children()}
  </main>
</div>

<style>
  .auth {
    position: relative;
    display: grid;
    place-items: center;
    min-block-size: 100dvh;
    padding: var(--space-6) var(--space-4);
    background:
      radial-gradient(1200px 500px at 50% -10%, var(--accent-soft), transparent 70%),
      var(--bg);
  }
  .corner {
    position: absolute;
    inset-block-start: var(--space-3);
    inset-inline-end: var(--space-3);
    display: flex;
    gap: var(--space-2);
  }
  .lang {
    inline-size: auto;
  }
  .card {
    inline-size: min(400px, 100%);
    padding: var(--space-8);
    border: 1px solid var(--border);
    border-radius: 14px;
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-block-end: var(--space-6);
    font-weight: 650;
    letter-spacing: -0.01em;
  }
  .mark {
    display: grid;
    place-items: center;
    inline-size: 32px;
    block-size: 32px;
    border-radius: 9px;
    background: var(--accent);
    color: var(--fg-on-accent);
  }
  h1 {
    font-size: var(--text-xl);
  }
  .sub {
    margin-block-start: var(--space-1);
    color: var(--fg-muted);
  }
  .card :global(form) {
    display: grid;
    gap: var(--space-4);
    margin-block-start: var(--space-6);
  }
  @media (max-width: 480px) {
    .card {
      padding: var(--space-6) var(--space-5);
    }
  }
</style>
