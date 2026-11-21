<script lang="ts">
  // 订阅链接访问限制（需求 R42）：限定客户端、国家或地区、每天次数。保存后立即生效，
  // 不符合的请求被挡住（403 或 429），在订阅分析里能看到。
  import Dialog from "../Dialog.svelte";
  import { api, type Link, type Schemas } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { flagOf, regionName } from "../../lib/regions";
  import { toasts } from "../../lib/toast.svelte";

  type Limits = Schemas["LinkLimits"];
  let { open = $bindable(false), link, base, onsaved }: { open?: boolean; link: Link | null; base: string; onsaved: () => void } = $props();

  // 常见客户端的名称（User-Agent 中「名称/版本」的名称）；最近一次拉取的客户端排在最前
  const COMMON = ["mihomo", "clash-verge", "ClashMetaForAndroid", "FlClash", "Stash", "clash.meta"];
  const COMMON_CC = ["CN", "HK", "TW", "JP", "SG", "US"];

  let clients = $state<string[]>([]);
  let countries = $state<string[]>([]);
  let perDay = $state(0);
  let clientInput = $state("");
  let ccInput = $state("");
  let busy = $state(false);

  $effect(() => {
    if (!open || !link) return;
    clients = [...(link.limits?.clients ?? [])];
    countries = [...(link.limits?.countries ?? [])];
    perDay = link.limits?.perDay ?? 0;
    clientInput = "";
    ccInput = "";
  });

  const lastClient = $derived(link?.lastUa ? (link.lastUa.split("/")[0] ?? "").trim() : "");
  const suggestions = $derived([...new Set([lastClient, ...COMMON].filter((x) => x && !clients.some((c) => c.toLowerCase() === x.toLowerCase())))]);
  const ccSuggestions = $derived(COMMON_CC.filter((c) => !countries.includes(c)));

  function addClient(v: string) {
    for (const x of v.split(/[,，\s]+/)) {
      const c = (x.split("/")[0] ?? "").trim();
      if (c && !clients.some((y) => y.toLowerCase() === c.toLowerCase())) clients = [...clients, c];
    }
    clientInput = "";
  }
  function addCountry(v: string) {
    for (const x of v.split(/[,，\s]+/)) {
      const c = x.trim().toUpperCase();
      if (/^[A-Z]{2}$/.test(c) && !countries.includes(c)) countries = [...countries, c];
    }
    ccInput = "";
  }

  async function save() {
    if (!link) return;
    if (clientInput.trim()) addClient(clientInput);
    if (ccInput.trim()) addCountry(ccInput);
    busy = true;
    try {
      const limits: Limits = { clients, countries, perDay: Math.max(0, Math.floor(perDay || 0)) };
      await api.patch(base + "/" + link.id, { limits }, { ifMatch: link.version });
      toasts.push("ok", t("limits.saved"));
      open = false;
      onsaved();
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      busy = false;
    }
  }
  const ccName = (c: string) => flagOf(c) + " " + regionName(c, i18n.intlLocale);
  function onEnter(e: KeyboardEvent, add: () => void) {
    if (e.key !== "Enter") return;
    e.preventDefault();
    add();
  }
</script>

<Dialog bind:open title={t("limits.title")}>
  <p class="muted">{t("limits.desc")}</p>

  <fieldset>
    <legend class="label">{t("limits.clients")}</legend>
    {#if clients.length}
      <ul class="chips">
        {#each clients as c (c)}<li><span class="mono">{c}</span><button type="button" aria-label={t("limits.remove", { item: c })} onclick={() => (clients = clients.filter((x) => x !== c))}>×</button></li>{/each}
      </ul>
    {/if}
    <input class="input mono" bind:value={clientInput} placeholder={t("limits.clients_placeholder")} aria-label={t("limits.clients")} onkeydown={(e) => onEnter(e, () => addClient(clientInput))} />
    <div class="suggest">
      {#each suggestions as s (s)}<button class="btn sm ghost" type="button" onclick={() => addClient(s)}>+ {s}</button>{/each}
    </div>
    <p class="hint">{t("limits.clients_hint")}</p>
  </fieldset>

  <fieldset>
    <legend class="label">{t("limits.countries")}</legend>
    {#if countries.length}
      <ul class="chips">
        {#each countries as c (c)}<li><span>{ccName(c)}</span><button type="button" aria-label={t("limits.remove", { item: c })} onclick={() => (countries = countries.filter((x) => x !== c))}>×</button></li>{/each}
      </ul>
    {/if}
    <input class="input mono" bind:value={ccInput} placeholder={t("limits.countries_placeholder")} aria-label={t("limits.countries")} onkeydown={(e) => onEnter(e, () => addCountry(ccInput))} />
    <div class="suggest">
      {#each ccSuggestions as c (c)}<button class="btn sm ghost" type="button" onclick={() => addCountry(c)}>+ {ccName(c)}</button>{/each}
    </div>
    <p class="hint">{t("limits.countries_hint")}</p>
  </fieldset>

  <label class="label" for="lim-day">{t("limits.per_day")}</label>
  <input id="lim-day" class="input num" type="number" min="0" max="100000" bind:value={perDay} />
  <p class="hint">{t("limits.per_day_hint")}</p>

  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" disabled={busy} onclick={() => void save()}>{t("common.save")}</button>
  {/snippet}
</Dialog>

<style>
  fieldset {
    display: grid;
    gap: var(--space-2);
    margin-block: var(--space-4);
    padding: 0;
    border: 0;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .chips li {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding-block: 2px;
    padding-inline: 10px 4px;
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-sm);
  }
  .chips button {
    inline-size: 22px;
    block-size: 22px;
    border: 0;
    border-radius: 50%;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .chips button:hover {
    background: var(--bg-raised);
  }
  .suggest {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .num {
    max-inline-size: 160px;
  }
</style>
