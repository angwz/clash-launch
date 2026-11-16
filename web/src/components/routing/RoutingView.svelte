<script lang="ts">
  // 配置编辑器的「分流」分节（routing 第 1、11 节）：分流地图、流向追踪、常用网站体检、讲解图层与编辑。
  // 图按编辑器当前的方案文档（含未保存的修改）在服务端计算；有草稿时按「文档 + 草稿的 routing 段」计算，
  // 应用前就能在地图与追踪里看到效果。规则内容未缓存时服务端在后台拉取，这里稍后再取。
  //
  // 编辑（第 11.2 节）：跟随上游时第一次改动先取 /routing/fork 作为基线；改动都进草稿，「应用」才写入 routing 段。
  import { onMount, untrack } from "svelte";
  import { parse as parseYaml, stringify } from "yaml";
  import Icon from "../Icon.svelte";
  import DetailPanel from "./DetailPanel.svelte";
  import DraftBar from "./DraftBar.svelte";
  import ExplainLayer from "./ExplainLayer.svelte";
  import HealthDialog from "./HealthDialog.svelte";
  import RoutingList from "./RoutingList.svelte";
  import RoutingMap, { type DragOps } from "./RoutingMap.svelte";
  import TracePanel from "./TracePanel.svelte";
  import UpstreamDialog from "./UpstreamDialog.svelte";
  import { api } from "../../lib/api/client";
  import type { EditorState } from "../../lib/editor.svelte";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { prefs } from "../../lib/prefs.svelte";
  import { ProfileDoc } from "../../lib/profile-doc";
  import { addCheck, addMember, applyUpstream, changes, merge, metaChanges, move, removeCheck, setTarget, setType, type GroupType, type Routing } from "../../lib/routing/draft";
  import { builtinLabel } from "../../lib/routing/labels";
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

  // —— 草稿 ——
  let draft = $state.raw<Routing | null>(null); // 尚未应用的 routing；null 表示没有改动
  let baseline = $state.raw<Routing | null>(null); // 草稿的基线：已应用的 routing，或跟随上游时 fork 得到的
  let stash: Record<string, Record<string, unknown>> = {}; // 改类型时暂存的参数，切回时取回
  let selected = $state<string | null>(null);
  let editError = $state("");

  const applied = $derived(editor.routing);
  const changeList = $derived(draft && baseline ? [...changes(baseline, draft), ...metaChanges(baseline, draft)] : []);
  let upstreamOpen = $state(false);
  const pending = $derived(changeList.length > 0);
  // 地图与追踪用的文档：有改动时把草稿写进 routing 段
  const graphDoc = $derived.by(() => {
    if (!draft || !pending) return editor.text;
    const d = new ProfileDoc(editor.text);
    d.routing = draft;
    return d.toString();
  });
  const draftText = () => (draft && pending ? stringify(draft, { lineWidth: 0 }) : undefined);

  const reduce = typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
  const rv = $derived<RoutingPrefs>((prefs.data as { routingView?: RoutingPrefs }).routingView ?? {});
  const particles = $derived<ParticleMode>(reduce ? "off" : (rv.particles ?? "all"));
  const plan = $derived(graph ? planView(graph, width, rv) : null);
  const hits = $derived(trace ? [trace.hit, ...(trace.needsIp?.candidates ?? [])] : []);
  const rows = $derived(graph && plan ? ruleRows(graph, plan, expanded, hits) : []);
  const doc = () => editor.text;

  // 取图：文档或草稿变化后稍候再取；服务端仍在拉取规则内容时每 2 秒再取一次（最多 2 分钟）
  let seq = 0;
  let polls = 0;
  let tracedDoc = "";
  let timer: ReturnType<typeof setTimeout> | undefined;
  async function load() {
    const my = ++seq;
    const docNow = graphDoc;
    try {
      const g = await api.post<Graph>(`/api/v1/profiles/${editor.profile!.id}/routing/graph`, { doc: docNow });
      if (my !== seq) return;
      graph = g;
      error = "";
      clearTimeout(timer);
      if (g.pending > 0 && polls++ < 60) timer = setTimeout(() => void load(), 2000);
      // 文档或草稿变了且正在追踪：重新追踪同一个目标（后台轮询不重复追踪，避免占用追踪配额）
      if (traceHost && panel && tracedDoc !== docNow) {
        tracedDoc = docNow;
        void panel.run(traceHost);
      }
    } catch (e) {
      if (my === seq) error = errorText(e);
    }
  }
  $effect(() => {
    void graphDoc;
    polls = 0;
    clearTimeout(timer);
    timer = setTimeout(() => void load(), graph ? 400 : 0);
  });

  /** 草稿的当前值；第一次改动时取基线（已应用的 routing，或 fork 规则源的当前内容）。 */
  async function current(): Promise<Routing | null> {
    if (draft) return draft;
    if (applied) {
      baseline = applied;
      return applied;
    }
    try {
      const res = await api.post<{ routing: string }>(`/api/v1/profiles/${editor.profile!.id}/routing/fork`, { doc: editor.text });
      const r = parseYaml(res.routing) as Routing;
      baseline = r;
      return r;
    } catch (e) {
      editError = errorText(e);
      return null;
    }
  }

  /** fail 是不能执行时显示的文字（已翻译）。 */
  async function edit(op: (r: Routing) => Routing | null, fail?: string) {
    editError = "";
    const cur = await current();
    if (!cur) return;
    const next = op(cur);
    if (!next) {
      editError = fail ?? t("routing.err.generic");
      return;
    }
    draft = next;
  }

  async function changeType(group: string, type: GroupType) {
    await edit((r) => {
      const res = setType(r, group, type, stash[group]);
      stash = { ...stash, [group]: res.stash };
      return res.routing;
    });
  }

  function cancel() {
    draft = null;
    baseline = null;
    stash = {};
    editError = "";
  }

  function apply() {
    if (!draft) return;
    editor.setRouting(draft); // 之后照常自动保存、预览与内核检查；编辑器的撤销提示照常可用
    draft = null;
    baseline = null;
    stash = {};
  }

  // —— 拖动（第 11.2 节）——
  const BUILTIN_TARGETS = ["DIRECT", "REJECT", "REJECT-DROP"];
  const ruleEntry = (key: string): number | undefined => {
    const m = /^(rule|bundle):(\d+)$/.exec(key);
    return m ? graph?.rules[Number(m[2])]?.entry : undefined;
  };
  const exitRowOf = (key: string) => plan?.exits.find((e) => "exit:" + e.id === key);
  // 判断能否放下用草稿或已应用的 routing；跟随上游且还没有改动时只按地图数据粗判，真正执行时再取基线
  const probe = $derived(draft ?? applied ?? null);
  const ruleByEntry = (e: number) => graph?.rules.find((r) => r.entry === e);

  const drag: DragOps = {
    draggable: (key) => {
      if (!graph?.editable) return false;
      if (/^(rule|bundle):/.test(key)) {
        const e = ruleEntry(key);
        return e !== undefined && ruleByEntry(e)?.type !== "MATCH";
      }
      if (key.startsWith("group:")) return graph.groups.find((g) => "group:" + g.name === key)?.origin !== "raw";
      return key.startsWith("exit:");
    },
    can: (src, dst, zone) => {
      const se = ruleEntry(src);
      if (se !== undefined) {
        if (dst.startsWith("group:") || (dst.startsWith("exit:") && BUILTIN_TARGETS.includes(exitRowOf(dst)?.label ?? ""))) {
          const to = dst.startsWith("group:") ? dst.slice(6) : exitRowOf(dst)!.label;
          return ruleByEntry(se)?.target === to ? { ok: false, label: t("routing.drag.same") } : { ok: true, label: t("routing.drag.target", { to: builtinLabel(to) }) };
        }
        const de = ruleEntry(dst);
        if (de === undefined) return { ok: false, label: t("routing.drag.no") };
        if (zone === "mid") {
          const ok = probe ? merge(probe, se, de) !== null : !!ruleByEntry(de)?.set && !!ruleByEntry(se)?.set;
          return ok ? { ok: true, label: t("routing.drag.merge") } : { ok: false, label: t("routing.drag.no_merge") };
        }
        const ok = probe ? move(probe, se, de, zone === "after") !== null : !(zone === "after" && ruleByEntry(de)?.type === "MATCH");
        return ok ? { ok: true, label: zone === "after" ? t("routing.drag.after") : t("routing.drag.before") } : { ok: false, label: t("routing.drag.no_move") };
      }
      // 组或出口 → 组：加入成员
      if (!dst.startsWith("group:")) return { ok: false, label: t("routing.drag.no") };
      const group = dst.slice(6);
      const member = src.startsWith("group:") ? src.slice(6) : memberOf(exitRowOf(src));
      if (!member) return { ok: false, label: t("routing.drag.no_member") };
      if (probe && addMember(probe, group, member) === null) return { ok: false, label: t("routing.drag.cycle") };
      return { ok: true, label: t("routing.drag.member", { to: group }) };
    },
    drop: (src, dst, zone) => {
      const se = ruleEntry(src);
      if (se !== undefined) {
        const de = ruleEntry(dst);
        if (dst.startsWith("group:")) void edit((r) => setTarget(r, se, dst.slice(6)));
        else if (dst.startsWith("exit:")) void edit((r) => setTarget(r, se, exitRowOf(dst)!.label));
        else if (de !== undefined && zone === "mid") void edit((r) => merge(r, se, de), t("routing.err.merge"));
        else if (de !== undefined) void edit((r) => move(r, se, de, zone === "after"), t("routing.err.move"));
        return;
      }
      const member = src.startsWith("group:") ? src.slice(6) : memberOf(exitRowOf(src));
      if (member && dst.startsWith("group:")) void edit((r) => addMember(r, dst.slice(6), member), t("routing.err.cycle"));
    },
  };

  /** 出口行加入组时的成员名：单个节点、内置出口直接写名字；按地区或来源归并的行代表多个节点，不能整体加入。 */
  function memberOf(row: ReturnType<typeof exitRowOf>): string | undefined {
    if (!row) return undefined;
    if (row.kind === "node" || row.kind === "builtin") return row.members[0];
    return undefined;
  }

  // 改动后选中项可能不存在了（例如合集拆散后行号变化）
  $effect(() => {
    void rows;
    untrack(() => {
      if (selected && !rowExists(selected)) selected = null;
    });
  });
  function rowExists(key: string): boolean {
    if (!graph || !plan) return false;
    if (key.startsWith("group:")) return graph.groups.some((g) => "group:" + g.name === key);
    if (key.startsWith("exit:")) return plan.exits.some((e) => "exit:" + e.id === key);
    return rows.some((r) => (r.kind === "rule" ? "rule:" + r.index : r.kind === "bundle" ? "bundle:" + r.from : "") === key);
  }

  async function savePref(patch: RoutingPrefs) {
    await prefs.save({ ...prefs.data, routingView: { ...rv, ...patch } } as typeof prefs.data);
  }

  // 任何一次追踪（包括用户手动发起的）都记下当时的文档：之后只有文档变了才自动重新追踪
  $effect(() => {
    void trace;
    untrack(() => (tracedDoc = graphDoc));
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
      if (p && traceHost && tracedDoc !== graphDoc) {
        tracedDoc = graphDoc;
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
        <RoutingMap {graph} {plan} {rows} {trace} {particles} candidates={!!rv.candidates} onExpand={(id) => (expanded = new Set([...expanded, id]))} {selected} onSelect={(k) => (selected = selected === k ? null : k)} drag={graph.editable ? drag : undefined} />
      {:else}
        <RoutingList {graph} {plan} {rows} {trace} onExpand={(id) => (expanded = new Set([...expanded, id]))} />
      {/if}
      {#if graph.upstreamChanges && !pending}
        <div class="upstream" role="status">
          <span>{t("routing.upstream.banner", { n: graph.upstreamChanges })}</span>
          <button class="btn sm" type="button" onclick={() => (upstreamOpen = true)}>{t("routing.upstream.review")}</button>
        </div>
      {/if}
      {#if pending}
        <DraftBar changes={changeList} own={!!applied} error={editError} onCancel={cancel} onApply={apply} />
      {:else if editError}
        <p class="edit-err" role="alert">{editError}</p>
      {/if}
      <p class="caption">{graph.editable ? t("routing.caption_edit") : t("routing.caption")}</p>
    </div>

    <div class="panes" class:two={!!selected}>
      {#if graph.traceEnabled}
        <div class="card pad">
          <TracePanel bind:this={panel} {graph} profileId={editor.profile!.id} {doc} draft={draftText} bind:trace bind:host={traceHost} onPin={graph.editable ? (h, via) => void edit((r) => addCheck(r, h, via)) : undefined} />
          {#if graph.checks.length}
            <div class="checks">
              <h4>{t("routing.checks.title")}</h4>
              <ul>
                {#each graph.checks as c (c.host)}
                  <li class={c.ok === undefined ? "unk" : c.ok ? "ok" : "bad"}>
                    <span class="st">{c.ok === undefined ? "?" : c.ok ? "✓" : "✗"}</span>
                    <span class="grow"><code>{c.host}</code> → {builtinLabel(c.via)}{#if c.ok === false && c.path.length} <span class="got">{t("routing.checks.got", { got: builtinLabel(c.path[0]!) })}</span>{/if}</span>
                    {#if graph.editable}<button class="btn ghost sm" type="button" onclick={() => void edit((r) => removeCheck(r, c.host))}>{t("common.remove")}</button>{/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </div>
      {:else}
        <p class="note">{t("routing.trace_off")}</p>
      {/if}
      {#if selected}
        <div class="card pad">
          <DetailPanel {graph} {plan} {selected} routing={draft ?? applied} editable={graph.editable} onEdit={(op, fail) => void edit(op, fail)} onType={(g, ty) => void changeType(g, ty)} onTrace={(h) => void panel?.run(h)} onClose={() => (selected = null)} />
        </div>
      {/if}
    </div>
  {/if}
</section>

<ExplainLayer bind:open={explainOpen} />
{#if editor.profile}
  <UpstreamDialog bind:open={upstreamOpen} profileId={editor.profile.id} doc={() => graphDoc} onAccept={(up, accepted) => void edit((r) => applyUpstream(r, up, accepted, new Date().toISOString()))} />
{/if}
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
  .panes {
    display: grid;
    gap: var(--space-4);
    align-items: start;
  }
  .panes.two {
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
  }
  .edit-err {
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-sm);
    color: var(--err);
    border-block-start: 1px solid var(--border);
  }
  .upstream {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-3);
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-block-start: 1px solid var(--border);
    background: var(--info-soft);
    color: var(--info);
    font-size: var(--text-sm);
  }
  .checks {
    margin-block-start: var(--space-4);
    padding-block-start: var(--space-3);
    border-block-start: 1px solid var(--border);
    display: grid;
    gap: var(--space-2);
  }
  .checks h4 {
    font-size: var(--text-sm);
    font-weight: 600;
  }
  .checks ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 4px;
  }
  .checks li {
    display: flex;
    gap: 8px;
    align-items: center;
    font-size: var(--text-sm);
  }
  .checks .st {
    inline-size: 1.2em;
    text-align: center;
    font-weight: 700;
  }
  .checks li.ok .st {
    color: var(--ok);
  }
  .checks li.bad .st,
  .checks .got {
    color: var(--err);
  }
  .checks li.unk .st {
    color: var(--fg-subtle);
  }
  .checks .grow {
    flex: 1;
    min-inline-size: 0;
    overflow-wrap: anywhere;
  }
  .caption {
    padding: var(--space-2) var(--space-3) var(--space-3);
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    max-inline-size: 110ch;
  }
</style>
