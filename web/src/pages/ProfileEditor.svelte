<script lang="ts">
  // 方案编辑（第 16.1 节）：分节导航 · 字段卡片 · 预览栏。
  // 预览：宽屏编辑设置时停靠在右侧；「源与规则」与 768–1279 时收起，点击后浮层滑出；< 768 单栏，底部切换「设置 / 预览」。
  import { onMount, tick, untrack } from "svelte";
  import Icon from "../components/Icon.svelte";
  import PreviewPane from "../components/editor/PreviewPane.svelte";
  import RawEditor from "../components/editor/RawEditor.svelte";
  import SectionView from "../components/editor/SectionView.svelte";
  import SourcesPanel from "../components/editor/SourcesPanel.svelte";
  import ChainsPanel from "../components/editor/ChainsPanel.svelte";
  import SaveTemplateDialog from "../components/editor/SaveTemplateDialog.svelte";
  import DeleteProfileDialog from "../components/DeleteProfileDialog.svelte";
  import ExportDialog from "../components/editor/ExportDialog.svelte";
  import { catalog, type MetaField, type NavSection } from "../lib/catalog.svelte";
  import { EditorState } from "../lib/editor.svelte";
  import { errorText } from "../lib/errors";
  import { api } from "../lib/api/client";
  import { toasts } from "../lib/toast.svelte";
  import { t } from "../lib/i18n/i18n.svelte";
  import { palette, type PaletteItem } from "../lib/palette.svelte";
  import { router } from "../lib/router.svelte";
  import { shell } from "../lib/shell.svelte";
  import { ordered, prefs } from "../lib/prefs.svelte";

  let { params }: { params: Record<string, string> } = $props();

  const editor = new EditorState();
  let loadError = $state("");
  let section = $state(new URLSearchParams(location.search).get("section") || "sources");
  let rawOpen = $state(false);
  // 「更多」的展开状态由组件记住：配置对象被替换（如开关删除保护）时导航会重新渲染，不应收起
  let advOpen = $state(false);
  $effect(() => {
    if (rawOpen) advOpen = true;
  });
  let savingTemplate = $state(false);
  let deletingProfile = $state(false);
  let exporting = $state(false); // 导出配置（R17）

  // 删除保护（R55）：开启后三处删除入口（配置卡片、这里的「更多」、命令面板）都不可用，服务端同样拒绝
  const isProtected = $derived(!!editor.profile?.protected);
  async function setProtected(on: boolean) {
    const p = editor.profile;
    if (!p) return;
    try {
      await api.put("/api/v1/profiles/" + p.id + "/protection", { protected: on });
      editor.profile = { ...p, protected: on };
      toasts.push("ok", on ? t("profiles.protect_on") : t("profiles.protect_off"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  function askDelete() {
    if (isProtected) toasts.push("info", t("profiles.protected_hint"));
    else deletingProfile = true;
  }

  // 删除后离开编辑器：先撤掉「离开时自动保存」，否则会去保存一个已经不存在的配置
  function afterDelete() {
    router.guard = null;
    router.go("/", true); // 配置列表在首页；替换当前历史记录：返回时不再回到已删除的配置
  }
  let rawKey = $state<string | undefined>();
  let previewOpen = $state(false); // 浮层 / 手机视图中预览是否可见
  // 预览的呈现（反馈 #28）：宽屏上编辑设置时停靠在右侧并高亮当前字段；
  // 在「源与规则」与窄屏上默认收起，点击后以浮层滑出。停靠可手动收起，本次会话内记住。
  let wide = $state(matchMedia("(min-width: 1280px)").matches);
  let mobile = $state(matchMedia("(max-width: 767px)").matches);
  let dockHidden = $state(false);
  const docked = $derived(wide && !dockHidden && section !== "sources" && section !== "routing");
  // 分流分节按需加载（首屏预算不变，routing 第 13 节第 2 步）；从命令面板进入时带着要追踪的目标
  let routingTrace = $state(new URLSearchParams(location.search).get("trace") ?? "");
  const floating = $derived(!docked && !mobile);

  const nav = $derived(catalog.meta?.pack.nav ?? []);
  // 「节点与策略组」「规则」两个生成分节只是指回「节点与分流」，与左侧主菜单重复，不再列出（反馈 #33）
  // 按用户偏好排序（反馈 #37）
  const settingsNav = $derived(ordered(nav.filter((s) => !s.generated), (s) => s.id, prefs.data.sections));
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
      runCheck: () => void editor.runPreview(true, true),
    };
  });

  // 切换分节新增一条历史记录（需求 R3）：浏览器返回回到上一个分节，而不是离开编辑器
  function select(id: string) {
    rawOpen = false;
    if (id !== section) {
      const q = new URLSearchParams(location.search);
      q.set("section", id);
      router.go(location.pathname + "?" + q.toString());
    }
    section = id;
    document.getElementById("editor-main")?.scrollTo({ top: 0 });
  }
  // 返回与前进：分节跟随地址栏
  $effect(() => {
    const s = new URLSearchParams(router.search).get("section") || "sources";
    if (s !== untrack(() => section)) {
      section = s;
      rawOpen = false;
    }
  });

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
    if (wide && section !== "sources" && section !== "routing") dockHidden = false;
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
      { id: "cmd.check", kind: "command", title: t("check.run"), hint: "mihomo -t", icon: "ok", run: () => void editor.runPreview(true, true) },
      { id: "routing.open", kind: "command", title: t("routing.title"), hint: "routing", icon: "rules", run: () => select("routing") },
      { id: "profile.export", kind: "command", title: t("export.title"), hint: "export", icon: "download", run: () => (exporting = true) },
      { id: "profile.delete", kind: "command", title: t("profiles.delete"), hint: "delete", icon: "trash", run: askDelete },
    ];
    // 输入像域名或 IP 的内容时，提供「追踪」：跳到分流分节并开始追踪（routing 第 11.1 节）
    const q = palette.query.replace(/^>\s*/, "").trim();
    if (/^[^\s,]+\.[^\s,]+$|^[0-9a-f:]+:[0-9a-f:]*$/i.test(q)) {
      out.unshift({ id: "routing.trace", kind: "command", title: t("routing.palette_trace", { host: q }), hint: q, icon: "search", run: () => ((routingTrace = q), select("routing")) });
    }
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
        <strong class="pname" title={editor.profile.name}>{editor.profile.name}</strong>
        <a class="plink" href={"/profiles/" + editor.profile.id + "/links"}>{t("profiles.links")}<Icon name="arrow-right" size={12} /></a>
      </div>
      <ul>
        <li>
          <button type="button" class:active={section === "sources" && !rawOpen} aria-current={section === "sources" && !rawOpen ? "true" : undefined} onclick={() => select("sources")}>
            <span class="t">{t("editor.sources_rules")}</span>
            <span class="n">{editor.sourceRefs.length || ""}</span>
          </button>
        </li>
        <li>
          <button type="button" class:active={section === "routing" && !rawOpen} aria-current={section === "routing" && !rawOpen ? "true" : undefined} onclick={() => select("routing")}>
            <span class="t">{t("routing.title")}</span>
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
      <!-- 低频与兜底功能收在末端，默认收起（反馈 #32、#43、#49）：本分区说明、历史版本、直接编辑 YAML -->
      <details class="adv" bind:open={advOpen}>
        <summary class="subtle"><span>{t("editor.advanced")}</span><Icon name="chevron-down" size={12} /></summary>
      <ul>
        <li>
          <button type="button" onclick={() => (savingTemplate = true)}><span class="t">{t("templates.save_title")}</span></button>
        </li>
        {#if current}
          <li>
            <a class="item" href={"/help?tab=wiki&s=" + encodeURIComponent(current.id)}><span class="t">{t("editor.section_doc")}</span></a>
          </li>
        {/if}
        <li>
          <a class="item" href={"/profiles/" + editor.profile.id + "/history"}><span class="t">{t("profiles.history")}</span></a>
        </li>
        <li>
          <button type="button" onclick={() => (exporting = true)}><span class="t">{t("export.title")}</span></button>
        </li>
        <li>
          <button type="button" role="switch" aria-checked={isProtected} onclick={() => void setProtected(!isProtected)}>
            <span class="t">{t("profiles.protected")}</span>
            <span class="sw" class:on={isProtected}>{isProtected ? t("common.on") : t("common.off")}</span>
          </button>
        </li>
        <li>
          <button type="button" class="danger-item" disabled={isProtected} title={isProtected ? t("profiles.protected_hint") : undefined} onclick={askDelete}><span class="t">{t("profiles.delete")}</span></button>
        </li>
        <li>
          <button type="button" class:active={rawOpen} aria-current={rawOpen ? "true" : undefined} onclick={() => openRaw()}>
            <span class="t">{t("raw.title")}</span>
            {#if editor.raw.trim()}<span class="raw-dot" title={t("raw.indicator")}><Icon name="bolt" size={11} /></span>{/if}
          </button>
        </li>
      </ul>
      </details>
      <select class="select secnav-select" aria-label={t("editor.sections")} value={rawOpen ? "@raw" : section} onchange={(e) => (e.currentTarget.value === "@raw" ? openRaw() : select(e.currentTarget.value))}>
        <option value="sources">{t("editor.sources_rules")}</option>
        <option value="routing">{t("routing.title")}</option>
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
      <!-- 预览开关：粘在滚动区顶端右侧。放在滚动容器内部，滚动条在它旁边而不是盖住它（反馈 #42、#47） -->
      {#if !docked && !mobile}
        <div class="preview-dock">
          <button class="btn preview-toggle" type="button" aria-expanded={previewOpen} onclick={togglePreview}>
            <Icon name="code" size={15} />{t("preview.title")}
            {#if editor.counts.errors}<span class="badge-err">{editor.counts.errors}</span>{/if}
          </button>
        </div>
      {/if}
      <div class="content-inner" class:full={section === "routing" && !rawOpen}>
        {#if rawOpen}
          <RawEditor {editor} focusKey={rawKey} onClose={() => (rawOpen = false)} />
        {:else if section === "routing"}
          {#await import("../components/routing/RoutingView.svelte") then m}
            <m.default {editor} initialTrace={routingTrace} />
          {/await}
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

    <SaveTemplateDialog bind:open={savingTemplate} doc={editor.text} profileName={editor.profile.name} />
    <DeleteProfileDialog bind:open={deletingProfile} profile={editor.profile} onDeleted={afterDelete} />
    <ExportDialog bind:open={exporting} profileId={editor.profile.id} />

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
  /* 配置名与配置链接：名称是这一栏的标题，链接是次要入口；下方一条分隔线把它与导航分开（反馈 #62） */
  .secnav-head {
    display: grid;
    gap: 2px;
    padding-block: 0 var(--space-3);
    padding-inline: var(--space-2);
    margin-block-end: var(--space-2);
    border-block-end: 1px solid var(--border);
  }
  .plink {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    inline-size: fit-content;
    color: var(--accent-fg);
    font-size: var(--text-sm);
  }
  .plink:hover {
    text-decoration: none;
    opacity: 0.8;
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
  .secnav a.item:hover {
    text-decoration: none;
  }
  .label-sm {
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .adv {
    margin-block-start: auto;
    padding-block-start: var(--space-4);
  }
  /* 去掉默认三角标记，箭头放在文字之后：文字与上方各项左缘对齐（反馈 #56） */
  .adv summary {
    display: flex;
    align-items: center;
    gap: 4px;
    padding-block: var(--space-1);
    padding-inline: var(--space-2);
    font-size: var(--text-xs);
    cursor: pointer;
    list-style: none;
  }
  .adv summary::-webkit-details-marker {
    display: none;
  }
  .adv summary :global(.icon) {
    transform: rotate(-90deg);
    transition: transform var(--dur-fast) var(--ease);
  }
  .adv[open] summary :global(.icon) {
    transform: none;
  }
  .adv summary:hover {
    color: var(--fg-muted);
  }
  .group-label {
    padding-inline: var(--space-2);
    margin-block: var(--space-4) var(--space-1);
    color: var(--fg-subtle);
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
  .secnav .sw {
    margin-inline-start: auto;
    color: var(--fg-muted);
    font-size: var(--text-xs);
  }
  .secnav .sw.on {
    color: var(--ok);
    font-weight: 560;
  }
  .secnav button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .secnav button.danger-item:hover:not(:disabled) {
    background: var(--err-soft);
    color: var(--err);
  }
  /* 选中项：强调色浅底、强调色文字，前缘一条短竖条 */
  .secnav button,
  .secnav a.item {
    position: relative;
  }
  .secnav button.active {
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-weight: 600;
  }
  .secnav button.active::before {
    content: "";
    position: absolute;
    inset-block: 8px;
    inset-inline-start: 0;
    inline-size: 3px;
    border-radius: 0 3px 3px 0;
    background: var(--accent);
  }
  .t {
    flex: 1;
    min-inline-size: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  /* 已修改字段数：小号胶囊；选中时随强调色 */
  .n:not(:empty) {
    min-inline-size: 18px;
    padding-inline: 5px;
    border-radius: var(--radius-full);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: 11px;
    line-height: 18px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
  .active .n:not(:empty) {
    background: var(--accent);
    color: var(--fg-on-accent);
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
    /* 左对齐而不是居中：切换「节点与分流」与各设置分区时内容的左缘不跳动（反馈 #56） */
    margin-inline: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }
  /* 分流地图需要整个宽度（列数随配置变化，地图在自身容器内横向滚动） */
  .content-inner.full {
    max-inline-size: none;
    min-inline-size: 0;
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
  .preview-dock {
    position: sticky;
    z-index: 10;
    inset-block-start: 0;
    display: flex;
    justify-content: flex-end;
    block-size: 0;
    padding-inline-end: var(--space-4);
  }
  .preview-toggle {
    margin-block-start: var(--space-3);
    box-shadow: var(--shadow-sm);
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
