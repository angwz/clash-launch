<script lang="ts">
  // 把当前配置存为模板（反馈 #51）：名称、说明（如适用的客户端与 mihomo 版本）、是否设为新建配置的默认。
  // 模板保存配置文档的全部内容（节点与写法、分流规则、链式代理、内核设置、直接编辑），新建时只换名称。
  import Dialog from "../Dialog.svelte";
  import { api, type Template } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  let { open = $bindable(false), doc, profileName }: { open?: boolean; doc: string; profileName: string } = $props();

  let name = $state("");
  let note = $state("");
  let isDefault = $state(true);
  let busy = $state(false);
  $effect(() => {
    if (open) {
      name = profileName;
      note = "";
      isDefault = true;
    }
  });

  async function save(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      await api.post<Template>("/api/v1/templates", { name: name.trim(), note: note.trim(), doc, default: isDefault });
      toasts.push("ok", t("templates.saved", { name: name.trim() }));
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
    <div>
      <label class="label" for="tpl-name">{t("templates.name")}</label>
      <input id="tpl-name" class="input" bind:value={name} required maxlength="64" />
    </div>
    <div>
      <label class="label" for="tpl-note">{t("templates.note")}</label>
      <textarea id="tpl-note" class="input" rows="2" bind:value={note} maxlength="500" placeholder={t("templates.note_placeholder")}></textarea>
    </div>
    <label class="check-row">
      <input id="tpl-default" type="checkbox" bind:checked={isDefault} />
      <span>{t("templates.set_default")}</span>
    </label>
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="save-template" disabled={busy || !name.trim()}>{t("common.save")}</button>
  {/snippet}
</Dialog>

<style>
  .check-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
</style>
