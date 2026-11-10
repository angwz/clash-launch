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
  import { toasts } from "../../lib/toast.svelte";

  let { field, path, editor, labelId }: { field: MetaField; path: string[]; editor: EditorState; labelId: string } = $props();

  const value = $derived(editor.value(path));
  const set = $derived(editor.isSet(path));
  // 「数字或字符串」的标量联合（如嗅探端口：443 或 8080-8880）也按一行一项编辑：纯整数存为数字
  const intOrString = $derived(
    field.items?.type === "anyOf" && (field.items.of ?? []).every((o) => o.type === "int" || o.type === "string") && (field.items.of ?? []).some((o) => o.type === "int"),
  );
  const isScalarList = $derived(field.type === "list" && !!field.items && (["string", "int", "enum"].includes(field.items.type) || intOrString));
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
      const out = field.items?.type === "int" ? items.map(Number) : intOrString ? items.map((x) => (/^-?[0-9]+$/.test(x) ? Number(x) : x)) : items;
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

  // 开关：打开对象的总开关（enable）时，把该对象尚未设置的子字段填为目录中的推荐值；已设置的值不动。
  function toggle() {
    const next = !(effective === true);
    editor.set(path, next);
    if (next && field.key === "enable") fillRecommended();
  }

  function fillRecommended() {
    const base = path.slice(0, -1);
    const rec = catalog.field(base.join("."))?.recommended;
    if (!rec) return;
    let n = 0;
    for (const [k, v] of Object.entries(rec)) {
      if (editor.isSet([...base, k])) continue;
      editor.set([...base, k], $state.snapshot(v));
      n++;
    }
    if (n) toasts.push("info", t("field.recommended_filled", { n }));
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
  {#if !set && field.default !== undefined}<span class="dflt-tag">{t("field.default_tag")}</span>{/if}
  <button
    {id}
    type="button"
    class="switch"
    role="switch"
    aria-checked={effective === true}
    aria-labelledby={labelId}
    data-set={set}
    onclick={toggle}
  >
    <span class="knob"></span>
  </button>
{:else if kind === "segmented"}
  {#if !set && field.default !== undefined}<span class="dflt-tag">{t("field.default_tag")}</span>{/if}
  <div class="seg" role="radiogroup" aria-labelledby={labelId}>
    {#each options as o (o)}
      <button
        type="button"
        role="radio"
        aria-checked={effective === o}
        class:on={set && value === o}
        class:implicit={!set && effective === o}
        title={(!set && effective === o ? t("field.default_tag") + " · " : "") + (optLabel(o) || o)}
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
    transition:
      background var(--dur) var(--ease),
      box-shadow var(--dur) var(--ease),
      outline-color var(--dur) var(--ease);
  }
  .switch:active .knob {
    inline-size: 20px;
  }
  .switch[aria-checked="true"]:active .knob {
    inset-inline-start: 12px;
  }
  .switch[aria-checked="true"] {
    background: var(--accent);
  }
  /* 未设置：只是显示默认值，用虚线框与灰色区分于「已设置」（反馈 #17：默认值不能看起来像已选中） */
  .switch[data-set="false"] {
    background: transparent;
    box-shadow: inset 0 0 0 1px var(--border-strong);
    outline: 1px dashed var(--border-strong);
    outline-offset: 2px;
  }
  .switch[data-set="false"] .knob {
    background: var(--border-strong);
    box-shadow: none;
  }
  .dflt-tag {
    font-size: var(--text-xs);
    color: var(--fg-subtle, var(--fg-muted));
    margin-inline-end: var(--space-2);
    white-space: nowrap;
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
    transition:
      inset-inline-start var(--dur) var(--ease-spring),
      inline-size var(--dur-fast) var(--ease),
      background var(--dur) var(--ease);
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
    transition:
      background var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease),
      box-shadow var(--dur-fast) var(--ease),
      outline-color var(--dur-fast) var(--ease);
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
    background: transparent;
    outline: 1px dashed var(--border-strong);
    outline-offset: -3px;
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
