<script lang="ts">
  // 版本历史：任意两个版本的差异（逐行比较方案文档），一键回滚（回滚也生成新版本）。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import YamlView from "../components/editor/YamlView.svelte";
  import { api, listAll, type Profile, type Revision } from "../lib/api/client";
  import { lineDiff } from "../lib/diff";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { params }: { params: Record<string, string> } = $props();

  let profile = $state<Profile | null>(null);
  let revs = $state<Revision[]>([]);
  let a = $state<number | null>(null); // 旧
  let b = $state<number | null>(null); // 新
  let error = $state("");

  async function load() {
    try {
      [profile, revs] = await Promise.all([api.get<Profile>("/api/v1/profiles/" + params.id), listAll<Revision>("/api/v1/profiles/" + params.id + "/revisions")]);
      b = revs[0]?.n ?? null;
      a = revs[1]?.n ?? revs[0]?.n ?? null;
    } catch (e) {
      error = errorText(e);
    }
  }
  $effect(() => void load());

  const diff = $derived.by(() => {
    const ra = revs.find((r) => r.n === a);
    const rb = revs.find((r) => r.n === b);
    if (!ra || !rb) return null;
    return lineDiff(ra.doc, rb.doc);
  });

  async function rollback(n: number) {
    if (!profile || !confirm(t("history.rollback_confirm", { n }))) return;
    try {
      const p = await api.post<Profile>("/api/v1/profiles/" + profile.id + "/rollback", { n }, { ifMatch: profile.version });
      toasts.push("ok", t("history.rolled_back", { n, v: p.version }));
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
</script>

<div class="page">
  <PageHeader title={t("history.title")} subtitle={profile?.name}>
    {#snippet actions()}
      <a class="btn" href={"/profiles/" + params.id}><Icon name="settings" size={15} />{t("links.edit_profile")}</a>
    {/snippet}
  </PageHeader>
  {#if error}<p class="field-error" role="alert">{error}</p>{/if}

  <div class="layout">
    <ol class="revs panel">
      {#each revs as r (r.n)}
        <li class:sel-a={r.n === a} class:sel-b={r.n === b}>
          <div class="rv">
            <strong class="mono">v{r.n}</strong>
            <span class="muted">{i18n.date(r.createdAt)}</span>
            {#if r.note}<span class="note">{r.note === "autosave" ? t("history.autosave") : r.note}</span>{/if}
          </div>
          <div class="pick">
            <button class="btn sm ghost" type="button" aria-pressed={r.n === a} onclick={() => (a = r.n)}>{t("history.as_old")}</button>
            <button class="btn sm ghost" type="button" aria-pressed={r.n === b} onclick={() => (b = r.n)}>{t("history.as_new")}</button>
            {#if r.n !== revs[0]?.n}
              <button class="btn sm" type="button" onclick={() => void rollback(r.n)}>{t("history.rollback")}</button>
            {/if}
          </div>
        </li>
      {/each}
    </ol>
    <section class="panel diff">
      <header class="dh">
        <span>{t("history.compare", { a: a ?? "—", b: b ?? "—" })}</span>
        <button class="btn sm ghost" type="button" onclick={() => router.go("/profiles/" + params.id)}>{t("common.close")}</button>
      </header>
      {#if diff}
        {#if diff.every((d) => d.op === " ")}
          <p class="muted pad">{t("history.same")}</p>
        {:else}
          <YamlView lines={diff.map((d) => d.text)} ops={diff.map((d) => d.op)} />
        {/if}
      {/if}
    </section>
  </div>
</div>

<style>
  .page {
    max-inline-size: 1280px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .layout {
    display: grid;
    grid-template-columns: 340px minmax(0, 1fr);
    gap: var(--space-4);
    align-items: start;
  }
  .revs {
    display: grid;
    max-block-size: 70dvh;
    overflow-y: auto;
  }
  .revs li {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-block-end: 1px solid var(--border);
  }
  .revs li.sel-b {
    background: color-mix(in srgb, var(--ok) 7%, transparent);
  }
  .revs li.sel-a {
    background: color-mix(in srgb, var(--err) 7%, transparent);
  }
  .rv {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    align-items: baseline;
    font-size: var(--text-sm);
  }
  .note {
    flex-basis: 100%;
  }
  .pick {
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
  }
  .pick [aria-pressed="true"] {
    background: var(--bg-active);
  }
  .diff {
    overflow: hidden;
  }
  .dh {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--space-2) var(--space-4);
    border-block-end: 1px solid var(--border);
    font-size: var(--text-sm);
  }
  .pad {
    padding: var(--space-6);
  }
  @media (max-width: 900px) {
    .layout {
      grid-template-columns: 1fr;
    }
  }
</style>
