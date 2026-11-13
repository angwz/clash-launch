<script lang="ts">
  // 链式代理（反馈 #24）：每条规则把选中的节点经「前置」拨号，写入 mihomo 的 dialer-proxy。
  // 规则按顺序套用，后面的覆盖前面的。实际套用结果、可选的前置名字都来自后端预览，不在前端推算；
  // 循环、无法整体套用的 provider 等问题由后端诊断报告（预览的诊断页）。
  import Icon from "../Icon.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import type { ChainRule } from "../../lib/profile-doc";

  let { editor }: { editor: EditorState } = $props();

  const rules = $derived(editor.chains);
  const results = $derived(editor.preview?.chains ?? []);
  const outbounds = $derived(editor.preview?.outbounds ?? []);
  // 选择器的 source 条件按源名称匹配；只列出方案中启用的源
  const sourceNames = $derived(
    editor.sourceRefs
      .filter((r) => !r.disabled)
      .map((r) => editor.sources.find((s) => s.id === r.ref)?.name)
      .filter((n): n is string => !!n),
  );

  function list(v: unknown): string[] {
    return Array.isArray(v) ? v.map(String) : [];
  }

  function update(i: number, patch: Partial<ChainRule>, sel?: Record<string, unknown>) {
    editor.setChains(rules.map((r, j) => (j === i ? { ...r, ...patch, select: sel ? { ...(r.select ?? {}), ...sel } : r.select } : r)));
  }

  function toggleSource(i: number, name: string) {
    const cur = list(rules[i]!.select?.source);
    update(i, {}, { source: cur.includes(name) ? cur.filter((x) => x !== name) : [...cur, name] });
  }

  function move(from: number, to: number) {
    if (to < 0 || to >= rules.length) return;
    const l = [...rules];
    const [x] = l.splice(from, 1);
    l.splice(to, 0, x!);
    editor.setChains(l);
  }

  function regions(text: string): string[] {
    return text
      .split(/[\s,，]+/)
      .map((s) => s.trim().toUpperCase())
      .filter(Boolean);
  }
</script>

<section class="panel-sec" aria-labelledby="sec-chains">
  <header class="head">
    <div>
      <h2 id="sec-chains">{t("chains.title")}</h2>
      <p class="muted sub">{t("chains.sub")}</p>
    </div>
    <button class="btn sm" type="button" onclick={() => editor.setChains([...rules, { via: "" }])}><Icon name="plus" size={14} />{t("chains.add")}</button>
  </header>

  {#if rules.length === 0}
    <p class="empty muted">{t("chains.empty")}</p>
  {:else}
    <datalist id="chain-outbounds">
      {#each outbounds as o (o)}<option value={o}></option>{/each}
    </datalist>
    <ol class="list">
      {#each rules as r, i (i)}
        {@const res = results[i]}
        <li class="rule" class:disabled={r.disabled}>
          <div class="row">
            <span class="idx mono" aria-hidden="true">{i + 1}</span>
            <label class="field via">
              <span class="label">{t("chains.via")}</span>
              <input class="input" list="chain-outbounds" value={r.via} placeholder={t("chains.via_placeholder")} onchange={(e) => update(i, { via: e.currentTarget.value.trim() })} />
            </label>
            <div class="actions">
              <button class="btn ghost icon sm" type="button" aria-label={t("editor.move_up")} disabled={i === 0} onclick={() => move(i, i - 1)}><Icon name="arrow-up" size={14} /></button>
              <button class="btn ghost icon sm" type="button" aria-label={t("editor.move_down")} disabled={i === rules.length - 1} onclick={() => move(i, i + 1)}><Icon name="arrow-down" size={14} /></button>
              <button class="btn ghost icon sm" type="button" aria-pressed={!r.disabled} aria-label={r.disabled ? t("editor.enable") : t("editor.disable")} onclick={() => update(i, { disabled: !r.disabled })}><Icon name={r.disabled ? "eye-off" : "eye"} size={14} /></button>
              <button class="btn ghost icon sm danger" type="button" aria-label={t("common.remove")} onclick={() => editor.setChains(rules.filter((_, j) => j !== i))}><Icon name="x" size={14} /></button>
            </div>
          </div>

          <div class="cond">
            <span class="label">{t("chains.apply_to")}</span>
            <div class="chips" role="group" aria-label={t("chains.sources")}>
              {#each sourceNames as n (n)}
                <button type="button" class="chip" aria-pressed={list(r.select?.source).includes(n)} onclick={() => toggleSource(i, n)}>{n}</button>
              {/each}
            </div>
            <div class="pair">
              <label class="field">
                <span class="label">{t("chains.region")}</span>
                <input class="input mono ltr" value={list(r.select?.region).join(", ")} placeholder="HK, JP" onchange={(e) => update(i, {}, { region: regions(e.currentTarget.value) })} />
              </label>
              <label class="field">
                <span class="label">{t("chains.name")}</span>
                <input class="input mono ltr" value={typeof r.select?.name === "string" ? r.select.name : ""} placeholder="(?i)premium" spellcheck="false" onchange={(e) => update(i, {}, { name: e.currentTarget.value })} />
              </label>
            </div>
            <p class="hint">{t("chains.cond_hint")}</p>
          </div>

          {#if res && !r.disabled}
            <p class="path" aria-live="polite">
              <span class="hop muted">{t("chains.local")}</span><Icon name="chevron-right" size={12} />
              <span class="hop strong">{r.via || "?"}</span><Icon name="chevron-right" size={12} />
              <span class="hop">{t("chains.count", { n: String(res.nodes.length + res.providers.length) })}</span><Icon name="chevron-right" size={12} />
              <span class="hop muted">{t("chains.target")}</span>
            </p>
            {#if res.nodes.length || res.providers.length}
              <p class="members muted">{[...res.nodes, ...res.providers.map((p) => p + " " + t("chains.provider_suffix"))].join("、")}</p>
            {/if}
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
</section>

<style>
  .panel-sec {
    display: grid;
    gap: var(--space-3);
    margin-block-start: var(--space-8);
  }
  .head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  h2 {
    font-size: var(--text-xl);
  }
  .sub {
    font-size: var(--text-sm);
    margin-block-start: 2px;
  }
  .list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: var(--space-3);
  }
  .rule {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    transition: opacity 0.15s ease;
  }
  .rule.disabled {
    opacity: 0.55;
  }
  .row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: end;
    gap: var(--space-3);
  }
  .idx {
    inline-size: 1.75rem;
    block-size: 1.75rem;
    display: inline-grid;
    place-items: center;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    margin-block-end: 4px;
  }
  .field {
    display: grid;
    gap: 4px;
    min-inline-size: 0;
  }
  .actions {
    display: flex;
    gap: 2px;
  }
  .cond {
    display: grid;
    gap: var(--space-2);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .chip {
    padding: 2px 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--fg-muted);
    font-size: var(--text-sm);
    cursor: pointer;
    transition:
      background 0.15s ease,
      color 0.15s ease;
  }
  .chip[aria-pressed="true"] {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-fg, #fff);
  }
  .pair {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }
  .path {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .hop.strong {
    font-weight: 600;
  }
  .members {
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }
  @media (max-width: 767px) {
    .pair {
      grid-template-columns: 1fr;
    }
    .row {
      grid-template-columns: auto minmax(0, 1fr);
    }
    .actions {
      grid-column: 1 / -1;
      justify-content: flex-end;
    }
  }
</style>
