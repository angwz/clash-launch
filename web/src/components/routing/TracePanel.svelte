<script lang="ts">
  // 流向追踪（routing 第 1 节 #3、第 11.1 节）：输入域名或 IP，服务器逐条比对，这里逐层列出依据。
  // 追踪框有内容时悬停或聚焦出现 ×，点它或按 Esc 清空并结束追踪。
  import Icon from "../Icon.svelte";
  import { api, ApiError } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { defaultPath, type Graph, type Trace } from "../../lib/routing/layout";
  import { builtinLabel, ruleName, typeLabel } from "../../lib/routing/labels";

  let {
    graph,
    profileId,
    doc,
    draft = () => undefined,
    trace = $bindable(null),
    host = $bindable(""),
  }: { graph: Graph; profileId: string; doc: () => string; draft?: () => string | undefined; trace?: Trace | null; host?: string } = $props();

  let input = $state(host);
  let process = $state("");
  let udp = $state(false);
  let busy = $state(false);
  let error = $state("");
  let inputEl: HTMLInputElement | undefined = $state();
  let seq = 0; // 只采用最后一次请求的结果：先发出的请求可能后返回
  let before = $state.raw<Trace | null>(null); // 有草稿时：应用前的追踪结果

  // 示例：取几个规则集的样例（服务器从规则内容中取出），不同目标各取一个
  const examples = $derived.by(() => {
    const out: string[] = [];
    const seen = new Set<string>();
    for (const r of graph.rules) {
      const s = r.set?.samples?.[0];
      if (!s || seen.has(r.target) || out.includes(s.host)) continue;
      seen.add(r.target);
      out.push(s.host);
      if (out.length >= 5) break;
    }
    return out;
  });

  export async function run(h = input) {
    const q = h.trim();
    if (!q) return clear();
    input = q;
    const my = ++seq;
    busy = true;
    error = "";
    try {
      const res = await api.post<{ current: Trace; draft?: Trace }>(`/api/v1/profiles/${profileId}/routing/trace`, { doc: doc(), draft: draft(), host: q, process: process.trim() || undefined, network: udp ? "udp" : "tcp" });
      if (my !== seq) return;
      // 有草稿时地图画的是草稿，所以逐步卡片也按草稿；应用前的结果用于对比
      trace = res.draft ?? res.current;
      before = res.draft ? res.current : null;
      host = q;
    } catch (e) {
      if (my !== seq) return;
      trace = null;
      error = e instanceof ApiError && e.type === "invalid_host" ? t("routing.trace.invalid") : errorText(e);
    } finally {
      if (my === seq) busy = false;
    }
  }

  function clear() {
    input = "";
    host = "";
    trace = null;
    before = null;
    error = "";
    inputEl?.focus();
  }

  // 地图上的规则名、组类型等
  const rule = (i: number) => graph.rules[i];
  const misses = $derived(trace ? trace.steps.filter((s) => s.index !== trace!.hit) : []);
  const unknown = $derived(misses.filter((s) => s.result === "unknown"));
  const hitRule = $derived(trace && trace.hit >= 0 ? rule(trace.hit) : undefined);
  const branch = $derived.by(() => {
    if (!trace?.needsIp) return null;
    const at = trace.needsIp.candidates.find((c) => c !== trace!.hit);
    if (at === undefined) return null;
    const r = rule(at);
    if (!r) return null;
    const p = defaultPath(graph, r.target, (n) => n);
    return { at, rule: r, exit: p.hops[p.hops.length - 1]?.name ?? r.target };
  });
  const exitText = $derived.by(() => {
    if (!trace) return "";
    const e = trace.exit;
    if (e.kind === "builtin") return builtinLabel(e.name);
    if (e.kind === "provider") return t("routing.trace.exit_provider", { name: e.name });
    return e.name;
  });
</script>

