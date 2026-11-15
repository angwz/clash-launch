<script lang="ts">
  // 讲解图层（routing 第 1 节、第 13 节第 1 步）：五步讲清一次请求怎样经规则、策略组到出口。只读，不改配置。
  // 图用 SVG 几何属性与类名着色（站内 CSP 不允许 style 属性）；小圆点沿路径移动用 SMIL animateMotion，
  // 系统「减少动态效果」时不画。图中的规则与网址是示意；接入分流地图后（第 2 步）改用用户自己的规则集与组名。
  import Dialog from "../Dialog.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";

  let { open = $bindable(false) }: { open?: boolean } = $props();

  const STEPS = ["1", "2", "3", "4", "5"] as const;
  let step = $state(0);
  const reduce = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

  // 每次打开都从第一步开始
  $effect(() => {
    if (open) step = 0;
  });

  // 第几个方框在当前一步高亮：最后一步同时高亮策略组与出口
  const lit = (box: number) => box === step || (step === 4 && box >= 3);
  // 被规则认出之前，请求还没有颜色
  const colored = $derived(step >= 2);

  // 窄屏时图比容器宽：切换步骤后把当前方框滚到中间（坐标与 viewBox 一致）
  const CENTERS = [80, 265, 495, 698, 810];
  let scroller: HTMLDivElement | undefined = $state();
  $effect(() => {
    const el = scroller;
    const x = CENTERS[step]!;
    if (!el || el.scrollWidth <= el.clientWidth) return;
    const scale = el.scrollWidth / 880;
    el.scrollTo({ left: x * scale - el.clientWidth / 2, behavior: reduce ? "auto" : "smooth" });
  });

  function onKey(e: KeyboardEvent) {
    if (e.key === "ArrowRight" && step < STEPS.length - 1) step++;
    else if (e.key === "ArrowLeft" && step > 0) step--;
    else return;
    e.preventDefault();
  }
</script>

