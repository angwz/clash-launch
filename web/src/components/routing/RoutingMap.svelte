<script lang="ts" module>
  /** 拖动：判断能否放下（返回提示文字，null 表示不能）与执行。zone 只对规则行有意义：before、after 为移动，mid 为合并。 */
  export interface DragOps {
    can: (src: string, dst: string, zone: "before" | "mid" | "after") => { ok: boolean; label: string } | null;
    drop: (src: string, dst: string, zone: "before" | "mid" | "after") => void;
    draggable: (key: string) => boolean;
  }
</script>

<script lang="ts">
  // 分流地图（routing 第 8、10 节）：请求 → 规则 → 策略组（按默认路径分列）→ 出口 → 目的地。
  // 布局与连线由 lib/routing 的纯函数计算；这里只负责把方框放进 DOM、量出位置、画 SVG 轨道与粒子。
  // 站内 CSP 不允许 style 属性：列宽与最小宽度用 CSSOM（style.setProperty）设置，几何只写进 SVG 的 d。
  import { onMount, untrack } from "svelte";
  import { catalog } from "../../lib/catalog.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { defaultPath, links, type Box, type Graph, type Rule, type Trace } from "../../lib/routing/layout";
  import { ruleBoxOf, rowKey, type RuleRow, type ViewPlan } from "../../lib/routing/plan";
  import { ParticleField, type ParticleMode } from "../../lib/routing/particles";
  import { exitLabel, fmtCount, ruleName, setBases, targetLabel, typeChar, typeLabel } from "../../lib/routing/labels";

  let {
    graph,
    plan,
    rows,
    trace,
    particles,
    candidates,
    onExpand,
    selected = null,
    onSelect = () => {},
    drag = undefined,
  }: {
    graph: Graph;
    plan: ViewPlan;
    rows: RuleRow[];
    trace: Trace | null;
    particles: ParticleMode;
    candidates: boolean;
    onExpand: (id: string) => void;
    selected?: string | null;
    onSelect?: (key: string) => void;
    drag?: DragOps;
  } = $props();

  let mapEl: HTMLDivElement | undefined = $state();
  let colsEl: HTMLDivElement | undefined = $state();
  let canvas: HTMLCanvasElement | undefined = $state();
  let boxes = $state.raw(new Map<string, Box>());
  let size = $state({ w: 0, h: 0, rulesLeft: 0 });
  let hover = $state<string | null>(null);
  let lastSig = "";
  // 必须是响应式的：下面几个 effect 在粒子层创建之前就会运行一次，创建后要重跑
  let field = $state.raw<ParticleField | null>(null);

  const exitRow = (name: string) => plan.exitRow.get(name) ?? "node:" + name;
  const ruleBox = $derived(ruleBoxOf(rows));
  const groupCols = $derived(plan.columns.cols.map((c) => c.filter((g) => !plan.unused.includes(g.name))));
  const unused = $derived(graph.groups.filter((g) => plan.unused.includes(g.name)));
  const gw = $derived(plan.compact ? 124 : 164);

  // 方框键：rule:<序号> | seg:<段 ID> | group:<名> | exit:<出口行 ID>
  function pathBoxes(target: string): string[] {
    return defaultPath(graph, target, exitRow).hops.map((h) => h.box);
  }

  // 聚焦集合：追踪时为命中行及其默认路径；否则为悬停的方框及与它直接相连的方框
  const focus = $derived.by((): Set<string> | null => {
    if (trace && trace.hit >= 0) {
      const s = new Set([ruleBox(trace.hit), ...pathBoxes(graph.rules[trace.hit]!.target)]);
      return s;
    }
    const cur = hover ?? selected;
    if (!cur) return null;
    const s = new Set([cur]);
    if (/^(rule|seg|bundle):/.test(cur)) {
      for (const r of graph.rules) if (ruleBox(r.index) === cur) for (const b of pathBoxes(r.target)) s.add(b);
    } else if (cur.startsWith("group:")) {
      const name = cur.slice(6);
      const g = graph.groups.find((x) => x.name === name);
      for (const m of g?.members ?? []) s.add(m.kind === "group" ? "group:" + m.name : "exit:" + exitRow(m.name));
      for (const r of graph.rules) if (r.target === name) s.add(ruleBox(r.index));
      for (const p of graph.groups) if (p.members.some((m) => m.name === name)) s.add("group:" + p.name);
    } else if (cur.startsWith("exit:")) {
      const row = plan.exits.find((e) => "exit:" + e.id === cur);
      for (const n of row?.members ?? []) {
        for (const r of graph.rules) if (r.target === n) s.add(ruleBox(r.index));
        for (const p of graph.groups) if (p.members.some((m) => m.name === n)) s.add("group:" + p.name);
      }
    }
    return s;
  });

  const focusedGroup = $derived.by(() => {
    const cur = hover ?? selected;
    return cur?.startsWith("group:") ? cur.slice(6) : null;
  });
  const paths = $derived(
    links({ graph, boxes, ruleBox, exitRow, bigGroups: plan.bigGroups, showCandidates: (g) => candidates || g === focusedGroup }).map((l) => {
      const [from, to] = l.key.split(">") as [string, string];
      return { ...l, hot: !!focus && focus.has(from) && focus.has(to) };
    }),
  );

  // 量出每个方框相对地图的位置
  function measure() {
    if (!mapEl) return;
    const b = mapEl.getBoundingClientRect();
    const m = new Map<string, Box>();
    mapEl.querySelectorAll<HTMLElement>("[data-box]").forEach((el) => {
      const r = el.getBoundingClientRect();
      m.set(el.dataset.box!, { l: r.left - b.left, r: r.right - b.left, t: r.top - b.top, b: r.bottom - b.top, y: r.top - b.top + r.height / 2 });
    });
    const rc = mapEl.querySelector<HTMLElement>(".col.rules");
    // 位置没变就不更新：否则每次重新计算都会重建粒子场景，粒子永远攒不起来
    const next = { w: mapEl.clientWidth, h: mapEl.clientHeight, rulesLeft: rc ? rc.getBoundingClientRect().left - b.left : 0 };
    const sig = JSON.stringify([...m]) + JSON.stringify(next);
    if (sig === lastSig) return;
    lastSig = sig;
    boxes = m;
    size = next;
  }

  // 网格列：请求道 · 规则 · 各组列 · 出口 · 目的地道
  $effect(() => {
    if (!colsEl || !mapEl) return;
    const n = groupCols.length;
    colsEl.style.setProperty("grid-template-columns", `48px minmax(220px, 1.4fr) repeat(${n}, minmax(${gw}px, 1fr)) minmax(120px, 0.85fr) 48px`);
    mapEl.style.setProperty("min-inline-size", `${48 * 2 + 220 + gw * n + 120 + 28 * (n + 3) + 32}px`);
  });

  // 内容变化后重新量位置
  $effect(() => {
    void rows;
    void plan;
    void graph;
    requestAnimationFrame(measure);
  });

  // 粒子：场景只在样例内容、方框位置或上限真的变化时重建（重建会清空粒子）。
  // 宽度抖动（滚动条出现又消失）会让智能视图重算出一个内容相同的新对象，不能因此重建；
  // 所以这里依赖的是内容键（字符串），图与计划只在闭包里读取。
  const sceneSamples = $derived(graph.rules.flatMap((r) => (r.set?.samples ?? []).map((s) => ({ host: s.host, hit: s.hit, weight: Math.log10((r.set?.count ?? 0) + 10) / Math.max(1, r.set!.samples!.length) }))));
  const sceneKey = $derived(JSON.stringify([sceneSamples, plan.particleCap, plan.labelProb, rows.map(rowKey)]));
  $effect(() => {
    void sceneKey;
    const b = boxes;
    const sz = size;
    if (!field || !b.size) return;
    untrack(() =>
      field!.setScene({
        boxes: b,
        ruleBox,
        pathOf: (hit: number) => {
          const r = graph.rules[hit];
          const p = defaultPath(graph, r?.target ?? "", exitRow);
          return { boxes: p.hops.map((h) => h.box), kind: p.kind };
        },
        samples: sceneSamples,
        rulesLeft: sz.rulesLeft,
        width: sz.w,
        height: sz.h,
        cap: plan.particleCap,
        labelProb: plan.labelProb,
      }),
    );
  });
  $effect(() => {
    field?.setTrace(trace && trace.hit >= 0 ? { host: trace.ip || trace.host, steps: trace.steps, hit: trace.hit, needsIp: trace.needsIp } : null);
  });
  $effect(() => field?.setMode(particles));
  $effect(() => field?.setFocus(trace ? null : focus));

  onMount(() => {
    if (!canvas || !mapEl) return;
    field = new ParticleField(canvas);
    field.setMode(particles);
    field.start();
    const ro = new ResizeObserver(() => requestAnimationFrame(measure));
    ro.observe(mapEl);
    const io = new IntersectionObserver((es) => field?.setVisible(!!es[0]?.isIntersecting));
    io.observe(mapEl);
    const mo = new MutationObserver(() => field?.readColors());
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const onScheme = () => field?.readColors();
    mq.addEventListener("change", onScheme);
    return () => {
      ro.disconnect();
      io.disconnect();
      mo.disconnect();
      mq.removeEventListener("change", onScheme);
      field?.stop();
      field = null;
    };
  });


  // —— 选中与拖动（第 3 步）——
  type Zone = "before" | "mid" | "after";
  let press: { key: string; x: number; y: number; el: HTMLElement; id: number } | null = null;
  let dragging = $state<string | null>(null);
  let over = $state<{ key: string; zone: Zone; ok: boolean; label: string } | null>(null);
  let ghost: HTMLDivElement | undefined = $state();

  function zoneOf(el: HTMLElement, y: number): Zone {
    if (!/^(rule|bundle):/.test(el.dataset.box ?? "")) return "mid";
    const r = el.getBoundingClientRect();
    const f = (y - r.top) / r.height;
    return f < 0.28 ? "before" : f > 0.72 ? "after" : "mid";
  }

  function onDown(e: PointerEvent, key: string) {
    if (e.button !== 0) return;
    press = { key, x: e.clientX, y: e.clientY, el: e.currentTarget as HTMLElement, id: e.pointerId };
  }

  function onMove(e: PointerEvent) {
    if (!press || e.pointerId !== press.id) return;
    if (!dragging) {
      if (Math.hypot(e.clientX - press.x, e.clientY - press.y) < 6 || !drag?.draggable(press.key)) return;
      dragging = press.key;
      press.el.setPointerCapture(e.pointerId);
    }
    // 悬停目标：指针下最上层的方框（跳过自己）
    const el = document.elementsFromPoint(e.clientX, e.clientY).map((x) => (x as HTMLElement).closest<HTMLElement>("[data-box]")).find((x) => x && x.dataset.box !== dragging);
    if (el?.dataset.box) {
      const zone = zoneOf(el, e.clientY);
      const res = drag!.can(dragging, el.dataset.box, zone);
      over = { key: el.dataset.box, zone, ok: !!res?.ok, label: res?.label ?? "" };
    } else over = null;
    // 跟随指针的提示：固定定位，用 translate 设置位置（CSP 不允许 style 属性，CSSOM 可以）
    ghost?.style.setProperty("translate", `${e.clientX + 12}px ${e.clientY + 14}px`);
  }

  function onUp(e: PointerEvent) {
    if (!press || e.pointerId !== press.id) return;
    const key = press.key;
    if (dragging) {
      if (over?.ok) drag!.drop(dragging, over.key, over.zone);
    } else onSelect(key);
    press = null;
    dragging = null;
    over = null;
  }

  function onCancel() {
    press = null;
    dragging = null;
    over = null;
  }

  const dropCls = (key: string) => (over?.key !== key ? "" : !over.ok ? "nodrop" : over.zone === "before" ? "ins-before" : over.zone === "after" ? "ins-after" : "drop");
  const onKey = (e: KeyboardEvent, key: string) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onSelect(key);
    }
  };

  const mark = (key: string) => (focus ? (focus.has(key) ? "hot" : "dim") : "");
  const traced = (r: Rule) => !!trace && trace.hit === r.index;
