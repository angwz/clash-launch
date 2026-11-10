<script lang="ts">
  // 规则源：预设画廊（domainrouter_v2、ACL4SSR 等）与自定义（toml 或外部配置 ini）。
  import Dialog from "../components/Dialog.svelte";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import { api, listAll, type RuleSource, type RuleSourceInput } from "../lib/api/client";
  import { catalog, type RulePreset } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { presetInput } from "../lib/rules";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let items = $state<RuleSource[] | null>(null);
  let error = $state("");
  let customOpen = $state(false);
  let custom = $state<RuleSourceInput>({ name: "", kind: "toml" });

  $effect(() => {
    void Promise.all([listAll<RuleSource>("/api/v1/rule-sources"), catalog.load()])
      .then(([l]) => (items = l))
      .catch((e) => (error = errorText(e)));
  });

  async function addPreset(p: RulePreset, vid: string) {
    const v = p.variants.find((x) => x.id === vid) ?? p.variants[0]!;
    try {
      const rs = await api.post<RuleSource>("/api/v1/rule-sources", presetInput(p, v, catalog.label(p.label) + " · " + catalog.label(v.label)));
      toasts.push("ok", t("rules.added"));
      router.go("/rules/" + rs.id);
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function createCustom(e: SubmitEvent) {
    e.preventDefault();
    try {
      const rs = await api.post<RuleSource>("/api/v1/rule-sources", custom);
      customOpen = false;
      router.go("/rules/" + rs.id);
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  let variantOf = $state<Record<string, string>>({});
</script>

<div class="page">
  <PageHeader title={t("rules.title")} subtitle={t("rules.subtitle")}>
    {#snippet actions()}
      <button class="btn" type="button" onclick={() => { custom = { name: "", kind: "toml" }; customOpen = true; }}><Icon name="plus" size={15} />{t("rules.custom")}</button>
    {/snippet}
  </PageHeader>
  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  {#if items && items.length}
    <h2 class="h">{t("rules.mine")}</h2>
    <ul class="mine panel">
      {#each items as r (r.id)}
        <li>
          <a href={"/rules/" + r.id}>{r.name}</a>
          <span class="muted mono">{r.kind}</span>
          <span class="muted">{i18n.relative(r.updatedAt)}</span>
        </li>
      {/each}
    </ul>
  {/if}

  <h2 class="h">{t("rules.presets")}</h2>
  <ul class="gallery">
    {#each catalog.meta?.ruleSources ?? [] as p (p.id)}
      <li class="preset panel" class:rec={p.recommended}>
        <div class="ph">
          <h3>{catalog.label(p.label)}</h3>
          {#if p.recommended}<span class="rec-badge">{t("rules.recommended")}</span>{/if}
        </div>
        <p class="muted notes">{catalog.label(p.notes)}</p>
        <p class="meta subtle">
          <a href={p.repo} target="_blank" rel="noopener noreferrer" data-external>{p.repo.replace("https://github.com/", "")}<Icon name="external" size={11} /></a>
          · {p.license}
          {#if p.stars} · ★ {i18n.number(p.stars)}{/if}
          · {t("rules.checked", { date: p.checked })}
        </p>
        <div class="add">
          {#if p.variants.length > 1}
            <select class="select" aria-label={t("rules.variant")} value={variantOf[p.id] ?? p.variants[0]!.id} onchange={(e) => (variantOf = { ...variantOf, [p.id]: e.currentTarget.value })}>
              {#each p.variants as v (v.id)}<option value={v.id}>{catalog.label(v.label)}</option>{/each}
            </select>
          {/if}
          <button class="btn primary sm" type="button" onclick={() => void addPreset(p, variantOf[p.id] ?? p.variants[0]!.id)}><Icon name="plus" size={14} />{t("common.add")}</button>
        </div>
      </li>
    {/each}
  </ul>
</div>

<Dialog bind:open={customOpen} title={t("rules.custom")} wide>
  <form id="new-rule" class="form" onsubmit={createCustom}>
    <div>
      <label class="label" for="nr-name">{t("sources.col_name")}</label>
      <input id="nr-name" class="input" bind:value={custom.name} required />
    </div>
    <div class="seg-row" role="radiogroup" aria-label={t("rules.format")}>
      <label><input type="radio" bind:group={custom.kind} value="toml" />groups.toml + rulesets.toml</label>
      <label><input type="radio" bind:group={custom.kind} value="ini" />{t("rules.ini")}</label>
    </div>
    {#if custom.kind === "toml"}
      <div>
        <label class="label" for="nr-g">groups.toml URL</label>
        <input id="nr-g" class="input mono ltr" bind:value={custom.groupsUrl} placeholder="https://example.com/groups.toml" />
      </div>
      <div>
        <label class="label" for="nr-r">rulesets.toml URL</label>
        <input id="nr-r" class="input mono ltr" bind:value={custom.rulesetsUrl} placeholder="https://example.com/rulesets.toml" />
      </div>
      <p class="hint">{t("rules.paste_later")}</p>
    {:else}
      <div>
        <label class="label" for="nr-i">INI URL</label>
        <input id="nr-i" class="input mono ltr" bind:value={custom.iniUrl} placeholder="https://example.com/config.ini" />
      </div>
    {/if}
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (customOpen = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="new-rule" disabled={!custom.name.trim()}>{t("common.create")}</button>
  {/snippet}
</Dialog>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .h {
    font-size: var(--text-lg);
    margin-block: var(--space-6) var(--space-3);
  }
  .mine li {
    display: flex;
    gap: var(--space-3);
    align-items: baseline;
    padding: var(--space-3) var(--space-4);
    border-block-end: 1px solid var(--border);
  }
  .mine li:last-child {
    border-block-end: 0;
  }
  .mine a {
    flex: 1;
    color: var(--fg);
    font-weight: 560;
  }
  .gallery {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: var(--space-4);
  }
  .preset {
    display: grid;
    gap: var(--space-2);
    align-content: start;
    padding: var(--space-4) var(--space-5);
  }
  .preset.rec {
    border-color: color-mix(in srgb, var(--accent) 45%, var(--border));
  }
  .ph {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .rec-badge {
    padding-inline: 7px;
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-xs);
    font-weight: 560;
  }
  .notes {
    font-size: var(--text-sm);
  }
  .meta {
    font-size: var(--text-xs);
  }
  .meta a {
    display: inline-flex;
    gap: 2px;
    align-items: center;
  }
  .add {
    display: flex;
    gap: var(--space-2);
    margin-block-start: var(--space-2);
  }
  .add .select {
    flex: 1;
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .seg-row {
    display: flex;
    gap: var(--space-5);
    font-size: var(--text-sm);
  }
  .seg-row label {
    display: inline-flex;
    gap: var(--space-2);
    align-items: center;
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
