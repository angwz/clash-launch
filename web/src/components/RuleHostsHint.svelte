<script lang="ts">
  // 规则地址的来源提示（反馈 #72）：受限时列出允许的站点，并提示其他来源改为粘贴文本。不受限时不显示。
  import { ruleHosts, type RuleHosts } from "../lib/rule-hosts";
  import { t } from "../lib/i18n/i18n.svelte";

  let info = $state<RuleHosts | null>(null);
  $effect(() => {
    ruleHosts().then((x) => (info = x), () => undefined);
  });
</script>

{#if info?.restricted}
  <p class="hint">
    {t("rules.hosts_hint")}
    {#each info.hosts as h, i (h)}<code class="ltr">{h}</code>{i < info.hosts.length - 1 ? " " : ""}{/each}
  </p>
{/if}

<style>
  code {
    font-size: var(--text-xs);
    padding-inline: 4px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
  }
</style>