<Dialog bind:open title={t("routing.explain.title")} xl>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="layer" onkeydown={onKey}>
    <div class="stage-scroll" bind:this={scroller}>
      <svg class="stage" viewBox="0 0 880 250" role="img" aria-label={t("routing.explain.figure")}>
        <path id="xl-w-p" class="wire p" d="M140 125 L 330 125 C 380 125, 380 70, 430 70 L 560 70 C 610 70, 610 70, 650 70 L 760 70" />
        <path id="xl-w-d" class="wire d" d="M140 125 L 330 125 C 380 125, 380 125, 430 125 L 560 125 C 610 125, 610 125, 650 125 L 760 125" />
        <path id="xl-w-r" class="wire r" d="M140 125 L 330 125 C 380 125, 380 185, 430 185 L 560 185 C 590 185, 610 205, 650 205" />
        <!-- 移动的小圆点画在方框下面：看起来是穿过客户端与规则；起点错开用负的 begin，避免开始前停在原点 -->
        {#if !reduce}
          {#each ["p", "d", "r"] as w, i (w)}
            <circle class="dot" class:a={colored && i === 0} class:b={colored && i === 1} class:c={colored && i === 2} r="4">
              <animateMotion dur="3.4s" repeatCount="indefinite" begin={-i * 1.1 + "s"}><mpath href={"#xl-w-" + w} /></animateMotion>
            </circle>
          {/each}
        {/if}
        <g class="box" class:on={lit(0)}>
          <rect x="20" y="95" width="120" height="60" rx="8" />
          <text x="80" y="122" text-anchor="middle">{t("routing.explain.box_app")}</text>
          <text class="sub" x="80" y="140" text-anchor="middle">{t("routing.explain.box_app_sub")}</text>
        </g>
        <g class="box" class:on={lit(1)}>
          <rect x="200" y="95" width="130" height="60" rx="8" />
          <text x="265" y="122" text-anchor="middle">{t("routing.explain.box_client")}</text>
          <text class="sub" x="265" y="140" text-anchor="middle">{t("routing.explain.box_client_sub")}</text>
        </g>
        <g class="box" class:on={lit(2)}>
          <rect x="430" y="40" width="130" height="170" rx="8" />
          <text x="495" y="64" text-anchor="middle">{t("routing.explain.box_rules")}</text>
          <text class="sub" x="495" y="90" text-anchor="middle">{t("routing.explain.rule_1")}</text>
          <text class="sub" x="495" y="110" text-anchor="middle">{t("routing.explain.rule_2")}</text>
          <text class="sub" x="495" y="130" text-anchor="middle">{t("routing.explain.rule_3")}</text>
          <text class="sub" x="495" y="148" text-anchor="middle">…</text>
          <text class="sub" x="495" y="168" text-anchor="middle">{t("routing.explain.rule_last")}</text>
          <!-- 底部说明不是一条规则：用细线隔开、颜色更淡 -->
          <line class="sep" x1="446" y1="182" x2="544" y2="182" />
          <text class="cap" x="495" y="200" text-anchor="middle">{t("routing.explain.box_rules_sub")}</text>
        </g>
        <g class="box" class:on={lit(3)}>
          <rect x="650" y="40" width="96" height="50" rx="8" />
          <text x="698" y="62" text-anchor="middle">{t("routing.explain.box_group")}</text>
          <text class="sub" x="698" y="79" text-anchor="middle">{t("routing.explain.box_group_sub")}</text>
        </g>
        <g class="box" class:on={lit(4)}>
          <rect x="760" y="40" width="100" height="50" rx="8" />
          <text x="810" y="62" text-anchor="middle">{t("routing.explain.box_node")}</text>
          <text class="sub" x="810" y="79" text-anchor="middle">{t("routing.explain.box_node_sub")}</text>
        </g>
        <g class="box" class:on={lit(4)}>
          <rect x="760" y="100" width="100" height="50" rx="8" />
          <text x="810" y="122" text-anchor="middle">{t("routing.explain.box_direct")}</text>
          <text class="sub" x="810" y="139" text-anchor="middle">{t("routing.explain.box_direct_sub")}</text>
        </g>
        <g class="box" class:on={lit(4)}>
          <rect x="650" y="185" width="96" height="40" rx="8" />
          <text x="698" y="210" text-anchor="middle">{t("routing.explain.box_reject")}</text>
        </g>
        <!-- 三个示例请求：颜色在规则认出它之后才出现 -->
        <circle class="dot" class:a={colored} cx="146" cy="27" r="4" /><text class="lbl" x="156" y="31">chatgpt.com</text>
        <circle class="dot" class:b={colored} cx="276" cy="27" r="4" /><text class="lbl" x="286" y="31">baidu.com</text>
        <circle class="dot" class:c={colored} cx="396" cy="27" r="4" /><text class="lbl" x="406" y="31">doubleclick.net</text>
      </svg>
    </div>

    <div class="steps" role="tablist" aria-label={t("routing.explain.steps")}>
      {#each STEPS as s, i (s)}
        <button class="step" class:on={i === step} type="button" role="tab" aria-selected={i === step} onclick={() => (step = i)}>
          {i + 1}. {t("routing.explain.title_" + s)}
        </button>
      {/each}
    </div>

    <div class="explain" role="tabpanel" aria-live="polite">
      <h3>{t("routing.explain.title_" + STEPS[step])}</h3>
      <p>{t("routing.explain.body_" + STEPS[step])}</p>
    </div>
  </div>

  {#snippet footer()}
    <button class="btn ghost" type="button" disabled={step === 0} onclick={() => step--}>{t("routing.explain.prev")}</button>
    {#if step < STEPS.length - 1}
      <button class="btn primary" type="button" onclick={() => step++}>{t("routing.explain.next")}</button>
    {:else}
      <button class="btn primary" type="button" onclick={() => (open = false)}>{t("routing.explain.done")}</button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .layer {
    display: grid;
    gap: var(--space-3);
  }
  /* 窄屏时图在自身容器内横向滚动，页面本身不出现横向滚动条 */
  .stage-scroll {
    overflow-x: auto;
    margin-inline: calc(-1 * var(--space-2));
  }
  .stage {
    display: block;
    min-inline-size: 720px;
    inline-size: 100%;
    block-size: auto;
  }
  .box rect {
    fill: var(--bg-sunken);
    stroke: var(--border-strong);
    stroke-width: 1;
    transition:
      fill var(--dur) var(--ease),
      stroke var(--dur) var(--ease);
  }
  .box text {
    fill: var(--fg-muted);
    font-size: 13px;
    font-family: var(--font-sans);
  }
  .box .sub {
    font-size: 11px;
    fill: var(--fg-subtle);
  }
  .box.on rect {
    fill: var(--accent-soft);
    stroke: var(--accent);
    stroke-width: 1.5;
  }
  /* 高亮时只提亮标题，小字保持次要颜色：深色主题下小号亮字会发粗，层次也会消失 */
  .box.on text {
    fill: var(--fg);
  }
  .box.on .sub {
    fill: var(--fg-muted);
  }
  .box .cap,
  .box.on .cap {
    font-size: 10.5px;
    fill: var(--fg-subtle);
  }
  .box .sep {
    stroke: var(--border-strong);
    stroke-width: 1;
  }
  .wire {
    fill: none;
    stroke-width: 1.5;
    opacity: 0.7;
  }
  .wire.p {
    stroke: var(--flow-proxy);
  }
  .wire.d {
    stroke: var(--flow-direct);
  }
  .wire.r {
    stroke: var(--flow-reject);
    stroke-dasharray: 4 4;
  }
  .dot {
    fill: var(--flow-none);
    transition: fill var(--dur) var(--ease);
  }
  .dot.a {
    fill: var(--flow-proxy);
  }
  .dot.b {
    fill: var(--flow-direct);
  }
  .dot.c {
    fill: var(--flow-reject);
  }
  .lbl {
    font-size: 11px;
    fill: var(--fg-subtle);
    font-family: var(--font-mono);
  }
  .steps {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-2);
  }
  .step {
    font: inherit;
    text-align: start;
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--fg-muted);
    cursor: pointer;
  }
  .step:hover {
    background: var(--bg-hover);
  }
  .step.on {
    border-color: var(--accent);
    color: var(--fg);
    background: var(--accent-soft);
  }
  .explain {
    display: grid;
    gap: var(--space-1);
    min-block-size: 6.5em; /* 切换步骤时高度不跳动 */
  }
  .explain h3 {
    font-size: var(--text-md);
    font-weight: 620;
  }
  .explain p {
    max-inline-size: 70ch;
    color: var(--fg-muted);
  }
</style>
