<script lang="ts">
  // 窄屏的列表模式（routing 第 9 节 #12）：没有连线与粒子，每条规则一行，写明交给谁、默认从哪里出去。
  import { t } from "../../lib/i18n/i18n.svelte";
  import { defaultPath, type Graph, type Trace } from "../../lib/routing/layout";
  import type { RuleRow, ViewPlan } from "../../lib/routing/plan";
  import { builtinLabel, fmtCount, ruleName, targetLabel } from "../../lib/routing/labels";

  let { graph, plan, rows, trace, onExpand }: { graph: Graph; plan: ViewPlan; rows: RuleRow[]; trace: Trace | null; onExpand: (id: string) => void } = $props();

  function via(target: string): string {
    const p = defaultPath(graph, target, (n) => plan.exitRow.get(n) ?? n);
    const names = p.hops.map((h) => h.name);
    const last = names.pop() ?? target;
    return [...names, builtinLabel(last)].join(" → ");
  }
</script>

<ol class="list" aria-label={t("routing.col.rules")}>
  {#each rows as row (row.kind === "seg" ? row.run.id : row.index)}
    {#if row.kind === "seg"}
      {@const rs = graph.rules.slice(row.run.from, row.run.to + 1)}
      <li>
        <button class="item seg" type="button" onclick={() => onExpand(row.run.id)}>
          <span class="pri">{row.run.from + 1}–{row.run.to + 1}</span>
          <span class="main">
            <span class="nm">{t("routing.seg_" + row.run.kind, { n: rs.length, target: targetLabel(row.run.target) })}</span>
            <span class="to">{via(row.run.target)}</span>
          </span>
        </button>
      </li>
    {:else}
      {@const r = graph.rules[row.index]!}
      <li class="item" class:traced={trace?.hit === r.index}>
        <span class="pri">{r.index + 1}</span>
        <span class="main">
          <span class="nm">{ruleName(r)}{#if r.set?.count} <span class="meta">{fmtCount(r.set.count)}</span>{/if}</span>
          <span class="to">{via(r.target)}</span>
        </span>
      </li>
    {/if}
  {/each}
</ol>

<style>
  .list {
    list-style: none;
    margin: 0;
    padding: var(--space-3) var(--space-4);
    display: grid;
    gap: 6px;
  }
  .item {
    display: flex;
    gap: 10px;
    padding: 8px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font: inherit;
    font-size: var(--text-sm);
    color: var(--fg);
    text-align: start;
    inline-size: 100%;
  }
  .item.seg {
    background: var(--bg-sunken);
    border-style: dashed;
    cursor: pointer;
  }
  .item.traced {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .pri {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    min-inline-size: 2.2em;
    text-align: end;
    padding-block-start: 2px;
    white-space: nowrap;
  }
  .main {
    display: grid;
    gap: 2px;
    min-inline-size: 0;
  }
  .nm {
    overflow-wrap: anywhere;
  }
  .meta {
    margin-inline-start: 6px;
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .to {
    font-size: var(--text-xs);
    color: var(--fg-muted);
    overflow-wrap: anywhere;
  }
</style>
