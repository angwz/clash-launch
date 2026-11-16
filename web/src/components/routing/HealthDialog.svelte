<script lang="ts">
  // 常用网站体检（routing 第 1 节 #4）：服务器按 catalog/routing/health.yaml 逐个追踪，这里按类别列出命中的规则与出口。
  // 点一行即对它做完整追踪。
  import Dialog from "../Dialog.svelte";
  import { api, type Schemas } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { builtinLabel } from "../../lib/routing/labels";

  type Item = Schemas["RoutingHealthItem"];

  let { open = $bindable(false), profileId, doc, onTrace }: { open?: boolean; profileId: string; doc: () => string; onTrace: (host: string) => void } = $props();

  let items = $state<Item[] | null>(null);
  let error = $state("");
  let busy = $state(false);

  $effect(() => {
    if (open) void load();
  });

  async function load() {
    busy = true;
    error = "";
    try {
      items = (await api.post<{ items: Item[] }>(`/api/v1/profiles/${profileId}/routing/health`, { doc: doc() })).items;
    } catch (e) {
      error = errorText(e);
    } finally {
      busy = false;
    }
  }

  const groups = $derived.by(() => {
    const m = new Map<string, Item[]>();
    for (const it of items ?? []) m.set(it.category, [...(m.get(it.category) ?? []), it]);
    return [...m];
  });
  const exitText = (it: Item) => (it.exit.kind === "builtin" ? builtinLabel(it.exit.name) : it.exit.kind === "provider" ? t("routing.trace.exit_provider", { name: it.exit.name }) : it.exit.name);
  const kind = (it: Item) => (it.exit.name === "DIRECT" ? "direct" : it.exit.name.startsWith("REJECT") ? "reject" : "proxy");
</script>

<Dialog bind:open title={t("routing.health.title")} wide>
  <p class="muted sub">{t("routing.health.sub")}</p>
  {#if busy}
    <p class="muted" aria-busy="true">{t("routing.health.running")}</p>
  {:else if error}
    <p class="err" role="alert">{error}</p>
  {:else if items}
    <table class="tbl">
      <thead><tr><th>{t("routing.health.site")}</th><th>{t("routing.health.rule")}</th><th>{t("routing.health.exit")}</th></tr></thead>
      <tbody>
        {#each groups as [cat, list] (cat)}
          <tr class="cat"><th colspan="3">{t("routing.health.category." + cat)}</th></tr>
          {#each list as it (it.host)}
            <tr>
              <td><button class="link mono" type="button" onclick={() => ((open = false), onTrace(it.host))}>{it.host}</button></td>
              <td class="mono rule">{it.rule}{#if it.uncertain} <span class="unc" title={t("routing.health.uncertain_hint")}>{t("routing.health.uncertain")}</span>{/if}</td>
              <td><span class="k k-{kind(it)}"></span>{exitText(it)}</td>
            </tr>
          {/each}
        {/each}
      </tbody>
    </table>
  {/if}
</Dialog>

<style>
  .sub {
    font-size: var(--text-sm);
    margin-block-end: var(--space-3);
  }
  .err {
    color: var(--err);
  }
  .tbl {
    inline-size: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }
  .tbl th,
  .tbl td {
    text-align: start;
    padding: 6px 8px;
    border-block-end: 1px solid var(--border);
    vertical-align: top;
  }
  .tbl thead th {
    font-size: var(--text-xs);
    color: var(--fg-subtle);
    font-weight: 500;
  }
  .tbl tr.cat th {
    padding-block-start: var(--space-3);
    font-size: var(--text-xs);
    color: var(--fg-muted);
    font-weight: 600;
  }
  .mono {
    font-family: var(--font-mono);
  }
  .rule {
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
    color: var(--fg-muted);
  }
  .link {
    border: 0;
    background: none;
    padding: 0;
    color: var(--accent-fg);
    cursor: pointer;
    font-size: var(--text-sm);
  }
  .link:hover {
    text-decoration: underline;
  }
  .unc {
    font-family: var(--font-sans);
    padding: 0 6px;
    border-radius: 999px;
    background: var(--warn-soft);
    color: var(--warn);
  }
  .k {
    display: inline-block;
    inline-size: 8px;
    block-size: 8px;
    border-radius: 50%;
    margin-inline-end: 6px;
  }
  .k-proxy {
    background: var(--flow-proxy);
  }
  .k-direct {
    background: var(--flow-direct);
  }
  .k-reject {
    background: var(--flow-reject);
  }
</style>
