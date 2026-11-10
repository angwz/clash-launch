<script lang="ts">
  // 节点源详情：基本设置、整理规则（第 5.5 节）与实时的节点表格。
  // 节点表格：协议、地址（打码）、名称、地区与识别理由、倍率、标签；点击地区可手动指定（写入 overrides）。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import { api, type Diagnostic, type Node, type Source } from "../lib/api/client";
  import { catalog } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { params }: { params: Record<string, string> } = $props();

  interface Processing {
    include?: string;
    exclude?: string;
    keepInfo?: boolean;
    prefix?: string;
    flags?: "prefix" | "replace" | "off";
    noDecor?: boolean;
    rename?: { pattern: string; replace: string }[];
    overrides?: Record<string, { region?: string; emoji?: string; tags?: string[]; rate?: number }>;
    keepDuplicates?: boolean;
    sort?: "source" | "name";
    tags?: string[];
  }

  let src = $state<Source | null>(null);
  let proc = $state<Processing>({});
  let name = $state("");
  let url = $state("");
  let content = $state("");
  let ua = $state("");
  let interval = $state(0); // 分钟（界面）；接口以秒为单位
  let nodes = $state<Node[]>([]);
  let diags = $state<Diagnostic[]>([]);
  let format = $state("");
  let error = $state("");
  let saving = $state(false);
  let editingRegion = $state<string | null>(null);
  let regionDraft = $state("");

  async function load() {
    try {
      const s = await api.get<Source>("/api/v1/sources/" + params.id);
      src = s;
      name = s.name;
      url = s.url ?? "";
      content = s.content ?? "";
      ua = s.ua ?? "";
      interval = Math.round(s.interval / 60);
      proc = (s.processing ?? {}) as Processing;
      await Promise.all([loadNodes(), catalog.load()]);
    } catch (e) {
      error = errorText(e);
    }
  }

  async function loadNodes() {
    const r = await api.get<{ nodes: Node[]; diagnostics: Diagnostic[]; format?: string }>("/api/v1/sources/" + params.id + "/nodes");
    nodes = r.nodes;
    diags = r.diagnostics;
    format = r.format ?? "";
  }

  $effect(() => void load());

  async function save() {
    if (!src) return;
    saving = true;
    try {
      const body: Record<string, unknown> = { name, kind: src.kind, ua: ua || undefined, interval: interval * 60, processing: clean(proc) };
      if (src.kind === "subscription") body.url = url;
      else body.content = content;
      src = await api.put<Source>("/api/v1/sources/" + src.id, body, { ifMatch: src.version });
      toasts.push("ok", t("common.saved"));
      await loadNodes();
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      saving = false;
    }
  }

  function clean(p: Processing): Processing {
    const out: Processing = {};
    for (const [k, v] of Object.entries(p)) {
      if (v === "" || v === false || v === undefined || (Array.isArray(v) && v.length === 0)) continue;
      if (typeof v === "object" && !Array.isArray(v) && Object.keys(v as object).length === 0) continue;
      (out as Record<string, unknown>)[k] = v;
    }
    return out;
  }

  async function refresh() {
    if (!src) return;
    try {
      await api.post("/api/v1/sources/" + src.id + "/refresh");
      toasts.push("ok", t("source.refresh_queued"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function remove() {
    if (!src || !confirm(t("source.delete_confirm", { name: src.name }))) return;
    try {
      await api.del("/api/v1/sources/" + src.id);
      router.go("/sources");
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  function startRegion(n: Node) {
    editingRegion = n.fingerprint;
    regionDraft = proc.overrides?.[n.fingerprint]?.region ?? n.region.code ?? "";
  }

  async function commitRegion(n: Node) {
    const code = regionDraft.trim().toUpperCase();
    const ov = { ...(proc.overrides ?? {}) };
    if (code) ov[n.fingerprint] = { ...(ov[n.fingerprint] ?? {}), region: code };
    else if (ov[n.fingerprint]) {
      const { region: _, ...rest } = ov[n.fingerprint]!;
      if (Object.keys(rest).length) ov[n.fingerprint] = rest;
      else delete ov[n.fingerprint];
    }
    proc = { ...proc, overrides: ov };
    editingRegion = null;
    await save();
  }

  function flag(code?: string): string {
    if (!code || !/^[A-Z]{2}$/.test(code)) return "";
    return String.fromCodePoint(...[...code].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
  }

  function renameText(): string {
    return (proc.rename ?? []).map((r) => r.pattern + " => " + r.replace).join("\n");
  }

  function setRename(text: string) {
    proc = {
      ...proc,
      rename: text
        .split("\n")
        .map((l) => l.trim())
        .filter(Boolean)
        .map((l) => {
          const i = l.indexOf(" => ");
          return i < 0 ? { pattern: l, replace: "" } : { pattern: l.slice(0, i), replace: l.slice(i + 4) };
        }),
    };
  }
</script>

<div class="page">
  {#if error}
    <p class="field-error" role="alert">{error}</p>
  {:else if !src}
    <p class="muted">{t("common.loading")}</p>
  {:else}
    <PageHeader title={src.name} subtitle={t("source.kind." + src.kind) + (format ? " · " + format : "")}>
      {#snippet actions()}
        {#if src?.kind === "subscription"}<button class="btn" type="button" onclick={() => void refresh()}><Icon name="refresh" size={15} />{t("source.refresh")}</button>{/if}
        <button class="btn danger" type="button" onclick={() => void remove()}><Icon name="trash" size={15} />{t("common.delete")}</button>
        <button class="btn primary" type="button" disabled={saving} onclick={() => void save()}>{t("common.save")}</button>
      {/snippet}
    </PageHeader>

    <div class="layout">
      <section class="panel box" aria-labelledby="s-basic">
        <h2 id="s-basic">{t("source.basic")}</h2>
        <div class="grid2">
          <div>
            <label class="label" for="sd-name">{t("sources.col_name")}</label>
            <input id="sd-name" class="input" bind:value={name} />
          </div>
          {#if src.kind === "subscription"}
            <div>
              <label class="label" for="sd-int">{t("source.interval")}</label>
              <input id="sd-int" class="input mono ltr" type="number" min="0" bind:value={interval} />
            </div>
          {/if}
        </div>
        {#if src.kind === "subscription"}
          <div>
            <label class="label" for="sd-url">{t("source.url")}</label>
            <input id="sd-url" class="input mono ltr" bind:value={url} spellcheck="false" />
          </div>
          <div>
            <label class="label" for="sd-ua">{t("source.ua")}</label>
            <input id="sd-ua" class="input mono ltr" bind:value={ua} placeholder="clash.meta" spellcheck="false" />
          </div>
          {#if src.snapshot}
            <p class="hint">
              {#if src.snapshot.fetchedAt}{t("source.fetched", { when: i18n.date(src.snapshot.fetchedAt) })}{/if}
              {#if src.snapshot.lastError} · <span class="err-text">{src.snapshot.lastError}</span>{/if}
            </p>
          {/if}
        {:else}
          <div>
            <label class="label" for="sd-content">{t("source.content")}</label>
            <textarea id="sd-content" class="textarea mono ltr" rows="6" bind:value={content} spellcheck="false"></textarea>
          </div>
        {/if}
      </section>

      <section class="panel box" aria-labelledby="s-proc">
        <h2 id="s-proc">{t("source.processing")}</h2>
        <div class="grid2">
          <div>
            <label class="label" for="sd-inc">{t("proc.include")}</label>
            <input id="sd-inc" class="input mono ltr" bind:value={proc.include} placeholder="香港|HK" spellcheck="false" />
          </div>
          <div>
            <label class="label" for="sd-exc">{t("proc.exclude")}</label>
            <input id="sd-exc" class="input mono ltr" bind:value={proc.exclude} placeholder="测试|过期" spellcheck="false" />
          </div>
          <div>
            <label class="label" for="sd-prefix">{t("proc.prefix")}</label>
            <input id="sd-prefix" class="input" bind:value={proc.prefix} />
          </div>
          <div>
            <label class="label" for="sd-flags">{t("proc.flags")}</label>
            <select id="sd-flags" class="select" bind:value={proc.flags}>
              <option value={undefined}>{t("proc.flags_prefix")}</option>
              <option value="replace">{t("proc.flags_replace")}</option>
              <option value="off">{t("proc.flags_off")}</option>
            </select>
          </div>
        </div>
        <div>
          <label class="label" for="sd-rename">{t("proc.rename")}</label>
          <textarea id="sd-rename" class="textarea mono ltr" rows="3" value={renameText()} onchange={(e) => setRename(e.currentTarget.value)} placeholder="^\[(.+?)\] => $1" spellcheck="false"></textarea>
          <p class="hint">{t("proc.rename_hint")}</p>
        </div>
        <div class="checks">
          <label><input type="checkbox" bind:checked={proc.keepInfo} />{t("proc.keep_info")}</label>
          <label><input type="checkbox" bind:checked={proc.keepDuplicates} />{t("proc.keep_dup")}</label>
          <label><input type="checkbox" bind:checked={proc.noDecor} />{t("proc.no_decor")}</label>
          <label><input type="checkbox" checked={proc.sort === "name"} onchange={(e) => (proc = { ...proc, sort: e.currentTarget.checked ? "name" : undefined })} />{t("proc.sort_name")}</label>
        </div>
      </section>
    </div>

    <section class="nodes" aria-labelledby="s-nodes">
      <div class="nodes-head">
        <h2 id="s-nodes">{t("source.nodes", { n: nodes.length })}</h2>
        {#if diags.length}<span class="muted">{t("source.skipped", { n: diags.length })}</span>{/if}
      </div>
      {#if nodes.length === 0}
        <p class="empty muted">{src.kind === "subscription" && !src.snapshot?.fetchedAt ? t("source.not_fetched") : t("source.no_nodes")}</p>
      {:else}
        <div class="panel table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>{t("node.name")}</th>
                <th>{t("node.region")}</th>
                <th>{t("node.type")}</th>
                <th>{t("node.server")}</th>
                <th class="num">{t("node.rate")}</th>
                <th>{t("node.tags")}</th>
              </tr>
            </thead>
            <tbody>
              {#each nodes as n (n.fingerprint)}
                <tr>
                  <td class="nname">{n.name}</td>
                  <td>
                    {#if editingRegion === n.fingerprint}
                      <form class="region-edit" onsubmit={(e) => { e.preventDefault(); void commitRegion(n); }}>
                        <input class="input mono ltr" bind:value={regionDraft} maxlength="2" aria-label={t("node.region")} placeholder="HK" />
                        <button class="btn sm primary" type="submit">{t("common.ok")}</button>
                        <button class="btn sm ghost" type="button" onclick={() => (editingRegion = null)}>{t("common.cancel")}</button>
                      </form>
                    {:else}
                      <button
                        type="button"
                        class="region"
                        class:unknown={!n.region.code}
                        title={t("region.why." + (n.region.kind ?? "unknown"), { match: n.region.match ?? "" })}
                        onclick={() => startRegion(n)}
                      >
                        <span class="flag">{flag(n.region.code) || "·"}</span>
                        <span class="mono">{n.region.code || t("region.unknown")}</span>
                        <span class="why">{t("region.kind." + (n.region.kind ?? "unknown"))}</span>
                      </button>
                    {/if}
                  </td>
                  <td class="mono">{n.type}</td>
                  <td class="mono ltr subtle">{n.server}:{n.port}</td>
                  <td class="num mono">{n.rate === 1 ? "" : "×" + n.rate}</td>
                  <td>{#each n.tags ?? [] as tag (tag)}<span class="tag">{tag}</span>{/each}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
      {#if diags.length}
        <details class="skipped">
          <summary>{t("source.skipped_list", { n: diags.length })}</summary>
          <ul>
            {#each diags as d, i (i)}
              <li><StatusBadge tone={d.severity === "error" ? "err" : d.severity === "warning" ? "warn" : "info"} label={d.code} /> {catalog.diagMessage(d.code, d.args, new Map([[src!.id, src!.name]]))}</li>
            {/each}
          </ul>
        </details>
      {/if}
    </section>
  {/if}
</div>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .layout {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
    gap: var(--space-4);
    margin-block-end: var(--space-8);
  }
  .box {
    display: grid;
    gap: var(--space-4);
    align-content: start;
    padding: var(--space-5);
  }
  .box h2,
  .nodes h2 {
    font-size: var(--text-lg);
  }
  .grid2 {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-3);
  }
  .checks {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .checks label {
    display: inline-flex;
    gap: var(--space-2);
    align-items: center;
  }
  .checks input {
    accent-color: var(--accent);
  }
  .err-text {
    color: var(--err);
  }
  .nodes {
    display: grid;
    gap: var(--space-3);
  }
  .nodes-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
  }
  .table-wrap {
    overflow-x: auto;
  }
  .nname {
    white-space: nowrap;
  }
  .num {
    text-align: end;
  }
  .region {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: 2px;
    padding-inline: var(--space-2);
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--fg);
    font: inherit;
    cursor: pointer;
    white-space: nowrap;
  }
  .region:hover {
    border-color: var(--border-strong);
  }
  .region.unknown {
    color: var(--fg-subtle);
  }
  .why {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
  }
  .region-edit {
    display: inline-flex;
    gap: var(--space-1);
  }
  .region-edit .input {
    inline-size: 56px;
    min-block-size: 26px;
  }
  .tag {
    display: inline-block;
    margin-inline-end: var(--space-1);
    padding-inline: 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: var(--text-xs);
  }
  .empty {
    padding: var(--space-6);
    text-align: center;
    border: 1px dashed var(--border-strong);
    border-radius: var(--radius-lg);
  }
  .skipped summary {
    cursor: pointer;
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .skipped ul {
    display: grid;
    gap: var(--space-2);
    margin-block-start: var(--space-2);
    font-size: var(--text-sm);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
