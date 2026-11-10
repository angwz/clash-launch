<script lang="ts">
  import { t } from "../lib/i18n/i18n.svelte";
</script>

<div class="nf">
  <p class="code mono">404</p>
  <h1>{t("notfound.title")}</h1>
  <a class="btn" href="/">{t("notfound.home")}</a>
</div>

<style>
  .nf {
    display: grid;
    justify-items: center;
    align-content: center;
    gap: var(--space-3);
    min-block-size: 60dvh;
    padding: var(--space-8);
    text-align: center;
  }
  .code {
    color: var(--fg-subtle);
    font-size: var(--text-2xl);
  }
</style>