</script>

<div class="map-scroll">
  <!-- 方框的指针事件只用于悬停高亮、选中与拖动；键盘用户用 Tab 选中后在详情里编辑 -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="map" class:dragging={!!dragging} bind:this={mapEl} onpointerleave={() => (hover = null)} onpointermove={onMove} onpointerup={onUp} onpointercancel={onCancel}>
    <svg class="links" class:focus={!!focus} aria-hidden="true">
      {#each paths as p (p.key)}<path class={p.kind} class:hot={p.hot} d={p.d} />{/each}
    </svg>
    <canvas class="fx" bind:this={canvas} aria-hidden="true"></canvas>
    <div class="cols" class:compact={plan.compact} bind:this={colsEl}>
      <div class="col lane"><div class="col-h"><span class="eyebrow">{t("routing.col.requests")}</span></div></div>

      <div class="col rules">
        <div class="col-h"><span class="eyebrow">{t("routing.col.rules")}</span>{#if drag}<span class="hint">{t("routing.drag_hint")}</span>{/if}</div>
        {#each rows as row (rowKey(row))}
          {@const key = rowKey(row)}
          {#if row.kind === "seg"}
            {@const rs = graph.rules.slice(row.run.from, row.run.to + 1)}
            <button class="row seg {mark(key)}" type="button" data-box={key} title={t("routing.seg_expand")} onpointerenter={() => (hover = key)} onfocus={() => (hover = key)} onclick={() => onExpand(row.run.id)}>
              <span class="pri">{row.run.from + 1}–{row.run.to + 1}</span>
              <span class="rmain">
                <span class="rtop"><span class="nm">{t("routing.seg_" + row.run.kind, { n: rs.length, target: targetLabel(row.run.target) })}</span><span class="meta">{fmtCount(rs.reduce((s, r) => s + (r.set?.count ?? 0), 0))}</span></span>
                <span class="mtags">{#each setBases(rs).slice(0, 4) as b (b)}<span class="mtag">{b}</span>{/each}{#if setBases(rs).length > 4}<span class="mtag">+{setBases(rs).length - 4}</span>{/if}</span>
              </span>
            </button>
          {:else}
            {@const rs = row.kind === "bundle" ? graph.rules.slice(row.from, row.to + 1) : [graph.rules[row.index]!]}
            {@const r = rs[0]!}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div
              class="row {mark(key)} {dropCls(key)}"
              class:sel={selected === key}
              class:traced={rs.some(traced)}
              class:raw={r.origin === "raw"}
              class:grab={!!drag?.draggable(key)}
              data-box={key}
              tabindex="0"
              title={catalog.label(r.set?.description?.text) || r.raw}
              onpointerenter={() => (hover = key)}
              onfocus={() => (hover = key)}
              onpointerdown={(e) => onDown(e, key)}
              onkeydown={(e) => onKey(e, key)}
            >
              <span class="pri">{row.kind === "bundle" ? `${row.from + 1}–${row.to + 1}` : r.index + 1}</span>
              <span class="rmain">
                <span class="rtop">
                  <span class="nm">{row.kind === "bundle" ? row.name : ruleName(r)}</span>
                  {#if row.kind === "bundle"}<span class="btag">{t("routing.bundle")}</span>{/if}
                  <span class="meta">{fmtCount(rs.reduce((s, x) => s + (x.set?.count ?? 0), 0))}</span>
                </span>
                {#if r.set || r.origin === "raw"}
                  <span class="mtags">
                    {#each rs as x (x.index)}{#if x.set}<span class="mtag">{x.set.behavior}</span>{/if}{/each}
                    {#if r.origin === "raw"}<span class="mtag raw">{t("routing.raw")}</span>{/if}
                    {#if rs.some((x) => x.set?.stale)}<span class="mtag warn">{t("routing.stale")}</span>{/if}
                  </span>
                {/if}
              </span>
            </div>
          {/if}
        {/each}
      </div>

      {#each groupCols as col, ci (ci)}
        <div class="col groups">
          <div class="col-h"><span class="eyebrow">{ci === 0 ? t("routing.col.groups") : t("routing.col.layer", { n: ci + 1 })}</span></div>
          {#each [...col, ...(ci === 0 ? unused : [])] as g (g.name)}
            {@const key = "group:" + g.name}
            {#if ci === 0 && g === unused[0]}<span class="unused-h">{t("routing.unused")}</span>{/if}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
            <div
              class="row {mark(key)} {dropCls(key)}"
              class:sel={selected === key}
              class:raw={g.origin === "raw"}
              class:dimmed={unused.includes(g)}
              class:grab={!!drag?.draggable(key)}
              data-box={key}
              tabindex="0"
              title={g.name + " · " + typeLabel(g.type)}
              onpointerenter={() => (hover = key)}
              onfocus={() => (hover = key)}
              onpointerdown={(e) => onDown(e, key)}
              onkeydown={(e) => onKey(e, key)}
            >
              <span class="nm">{g.name}</span>
              {#if g.lints.some((l) => l !== "group.latency_only" && l !== "group.unused")}<span class="lint-dot" title={t("routing.detail.has_lints")}></span>{/if}
              {#if plan.compact}<span class="tch">{typeChar(g.type)}</span>{:else}<span class="gtype">{typeLabel(g.type)}</span>{/if}
            </div>
          {/each}
        </div>
      {/each}

      <div class="col exits">
        <div class="col-h"><span class="eyebrow">{t("routing.col.exits")}</span></div>
        {#each plan.exits as e (e.id)}
          {@const key = "exit:" + e.id}
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <div
            class="row {mark(key)} {dropCls(key)}"
            class:sel={selected === key}
            class:builtin={e.kind === "builtin"}
            class:grab={!!drag?.draggable(key)}
            data-box={key}
            tabindex="0"
            title={e.filter ? e.label + " · " + e.filter : e.label}
            onpointerenter={() => (hover = key)}
            onfocus={() => (hover = key)}
            onpointerdown={(ev) => onDown(ev, key)}
            onkeydown={(ev) => onKey(ev, key)}
          >
            <span class="nm">{exitLabel(e)}</span>
            <span class="meta">
              {#if e.kind === "region" || e.kind === "source"}{t("routing.nodes_n", { n: e.members.length })}{:else if e.kind === "provider"}{t("routing.client_fetch")}{:else if e.via}{t("routing.via", { via: e.via })}{/if}
            </span>
          </div>
        {/each}
      </div>
      <div class="col lane"><div class="col-h"><span class="eyebrow">{t("routing.col.dest")}</span></div></div>
    </div>
  </div>
</div>
{#if dragging}
  <div class="drag-ghost" class:bad={!!over && !over.ok} bind:this={ghost} aria-hidden="true">{over?.label || t("routing.drag_none")}</div>
{/if}

<style>
  .map-scroll {
    overflow-x: auto;
  }
  .map {
    position: relative;
    padding: var(--space-3) var(--space-4) var(--space-4);
  }
  .links,
  .fx {
    position: absolute;
    inset: 0;
    inline-size: 100%;
    block-size: 100%;
    pointer-events: none;
  }
  .links {
    z-index: 0;
    overflow: visible;
  }
  .fx {
    z-index: 1;
  }
  .cols {
    position: relative;
    z-index: 2;
    display: grid;
    gap: 28px;
  }
  .col {
    display: grid;
    align-content: start;
    gap: 6px;
    min-inline-size: 0;
  }
  .col.lane .col-h {
    justify-content: center;
  }
  .col-h {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 6px;
    padding: 0 2px 4px;
    min-block-size: 22px;
  }
  .eyebrow {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    white-space: nowrap;
    letter-spacing: 0.02em;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    color: var(--fg);
    font: inherit;
    font-size: var(--text-sm);
    text-align: start;
    min-inline-size: 0;
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }
  .row:hover {
    border-color: var(--border-strong);
  }
  .row.hot {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .row.dim {
    opacity: 0.4;
  }
  .row.sel {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
  }
  .row.grab {
    cursor: grab;
    user-select: none;
    -webkit-user-select: none;
    touch-action: none;
  }
  .map.dragging .row {
    cursor: grabbing;
  }
  .row.drop {
    border-style: dashed;
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .row.nodrop {
    border-style: dashed;
    border-color: var(--err);
    background: var(--err-soft);
  }
  .row.ins-before {
    box-shadow: 0 -3px 0 0 var(--accent);
  }
  .row.ins-after {
    box-shadow: 0 3px 0 0 var(--accent);
  }
  .btag {
    font-size: 0.66rem;
    line-height: 1.5;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-fg);
    white-space: nowrap;
  }
  .lint-dot {
    inline-size: 6px;
    block-size: 6px;
    border-radius: 50%;
    background: var(--warn);
    flex: none;
  }
  .col-h .hint {
    font-size: 0.66rem;
    color: var(--fg-subtle);
    text-align: end;
  }
  /* 跟随指针的提示：指针坐标是物理坐标，所以这里用物理定位（由 translate 设置位置） */
  .drag-ghost {
    position: fixed;
    inset: 0 auto auto 0;
    z-index: 50;
    pointer-events: none;
    padding: 5px 10px;
    border-radius: var(--radius-md);
    background: var(--accent);
    color: var(--fg-on-accent);
    font-size: var(--text-xs);
    box-shadow: var(--shadow-lg);
    white-space: nowrap;
  }
  .drag-ghost.bad {
    background: var(--err);
  }
  .row.traced {
    box-shadow: 0 0 0 1px var(--accent);
  }
  .row.builtin {
    background: var(--bg-sunken);
  }
  .row.dimmed {
    opacity: 0.6;
  }
  .row.seg {
    background: var(--bg-sunken);
    border-style: dashed;
    cursor: pointer;
  }
  .row.seg .nm {
    color: var(--fg-muted);
  }
  .pri {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    min-inline-size: 1.5em;
    text-align: end;
    align-self: flex-start;
    padding-block-start: 1px;
    white-space: nowrap;
  }
  .rmain {
    flex: 1;
    min-inline-size: 0;
    display: grid;
    gap: 2px;
  }
  .rtop {
    display: flex;
    align-items: center;
    gap: 6px;
    min-inline-size: 0;
  }
  .nm {
    flex: 1;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
  .mtags {
    display: flex;
    gap: 3px;
    flex-wrap: wrap;
  }
  .mtag {
    font-family: var(--font-mono);
    font-size: 0.66rem;
    line-height: 1.5;
    padding: 0 5px;
    border-radius: 3px;
    background: var(--bg-sunken);
    color: var(--fg-subtle);
  }
  .mtag.raw {
    background: var(--raw-soft);
    color: var(--raw);
  }
  .mtag.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .gtype {
    font-size: var(--text-xs);
    padding: 0 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--fg-muted);
    white-space: nowrap;
  }
  .tch {
    font-size: var(--text-xs);
    inline-size: 18px;
    block-size: 18px;
    display: inline-grid;
    place-items: center;
    border-radius: 4px;
    background: var(--bg-sunken);
    color: var(--fg-muted);
    flex: none;
  }
  .cols.compact .groups .row {
    padding: 5px 8px;
    gap: 6px;
  }
  .unused-h {
    margin-block-start: var(--space-3);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .links path {
    fill: none;
    stroke: var(--border-strong);
    stroke-width: 1.4;
    transition:
      stroke var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }
  .links path.alt {
    stroke-dasharray: 3 4;
  }
  .links path.cand {
    stroke-dasharray: 2 5;
    opacity: 0.85;
  }
  .links path.hot {
    stroke: var(--accent);
    stroke-width: 2;
  }
  .links.focus path:not(.hot) {
    opacity: 0.28;
  }
</style>
