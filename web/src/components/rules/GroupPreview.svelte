<script lang="ts">
  // 策略组预览（O2）：用一份配置（或全部节点源）的节点展开正在编辑的规则，每组最终包含哪些组和节点一目了然。
  // 成员分三类着色：其他策略组、内置出口（DIRECT 等）、节点；空组与引用不存在的名字会出现在诊断里。
  import { t } from "../../lib/i18n/i18n.svelte";

  interface Group {
    name: string;
    type: string;
    members: string[];
    use?: string[];
    filter?: string;
  }
  let { groups, nodes, rules }: { groups: Group[]; nodes: number; rules: number } = $props();

  const BUILTIN = new Set(["DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"]);
  const names = $derived(new Set(groups.map((g) => g.name)));
  const kind = (m: string) => (names.has(m) ? "group" : BUILTIN.has(m) ? "builtin" : "node");
  let open = $state<Record<string, boolean>>({});
  const SHOW = 12;
</script>

<p class="muted sum">{t("rules.preview_sum", { groups: groups.length, nodes, rules })}</p>
<ul class="groups">
  {#each groups as g (g.name)}
    {@const more = g.members.length > SHOW && !open[g.name]}
    <li class="panel">
      <div class="head">
        <strong>{g.name}</strong>
        <span class="type mono">{g.type}</span>
        <span class="muted n">{t("rules.preview_members", { n: g.members.length })}</span>
      </div>
      <ul class="members">
        {#each more ? g.members.slice(0, SHOW) : g.members as m, i (i)}
          <li data-kind={kind(m)}>{m}</li>
        {/each}
        {#if more}<li><button class="btn sm ghost" type="button" onclick={() => (open[g.name] = true)}>{t("rules.preview_more", { n: g.members.length - SHOW })}</button></li>{/if}
        {#each g.use ?? [] as u (u)}<li data-kind="provider" title={t("rules.preview_provider")}>⇢ {u}{#if g.filter} · {g.filter}{/if}</li>{/each}
      </ul>
    </li>
  {/each}
</ul>

<style>
  .sum {
    font-size: var(--text-sm);
  }
  .groups {
    display: grid;
    gap: var(--space-2);
  }
  .groups > li {
    display: grid;
    gap: var(--space-2);
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .type {
    color: var(--fg-muted);
    font-size: var(--text-xs);
  }
  .n {
    margin-inline-start: auto;
    font-size: var(--text-xs);
  }
  .members {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .members li {
    padding-block: 1px;
    padding-inline: 8px;
    border-radius: var(--radius-full);
    background: var(--bg-sunken);
    font-size: var(--text-xs);
  }
  .members li[data-kind="group"] {
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-weight: 560;
  }
  .members li[data-kind="builtin"] {
    background: none;
    border: 1px solid var(--border);
    color: var(--fg-muted);
  }
  .members li[data-kind="provider"] {
    background: var(--warn-soft);
    color: var(--warn);
  }
</style>
