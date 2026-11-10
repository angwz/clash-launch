<script lang="ts">
  // 规则源详情：地址或粘贴的文本（toml / ini）。策略组预览与可点选的选择器标签在后续迭代中加入。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import { api, type RuleSource } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { params }: { params: Record<string, string> } = $props();
  let rs = $state<RuleSource | null>(null);
  let error = $state("");
  let saving = $state(false);

  $effect(() => {
    api
      .get<RuleSource>("/api/v1/rule-sources/" + params.id)
      .then((r) => (rs = r))
      .catch((e) => (error = errorText(e)));
  });

  async function save() {
    if (!rs) return;
    saving = true;
    try {
      const { id: _id, version, createdAt: _c, updatedAt: _u, ...body } = rs;
      rs = await api.put<RuleSource>("/api/v1/rule-sources/" + rs.id, body, { ifMatch: version });
      toasts.push("ok", t("common.saved"));
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      saving = false;
    }
  }

  async function refresh() {
    try {
      await api.post("/api/v1/rule-sources/" + params.id + "/refresh");
      toasts.push("ok", t("source.refresh_queued"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function remove() {
    if (!rs || !confirm(t("source.delete_confirm", { name: rs.name }))) return;
    try {
      await api.del("/api/v1/rule-sources/" + rs.id);
      router.go("/rules");
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
</script>

<div class="page">
  {#if error}
    <p class="field-error" role="alert">{error}</p>
  {:else if rs}
    <PageHeader title={rs.name} subtitle={rs.kind === "ini" ? t("rules.ini") : "groups.toml + rulesets.toml"}>
      {#snippet actions()}
        <button class="btn" type="button" onclick={() => void refresh()}><Icon name="refresh" size={15} />{t("source.refresh")}</button>
        <button class="btn danger" type="button" onclick={() => void remove()}><Icon name="trash" size={15} />{t("common.delete")}</button>
        <button class="btn primary" type="button" disabled={saving} onclick={() => void save()}>{t("common.save")}</button>
      {/snippet}
    </PageHeader>
    <div class="grid">
      <div>
        <label class="label" for="rd-name">{t("sources.col_name")}</label>
        <input id="rd-name" class="input" bind:value={rs.name} />
      </div>
      {#if rs.kind === "toml"}
        <div>
          <label class="label" for="rd-g">groups.toml URL</label>
          <input id="rd-g" class="input mono ltr" bind:value={rs.groupsUrl} spellcheck="false" />
        </div>
        <div>
          <label class="label" for="rd-r">rulesets.toml URL</label>
          <input id="rd-r" class="input mono ltr" bind:value={rs.rulesetsUrl} spellcheck="false" />
        </div>
        <div>
          <label class="label" for="rd-gt">{t("rules.groups_text")}</label>
          <textarea id="rd-gt" class="textarea mono ltr" rows="10" bind:value={rs.groupsText} spellcheck="false" placeholder={t("rules.text_hint")}></textarea>
        </div>
        <div>
          <label class="label" for="rd-rt">{t("rules.rulesets_text")}</label>
          <textarea id="rd-rt" class="textarea mono ltr" rows="10" bind:value={rs.rulesetsText} spellcheck="false" placeholder={t("rules.text_hint")}></textarea>
        </div>
      {:else}
        <div>
          <label class="label" for="rd-i">INI URL</label>
          <input id="rd-i" class="input mono ltr" bind:value={rs.iniUrl} spellcheck="false" />
        </div>
        <div>
          <label class="label" for="rd-it">{t("rules.ini_text")}</label>
          <textarea id="rd-it" class="textarea mono ltr" rows="14" bind:value={rs.iniText} spellcheck="false" placeholder={t("rules.text_hint")}></textarea>
        </div>
      {/if}
    </div>
  {:else}
    <p class="muted">{t("common.loading")}</p>
  {/if}
</div>

<style>
  .page {
    max-inline-size: 960px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .grid {
    display: grid;
    gap: var(--space-4);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
