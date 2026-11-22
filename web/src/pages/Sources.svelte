<script lang="ts">
  import { hostOf, uniqueName } from "../lib/names";
  import Emoji from "../components/Emoji.svelte";
  // 节点源列表与新建（第 16.1 节「节点源」）：订阅地址、粘贴分享链接或节点（单条 / 批量）、文件内容。
  import Dialog from "../components/Dialog.svelte";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import ShareDialog, { type ShareTarget } from "../components/ShareDialog.svelte";
  import NodeSummary, { type Summary } from "../components/NodeSummary.svelte";
  import { session } from "../lib/session.svelte";
  import { api, listAll, type Source, type SourceInput } from "../lib/api/client";
  // 列表项另带授权信息与节点概况（反馈 #75、#76）
  type GSource = Source & { grant?: { id: string; from: string; fromNickname?: string }; summary?: Summary };
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { nodeModePolicy } from "../lib/node-mode";
  import { live } from "../lib/live";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";
  import { nameWithAt } from "../lib/names";
  import { pending } from "../lib/pending.svelte";
  import { Selection } from "../lib/selection.svelte";
  import BatchBar from "../components/BatchBar.svelte";

  let items = $state<GSource[] | null>(null);
  // 分享或授权给好友（反馈 #76）
  let shareTarget = $state<ShareTarget | null>(null);
  let refreshing = $state<string | null>(null);
  // 自己的节点与别人授权给我的节点（反馈 #75）分开显示
  // 等待删除（可撤销，R33）的条目先从列表隐藏
  const mine = $derived((items ?? []).filter((s) => !s.grant && !pending.has(s.id)));
  // 多选与批量操作（R35）
  const sel = new Selection();
  $effect(() => sel.keep(mine.map((s) => s.id)));
  const allOn = $derived(mine.length > 0 && mine.every((s) => sel.has(s.id)));
  const selected = $derived(mine.filter((s) => sel.has(s.id)));
  let batchBusy = $state(false);
  const granted = $derived((items ?? []).filter((s) => !!s.grant));
  async function removeGrant(s: GSource) {
    if (!s.grant || !confirm(t("messages.remove_confirm", { name: s.name }))) return;
    try {
      await api.post("/api/v1/grants/" + s.grant.id + "/revoke", {});
      toasts.push("ok", t("messages.removed"));
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  // 列表中直接删除（R50）：可在 10 秒内撤销（R33），到期后服务端先收回授权、通知对方（R2）
  function remove(list: GSource[]) {
    if (!list.length) return;
    const label = list.length === 1 ? t("sources.deleted", { name: list[0]!.name }) : t("batch.deleted_n", { n: list.length });
    pending.remove(list.map((s) => ({ id: s.id, path: "/api/v1/sources/" + s.id })), label, () => void load());
    for (const s of list) sel.toggle(s.id, false);
  }
  // 批量重新拉取（R35）：逐个进行，避免同时打到同一家机场；汇报成功与失败的数量
  async function refreshSelected() {
    const subs = selected.filter((s) => s.kind === "subscription");
    if (!subs.length) return;
    batchBusy = true;
    let failed = 0;
    for (const s of subs) {
      refreshing = s.id;
      try {
        const r = await api.post<{ lastError?: string }>("/api/v1/sources/" + s.id + "/refresh");
        if (r.lastError) failed++;
      } catch {
        failed++;
      }
    }
    refreshing = null;
    batchBusy = false;
    await load();
    if (failed) toasts.push("err", t("batch.refresh_partial", { ok: subs.length - failed, failed }));
    else toasts.push("ok", t("batch.refresh_done", { n: subs.length }));
  }
  // 订阅只由客户端拉取（反馈 #71）：不显示拉取状态与「立即拉取」
  let clientFetch = $state(false);
  $effect(() => {
    nodeModePolicy().then((p) => (clientFetch = p.clientFetch), () => undefined);
  });
  let error = $state("");
  let open = $state(false);
  let busy = $state(false);
  let form = $state<{ kind: SourceInput["kind"]; name: string; url: string; content: string }>({ kind: "subscription", name: "", url: "", content: "" });
  // 默认名称（反馈 #65）：订阅取域名，手动节点按序号；与已有名称不重复
  const defaultName = $derived(
    uniqueName(form.kind === "subscription" ? (hostOf(form.url) ?? t("sources.default_subscription")) : t("profiles.manual_nodes"), (items ?? []).map((s) => s.name)),
  );

  async function load() {
    try {
      items = await listAll<Source>("/api/v1/sources");
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());
  // 自动刷新（反馈 #73）：后台拉取会改变订阅的状态；页面可见时每 30 秒刷新
  $effect(() => live(async () => (items = await listAll<Source>("/api/v1/sources")), 30_000, ["source", "grant"]).stop);

  function status(s: Source): { tone: "ok" | "err" | "warn" | "muted"; label: string } {
    if (s.kind !== "subscription") return { tone: "muted", label: t("source.static") };
    if (clientFetch) return { tone: "muted", label: t("source.client_fetch_short") };
    const snap = s.snapshot;
    if (!snap?.fetchedAt && !snap?.lastError) return { tone: "muted", label: t("source.never") };
    if (snap.failingSince) return { tone: "warn", label: t("source.failing", { since: i18n.relative(snap.failingSince) }) };
    if (snap.lastError) return { tone: "err", label: t("source.error") };
    return { tone: "ok", label: t("source.fetched", { when: i18n.relative(snap.fetchedAt) }) };
  }

  function host(u?: string): string {
    if (!u) return "";
    try {
      return new URL(u).host;
    } catch {
      return "";
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      const body: SourceInput = { kind: form.kind, name: form.name.trim() || defaultName };
      if (form.kind === "subscription") body.url = form.url.trim();
      else body.content = form.content;
      const s = await api.post<Source>("/api/v1/sources", body);
      open = false;
      toasts.push("ok", t("source.created"));
      router.go("/sources/" + s.id);
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  // 立即拉取：拉取是同步完成的，完成后刷新这一行并告知结果（反馈 #76）
  async function refresh(s: Source) {
    refreshing = s.id;
    try {
      const r = await api.post<{ lastError?: string }>("/api/v1/sources/" + s.id + "/refresh");
      await load();
      if (r.lastError) toasts.push("err", t("source.refresh_failed", { reason: r.lastError }));
      else toasts.push("ok", t("source.refresh_done"));
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      refreshing = null;
    }
  }

  // 后端只区分订阅与手动（单条、批量、文件都按内容自动识别），界面也只呈现这两类。
  const kindIcon = (k: string) => (k === "subscription" ? "link" : "server") as "link" | "server";
</script>

<ShareDialog bind:target={shareTarget} />

<div class="page">
  <PageHeader title={t("sources.title")} subtitle={t("sources.subtitle")}>
    {#snippet actions()}
      <button class="btn primary" type="button" onclick={() => { form = { kind: "subscription", name: "", url: "", content: "" }; open = true; }}>
        <Icon name="plus" size={15} />{t("sources.new")}
      </button>
    {/snippet}
  </PageHeader>

  <!-- 隐私说明（反馈 #77）：如实说明加密与可见范围，细节与限制在帮助「隐私与安全」中 -->
  <p class="privacy"><Icon name="shield" size={14} />{t("sources.privacy")} <a href="/help?topic=08-privacy">{t("sources.privacy_more")}</a></p>

  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  {#if items === null}
    <div class="skeleton" aria-busy="true"></div>
  {:else if mine.length === 0 && granted.length === 0}
    <div class="empty panel">
      <Emoji name="mailbox" size={40} />
      <p>{t("sources.empty")}</p>
    </div>
  {:else}
    {#if mine.length}
    <BatchBar {sel}>
      {#if !clientFetch && selected.some((s) => s.kind === "subscription")}
        <button class="btn sm" type="button" disabled={batchBusy} onclick={() => void refreshSelected()}><Icon name="refresh" size={14} />{t("source.refresh")}</button>
      {/if}
      <button class="btn sm danger" type="button" onclick={() => remove(selected)}><Icon name="trash" size={14} />{t("common.delete")}</button>
    </BatchBar>
    <div class="panel table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th class="pick"><input type="checkbox" checked={allOn} aria-label={t("batch.select_all")} onchange={(e) => sel.setAll(mine.map((s) => s.id), e.currentTarget.checked)} /></th>
            <th>{t("sources.col_name")}</th>
            <th>{t("sources.col_kind")}</th>
            <th>{t("sources.col_status")}</th>
            <th><span class="visually-hidden">{t("common.actions")}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each mine as s (s.id)}
            {@const st = status(s)}
            <tr class:picked={sel.has(s.id)}>
              <td class="pick"><input type="checkbox" checked={sel.has(s.id)} aria-label={t("batch.select_x", { name: s.name })} onchange={(e) => sel.toggle(s.id, e.currentTarget.checked)} /></td>
              <td>
                <a class="name" href={"/sources/" + s.id}>{s.name}</a>
                {#if host(s.url)}<span class="host mono ltr">{host(s.url)}</span>{/if}
                {#if s.summary}<NodeSummary summary={s.summary} />{/if}
              </td>
              <td><span class="kind"><Icon name={kindIcon(s.kind)} size={14} />{t("source.kind." + s.kind)}</span></td>
              <td><StatusBadge tone={st.tone} label={st.label} /></td>
              <td class="act">
                {#if s.kind === "subscription" && !clientFetch}
                  <button class="btn ghost icon sm" class:spin={refreshing === s.id} type="button" disabled={refreshing === s.id} aria-label={t("source.refresh")} title={t("source.refresh")} onclick={() => void refresh(s)}><Icon name="refresh" size={14} /></button>
                {/if}
                <button class="btn ghost icon sm" type="button" aria-label={t("share.share_x", { name: s.name })} title={t("messages.share_hint")} onclick={() => (shareTarget = { kind: "source", id: s.id, name: s.name, mode: "share" })}><Icon name="send" size={14} /></button>
                <button class="btn ghost icon sm" type="button" aria-label={t("share.grant_x", { name: s.name })} title={t("messages.grant_hint")} onclick={() => (shareTarget = { kind: "source", id: s.id, name: s.name, mode: "grant" })}><Icon name="shield" size={14} /></button>
                <!-- 编辑：带文字的按钮，比只有箭头更容易发现和点中（反馈 #78） -->
                <a class="btn sm" href={"/sources/" + s.id}><Icon name="edit" size={14} />{t("common.edit")}</a>
                <!-- 删除放在最右端、与编辑隔开，样式与配置列表一致（R50） -->
                <button class="btn sm ghost danger" type="button" aria-label={t("sources.delete_named", { name: s.name })} title={t("common.delete")} onclick={() => remove([s])}><Icon name="trash" size={14} /></button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {/if}
    {#if granted.length}
      <!-- 别人授权给我的节点（反馈 #75）：只能在配置中使用，不能查看内容或修改；可以移除 -->
      <h2 class="sub-h"><Icon name="shield" size={15} />{t("sources.granted")}</h2>
      <p class="hint">{t("sources.granted_hint")}</p>
      <ul class="granted">
        {#each granted as s (s.id)}
          <li class="panel">
            <Icon name={kindIcon(s.kind)} size={16} />
            <span class="g-main">
              <strong>{s.name}</strong>
              <span class="muted">{t("source.kind." + s.kind)} · {t("sources.granted_from", { name: nameWithAt(s.grant!.from, s.grant!.fromNickname), when: i18n.date(s.createdAt) })}</span>
              {#if s.summary}<NodeSummary summary={s.summary} />{/if}
            </span>
            <button class="btn sm ghost" type="button" onclick={() => void removeGrant(s)}>{t("messages.remove")}</button>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<Dialog bind:open title={t("sources.new")} wide>
  <form id="new-source" class="form" onsubmit={create}>
    <div class="kinds" role="radiogroup" aria-label={t("sources.col_kind")}>
      {#each ["subscription", "batch"] as k (k)}
        <label class="kind-opt" class:on={form.kind === k}>
          <input type="radio" name="kind" value={k} bind:group={form.kind} />
          <Icon name={kindIcon(k)} size={16} />
          <span class="kt">{t("source.kind." + k)}</span>
          <span class="kd">{t("source.kind_hint." + k)}</span>
        </label>
      {/each}
    </div>
    <div>
      <label class="label" for="ns-name">{t("sources.col_name")}</label>
      <input id="ns-name" class="input" bind:value={form.name} maxlength="80" placeholder={defaultName} />
    </div>
    {#if form.kind === "subscription"}
      <div>
        <label class="label" for="ns-url">{t("source.url")}</label>
        <input id="ns-url" class="input mono ltr" type="url" bind:value={form.url} required placeholder="https://sub.example.com/…" autocomplete="off" spellcheck="false" />
        <!-- 「服务器直接拉取、出口 IP」是部署者需要知道的事，只给管理员看（反馈 #76） -->
        {#if session.isAdmin}<p class="hint">{t("source.url_hint")}</p>{/if}
      </div>
    {:else}
      <div>
        <label class="label" for="ns-content">{t("source.content")}</label>
        <textarea id="ns-content" class="textarea mono ltr" rows="8" bind:value={form.content} required placeholder={t("source.content_placeholder")} spellcheck="false"></textarea>
        <p class="hint">{t("source.content_hint")}</p>
      </div>
    {/if}
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="submit" form="new-source" disabled={busy || (form.kind === "subscription" ? !form.url.trim() : !form.content.trim())}>{t("common.create")}</button>
  {/snippet}
</Dialog>

<style>
  .pick {
    inline-size: 1%;
    padding-inline-end: 0;
  }
  .pick input {
    inline-size: 16px;
    block-size: 16px;
    accent-color: var(--accent);
    cursor: pointer;
  }
  tr.picked {
    background: var(--accent-soft);
  }
  .privacy {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-2);
    margin-block: calc(-1 * var(--space-2)) var(--space-4);
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .privacy :global(.icon) {
    color: var(--ok);
  }
  /* 别人授权给我的节点（反馈 #75） */
  .sub-h {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-block: var(--space-6) var(--space-1);
    font-size: var(--text-lg);
  }
  .granted {
    display: grid;
    gap: var(--space-2);
    margin: var(--space-3) 0 0;
    padding: 0;
    list-style: none;
  }
  .granted li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
  }
  .g-main {
    display: grid;
    flex: 1;
    min-inline-size: 0;
    font-size: var(--text-sm);
  }
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .table-wrap {
    overflow-x: auto;
    /* 让表内绝对定位的无障碍标签以此为包含块，否则会逃出滚动容器、撑宽页面（反馈 #41） */
    position: relative;
  }
  .name {
    display: block;
    color: var(--fg);
    font-weight: 560;
  }
  .host {
    display: block;
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .kind {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--fg-muted);
    white-space: nowrap;
  }
  /* 立即拉取进行中：图标旋转 */
  .spin :global(.icon) {
    animation: spin 0.9s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin :global(.icon) {
      animation: none;
    }
  }
  .act {
    text-align: end;
    white-space: nowrap;
  }
  .empty {
    padding: var(--space-8);
    display: grid;
    justify-items: center;
    gap: var(--space-2);
    text-align: center;
    color: var(--fg-muted);
  }
  .skeleton {
    block-size: 160px;
    border-radius: var(--radius-lg);
    background: var(--bg-sunken);
  }
  .form {
    display: grid;
    gap: var(--space-4);
  }
  .kinds {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-2);
  }
  .kind-opt {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 2px var(--space-2);
    align-items: center;
    padding: var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .kind-opt.on {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
    background: var(--accent-soft);
  }
  .kind-opt:has(input:focus-visible) {
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
  .kind-opt input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .kt {
    font-weight: 560;
  }
  .kd {
    grid-column: 1 / -1;
    font-size: var(--text-xs);
    color: var(--fg-muted);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
