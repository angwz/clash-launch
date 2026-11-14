<script lang="ts">
  import Emoji from "../components/Emoji.svelte";
  import Dialog from "../components/Dialog.svelte";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import { api, listAll, type Profile, type Template } from "../lib/api/client";
  import { catalog } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { t, i18n } from "../lib/i18n/i18n.svelte";
  import { createProfileFrom, nodeInput, renderTone } from "../lib/profiles";
  import { toasts } from "../lib/toast.svelte";
  import { router } from "../lib/router.svelte";
  import { ProfileDoc } from "../lib/profile-doc";

  let items = $state<Profile[] | null>(null);
  let error = $state("");
  let creating = $state(false);
  let name = $state("");
  let start = $state("p:mihomo"); // "t:<模板 ID>" 或 "p:<预设 ID>"
  let nodes = $state("");
  let templates = $state<Template[]>([]);
  let managing = $state(false);
  const chosenTemplate = $derived(start.startsWith("t:") ? templates.find((x) => "t:" + x.id === start) : undefined);
  const nodeKind = $derived(nodeInput(nodes, "")?.kind);

  async function loadTemplates() {
    templates = (await listAll<Template>("/api/v1/templates"));
  }

  // 打开新建对话框：重置输入，默认选中默认模板，没有时用第一个预设
  async function openNew() {
    name = "";
    nodes = "";
    creating = true;
    try {
      await loadTemplates();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
    const d = templates.find((x) => x.default);
    start = d ? "t:" + d.id : "p:" + (catalog.meta?.presets[0]?.id ?? "mihomo");
  }

  async function setDefault(x: Template) {
    try {
      await api.patch<Template>("/api/v1/templates/" + x.id, { default: true });
      await loadTemplates();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function removeTemplate(x: Template) {
    try {
      await api.del("/api/v1/templates/" + x.id);
      if (start === "t:" + x.id) start = "p:" + (catalog.meta?.presets[0]?.id ?? "mihomo");
      await loadTemplates();
      toasts.push("ok", t("templates.deleted", { name: x.name }));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
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
      const { profile, dropped } = await createProfileFrom({
        name: name.trim(),
        template: chosenTemplate,
        preset: start.startsWith("p:") ? start.slice(2) : undefined,
        nodes,
        manualName: t("profiles.manual_nodes"),
      });
      if (dropped) toasts.push("info", t("templates.dropped", { n: dropped }));
      creating = false;
      router.go("/profiles/" + profile.id);
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
      <button class="btn primary" type="button" onclick={() => void openNew()}><Icon name="plus" size={15} />{t("profiles.new")}</button>
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
    <!-- 起点：模板（反馈 #51）或预设；默认模板预先选中 -->
    <div>
      <label class="label" for="np-start">{t("profiles.start")}</label>
      <select id="np-start" class="select" bind:value={start}>
        {#if templates.length}
          <optgroup label={t("templates.group")}>
            {#each templates as x (x.id)}
              <option value={"t:" + x.id}>{x.name}{x.default ? " · " + t("templates.default") : ""}</option>
            {/each}
          </optgroup>
        {/if}
        <optgroup label={t("profiles.blank")}>
          {#each catalog.meta?.presets ?? [] as x (x.id)}
            <option value={"p:" + x.id}>{catalog.label(x.label)}</option>
          {/each}
        </optgroup>
      </select>
      <p class="hint">
        {#if chosenTemplate}{chosenTemplate.note || t("templates.no_note")}{:else}{catalog.label(catalog.meta?.presets.find((x) => "p:" + x.id === start)?.notes)}{/if}
        {#if templates.length}<button type="button" class="linkish" onclick={() => (managing = true)}>{t("templates.manage")}</button>{/if}
      </p>
    </div>
    <!-- 顺手添加节点（反馈 #52）：粘贴即可，格式由服务器识别；可以留空以后再加 -->
    <div>
      <label class="label" for="np-nodes">{t("profiles.nodes")}</label>
      <textarea id="np-nodes" class="input mono" rows="3" bind:value={nodes} placeholder={t("profiles.nodes_placeholder")} spellcheck="false"></textarea>
      <p class="hint">{nodeKind ? t("profiles.nodes_kind." + nodeKind) : t("profiles.nodes_hint")}</p>
    </div>
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (creating = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="new-profile" disabled={busy || !name.trim()}>{busy ? t("profiles.creating") : t("common.create")}</button>
  {/snippet}
</Dialog>

<Dialog bind:open={managing} title={t("templates.manage")}>
  <ul class="tpl-list">
    {#each templates as x (x.id)}
      <li>
        <div class="tpl-text">
          <strong>{x.name}</strong>
          {#if x.note}<span class="muted">{x.note}</span>{/if}
        </div>
        {#if x.default}
          <span class="badge-default">{t("templates.default")}</span>
        {:else}
          <button class="btn sm ghost" type="button" onclick={() => void setDefault(x)}>{t("templates.make_default")}</button>
        {/if}
        <button class="btn sm ghost danger" type="button" onclick={() => void removeTemplate(x)}>{t("common.delete")}</button>
      </li>
    {/each}
  </ul>
  <p class="hint">{t("templates.how")}</p>
</Dialog>

<style>
  .linkish {
    margin-inline-start: var(--space-2);
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-fg);
    font: inherit;
    cursor: pointer;
  }
  .tpl-list {
    display: grid;
    gap: var(--space-2);
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .tpl-list li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: var(--space-2);
    border-block-end: 1px solid var(--border);
  }
  .tpl-text {
    display: grid;
    flex: 1;
    min-inline-size: 0;
    font-size: var(--text-sm);
  }
  .badge-default {
    padding-inline: var(--space-2);
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-xs);
  }
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
