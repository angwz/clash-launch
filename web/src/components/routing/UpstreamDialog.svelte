<script lang="ts">
  // 上游同步（routing 第 12 节，决定 0007 第 8 条）：自己管理分流后，规则源的变化逐条列出，由用户决定采纳或忽略。
  // 默认勾选可以直接采纳的；与自己的改动冲突的默认不勾选（保留自己的）。确认后进入草稿，仍需「应用」才写入。
  import Dialog from "../Dialog.svelte";
  import { api, type Schemas } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { rulesetName, type Upstream, type UpstreamChange } from "../../lib/routing/draft";

  type Up = Schemas["RoutingUpstream"];

  let { open = $bindable(false), profileId, doc, onAccept }: { open?: boolean; profileId: string; doc: () => string; onAccept: (up: Upstream, accepted: UpstreamChange[]) => void } = $props();

  let up = $state.raw<Up | null>(null);
  let picked = $state(new Set<string>());
  let error = $state("");
  let busy = $state(false);

  $effect(() => {
    if (open) void load();
  });

  async function load() {
    busy = true;
    error = "";
    try {
      up = await api.post<Up>(`/api/v1/profiles/${profileId}/routing/upstream`, { doc: doc() });
      picked = new Set(up.changes.filter((c) => !c.conflict).map((c) => c.id));
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  const name = (c: UpstreamChange) => (c.kind === "ruleset" ? rulesetName({ group: "", ruleset: c.key }) : c.key);
  const val = (v: unknown) => (v === undefined || v === null ? "—" : typeof v === "string" ? v : JSON.stringify(v));

  function toggle(id: string, on: boolean) {
    const s = new Set(picked);
    if (on) s.add(id);
    else s.delete(id);
    picked = s;
  }

  function accept() {
    if (!up) return;
    onAccept({ digest: up.digest, groups: up.groups, rulesets: up.rulesets }, up.changes.filter((c) => picked.has(c.id)) as UpstreamChange[]);
    open = false;
  }
</script>

<Dialog bind:open title={t("routing.upstream.title")} wide>
  <p class="muted sub">{t("routing.upstream.sub")}</p>
  {#if busy}
    <p class="muted" aria-busy="true">{t("common.loading")}</p>
  {:else if error}
    <p class="err" role="alert">{error}</p>
  {:else if up && !up.changes.length}
    <p class="muted">{t("routing.upstream.none")}</p>
  {:else if up}
    <ul class="list">
      {#each up.changes as c (c.id)}
        <li class:conflict={c.conflict}>
          <label>
            <input type="checkbox" checked={picked.has(c.id)} onchange={(e) => toggle(c.id, e.currentTarget.checked)} />
            <span class="main">
              <span class="t">{t("routing.upstream." + c.kind + "_" + c.op, { name: name(c as UpstreamChange), field: c.field ?? "" })}</span>
              {#if c.op === "modify"}<span class="v">{val(c.base)} → {val(c.upstream)}</span>{/if}
              {#if c.conflict}<span class="v warn">{t("routing.upstream.conflict", { mine: val(c.op === "modify" ? c.mine : c.mine ? t("routing.upstream.edited") : "—") })}</span>{/if}
            </span>
          </label>
        </li>
      {/each}
    </ul>
  {/if}
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" disabled={!up || busy} onclick={accept}>{t("routing.upstream.accept", { n: picked.size })}</button>
  {/snippet}
</Dialog>

<style>
  .sub {
    font-size: var(--text-sm);
    margin-block-end: var(--space-3);
  }
  .err {
    color: var(--err);
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 6px;
  }
  li {
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: 8px 10px;
  }
  li.conflict {
    border-color: var(--warn);
    background: var(--warn-soft);
  }
  label {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    cursor: pointer;
  }
  .main {
    display: grid;
    gap: 2px;
    min-inline-size: 0;
  }
  .t {
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .v {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--fg-muted);
    overflow-wrap: anywhere;
  }
  .v.warn {
    font-family: var(--font-sans);
    color: var(--warn);
  }
</style>
