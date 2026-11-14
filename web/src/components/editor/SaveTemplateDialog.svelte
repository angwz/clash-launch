<script lang="ts">
  // 把当前配置存为模板（反馈 #51、#60）：新建一个模板，或用当前配置更新已有模板。
  // 模板保存配置文档的全部内容（节点与写法、分流规则、链式代理、内核设置、直接编辑），新建配置时只换名称。
  import Dialog from "../Dialog.svelte";
  import { api, listAll, type Template } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  let { open = $bindable(false), doc, profileName }: { open?: boolean; doc: string; profileName: string } = $props();

  let mode = $state<"new" | "update">("new");
  let templates = $state<Template[]>([]);
  let target = $state("");
  let name = $state("");
  let note = $state("");
  let isDefault = $state(true);
  let busy = $state(false);
  const chosen = $derived(templates.find((x) => x.id === target));

  $effect(() => {
    if (!open) return;
    mode = "new";
    name = "";
    note = "";
    isDefault = true;
    listAll<Template>("/api/v1/templates")
      .then((list) => {
        templates = list;
        target = list[0]?.id ?? "";
      })
      .catch((e) => toasts.push("err", errorText(e)));
  });

  // 名称留空时使用默认名称（反馈 #65）
  const defaultName = $derived(t("templates.default_name", { name: profileName }));

  async function save(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      if (mode === "update" && chosen) {
        await api.patch<Template>("/api/v1/templates/" + chosen.id, { doc, ...(isDefault ? { default: true } : {}) });
        toasts.push("ok", t("templates.updated", { name: chosen.name }));
      } else {
        const n = name.trim() || defaultName;
        await api.post<Template>("/api/v1/templates", { name: n, note: note.trim(), doc, default: isDefault });
        toasts.push("ok", t("templates.saved", { name: n }));
      }
      open = false;
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }
</script>

<Dialog bind:open title={t("templates.save_title")}>
  <form id="save-template" class="form" onsubmit={save}>
    <p class="muted">{t("templates.save_desc")}</p>
    {#if templates.length}
      <div class="modes" role="radiogroup" aria-label={t("templates.save_title")}>
        <label><input type="radio" name="tpl-mode" value="new" bind:group={mode} />{t("templates.mode_new")}</label>
        <label><input type="radio" name="tpl-mode" value="update" bind:group={mode} />{t("templates.mode_update")}</label>
      </div>
    {/if}
    {#if mode === "update" && templates.length}
      <div>
        <label class="label" for="tpl-target">{t("templates.update_which")}</label>
        <select id="tpl-target" class="select" bind:value={target}>
          {#each templates as x (x.id)}<option value={x.id}>{x.name}{x.default ? " · " + t("templates.default") : ""}</option>{/each}
        </select>
        <p class="hint">{t("templates.update_hint")}</p>
      </div>
    {:else}
      <div>
        <label class="label" for="tpl-name">{t("templates.name")}</label>
        <input id="tpl-name" class="input" bind:value={name} maxlength="64" placeholder={defaultName} />
      </div>
      <div>
        <label class="label" for="tpl-note">{t("templates.note")}</label>
        <textarea id="tpl-note" class="input" rows="2" bind:value={note} maxlength="500" placeholder={t("templates.note_placeholder")}></textarea>
      </div>
    {/if}
    <label class="check-row">
      <input id="tpl-default" type="checkbox" bind:checked={isDefault} />
      <span>{t("templates.set_default")}</span>
    </label>
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="save-template" disabled={busy || (mode === "update" && !chosen)}>{mode === "update" ? t("templates.update") : t("common.save")}</button>
  {/snippet}
</Dialog>

<style>
  .check-row,
  .modes label {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .modes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
  }
</style>
