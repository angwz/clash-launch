<script lang="ts">
  // 字段控件（第 16.2 节「控件（按类型）与默认值（灰显）」）。
  // 标量：开关、分段 / 下拉、整数、文本（敏感值可显示 / 隐藏）；标量列表：一行一项；
  // 映射、对象列表、any/anyOf/oneOf：YAML 片段。值写入文档前只检查类型与格式，语义校验由后端预览完成。
  import { parse, stringify } from "yaml";
  import type { MetaField } from "../../lib/catalog.svelte";
  import { catalog } from "../../lib/catalog.svelte";
  import type { EditorState } from "../../lib/editor.svelte";
  import { t } from "../../lib/i18n/i18n.svelte";
  import Icon from "../Icon.svelte";

  let { field, path, editor, labelId }: { field: MetaField; path: string[]; editor: EditorState; labelId: string } = $props();

  const value = $derived(editor.value(path));
  const set = $derived(editor.isSet(path));
  const isScalarList = $derived(field.type === "list" && !!field.items && ["string", "int", "enum"].includes(field.items.type));
  const kind = $derived(
    field.type === "bool"
      ? "bool"
      : field.type === "enum"
        ? field.ui?.widget === "segmented" || (field.enum?.length ?? 0) <= 3
          ? "segmented"
          : "select"
        : field.type === "int"
          ? "int"
          : field.type === "string"
            ? "string"
            : isScalarList
              ? "lines"
              : "yaml",
  );

  const effective = $derived(set ? value : field.default);
  const id = $derived("ctl-" + path.join("."));

  // —— 文本类控件的草稿：提交时才写入文档 ——
  let draft = $state("");
  let error = $state("");
  let reveal = $state(false);

  $effect(() => {
    // 文档变化（含外部修改、恢复默认）时同步草稿
    draft = kind === "lines" ? toLines(value) : kind === "yaml" ? toYAML(value) : value === undefined || value === null ? "" : String(value);
    error = "";
  });

  function toLines(v: unknown): string {
    return Array.isArray(v) ? v.map(String).join("\n") : "";
  }

  function toYAML(v: unknown): string {
    if (v === undefined) return "";
    return stringify(v, { lineWidth: 0, indent: 2 }).trimEnd();
  }

  function commitText() {
    const s = draft;
    if (kind === "int") {
      if (s.trim() === "") return reset();
      if (!/^-?\d+$/.test(s.trim())) return void (error = t("field.err_int"));
      const n = Number(s.trim());
      if (field.ui?.widget === "port" && (n < 0 || n > 65535)) return void (error = t("field.err_port"));
      error = "";
      if (n !== value) editor.set(path, n);
      return;
    }
    if (kind === "string") {
      error = "";
      if (s === "" && !set) return;
      if (s !== value) editor.set(path, s);
      return;
    }
    if (kind === "lines") {
      const items = s.split("\n").map((x) => x.trim()).filter(Boolean);
      if (!items.length) return reset();
      const out = field.items?.type === "int" ? items.map(Number) : items;
      if (field.items?.type === "int" && out.some((x) => !Number.isInteger(x))) return void (error = t("field.err_int_list"));
      error = "";
      editor.set(path, out);
      return;
    }
    if (kind === "yaml") {
      if (s.trim() === "") return reset();
      try {
        const v = parse(s, { uniqueKeys: true });
        if (field.type === "map" && (typeof v !== "object" || v === null || Array.isArray(v))) return void (error = t("field.err_map"));
        if (field.type === "list" && !Array.isArray(v)) return void (error = t("field.err_list"));
        error = "";
        editor.set(path, v);
      } catch (e) {
        error = t("field.err_yaml", { detail: e instanceof Error ? e.message.split("\n")[0] : String(e) });
      }
    }
  }

  function reset() {
    error = "";
    editor.reset(path);
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Enter" && (kind === "int" || kind === "string")) {
      e.preventDefault();
      commitText();
    } else if (e.key === "Escape") {
      draft = kind === "lines" ? toLines(value) : kind === "yaml" ? toYAML(value) : value === undefined ? "" : String(value);
      error = "";
    }
  }

  const placeholder = $derived(
    field.default !== undefined && field.default !== null
      ? t("field.default_is", { value: typeof field.default === "object" ? JSON.stringify(field.default) : String(field.default) })
      : kind === "lines"
        ? t("field.one_per_line")
        : "",
  );

  const options = $derived(field.enum ?? []);
  const optLabel = (v: string) => catalog.enumLabel(field, v);
</script>

