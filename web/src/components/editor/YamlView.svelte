<script lang="ts">
  // 只读 YAML 视图：行号 + 轻量高亮；可选标记差异行，可选高亮一段行（当前编辑的字段）并滚动到可见。
  import { highlightLine } from "../../lib/yaml-highlight";
  import type { LineRange } from "../../lib/yaml-range";

  let { lines, ops, mark }: { lines: string[]; ops?: string[]; mark?: LineRange | null } = $props();
  let box = $state<HTMLDivElement>();

  // 高亮范围变化时把它滚到视口中央；已经完整可见就不动，避免预览来回跳
  $effect(() => {
    if (!mark || !box) return;
    const first = box.querySelector<HTMLElement>(`tr[data-i="${mark.start}"]`);
    if (!first) return;
    const r = first.getBoundingClientRect();
    const b = box.getBoundingClientRect();
    if (r.top < b.top + 24 || r.bottom > b.bottom - 24) first.scrollIntoView({ block: "center", behavior: "smooth" });
  });
</script>

<!-- 可滚动区域需要能用键盘聚焦后滚动（WCAG 2.1.1） -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="yaml ltr" bind:this={box} role="region" aria-label="YAML" tabindex="0">
  <table>
    <tbody>
      {#each lines as line, i (i)}
        {@const op = ops?.[i] ?? " "}
        {@const m = !!mark && i >= mark.start && i < mark.end}
        <tr data-i={i} class:add={op === "+"} class:del={op === "-"} class:mark={m} class:partial={m && !mark?.exact}>
          <td class="ln" aria-hidden="true">{op === " " ? i + 1 : op}</td>
          <td class="code">{#each highlightLine(line) as tok, j (j)}<span class={"t-" + tok.k}>{tok.s}</span>{/each}{#if line === ""}&#8203;{/if}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

<style>
  .yaml {
    overflow: auto;
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    font-size: 12.5px;
    line-height: 1.6;
    background: var(--bg-code);
    tab-size: 2;
  }
  table {
    border-collapse: collapse;
    min-inline-size: 100%;
  }
  .ln {
    position: sticky;
    inset-inline-start: 0;
    inline-size: 1%;
    padding-inline: var(--space-3) var(--space-2);
    text-align: end;
    color: var(--fg-subtle);
    background: var(--bg-code);
    user-select: none;
    vertical-align: top;
    font-variant-numeric: tabular-nums;
    opacity: 0.75;
  }
  .code {
    padding-inline: var(--space-2) var(--space-4);
    white-space: pre;
  }
  tr.add {
    background: color-mix(in srgb, var(--ok) 12%, transparent);
  }
  tr.add .ln {
    color: var(--ok);
    background: color-mix(in srgb, var(--ok) 12%, var(--bg-code));
    opacity: 1;
  }
  /* 当前字段：浅色底 + 行号旁的强调竖线；只找到祖先时更淡 */
  tr.mark {
    background: var(--accent-soft, color-mix(in srgb, var(--accent) 12%, transparent));
    transition: background var(--dur) var(--ease);
  }
  tr.mark .ln {
    color: var(--accent-fg);
    box-shadow: inset -2px 0 0 var(--accent);
    opacity: 1;
  }
  tr.mark.partial {
    background: color-mix(in srgb, var(--accent) 5%, transparent);
  }
  tr.mark.partial .ln {
    box-shadow: inset -2px 0 0 color-mix(in srgb, var(--accent) 45%, transparent);
  }
  tr.del {
    background: color-mix(in srgb, var(--err) 12%, transparent);
  }
  tr.del .ln {
    color: var(--err);
    background: color-mix(in srgb, var(--err) 12%, var(--bg-code));
    opacity: 1;
  }
  .t-key {
    color: var(--syn-key);
  }
  .t-str {
    color: var(--syn-string);
  }
  .t-num {
    color: var(--syn-number);
  }
  .t-bool {
    color: var(--syn-bool);
  }
  .t-com {
    color: var(--syn-comment);
    font-style: italic;
  }
  .t-tag {
    color: var(--syn-tag);
  }
  .t-pun {
    color: var(--syn-punct);
  }
</style>
