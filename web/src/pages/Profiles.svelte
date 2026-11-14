<script lang="ts">
  import Emoji from "../components/Emoji.svelte";
  import Dialog from "../components/Dialog.svelte";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import { listAll, type Profile } from "../lib/api/client";
  import { catalog } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { t, i18n } from "../lib/i18n/i18n.svelte";
  import { createProfile, renderTone } from "../lib/profiles";
  import { router } from "../lib/router.svelte";
  import { ProfileDoc } from "../lib/profile-doc";

  let items = $state<Profile[] | null>(null);
  let error = $state("");
  let creating = $state(false);
  let name = $state("");
  let preset = $state("mihomo");
  let busy = $state(false);

  $effect(() => {
    void Promise.all([listAll<Profile>("/api/v1/profiles"), catalog.load()])
      .then(([list]) => (items = list))
      .catch((e) => (error = errorText(e)));
  });

  function targetOf(p: Profile): string {
    try {
      const preset = new ProfileDoc(p.doc).target.preset;
      return catalog.label(catalog.meta?.presets.find((x) => x.id === preset)?.label) || preset;
    } catch {
      return "—";
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      const p = await createProfile(name.trim(), preset);
      creating = false;
      router.go("/profiles/" + p.id);
    } catch (err) {
      error = errorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<div class="page">
  <PageHeader title={t("profiles.title")} subtitle={t("profiles.subtitle")}>
    {#snippet actions()}
      <a class="btn" href="/welcome"><Icon name="sparkle" size={15} />{t("profiles.wizard")}</a>
      <button class="btn primary" type="button" onclick={() => { name = ""; creating = true; }}><Icon name="plus" size={15} />{t("profiles.new")}</button>
    {/snippet}
  </PageHeader>

  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  {#if items === null}
    <div class="skeleton" aria-busy="true"></div>
  {:else if items.length === 0}
    <div class="empty panel">
      <div class="empty-mark"><Emoji name="rocket" size={32} /></div>
      <h2>{t("profiles.empty_title")}</h2>
      <p class="muted">{t("profiles.empty_body")}</p>
      <a class="btn primary" href="/welcome"><Icon name="sparkle" size={15} />{t("profiles.wizard")}</a>
    </div>
  {:else}
    <ul class="grid">
      {#each items as p (p.id)}
        {@const tone = renderTone(p)}
        <li class="card panel">
          <a class="cover" href={"/profiles/" + p.id} aria-label={p.name}></a>
          <div class="row">
            <h2 class="name">{p.name}</h2>
            <StatusBadge {tone} label={t("render." + (p.render?.status ?? "pending"))} />
          </div>
          <p class="meta muted">
            <span>{targetOf(p)}</span>
            <span aria-hidden="true">·</span>
            <span>{t("profiles.version", { n: p.version })}</span>
            <span aria-hidden="true">·</span>
            <time datetime={p.updatedAt} title={i18n.date(p.updatedAt)}>{i18n.relative(p.updatedAt)}</time>
          </p>
          <div class="links">
            <a class="btn sm ghost" href={"/profiles/" + p.id + "/links"}><Icon name="link" size={14} />{t("profiles.links")}</a>
            <a class="btn sm ghost" href={"/profiles/" + p.id + "/history"}><Icon name="history" size={14} />{t("profiles.history")}</a>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<Dialog bind:open={creating} title={t("profiles.new")}>
  <form id="new-profile" class="form" onsubmit={create}>
    <div>
      <label class="label" for="np-name">{t("profiles.name")}</label>
      <input id="np-name" class="input" bind:value={name} required maxlength="80" placeholder={t("profiles.name_placeholder")} />
    </div>
    <div>
      <label class="label" for="np-target">{t("profiles.target")}</label>
      <select id="np-target" class="select" bind:value={preset}>
        {#each catalog.meta?.presets ?? [] as x (x.id)}
          <option value={x.id}>{catalog.label(x.label)}</option>
        {/each}
      </select>
      <p class="hint">{catalog.label(catalog.meta?.presets.find((x) => x.id === preset)?.notes)}</p>
    </div>
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (creating = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="new-profile" disabled={busy || !name.trim()}>{t("common.create")}</button>
  {/snippet}
</Dialog>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: var(--space-4);
  }
  .card {
    position: relative;
    display: grid;
    gap: var(--space-2);
    padding: var(--space-4) var(--space-5);
    transition:
      border-color var(--dur-fast) var(--ease),
      box-shadow var(--dur-fast) var(--ease);
  }
  .card:hover {
    border-color: var(--border-strong);
    box-shadow: var(--shadow-sm);
  }
  .cover {
    position: absolute;
    inset: 0;
    border-radius: inherit;
  }
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }
  .name {
    font-size: var(--text-lg);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .links {
    position: relative;
    display: flex;
    gap: var(--space-1);
    margin-inline-start: calc(-1 * var(--space-2));
  }
  .empty {
    display: grid;
    justify-items: center;
    gap: var(--space-3);
    padding: var(--space-10) var(--space-6);
    text-align: center;
  }
  .empty-mark {
    display: grid;
    place-items: center;
    inline-size: 56px;
    block-size: 56px;
    border-radius: 16px;
    background: var(--accent-soft);
    color: var(--accent-fg);
  }
  .skeleton {
    block-size: 120px;
    border-radius: var(--radius-lg);
    background: var(--bg-sunken);
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