<section class="trace" aria-labelledby="trace-h">
  <h3 id="trace-h">{t("routing.trace.title")}</h3>
  <form class="bar" onsubmit={(e) => (e.preventDefault(), void run())}>
    <div class="field">
      <Icon name="search" size={15} />
      <input
        bind:this={inputEl}
        class="input"
        type="text"
        bind:value={input}
        placeholder={t("routing.trace.placeholder")}
        aria-label={t("routing.trace.placeholder")}
        autocomplete="off"
        spellcheck="false"
        onkeydown={(e) => {
          if (e.key === "Escape" && input) {
            e.preventDefault();
            e.stopPropagation();
            clear();
          }
        }}
      />
      {#if input}
        <button class="clear" type="button" aria-label={t("routing.trace.clear")} title={t("routing.trace.clear")} onclick={clear}><Icon name="x" size={14} /></button>
      {/if}
    </div>
    <button class="btn primary" type="submit" disabled={busy || !input.trim()}>{t("routing.trace.run")}</button>
  </form>
  <details class="more">
    <summary class="subtle">{t("routing.trace.more")}</summary>
    <div class="more-body">
      <label class="lbl">{t("routing.trace.process")}<input class="input" type="text" bind:value={process} placeholder="chrome.exe" autocomplete="off" spellcheck="false" /></label>
      <label class="chk"><input type="checkbox" bind:checked={udp} />{t("routing.trace.udp")}</label>
    </div>
  </details>
  {#if examples.length && !trace}
    <div class="chips" aria-label={t("routing.trace.examples")}>
      {#each examples as ex (ex)}<button class="chip" type="button" onclick={() => void run(ex)}>{ex}</button>{/each}
    </div>
  {/if}

  {#if error}
    <p class="err" role="alert">{error}</p>
  {:else if !trace}
    <p class="empty muted">{t("routing.trace.empty")}</p>
  {:else}
    <ol class="hops" aria-live="polite">
      <li class="hop">
        <span class="dot">1</span>
        <div>
          <div class="t">{t("routing.trace.request")} <code>{trace.ip || trace.host}</code></div>
          <div class="d">{trace.ip ? t("routing.trace.is_ip") : t("routing.trace.is_host")}</div>
          {#if misses.length}
            <details class="miss">
              <summary>{t("routing.trace.misses", { n: misses.length })}{#if unknown.length} · {t("routing.trace.unknowns", { n: unknown.length })}{/if}</summary>
              <ol>
                {#each misses as s (s.index)}
                  {@const r = rule(s.index)}
                  <li class:unk={s.result !== "miss"}>{s.index + 1}. {r ? ruleName(r) : ""}{#if s.result === "unknown"} — {t("routing.reason." + (s.reason || "parse"))}{:else if s.result === "needs-ip"} — {t("routing.trace.needs_ip_short")}{:else if s.reason} — {t("routing.reason." + s.reason)}{/if}</li>
                {/each}
              </ol>
            </details>
          {/if}
        </div>
      </li>
      {#if trace.needsIp && branch}
        <li class="hop branch">
          <span class="dot">?</span>
          <div>
            <div class="t">{t("routing.trace.needs_ip", { n: trace.needsIp.at + 1 })}</div>
            <div class="d">{t("routing.trace.needs_ip_d", { n: branch.at + 1, rule: ruleName(branch.rule), exit: builtinLabel(branch.exit) })}</div>
          </div>
        </li>
      {/if}
      {#if hitRule}
        <li class="hop">
          <span class="dot">2</span>
          <div>
            <div class="t">{t("routing.trace.hit", { n: trace.hit + 1, rule: ruleName(hitRule) })}</div>
            {#if hitRule.set}<div class="d">{t("routing.trace.set_d", { behavior: hitRule.set.behavior, n: hitRule.set.count ?? "?" })}</div>{/if}
            <span class="ev">{hitRule.raw}</span>
            {#if trace.steps.find((s) => s.index === trace!.hit)?.evidence}<span class="ev">{t("routing.trace.evidence", { e: trace.steps.find((s) => s.index === trace!.hit)!.evidence! })}</span>{/if}
          </div>
        </li>
      {/if}
      {#each trace.chain as c, i (c.group)}
        <li class="hop">
          <span class="dot">{i + 3}</span>
          <div>
            <div class="t">{t("routing.trace.group", { name: c.group })} <span class="gtype">{typeLabel(c.type)}</span></div>
            <div class="d">
              {#if c.type === "url-test"}{t("routing.trace.urltest", { list: [c.pick, ...c.candidates].join("、") })}
              {:else if c.type === "fallback"}{t("routing.trace.fallback", { pick: c.pick, list: c.candidates.join("、") || "—" })}
              {:else if c.type === "load-balance"}{t("routing.trace.lb", { n: c.candidates.length + 1 })}
              {:else}{t("routing.trace.select", { pick: c.pick })}{#if c.candidates.length} {t("routing.trace.candidates", { list: c.candidates.join("、") })}{/if}{/if}
            </div>
          </div>
        </li>
      {/each}
      <li class="hop k-{trace.exit.kind === 'builtin' ? (trace.exit.name === 'DIRECT' ? 'direct' : trace.exit.name.startsWith('REJECT') ? 'reject' : 'proxy') : 'proxy'}">
        <span class="dot">{trace.chain.length + 3}</span>
        <div>
          <div class="t">{trace.exit.name.startsWith("REJECT") ? t("routing.trace.blocked") : t("routing.trace.exit", { name: exitText })}</div>
          <div class="d">
            {#if trace.exit.kind === "provider"}{t("routing.trace.exit_provider_d")}
            {:else if trace.exit.name === "DIRECT"}{t("routing.trace.direct_d")}
            {:else if trace.exit.name.startsWith("REJECT")}{t("routing.trace.reject_d")}
            {:else if trace.exit.via}{t("routing.via", { via: trace.exit.via })}{/if}
          </div>
        </div>
      </li>
    </ol>
    {#if before && (before.hit !== trace.hit || before.exit.name !== trace.exit.name)}
      <p class="note warn">{t("routing.trace.draft_changed", { rule: before.hit >= 0 ? before.hit + 1 : "—", exit: before.exit.kind === "builtin" ? builtinLabel(before.exit.name) : before.exit.name })}</p>
    {/if}
    {#if trace.stale}<p class="note warn">{t("routing.trace.stale")}</p>{/if}
  {/if}
</section>

<style>
  .trace {
    display: grid;
    gap: var(--space-3);
    align-content: start;
    min-inline-size: 0;
  }
  h3 {
    font-size: var(--text-md);
    font-weight: 620;
  }
  .bar {
    display: flex;
    gap: var(--space-2);
  }
  .field {
    position: relative;
    flex: 1;
    display: flex;
    align-items: center;
    color: var(--fg-subtle);
  }
  .field :global(svg) {
    position: absolute;
    inset-inline-start: 10px;
    pointer-events: none;
  }
  .field .input {
    flex: 1;
    padding-inline: 32px 30px;
    font-family: var(--font-mono);
  }
  .clear {
    position: absolute;
    inset-inline-end: 6px;
    display: grid;
    place-items: center;
    inline-size: 22px;
    block-size: 22px;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: var(--fg-subtle);
    cursor: pointer;
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
  }
  .field:hover .clear,
  .field:focus-within .clear {
    opacity: 1;
  }
  .clear:hover {
    background: var(--bg-hover);
    color: var(--fg);
  }
  .more summary {
    cursor: pointer;
    font-size: var(--text-sm);
  }
  .more-body {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    align-items: end;
    padding-block-start: var(--space-2);
    font-size: var(--text-sm);
  }
  .lbl {
    display: grid;
    gap: 4px;
    color: var(--fg-muted);
  }
  .chk {
    display: flex;
    gap: 6px;
    align-items: center;
    color: var(--fg-muted);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .chip {
    font: inherit;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: 2px 9px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--fg-muted);
    cursor: pointer;
  }
  .chip:hover {
    color: var(--fg);
    border-color: var(--border-strong);
  }
  .empty,
  .err {
    font-size: var(--text-sm);
  }
  .err {
    color: var(--err);
  }
  .hops {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
  }
  .hop {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: 10px;
    position: relative;
    padding-block-end: 12px;
  }
  .hop:not(:last-child)::before {
    content: "";
    position: absolute;
    inset-inline-start: 10px;
    inset-block: 22px 0;
    border-inline-start: 2px solid var(--border);
  }
  .dot {
    inline-size: 22px;
    block-size: 22px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-size: var(--text-xs);
    font-weight: 700;
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  .hop.branch .dot {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .hop.k-proxy .dot {
    background: color-mix(in oklab, var(--flow-proxy) 18%, var(--bg-raised));
    color: var(--flow-proxy);
  }
  .hop.k-direct .dot {
    background: color-mix(in oklab, var(--flow-direct) 18%, var(--bg-raised));
    color: var(--flow-direct);
  }
  .hop.k-reject .dot {
    background: color-mix(in oklab, var(--flow-reject) 18%, var(--bg-raised));
    color: var(--flow-reject);
  }
  .t {
    font-weight: 600;
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .d {
    font-size: var(--text-sm);
    color: var(--fg-muted);
    overflow-wrap: anywhere;
  }
  .gtype {
    font-size: var(--text-xs);
    font-weight: 400;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--bg-sunken);
    color: var(--fg-muted);
  }
  .ev {
    margin-block-start: 4px;
    margin-inline-end: 4px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: 3px 8px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    display: inline-block;
    max-inline-size: 100%;
    overflow-wrap: anywhere;
  }
  .miss summary {
    cursor: pointer;
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .miss ol {
    margin: 4px 0 0;
    padding-inline-start: 4px;
    list-style: none;
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    display: grid;
    gap: 2px;
  }
  .miss li.unk {
    color: var(--warn);
  }
  .note {
    font-size: var(--text-sm);
    padding: 6px 10px;
    border-radius: var(--radius-md);
  }
  .note.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
</style>
