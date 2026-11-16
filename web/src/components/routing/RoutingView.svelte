<script lang="ts">
  // 配置编辑器的「分流」分节（routing 第 1、11 节）：分流地图、流向追踪、常用网站体检与讲解图层。只读（编辑在第 3 步）。
  // 图按编辑器当前的方案文档（含未保存的修改）在服务端计算；规则内容未缓存时服务端在后台拉取，这里稍后再取。
  import { onMount, untrack } from "svelte";
  import Icon from "../Icon.svelte";
  import ExplainLayer from "./ExplainLayer.svelte";
  import HealthDialog from "./HealthDialog.svelte";
  import RoutingList from "./RoutingList.svelte";
  import RoutingMap from "./RoutingMap.svelte";
  import TracePanel from "./TracePanel.svelte";
  import { api } from "../../lib/api/client";
  import type { EditorState } from "../../lib/editor.svelte";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { prefs } from "../../lib/prefs.svelte";
  import type { Graph, Trace } from "../../lib/routing/layout";
  import { planView, ruleRows, type RoutingPrefs } from "../../lib/routing/plan";
  import type { ParticleMode } from "../../lib/routing/particles";

  let { editor, initialTrace = "" }: { editor: EditorState; initialTrace?: string } = $props();

  let graph = $state.raw<Graph | null>(null);
  let error = $state("");
  let width = $state(1200);
  let expanded = $state(new Set<string>());
  let trace = $state.raw<Trace | null>(null);
  let traceHost = $state(""); // 由下面的 effect 按 initialTrace 设置
  let explainOpen = $state(false);
  let healthOpen = $state(false);
  let panel: TracePanel | undefined = $state();

  const reduce = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
  const rv = $derived<RoutingPrefs>((prefs.data as { routingView?: RoutingPrefs }).routingView ?? {});
  const particles = $derived<ParticleMode>(reduce ? "off" : (rv.particles ?? "all"));
  const plan = $derived(graph ? planView(graph, width, rv) : null);
  const hits = $derived(trace ? [trace.hit, ...(trace.needsIp?.candidates ?? [])] : []);
  const rows = $derived(graph && plan ? ruleRows(graph, plan, expanded, hits) : []);
  const doc = () => editor.text;

  // 取图：文档变化后稍候再取；服务端仍在拉取规则内容时每 2 秒再取一次（最多 2 分钟）
  let seq = 0;
  let polls = 0;
  let tracedDoc = "";
  let timer: ReturnType<typeof setTimeout> | undefined;
  async function load() {
    const my = ++seq;
    try {
      const g = await api.post<Graph>(`/api/v1/profiles/${editor.profile!.id}/routing/graph`, { doc: editor.text });
      if (my !== seq) return;
      graph = g;
      error = "";
      clearTimeout(timer);
      if (g.pending > 0 && polls++ < 60) timer = setTimeout(() => void load(), 2000);
      // 文档变了且正在追踪：对新文档重新追踪同一个目标（后台轮询不重复追踪，避免占用追踪配额）
      if (traceHost && panel && tracedDoc !== editor.text) {
        tracedDoc = editor.text;
        void panel.run(traceHost);
      }
    } catch (e) {
      if (my === seq) error = errorText(e);
    }
  }
  $effect(() => {
    void editor.text;
    polls = 0;
    clearTimeout(timer);
    timer = setTimeout(() => void load(), graph ? 600 : 0);
  });

  async function savePref(patch: RoutingPrefs) {
    await prefs.save({ ...prefs.data, routingView: { ...rv, ...patch } } as typeof prefs.data);
  }

  // 任何一次追踪（包括用户手动发起的）都记下当时的文档：之后只有文档变了才自动重新追踪
  $effect(() => {
    void trace;
    untrack(() => (tracedDoc = editor.text));
  });

  // 已在分流分节时从命令面板再次发起追踪
  $effect(() => {
    const h = initialTrace;
    untrack(() => {
      if (h && h !== traceHost) {
        traceHost = h;
        void panel?.run(h);
      }
    });
  });

  // 追踪框第一次出现时（例如从命令面板带着目标进入）开始追踪
  $effect(() => {
    const p = panel;
    untrack(() => {
      if (p && traceHost && tracedDoc !== editor.text) {
        tracedDoc = editor.text;
        void p.run(traceHost);
      }
    });
  });

  onMount(() => {
    return () => clearTimeout(timer);
  });

  const PARTICLE_MODES: ParticleMode[] = ["all", "focus", "off"];
</script>

