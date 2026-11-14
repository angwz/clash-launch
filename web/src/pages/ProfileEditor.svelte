<script lang="ts">
  // 方案编辑（第 16.1 节）：分节导航 · 字段卡片 · 预览栏。
  // 预览：宽屏编辑设置时停靠在右侧；「源与规则」与 768–1279 时收起，点击后浮层滑出；< 768 单栏，底部切换「设置 / 预览」。
  import { onMount, tick } from "svelte";
  import Icon from "../components/Icon.svelte";
  import PreviewPane from "../components/editor/PreviewPane.svelte";
  import RawEditor from "../components/editor/RawEditor.svelte";
  import SectionView from "../components/editor/SectionView.svelte";
  import SourcesPanel from "../components/editor/SourcesPanel.svelte";
  import ChainsPanel from "../components/editor/ChainsPanel.svelte";
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
  let previewOpen = $state(false); // 浮层 / 手机视图中预览是否可见
  // 预览的呈现（反馈 #28）：宽屏上编辑设置时停靠在右侧并高亮当前字段；
  // 在「源与规则」与窄屏上默认收起，点击后以浮层滑出。停靠可手动收起，本次会话内记住。
  let wide = $state(matchMedia("(min-width: 1280px)").matches);
  let mobile = $state(matchMedia("(max-width: 767px)").matches);
  let dockHidden = $state(false);
  const docked = $derived(wide && !dockHidden && section !== "sources");
  const floating = $derived(!docked && !mobile);

  const nav = $derived(catalog.meta?.pack.nav ?? []);
  // 「节点与策略组」「规则」两个生成分节只是指回「节点与分流」，与左侧主菜单重复，不再列出（反馈 #33）
  const settingsNav = $derived(nav.filter((s) => !s.generated));
  const current = $derived(settingsNav.find((s) => s.id === section));
  const core = $derived(editor.doc?.target.core || catalog.meta?.pack.coreMax || "");

  onMount(() => {
    void Promise.all([catalog.load(), editor.load(params.id!)]).catch((e) => (loadError = errorText(e)));

    const mqWide = matchMedia("(min-width: 1280px)");
    const mqMobile = matchMedia("(max-width: 767px)");
    const onMq = () => ((wide = mqWide.matches), (mobile = mqMobile.matches));
    mqWide.addEventListener("change", onMq);
    mqMobile.addEventListener("change", onMq);

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && previewOpen && floating) {
        previewOpen = false;
        return;
      }
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
    // 自动保存：离开时立即保存尚未保存的修改，不再询问（反馈 #35）
    router.guard = () => {
      if (editor.dirty) void editor.save();
      return true;
    };
    const off = palette.register("editor", paletteItems);
    return () => {
      window.removeEventListener("keydown", onKey);
      mqWide.removeEventListener("change", onMq);
      mqMobile.removeEventListener("change", onMq);
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
      preview: { available: !docked && !mobile, open: previewOpen, toggle: togglePreview },
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

  function togglePreview() {
    if (wide && section !== "sources") dockHidden = false;
    else previewOpen = !previewOpen;
  }

  // 预览高亮跟随鼠标所在或键盘聚焦的字段卡片（data-path）；离开主区时清除
  function track(e: Event) {
    const el = (e.target as HTMLElement | null)?.closest<HTMLElement>("[data-path]");
    editor.hover = el?.dataset.path ?? "";
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
  <div class="editor" class:preview-open={previewOpen} class:docked class:floating>
    <nav class="secnav" aria-label={t("editor.sections")}>
      <!-- 配置名放在分节导航顶端，顶栏不再重复（反馈 #36） -->
      <div class="secnav-head">
        <span class="subtle label-sm">{t("editor.profile")}</span>
        <strong class="pname" title={editor.profile.name}>
          {editor.profile.name}
        </strong>
        <span class="save-state" aria-live="polite">
          {#if editor.saving}{t("editor.saving")}{:else if editor.dirty}{t("editor.unsaved")}{:else}<Icon name="check" size={12} />{t("editor.all_saved")}{/if}
        </span>
        <div class="profile-links">
          <a href={"/profiles/" + editor.profile.id + "/links"}><Icon name="link" size={14} />{t("profiles.links")}</a>
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
        {#each settingsNav as s (s.id)}
          {@const n = sectionSetCount(s)}
          <li>
            <button type="button" class:active={section === s.id && !rawOpen} aria-current={section === s.id && !rawOpen ? "true" : undefined} onclick={() => select(s.id)}>
              <span class="t">{catalog.navTitle(s)}</span>
              {#if sectionHasRaw(s)}<span class="raw-dot" title={t("raw.badge_hint")}><Icon name="bolt" size={11} /></span>{/if}
              <span class="n">{n || ""}</span>
            </button>
          </li>
        {/each}
      </ul>
      <!-- 低频与兜底功能收在末端（反馈 #32、#43）：历史版本与直接编辑 YAML -->
      <span class="subtle label-sm group-label">{t("editor.advanced")}</span>
      <ul>
        <li>
          <a class="item" href={"/profiles/" + editor.profile.id + "/history"}><Icon name="history" size={15} /><span class="t">{t("profiles.history")}</span></a>
        </li>
        <li>
          <button type="button" class:active={rawOpen} aria-current={rawOpen ? "true" : undefined} onclick={() => openRaw()}>
            <Icon name="bolt" size={15} /><span class="t">{t("raw.title")}</span>
            {#if editor.raw.trim()}<span class="raw-dot" title={t("raw.indicator")}><Icon name="bolt" size={11} /></span>{/if}
          </button>
        </li>
      </ul>
      <select class="select secnav-select" aria-label={t("editor.sections")} value={rawOpen ? "@raw" : section} onchange={(e) => (e.currentTarget.value === "@raw" ? openRaw() : select(e.currentTarget.value))}>
        <option value="sources">{t("editor.sources_rules")}</option>
        {#each settingsNav as s (s.id)}<option value={s.id}>{catalog.navTitle(s)}</option>{/each}
        <option value="@raw">{t("raw.title")}</option>
      </select>
      {#if catalog.meta}
        <!-- 兼容性提示：目录按某个 mihomo 版本编写，更新或更旧的内核不保证完全兼容（反馈 #19） -->
        <p class="compat">{t("editor.compat", { core: catalog.meta.pack.coreMax, date: catalog.meta.pack.coreDate ?? "" })}</p>
      {/if}
    </nav>

    <!-- 悬停只用于预览高亮，不是交互控件；键盘用户经 focusin 得到同样效果 -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="content" id="editor-main" onpointerover={track} onfocusin={track} onpointerleave={() => (editor.hover = "")}>
      <div class="content-inner">
        {#if rawOpen}
          <RawEditor {editor} focusKey={rawKey} onClose={() => (rawOpen = false)} />
        {:else if section === "sources" || !current}
          <SourcesPanel {editor} />
          <ChainsPanel {editor} />
        {:else}
          {#key current.id}
            <SectionView section={current} {editor} {core} />
          {/key}
        {/if}
      </div>
    </div>

    <aside class="preview" aria-label={t("preview.title")}>
      <PreviewPane {editor} {floating} onLocate={(p) => void locate(p)} onHide={mobile ? undefined : docked ? () => (dockHidden = true) : () => (previewOpen = false)} />
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
  </div>
{/if}

<style>
  .editor {
    position: relative;
    /* 隐藏的浮层预览带位移停在右缘外，不裁剪会撑出横向滚动条（反馈 #41） */
    overflow: clip;
    display: grid;
    grid-template-columns: 208px minmax(0, 1fr);
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
  .pname {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-inline-size: 0;
    font-size: var(--text-lg);
    font-weight: 620;
    overflow-wrap: anywhere;
  }
  .save-state {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .secnav a.item:hover {
    text-decoration: none;
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
  .secnav button,
  .secnav a.item {
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
  .secnav button:hover,
  .secnav a.item:hover {
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
  .secnav-select {
    display: none;
  }
  .compat {
    margin-block-start: var(--space-5);
    padding-inline: var(--space-2);
    font-size: var(--text-xs);
    line-height: 1.5;
    color: var(--fg-subtle);
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

  /* —— 预览栏 —— */
  .preview {
    min-block-size: 0;
    border-inline-start: 1px solid var(--border);
    overflow: hidden;
  }
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

  /* 停靠：三栏 */
  .editor.docked {
    grid-template-columns: 208px minmax(0, 1fr) var(--preview-w);
  }
  @media (max-width: 1279px) {
    .editor {
      grid-template-columns: 196px minmax(0, 1fr);
    }
  }

  /* 浮层：从右侧滑出，悬浮在编辑区上方（圆角卡片，四周留白），Esc 或点击外部关闭 */
  .floating .preview {
    position: absolute;
    z-index: 20;
    inset-block: var(--space-2);
    inset-inline-end: var(--space-2);
    inline-size: min(680px, calc(100% - var(--space-8)));
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-lg);
    opacity: 0;
    visibility: hidden;
    transform: translateX(24px);
    transition:
      transform var(--dur) var(--ease),
      opacity var(--dur) var(--ease),
      visibility 0s linear var(--dur);
  }
  :global([dir="rtl"]) .floating .preview {
    transform: translateX(-24px);
  }
  .floating.preview-open .preview,
  :global([dir="rtl"]) .floating.preview-open .preview {
    opacity: 1;
    visibility: visible;
    transform: none;
    transition:
      transform var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }
  .floating.preview-open .scrim {
    display: block;
    position: absolute;
    inset: 0;
    z-index: 15;
    border: 0;
    background: var(--scrim);
    backdrop-filter: blur(2px);
  }
  @media (prefers-reduced-motion: reduce) {
    .floating .preview {
      transition: none;
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
    .preview-open .scrim {
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
  }
</style>
