<script lang="ts">
  // 首次向导（第 16.1 节）：选择目标客户端 → 添加节点或订阅 → 选择规则源（可一键使用 domainrouter_v2）→ 得到订阅链接与二维码。
  import Icon from "../components/Icon.svelte";
  import QRCode from "../components/QRCode.svelte";
  import { api, type Link, type Profile, type RuleSource, type Source, type SourceInput } from "../lib/api/client";
  import { catalog } from "../lib/catalog.svelte";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { ProfileDoc } from "../lib/profile-doc";
  import { createProfile } from "../lib/profiles";
  import { presetInput } from "../lib/rules";
  import { toasts } from "../lib/toast.svelte";

  let step = $state(0);
  let preset = $state("mihomo");
  let kind = $state<"subscription" | "batch">("subscription");
  let url = $state("");
  let content = $state("");
  let rules = $state<"domainrouter" | "none">("domainrouter");
  let busy = $state(false);
  let link = $state<Link | null>(null);
  let profileId = $state("");

  $effect(() => void catalog.load());

  const steps = $derived([t("wizard.step_target"), t("wizard.step_nodes"), t("wizard.step_rules"), t("wizard.step_done")]);
  const canNext = $derived(step === 0 || (step === 1 && (kind === "subscription" ? /^https?:\/\//.test(url.trim()) : content.trim() !== "")) || step === 2);

  async function finish() {
    busy = true;
    try {
      const srcBody: SourceInput = kind === "subscription" ? { name: t("wizard.source_name"), kind, url: url.trim() } : { name: t("wizard.source_name"), kind, content };
      const src = await api.post<Source>("/api/v1/sources", srcBody);
      let ruleId: string | undefined;
      if (rules === "domainrouter") {
        const p = catalog.meta?.ruleSources.find((x) => x.id === "domainrouter");
        if (p) ruleId = (await api.post<RuleSource>("/api/v1/rule-sources", presetInput(p, p.variants[0]!, catalog.label(p.label)))).id;
      }
      const prof = await createProfile(t("wizard.profile_name"), preset);
      const doc = new ProfileDoc(prof.doc);
      doc.sources = [{ ref: src.id }];
      if (ruleId) doc.rules = ruleId;
      const saved = await api.put<Profile>("/api/v1/profiles/" + prof.id, { doc: doc.toString(), note: "" }, { ifMatch: prof.version });
      link = await api.post<Link>("/api/v1/profiles/" + saved.id + "/links", { note: t("wizard.link_note") });
      profileId = saved.id;
      step = 3;
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      busy = false;
    }
  }

  async function copy() {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.url);
      toasts.push("ok", t("common.copied"));
    } catch {
      toasts.push("err", t("common.copy_failed"));
    }
  }
</script>

<div class="page">
  <h1>{t("wizard.title")}</h1>
  <p class="muted sub">{t("wizard.subtitle")}</p>

  <ol class="steps" aria-label={t("wizard.progress")}>
    {#each steps as s, i (i)}
      <li class:done={i < step} class:current={i === step} aria-current={i === step ? "step" : undefined}>
        <span class="num">{#if i < step}<Icon name="check" size={13} />{:else}{i + 1}{/if}</span>
        <span>{s}</span>
      </li>
    {/each}
  </ol>

  <section class="panel box">
    {#if step === 0}
      <h2>{t("wizard.target_q")}</h2>
      <div class="options">
        {#each catalog.meta?.presets ?? [] as p (p.id)}
          <label class="opt" class:on={preset === p.id}>
            <input type="radio" bind:group={preset} value={p.id} />
            <strong>{catalog.label(p.label)}</strong>
            <span class="muted">{catalog.label(p.notes)}</span>
          </label>
        {/each}
      </div>
      <p class="hint">{t("wizard.more_targets")}</p>
    {:else if step === 1}
      <h2>{t("wizard.nodes_q")}</h2>
      <div class="seg" role="radiogroup" aria-label={t("wizard.nodes_q")}>
        <label class:on={kind === "subscription"}><input type="radio" bind:group={kind} value="subscription" />{t("source.kind.subscription")}</label>
        <label class:on={kind === "batch"}><input type="radio" bind:group={kind} value="batch" />{t("wizard.paste")}</label>
      </div>
      {#if kind === "subscription"}
        <label class="label" for="wz-url">{t("source.url")}</label>
        <input id="wz-url" class="input mono ltr" type="url" bind:value={url} placeholder="https://sub.example.com/…" spellcheck="false" />
        <p class="hint">{t("source.url_hint")}</p>
      {:else}
        <label class="label" for="wz-c">{t("source.content")}</label>
        <textarea id="wz-c" class="textarea mono ltr" rows="7" bind:value={content} placeholder={t("source.content_placeholder")} spellcheck="false"></textarea>
      {/if}
    {:else if step === 2}
      <h2>{t("wizard.rules_q")}</h2>
      <div class="options">
        <label class="opt" class:on={rules === "domainrouter"}>
          <input type="radio" bind:group={rules} value="domainrouter" />
          <strong>domainrouter_v2 <span class="rec">{t("rules.recommended")}</span></strong>
          <span class="muted">{catalog.label(catalog.meta?.ruleSources.find((x) => x.id === "domainrouter")?.notes)}</span>
        </label>
        <label class="opt" class:on={rules === "none"}>
          <input type="radio" bind:group={rules} value="none" />
          <strong>{t("wizard.rules_later")}</strong>
          <span class="muted">{t("wizard.rules_later_desc")}</span>
        </label>
      </div>
    {:else if link}
      <div class="done">
        <div>
          <h2>{t("wizard.done_title")}</h2>
          <p class="muted">{t("wizard.done_desc")}</p>
          <div class="url-row">
            <code class="url ltr">{link.url}</code>
            <button class="btn sm" type="button" onclick={() => void copy()}><Icon name="copy" size={14} />{t("common.copy")}</button>
          </div>
          <div class="next">
            <a class="btn primary" href={"/profiles/" + profileId}>{t("wizard.open_editor")}</a>
            <a class="btn ghost" href="/">{t("nav.profiles")}</a>
          </div>
        </div>
        <QRCode text={link.url} label={t("links.qr")} />
      </div>
    {/if}

    {#if step < 3}
      <div class="nav">
        {#if step > 0}<button class="btn ghost" type="button" onclick={() => step--}>{t("wizard.back")}</button>{:else}<a class="btn ghost" href="/">{t("wizard.skip")}</a>{/if}
        {#if step < 2}
          <button class="btn primary" type="button" disabled={!canNext} onclick={() => step++}>{t("wizard.next")}</button>
        {:else}
          <button class="btn primary" type="button" disabled={busy} onclick={() => void finish()}>{busy ? t("wizard.working") : t("wizard.finish")}</button>
        {/if}
      </div>
    {/if}
  </section>
</div>

<style>
  .page {
    max-inline-size: 760px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .sub {
    margin-block: var(--space-1) var(--space-6);
  }
  .steps {
    display: flex;
    gap: var(--space-2);
    margin-block-end: var(--space-4);
    flex-wrap: wrap;
  }
  .steps li {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding-inline: var(--space-2) var(--space-3);
    block-size: 30px;
    border-radius: var(--radius-full);
    color: var(--fg-subtle);
    font-size: var(--text-sm);
  }
  .steps li.current {
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-weight: 560;
  }
  .steps li.done {
    color: var(--fg-muted);
  }
  .num {
    display: grid;
    place-items: center;
    inline-size: 20px;
    block-size: 20px;
    border-radius: 50%;
    border: 1px solid currentColor;
    font-size: var(--text-xs);
  }
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-6);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .options {
    display: grid;
    gap: var(--space-2);
  }
  .opt {
    display: grid;
    gap: 2px;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .opt.on {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
  }
  .opt input,
  .seg input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .opt:has(input:focus-visible),
  .seg label:has(input:focus-visible) {
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
  .opt .muted {
    font-size: var(--text-sm);
  }
  .rec {
    margin-inline-start: var(--space-2);
    padding-inline: 6px;
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-xs);
    font-weight: 560;
  }
  .seg {
    display: inline-flex;
    justify-self: start;
    padding: 2px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
  }
  .seg label {
    padding: var(--space-1) var(--space-3);
    border-radius: 4px;
    color: var(--fg-muted);
    cursor: pointer;
    font-size: var(--text-sm);
  }
  .seg label.on {
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-sm);
  }
  .nav {
    display: flex;
    justify-content: space-between;
    margin-block-start: var(--space-3);
  }
  .done {
    display: flex;
    gap: var(--space-6);
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
  }
  .done > div {
    display: grid;
    gap: var(--space-3);
    flex: 1;
    min-inline-size: 260px;
  }
  .url-row {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .url {
    flex: 1;
    min-inline-size: 0;
    padding: 5px var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--bg-sunken);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .next {
    display: flex;
    gap: var(--space-2);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
    .box {
      padding: var(--space-4);
    }
  }
</style>