<section class="routing" aria-labelledby="routing-h" bind:clientWidth={width}>
  <header class="head">
    <div>
      <div class="title">
        <h2 id="routing-h">{t("routing.title")}</h2>
        <button class="btn ghost icon sm" type="button" aria-label={t("routing.explain.open")} title={t("routing.explain.open")} onclick={() => (explainOpen = true)}><Icon name="help" size={15} /></button>
      </div>
      <p class="muted sub">{t("routing.sub")}</p>
    </div>
    {#if graph?.traceEnabled}
      <button class="btn sm" type="button" onclick={() => (healthOpen = true)}><Icon name="ok" size={14} />{t("routing.health.open")}</button>
    {/if}
  </header>

  {#if error}
    <p class="err" role="alert">{error}</p>
  {:else if !graph || !plan}
    <p class="muted" aria-busy="true">{t("common.loading")}</p>
  {:else}
    {#if graph.readonlyReason === "no_rules"}
      <p class="note">{t("routing.no_rules")}</p>
    {:else if graph.readonlyReason === "raw_override"}
      <p class="note raw">{t("routing.raw_override")}</p>
    {/if}

    <div class="card">
      <div class="toolbar">
        {#if plan.mode === "map"}
          <div class="seg" role="radiogroup" aria-label={t("routing.pref.rules")}>
            {#each ["auto", "all"] as const as m (m)}
              <button type="button" role="radio" aria-checked={(rv.rules ?? "auto") === m} class:on={(rv.rules ?? "auto") === m} onclick={() => ((expanded = new Set()), void savePref({ rules: m }))}>{t("routing.pref.rules_" + m)}</button>
            {/each}
          </div>
          <div class="seg" role="radiogroup" aria-label={t("routing.pref.particles")} title={reduce ? t("routing.pref.reduced") : ""}>
            {#each PARTICLE_MODES as m (m)}
              <button type="button" role="radio" aria-checked={particles === m} class:on={particles === m} disabled={reduce && m !== "off"} onclick={() => void savePref({ particles: m })}>{t("routing.pref.particles_" + m)}</button>
            {/each}
          </div>
          <button class="btn ghost sm" type="button" aria-pressed={!!rv.candidates} onclick={() => void savePref({ candidates: !rv.candidates })}>{t("routing.pref.candidates")}</button>
        {/if}
        <span class="grow"></span>
        <span class="legend" aria-hidden="true">
          <span><i class="d lp"></i>{t("routing.legend.proxy")}</span>
          <span><i class="d ld"></i>{t("routing.legend.direct")}</span>
          <span><i class="d lr"></i>{t("routing.legend.reject")}</span>
          <span><i class="d ln"></i>{t("routing.legend.none")}</span>
        </span>
      </div>
      <div class="viewnote">
        <b>{t("routing.why.title")}</b>
        {#each plan.why as w (w.key)}<span class="why">{t("routing.why." + w.key, w.params)}</span>{/each}
        {#if graph.pending}<span class="why pending">{t("routing.pending", { n: graph.pending })}</span>{/if}
      </div>
      {#if plan.mode === "map"}
        <RoutingMap {graph} {plan} {rows} {trace} {particles} candidates={!!rv.candidates} onExpand={(id) => (expanded = new Set([...expanded, id]))} />
      {:else}
        <RoutingList {graph} {plan} {rows} {trace} onExpand={(id) => (expanded = new Set([...expanded, id]))} />
      {/if}
      <p class="caption">{t("routing.caption")}</p>
    </div>

    {#if graph.traceEnabled}
      <div class="card pad">
        <TracePanel bind:this={panel} {graph} profileId={editor.profile!.id} {doc} bind:trace bind:host={traceHost} />
      </div>
    {:else}
      <p class="note">{t("routing.trace_off")}</p>
    {/if}
  {/if}
</section>

<ExplainLayer bind:open={explainOpen} />
{#if editor.profile}
  <HealthDialog bind:open={healthOpen} profileId={editor.profile.id} {doc} onTrace={(h) => void panel?.run(h)} />
{/if}

<style>
  .routing {
    display: grid;
    gap: var(--space-4);
    min-inline-size: 0;
  }
  .head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .title {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }
  h2 {
    font-size: var(--text-xl);
  }
  .sub {
    font-size: var(--text-sm);
    margin-block-start: 2px;
    max-inline-size: 70ch;
  }
  .err {
    color: var(--err);
  }
  .note {
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--info-soft);
    color: var(--info);
  }
  .note.raw {
    background: var(--raw-soft);
    color: var(--raw);
  }
  .card {
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    min-inline-size: 0;
    overflow: hidden;
  }
  .card.pad {
    padding: var(--space-4);
  }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-block-end: 1px solid var(--border);
    background: var(--bg-sunken);
    font-size: var(--text-sm);
  }
  .grow {
    flex: 1;
  }
  .seg {
    display: inline-flex;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    overflow: hidden;
  }
  .seg button {
    font: inherit;
    font-size: var(--text-xs);
    padding: 4px 10px;
    border: 0;
    background: var(--bg);
    color: var(--fg-muted);
    cursor: pointer;
  }
  .seg button + button {
    border-inline-start: 1px solid var(--border-strong);
  }
  .seg button.on {
    background: var(--accent);
    color: var(--fg-on-accent);
  }
  .seg button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  [aria-pressed="true"] {
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .legend {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .legend span {
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .d {
    inline-size: 8px;
    block-size: 8px;
    border-radius: 50%;
    display: inline-block;
  }
  .lp {
    background: var(--flow-proxy);
  }
  .ld {
    background: var(--flow-direct);
  }
  .lr {
    background: var(--flow-reject);
  }
  .ln {
    background: var(--flow-none);
  }
  .viewnote {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 8px;
    padding: 6px var(--space-3);
    border-block-end: 1px solid var(--border);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .viewnote b {
    color: var(--fg-muted);
    font-weight: 600;
  }
  .why {
    padding: 1px 8px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  .why.pending {
    background: var(--info-soft);
    color: var(--info);
  }
  .caption {
    padding: var(--space-2) var(--space-3) var(--space-3);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    max-inline-size: 110ch;
  }
</style>