{#if kind === "bool"}
  <button
    {id}
    type="button"
    class="switch"
    role="switch"
    aria-checked={effective === true}
    aria-labelledby={labelId}
    data-set={set}
    onclick={() => editor.set(path, !(effective === true))}
  >
    <span class="knob"></span>
  </button>
{:else if kind === "segmented"}
  <div class="seg" role="radiogroup" aria-labelledby={labelId}>
    {#each options as o (o)}
      <button
        type="button"
        role="radio"
        aria-checked={effective === o}
        class:on={effective === o}
        class:implicit={!set && effective === o}
        title={optLabel(o) || undefined}
        onclick={() => (set && value === o ? reset() : editor.set(path, o))}
      ><span class="ltr">{o}</span></button>
    {/each}
  </div>
{:else if kind === "select"}
  <select {id} class="select ltr ctl" aria-labelledby={labelId} value={set ? String(value) : ""} onchange={(e) => (e.currentTarget.value === "" ? reset() : editor.set(path, e.currentTarget.value))}>
    <option value="">{field.default !== undefined ? t("field.default_option", { value: String(field.default) }) : t("field.unset_option")}</option>
    {#each options as o (o)}
      <option value={o}>{o}{optLabel(o) ? " — " + optLabel(o) : ""}</option>
    {/each}
  </select>
{:else if kind === "int" || kind === "string"}
  <div class="text-wrap" class:wide={kind === "string"}>
    <input
      {id}
      class="input mono ltr ctl"
      class:num={kind === "int"}
      type={field.sensitive && !reveal ? "password" : "text"}
      inputmode={kind === "int" ? "numeric" : undefined}
      aria-labelledby={labelId}
      aria-invalid={error !== ""}
      bind:value={draft}
      onchange={commitText}
      onkeydown={onKey}
      {placeholder}
      autocomplete="off"
      spellcheck="false"
    />
    {#if field.sensitive}
      <button class="btn ghost icon sm reveal" type="button" aria-label={reveal ? t("field.hide") : t("field.show")} onclick={() => (reveal = !reveal)}>
        <Icon name={reveal ? "eye-off" : "eye"} size={14} />
      </button>
    {/if}
  </div>
{:else}
  <textarea
    {id}
    class="textarea mono ltr block"
    rows={Math.min(12, Math.max(kind === "lines" ? 3 : 4, draft.split("\n").length + 1))}
    aria-labelledby={labelId}
    aria-invalid={error !== ""}
    bind:value={draft}
    onchange={commitText}
    onkeydown={onKey}
    placeholder={kind === "yaml" ? t("field.yaml_placeholder") : placeholder}
    spellcheck="false"
  ></textarea>
{/if}
{#if error}<p class="field-error" role="alert">{error}</p>{/if}

<style>
  .switch {
    position: relative;
    flex: none;
    inline-size: 34px;
    block-size: 20px;
    padding: 0;
    border: 0;
    border-radius: var(--radius-full);
    background: var(--border-strong);
    cursor: pointer;
    transition: background var(--dur-fast) var(--ease);
  }
  .switch[aria-checked="true"] {
    background: var(--accent);
  }
  .switch[data-set="false"] {
    opacity: 0.6;
  }
  .knob {
    position: absolute;
    inset-block-start: 2px;
    inset-inline-start: 2px;
    inline-size: 16px;
    block-size: 16px;
    border-radius: 50%;
    background: #fff;
    box-shadow: 0 1px 2px rgb(0 0 0 / 0.25);
    transition: inset-inline-start var(--dur-fast) var(--ease);
  }
  .switch[aria-checked="true"] .knob {
    inset-inline-start: 16px;
  }
  .seg {
    display: inline-flex;
    flex-wrap: wrap;
    padding: 2px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
  }
  .seg button {
    block-size: 26px;
    padding-inline: var(--space-3);
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--fg-muted);
    font: inherit;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .seg button:hover {
    color: var(--fg);
  }
  .seg button.on {
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-sm);
  }
  .seg button.implicit {
    color: var(--fg-muted);
    font-style: italic;
  }
  .ctl {
    inline-size: auto;
    min-inline-size: 180px;
    max-inline-size: 100%;
  }
  .text-wrap {
    position: relative;
    display: inline-flex;
    max-inline-size: 100%;
  }
  .text-wrap.wide .ctl {
    inline-size: 280px;
  }
  .num {
    min-inline-size: 120px;
    inline-size: 140px;
  }
  .reveal {
    position: absolute;
    inset-inline-end: 3px;
    inset-block-start: 3px;
  }
  .block {
    inline-size: 100%;
  }
</style>
