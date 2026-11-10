<script lang="ts">
  // 只读 YAML 视图：行号 + 轻量高亮；可选标记差异行。
  import { highlightLine } from "../../lib/yaml-highlight";

  let { lines, ops }: { lines: string[]; ops?: string[] } = $props();
</script>

<!-- 可滚动区域需要能用键盘聚焦后滚动（WCAG 2.1.1） -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="yaml ltr" role="region" aria-label="YAML" tabindex="0">
  <table>
    <tbody>
      {#each lines as line, i (i)}
        {@const op = ops?.[i] ?? " "}
        <tr class:add={op === "+"} class:del={op === "-"}>
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
