<script lang="ts">
  // 分流地图（routing 第 8、10 节）：请求 → 规则 → 策略组（按默认路径分列）→ 出口 → 目的地。
  // 布局与连线由 lib/routing 的纯函数计算；这里只负责把方框放进 DOM、量出位置、画 SVG 轨道与粒子。
  // 站内 CSP 不允许 style 属性：列宽与最小宽度用 CSSOM（style.setProperty）设置，几何只写进 SVG 的 d。
  import { onMount, untrack } from "svelte";
  import { catalog } from "../../lib/catalog.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { defaultPath, links, type Box, type Graph, type Rule, type Trace } from "../../lib/routing/layout";
  import { ruleBoxOf, type RuleRow, type ViewPlan } from "../../lib/routing/plan";
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
  }: {
    graph: Graph;
    plan: ViewPlan;
    rows: RuleRow[];
    trace: Trace | null;
    particles: ParticleMode;
    candidates: boolean;
    onExpand: (id: string) => void;
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
    if (!hover) return null;
    const s = new Set([hover]);
    if (hover.startsWith("rule:") || hover.startsWith("seg:")) {
      for (const r of graph.rules) if (ruleBox(r.index) === hover) for (const b of pathBoxes(r.target)) s.add(b);
    } else if (hover.startsWith("group:")) {
      const name = hover.slice(6);
      const g = graph.groups.find((x) => x.name === name);
      for (const m of g?.members ?? []) s.add(m.kind === "group" ? "group:" + m.name : "exit:" + exitRow(m.name));
      for (const r of graph.rules) if (r.target === name) s.add(ruleBox(r.index));
      for (const p of graph.groups) if (p.members.some((m) => m.name === name)) s.add("group:" + p.name);
    } else if (hover.startsWith("exit:")) {
      const row = plan.exits.find((e) => "exit:" + e.id === hover);
      for (const n of row?.members ?? []) {
        for (const r of graph.rules) if (r.target === n) s.add(ruleBox(r.index));
        for (const p of graph.groups) if (p.members.some((m) => m.name === n)) s.add("group:" + p.name);
      }
    }
    return s;
  });

  const focusedGroup = $derived(hover?.startsWith("group:") ? hover.slice(6) : null);
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
  const sceneKey = $derived(JSON.stringify([sceneSamples, plan.particleCap, plan.labelProb, rows.map((r) => (r.kind === "seg" ? r.run.id : r.index))]));
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

  const mark = (key: string) => (focus ? (focus.has(key) ? "hot" : "dim") : "");
  const traced = (r: Rule) => !!trace && trace.hit === r.index;
</script>

<div class="map-scroll">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="map" bind:this={mapEl} onpointerleave={() => (hover = null)}>
    <svg class="links" class:focus={!!focus} aria-hidden="true">
      {#each paths as p (p.key)}<path class={p.kind} class:hot={p.hot} d={p.d} />{/each}
    </svg>
    <canvas class="fx" bind:this={canvas} aria-hidden="true"></canvas>
    <div class="cols" class:compact={plan.compact} bind:this={colsEl}>
      <div class="col lane"><div class="col-h"><span class="eyebrow">{t("routing.col.requests")}</span></div></div>

      <div class="col rules">
        <div class="col-h"><span class="eyebrow">{t("routing.col.rules")}</span></div>
        {#each rows as row (row.kind === "seg" ? row.run.id : row.index)}
          {#if row.kind === "seg"}
            {@const rs = graph.rules.slice(row.run.from, row.run.to + 1)}
            <button class="row seg {mark('seg:' + row.run.id)}" type="button" data-box={"seg:" + row.run.id} title={t("routing.seg_expand")} onpointerenter={() => (hover = "seg:" + row.run.id)} onfocus={() => (hover = "seg:" + row.run.id)} onclick={() => onExpand(row.run.id)}>
              <span class="pri">{row.run.from + 1}–{row.run.to + 1}</span>
              <span class="rmain">
                <span class="rtop"><span class="nm">{t("routing.seg_" + row.run.kind, { n: rs.length, target: targetLabel(row.run.target) })}</span><span class="meta">{fmtCount(rs.reduce((s, r) => s + (r.set?.count ?? 0), 0))}</span></span>
                <span class="mtags">{#each setBases(rs).slice(0, 4) as b (b)}<span class="mtag">{b}</span>{/each}{#if setBases(rs).length > 4}<span class="mtag">+{setBases(rs).length - 4}</span>{/if}</span>
              </span>
            </button>
          {:else}
            {@const r = graph.rules[row.index]!}
            <div class="row {mark('rule:' + r.index)}" class:traced={traced(r)} class:raw={r.origin === "raw"} data-box={"rule:" + r.index} title={catalog.label(r.set?.description?.text) || r.raw} onpointerenter={() => (hover = "rule:" + r.index)} onfocus={() => (hover = "rule:" + r.index)}>
              <span class="pri">{r.index + 1}</span>
              <span class="rmain">
                <span class="rtop"><span class="nm">{ruleName(r)}</span><span class="meta">{r.set?.count !== undefined ? fmtCount(r.set.count) : ""}</span></span>
                {#if r.set || r.origin === "raw"}
                  <span class="mtags">
                    {#if r.set}<span class="mtag">{r.set.behavior}</span>{/if}
                    {#if r.origin === "raw"}<span class="mtag raw">{t("routing.raw")}</span>{/if}
                    {#if r.set?.stale}<span class="mtag warn">{t("routing.stale")}</span>{/if}
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
          {#each col as g (g.name)}
            <div class="row {mark('group:' + g.name)}" class:raw={g.origin === "raw"} data-box={"group:" + g.name} title={g.name + " · " + typeLabel(g.type)} onpointerenter={() => (hover = "group:" + g.name)} onfocus={() => (hover = "group:" + g.name)}>
              <span class="nm">{g.name}</span>
              {#if plan.compact}<span class="tch">{typeChar(g.type)}</span>{:else}<span class="gtype">{typeLabel(g.type)}</span>{/if}
            </div>
          {/each}
          {#if ci === 0 && unused.length}
            <span class="unused-h">{t("routing.unused")}</span>
            {#each unused as g (g.name)}
              <div class="row dimmed {mark('group:' + g.name)}" data-box={"group:" + g.name} title={g.name} onpointerenter={() => (hover = "group:" + g.name)} onfocus={() => (hover = "group:" + g.name)}>
                <span class="nm">{g.name}</span><span class="tch">{typeChar(g.type)}</span>
              </div>
            {/each}
          {/if}
        </div>
      {/each}

      <div class="col exits">
        <div class="col-h"><span class="eyebrow">{t("routing.col.exits")}</span></div>
        {#each plan.exits as e (e.id)}
          <div class="row {mark('exit:' + e.id)}" class:builtin={e.kind === "builtin"} data-box={"exit:" + e.id} title={e.filter ? e.label + " · " + e.filter : e.label} onpointerenter={() => (hover = "exit:" + e.id)} onfocus={() => (hover = "exit:" + e.id)}>
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
