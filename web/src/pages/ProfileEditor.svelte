<script lang="ts">
  // 方案编辑（第 16.1 节）：分节导航 · 字段卡片 · 预览栏。
  // 宽 ≥ 1280 三栏；768–1279 预览栏改为抽屉；< 768 单栏，底部切换「设置 / 预览」（第 16.5 节）。
  import { onMount, tick } from "svelte";
  import Icon from "../components/Icon.svelte";
  import PreviewPane from "../components/editor/PreviewPane.svelte";
  import RawEditor from "../components/editor/RawEditor.svelte";
  import SectionView from "../components/editor/SectionView.svelte";
  import SourcesPanel from "../components/editor/SourcesPanel.svelte";
  import { catalog, type MetaField, type NavSection } from "../lib/catalog.svelte";
  import { EditorState } from "../lib/editor.svelte";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { palette, type PaletteItem } from "../lib/palette.svelte";
  import { router } from "../lib/router.svelte";
  import { shell } from "../lib/shell.svelte";

  let { params }: { params: Record<string, string> } = $props();

  const editor = new EditorState();
  let loadError = $state("");
  let section = $state(new URLSearchParams(location.search).get("section") || "sources");
  let rawOpen = $state(false);
  let rawKey = $state<string | undefined>();
  let previewOpen = $state(false); // 抽屉 / 手机视图中预览是否可见

  const nav = $derived(catalog.meta?.pack.nav ?? []);
  const current = $derived(nav.find((s) => s.id === section));
  const preset = $derived(catalog.meta?.presets.find((p) => p.id === editor.doc?.target.preset));
  const core = $derived(editor.doc?.target.core || catalog.meta?.pack.coreMax || "");
  const managed = $derived(preset?.managed ?? []);

  onMount(() => {
    void Promise.all([catalog.load(), editor.load(params.id!)]).catch((e) => (loadError = errorText(e)));

    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        void editor.save();
      }
    };
    const onUnload = (e: BeforeUnloadEvent) => {
      if (editor.dirty) e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    window.addEventListener("beforeunload", onUnload);
    router.guard = () => !editor.dirty || confirm(t("editor.leave_confirm"));
    const off = palette.register("editor", paletteItems);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("beforeunload", onUnload);
      router.guard = null;
      off();
      editor.dispose();
      shell.profile = null;
    };
  });

  // 向顶栏报告方案状态
  $effect(() => {
    if (!editor.profile) return;
    shell.profile = {
      id: editor.profile.id,
      name: editor.profile.name,
      check: editor.check,
      raw: editor.raw.trim() !== "",
      dirty: editor.dirty,
      diagnostics: editor.counts,
      openRaw: () => openRaw(),
      runCheck: () => void editor.runPreview(true),
    };
  });

  function select(id: string) {
    section = id;
    rawOpen = false;
    const q = new URLSearchParams(location.search);
    q.set("section", id);
    history.replaceState(null, "", location.pathname + "?" + q.toString());
    document.getElementById("editor-main")?.scrollTo({ top: 0 });
  }

  function openRaw(s?: NavSection) {
    rawKey = s?.keys[0];
    rawOpen = true;
  }

  /** 定位字段：切到所在分节，展开折叠的组，滚动并闪烁提示。诊断路径中的 [i] 下标被去掉。 */
  async function locate(path: string) {
    const clean = path.replace(/\[[^\]]*\]/g, "").replace(/\.+$/, "");
    const top = clean.split(".")[0]!;
    const sec = nav.find((s) => s.keys.includes(top));
    if (!sec) return;
    select(sec.id);
    previewOpen = false;
    editor.focus = clean;
    await tick();
    await tick();
    const parts = clean.split(".");
    for (let n = parts.length; n > 0; n--) {
      const el = document.getElementById("f-" + parts.slice(0, n).join("."));
      if (el) {
        el.scrollIntoView({ block: "center", behavior: "smooth" });
        el.classList.remove("flash");
        void el.offsetWidth;
        el.classList.add("flash");
        el.querySelector<HTMLElement>("input, select, textarea, button[role=switch], [role=radio]")?.focus({ preventScroll: true });
        break;
      }
    }
  }

  // 命令面板：字段搜索（标题、键、摘要、详细说明全文）与命令模式中的「原始编辑」。
  function paletteItems(): PaletteItem[] {
    const out: PaletteItem[] = [
      { id: "raw.open", kind: "command", title: t("raw.title"), hint: "raw edit", icon: "bolt", run: () => openRaw() },
      { id: "cmd.save", kind: "command", title: t("editor.save"), hint: "Ctrl S", icon: "check", run: () => void editor.save() },
      { id: "cmd.check", kind: "command", title: t("check.run"), hint: "mihomo -t", icon: "ok", run: () => void editor.runPreview(true) },
    ];
    const walk = (f: MetaField, path: string[], secTitle: string) => {
      if (f.type === "object" && f.fields?.length) {
        for (const c of f.fields) walk(c, [...path, c.key], secTitle);
        return;
      }
      const key = path.join(".");
      out.push({
        id: "field." + key,
        kind: "field",
        title: catalog.title(f),
        hint: key,
        text: catalog.summary(f) + " " + catalog.detail(f) + " " + secTitle,
        run: () => void locate(key),
      });
    };
    for (const s of nav) {
      if (s.generated) continue;
      for (const k of s.keys) {
        const f = catalog.top(k);
        if (f) walk(f, [k], catalog.navTitle(s));
      }
    }
    return out;
  }

  // 分节导航上的计数：已设置的顶层路径
  function sectionSetCount(s: NavSection): number {
    void editor.rev;
    const paths = editor.doc?.formPaths() ?? [];
    return paths.filter((p) => s.keys.includes(p[0]!)).length;
  }

  function sectionHasRaw(s: NavSection): boolean {
    return (editor.preview?.touched ?? []).some((p) => s.keys.includes(p.split(".")[0]!.replace(/\[.*$/, "")));
  }
</script>

{#if loadError}
  <div class="load-error" role="alert"><Icon name="error" />{loadError}</div>
{:else if !editor.profile || !catalog.meta}
  <div class="loading muted" aria-busy="true">{t("common.loading")}</div>
{:else}
  <div class="editor" class:preview-open={previewOpen}>
    <nav class="secnav" aria-label={t("editor.sections")}>
      <div class="secnav-head">
        <span class="subtle label-sm">{t("editor.profile")}</span>
        <div class="profile-links">
          <a href={"/profiles/" + editor.profile.id + "/links"}><Icon name="link" size={14} />{t("profiles.links")}</a>
          <a href={"/profiles/" + editor.profile.id + "/history"}><Icon name="history" size={14} />{t("profiles.history")}</a>
        </div>
      </div>
      <ul>
        <li>
          <button type="button" class:active={section === "sources" && !rawOpen} aria-current={section === "sources" && !rawOpen ? "true" : undefined} onclick={() => select("sources")}>
            <Icon name="server" size={15} />
            <span class="t">{t("editor.sources_rules")}</span>
            <span class="n">{editor.sourceRefs.length || ""}</span>
          </button>
        </li>
      </ul>
      <span class="subtle label-sm group-label">{t("editor.settings")}</span>
      <ul>
        {#each nav as s (s.id)}
          {@const n = sectionSetCount(s)}
          <li>
            <button type="button" class:active={section === s.id && !rawOpen} aria-current={section === s.id && !rawOpen ? "true" : undefined} onclick={() => select(s.id)}>
              <span class="t">{catalog.navTitle(s)}</span>
              {#if sectionHasRaw(s)}<span class="raw-dot" title={t("raw.badge_hint")}><Icon name="bolt" size={11} /></span>{/if}
              {#if s.generated}<span class="gen" title={t("editor.generated_hint")}><Icon name="sparkle" size={12} /></span>{/if}
              <span class="n">{n || ""}</span>
            </button>
          </li>
        {/each}
      </ul>
      <select class="select secnav-select" aria-label={t("editor.sections")} value={section} onchange={(e) => select(e.currentTarget.value)}>
        <option value="sources">{t("editor.sources_rules")}</option>
        {#each nav as s (s.id)}<option value={s.id}>{catalog.navTitle(s)}</option>{/each}
      </select>
    </nav>

    <div class="content" id="editor-main">
      <div class="content-inner">
        {#if rawOpen}
          <RawEditor {editor} focusKey={rawKey} onClose={() => (rawOpen = false)} />
        {:else if section === "sources" || !current}
          <SourcesPanel {editor} />
        {:else if current.generated}
          <section class="generated">
            <h2>{catalog.navTitle(current)}</h2>
            <div class="gen-card panel">
              <Icon name="sparkle" size={20} />
              <div>
                <p>{current.generated === "sources" ? t("editor.generated_sources") : t("editor.generated_rules")}</p>
                <div class="gen-actions">
                  <button class="btn sm" type="button" onclick={() => select("sources")}>{t("editor.sources_rules")}</button>
                  <a class="btn sm ghost" href={current.generated === "sources" ? "/sources" : "/rules"}>{current.generated === "sources" ? t("editor.manage_sources") : t("editor.manage_rules")}</a>
                </div>
                <p class="hint">{t("editor.generated_raw")}</p>
              </div>
            </div>
            <ul class="gen-keys">
              {#each current.keys as k (k)}
                {@const f = catalog.top(k)}
                {#if f}
                  <li><code class="ltr">{k}</code><span>{catalog.title(f)}</span><span class="muted">{catalog.summary(f)}</span></li>
                {/if}
              {/each}
            </ul>
          </section>
        {:else}
          {#key current.id}
            <SectionView section={current} {editor} {core} {managed} onRaw={(s) => openRaw(s)} />
          {/key}
        {/if}
      </div>

      {#if editor.dirty}
        <div class="savebar" role="region" aria-label={t("editor.unsaved")}>
          <span class="dot"></span>
          <span>{t("editor.unsaved")}</span>
          <span class="spacer"></span>
          <button class="btn sm ghost" type="button" onclick={() => void editor.discard()}>{t("editor.discard")}</button>
          <button class="btn sm primary" type="button" disabled={editor.saving} onclick={() => void editor.save()}>
            {editor.saving ? t("editor.saving") : t("editor.save")}<span class="kbd on-accent">Ctrl S</span>
          </button>
        </div>
      {/if}
    </div>

    <aside class="preview" aria-label={t("preview.title")}>
      <PreviewPane {editor} onLocate={(p) => void locate(p)} />
    </aside>
    {#if previewOpen}
      <button class="scrim" type="button" aria-label={t("common.close")} onclick={() => (previewOpen = false)}></button>
    {/if}

    <div class="mobile-switch" role="tablist" aria-label={t("editor.view")}>
      <button role="tab" type="button" aria-selected={!previewOpen} onclick={() => (previewOpen = false)}><Icon name="settings" size={15} />{t("editor.settings")}</button>
      <button role="tab" type="button" aria-selected={previewOpen} onclick={() => (previewOpen = true)}>
        <Icon name="code" size={15} />{t("preview.title")}
        {#if editor.counts.errors}<span class="badge-err">{editor.counts.errors}</span>{/if}
      </button>
    </div>
    <button class="btn preview-toggle" type="button" aria-expanded={previewOpen} onclick={() => (previewOpen = !previewOpen)}>
      <Icon name="code" size={15} />{t("preview.title")}
      {#if editor.counts.errors}<span class="badge-err">{editor.counts.errors}</span>{/if}
    </button>
  </div>
{/if}

<style>
  .editor {
    position: relative;
    display: grid;
    grid-template-columns: 208px minmax(0, 1fr) var(--preview-w);
    block-size: 100%;
    min-block-size: 0;
  }
  .loading,
  .load-error {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-8);
  }
  .load-error {
    color: var(--err);
  }

  /* —— 分节导航 —— */
  .secnav {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-4) var(--space-2);
    border-inline-end: 1px solid var(--border);
    overflow-y: auto;
  }
  .secnav-head {
    display: grid;
    gap: var(--space-2);
    padding-inline: var(--space-2);
    margin-block-end: var(--space-3);
  }
  .profile-links {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }
  .profile-links a {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--fg-muted);
  }
  .profile-links a:hover {
    color: var(--fg);
    text-decoration: none;
  }
  .label-sm {
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .group-label {
    padding-inline: var(--space-2);
    margin-block: var(--space-4) var(--space-1);
  }
  .secnav ul {
    display: grid;
    gap: 1px;
  }
  .secnav button {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    inline-size: 100%;
    block-size: 32px;
    padding-inline: var(--space-2);
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-md);
    text-align: start;
    cursor: pointer;
  }
  .secnav button:hover {
    background: var(--bg-hover);
    color: var(--fg);
  }
  .secnav button.active {
    background: var(--bg-active);
    color: var(--fg);
    font-weight: 560;
  }
  .t {
    flex: 1;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .n {
    font-size: var(--text-xs);
    color: var(--accent-fg);
    font-variant-numeric: tabular-nums;
  }
  .raw-dot {
    display: grid;
    place-items: center;
    inline-size: 16px;
    block-size: 16px;
    border-radius: 50%;
    background: var(--raw-soft);
    color: var(--raw);
  }
  .gen {
    display: grid;
    color: var(--fg-subtle);
  }
  .secnav-select {
    display: none;
  }

  /* —— 主区 —— */
  .content {
    position: relative;
    display: flex;
    flex-direction: column;
    min-block-size: 0;
    overflow-y: auto;
  }
  .content-inner {
    flex: 1;
    inline-size: 100%;
    max-inline-size: var(--content-max);
    margin-inline: auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }
  .savebar {
    position: sticky;
    inset-block-end: 0;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border-block-start: 1px solid var(--border);
    background: color-mix(in srgb, var(--bg-raised) 92%, transparent);
    backdrop-filter: blur(6px);
    font-size: var(--text-sm);
  }
  .savebar .dot {
    inline-size: 7px;
    block-size: 7px;
    border-radius: 50%;
    background: var(--warn);
  }
  .spacer {
    flex: 1;
  }
  .kbd.on-accent {
    margin-inline-start: var(--space-1);
    border-color: color-mix(in srgb, var(--fg-on-accent) 40%, transparent);
    background: transparent;
    color: var(--fg-on-accent);
    opacity: 0.85;
  }

  .generated {
    display: grid;
    gap: var(--space-4);
  }
  .generated h2 {
    font-size: var(--text-xl);
  }
  .gen-card {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
    color: var(--fg-muted);
  }
  .gen-card > :global(.icon) {
    color: var(--accent-fg);
    margin-block-start: 2px;
  }
  .gen-card p:first-child {
    color: var(--fg);
  }
  .gen-actions {
    display: flex;
    gap: var(--space-2);
    margin-block: var(--space-3);
  }
  .gen-keys {
    display: grid;
    gap: var(--space-2);
  }
  .gen-keys li {
    display: grid;
    grid-template-columns: 9rem 8rem minmax(0, 1fr);
    gap: var(--space-3);
    font-size: var(--text-sm);
  }
  .gen-keys code {
    color: var(--fg-subtle);
  }

  /* —— 预览栏 —— */
  .preview {
    min-block-size: 0;
    border-inline-start: 1px solid var(--border);
    overflow: hidden;
  }
  .preview-toggle,
  .mobile-switch,
  .scrim {
    display: none;
  }
  .badge-err {
    min-inline-size: 18px;
    padding-inline: 5px;
    border-radius: var(--radius-full);
    background: var(--err);
    color: #fff;
    font-size: 11px;
    text-align: center;
  }

  /* 768–1279：预览栏改为抽屉 */
  @media (max-width: 1279px) {
    .editor {
      grid-template-columns: 196px minmax(0, 1fr);
    }
    .preview {
      position: absolute;
      z-index: 20;
      inset-block: 0;
      inset-inline-end: 0;
      inline-size: min(var(--preview-w), 92%);
      border-inline-start: 1px solid var(--border);
      box-shadow: var(--shadow-lg);
      transform: translateX(105%);
      transition: transform var(--dur) var(--ease);
    }
    :global([dir="rtl"]) .preview {
      transform: translateX(-105%);
    }
    .preview-open .preview,
    :global([dir="rtl"]) .preview-open .preview {
      transform: none;
    }
    .preview-open .scrim {
      display: block;
      position: absolute;
      inset: 0;
      z-index: 15;
      border: 0;
      background: var(--scrim);
    }
    .preview-toggle {
      display: inline-flex;
      position: absolute;
      z-index: 10;
      inset-block-start: var(--space-3);
      inset-inline-end: var(--space-4);
      box-shadow: var(--shadow-sm);
    }
  }

  /* < 768：单栏，分节导航变为下拉，底部切换设置与预览 */
  @media (max-width: 767px) {
    .editor {
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: auto minmax(0, 1fr) auto;
    }
    .secnav {
      flex-direction: row;
      padding: var(--space-2) var(--space-3);
      border-inline-end: 0;
      border-block-end: 1px solid var(--border);
      overflow: visible;
    }
    .secnav > :not(.secnav-select) {
      display: none;
    }
    .secnav-select {
      display: block;
    }
    .content-inner {
      padding: var(--space-4) var(--space-4) var(--space-8);
    }
    .preview {
      position: static;
      grid-row: 2;
      inline-size: auto;
      border: 0;
      box-shadow: none;
      transform: none;
      display: none;
    }
    :global([dir="rtl"]) .preview {
      transform: none;
    }
    .preview-open .preview {
      display: block;
    }
    .preview-open .content {
      display: none;
    }
    .preview-open .scrim,
    .preview-toggle {
      display: none;
    }
    .mobile-switch {
      display: grid;
      grid-template-columns: 1fr 1fr;
      border-block-start: 1px solid var(--border);
      background: var(--bg-raised);
    }
    .mobile-switch button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: var(--space-2);
      block-size: 48px;
      border: 0;
      background: none;
      color: var(--fg-muted);
      font: inherit;
      font-weight: 540;
    }
    .mobile-switch button[aria-selected="true"] {
      color: var(--accent-fg);
      box-shadow: inset 0 2px 0 var(--accent);
    }
    .gen-keys li {
      grid-template-columns: 1fr;
      gap: 0;
    }
  }
</style>
