<script lang="ts">
  // 规则源详情：地址或粘贴的文本（toml / ini）。文本用代码编辑器（高亮、行内诊断），
  // 下方是策略组预览：用一份配置或全部节点源的节点展开正在编辑的规则（O2）。
  import RuleHostsHint from "../components/RuleHostsHint.svelte";
  import RulesCode from "../components/rules/RulesCode.svelte";
  import GroupPreview from "../components/rules/GroupPreview.svelte";
  import StatusBadge from "../components/StatusBadge.svelte";
  import { catalog } from "../lib/catalog.svelte";
  import { listAll, type Profile } from "../lib/api/client";
  import type { LineDiagnostic } from "../lib/cm-rules";
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import { api, type RuleSource } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  let { params }: { params: Record<string, string> } = $props();
  let rs = $state<RuleSource | null>(null);
  let error = $state("");
  let saving = $state(false);

  // —— 策略组预览与诊断（O2）——
  interface Located {
    code: string;
    severity: "error" | "warning" | "info";
    args?: Record<string, string>;
    file?: "groups" | "rulesets" | "ini";
    line?: number;
  }
  interface Preview {
    groups: { name: string; type: string; members: string[]; use?: string[]; filter?: string }[];
    rules: number;
    nodes: number;
    diagnostics: Located[];
  }
  let preview = $state<Preview | null>(null);
  let previewError = $state("");
  let profiles = $state<Profile[]>([]);
  let usingProfile = $state("");
  let editors: Record<string, { reveal: (n: number) => void } | undefined> = $state({});
  $effect(() => {
    void catalog.load();
    listAll<Profile>("/api/v1/profiles").then(
      (p) => (profiles = p),
      () => undefined,
    );
  });
  // 文本或所选配置变化后 600 ms 重新预览；被更新的输入取代的请求结果丢弃
  let seq = 0;
  $effect(() => {
    if (!rs) return;
    const body = { kind: rs.kind, groupsText: rs.groupsText ?? "", rulesetsText: rs.rulesetsText ?? "", iniText: rs.iniText ?? "", profile: usingProfile, source: rs.id };
    const n = ++seq;
    const timer = setTimeout(() => {
      api.post<Preview>("/api/v1/rule-sources/preview", body).then(
        (r) => {
          if (n !== seq) return;
          preview = r;
          previewError = "";
        },
        (e) => {
          if (n === seq) previewError = errorText(e);
        },
      );
    }, 600);
    return () => clearTimeout(timer);
  });
  const diags = $derived((preview?.diagnostics ?? []).filter((d) => d.severity !== "info" || d.file));
  const lineDiags = (file: string): LineDiagnostic[] =>
    diags.filter((d) => d.file === file && d.line).map((d) => ({ line: d.line!, severity: d.severity, message: catalog.diagMessage(d.code, d.args) }));
  const fileLabel = (f?: string) => (f === "groups" ? "groups.toml" : f === "rulesets" ? "rulesets.toml" : f === "ini" ? "ini" : "");

  $effect(() => {
    api
      .get<RuleSource>("/api/v1/rule-sources/" + params.id)
      .then((r) => (rs = r))
      .catch((e) => (error = errorText(e)));
  });

  async function save() {
    if (!rs) return;
    saving = true;
    try {
      const { id: _id, version, createdAt: _c, updatedAt: _u, ...body } = rs;
      rs = await api.put<RuleSource>("/api/v1/rule-sources/" + rs.id, body, { ifMatch: version });
      toasts.push("ok", t("common.saved"));
      // 保存成功后返回列表（R48）；失败时留在原页，便于修改后重试
      router.go("/rules");
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      saving = false;
    }
  }

  async function refresh() {
    try {
      // 拉取是同步完成的：完成后告知结果（反馈 #76）
      const r = await api.post<{ error?: string }>("/api/v1/rule-sources/" + params.id + "/refresh");
      if (r?.error) toasts.push("err", t("source.refresh_failed", { reason: r.error }));
      else toasts.push("ok", t("source.refresh_done"));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function remove() {
    if (!rs || !confirm(t("source.delete_confirm", { name: rs.name }))) return;
    try {
      await api.del("/api/v1/rule-sources/" + rs.id);
      router.go("/rules");
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
</script>

<div class="page">
  {#if error}
    <p class="field-error" role="alert">{error}</p>
  {:else if rs}
    <PageHeader title={rs.name} subtitle={rs.kind === "ini" ? t("rules.ini") : "groups.toml + rulesets.toml"}>
      {#snippet actions()}
        <button class="btn" type="button" onclick={() => void refresh()}><Icon name="refresh" size={15} />{t("source.refresh")}</button>
        <button class="btn danger" type="button" onclick={() => void remove()}><Icon name="trash" size={15} />{t("common.delete")}</button>
        <button class="btn primary" type="button" disabled={saving} onclick={() => void save()}>{t("common.save")}</button>
      {/snippet}
    </PageHeader>
    <div class="grid">
      <div>
        <label class="label" for="rd-name">{t("sources.col_name")}</label>
        <input id="rd-name" class="input" bind:value={rs.name} />
      </div>
      {#if rs.kind === "toml"}
        <div>
          <label class="label" for="rd-g">groups.toml URL</label>
          <input id="rd-g" class="input mono ltr" bind:value={rs.groupsUrl} spellcheck="false" />
        </div>
        <div>
          <label class="label" for="rd-r">rulesets.toml URL</label>
          <input id="rd-r" class="input mono ltr" bind:value={rs.rulesetsUrl} spellcheck="false" />
          <RuleHostsHint />
        </div>
        <div>
          <span class="label">{t("rules.groups_text")}</span>
          <RulesCode bind:this={editors.groups} id="rd-gt" lang="toml" label={t("rules.groups_text")} bind:value={rs.groupsText} placeholder={t("rules.text_hint")} diagnostics={lineDiags("groups")} />
        </div>
        <div>
          <span class="label">{t("rules.rulesets_text")}</span>
          <RulesCode bind:this={editors.rulesets} id="rd-rt" lang="toml" label={t("rules.rulesets_text")} bind:value={rs.rulesetsText} placeholder={t("rules.text_hint")} diagnostics={lineDiags("rulesets")} />
        </div>
      {:else}
        <div>
          <label class="label" for="rd-i">INI URL</label>
          <input id="rd-i" class="input mono ltr" bind:value={rs.iniUrl} spellcheck="false" />
          <RuleHostsHint />
        </div>
        <div>
          <span class="label">{t("rules.ini_text")}</span>
          <RulesCode bind:this={editors.ini} id="rd-it" lang="ini" label={t("rules.ini_text")} bind:value={rs.iniText} placeholder={t("rules.text_hint")} diagnostics={lineDiags("ini")} />
        </div>
      {/if}
    </div>

    <section class="preview" aria-labelledby="rd-pv">
      <div class="pv-head">
        <h2 id="rd-pv">{t("rules.preview")}</h2>
        <label class="pv-src">
          <span class="muted">{t("rules.preview_with")}</span>
          <select class="select" bind:value={usingProfile}>
            <option value="">{t("rules.preview_all_sources")}</option>
            {#each profiles as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
          </select>
        </label>
      </div>
      {#if previewError}<p class="field-error" role="alert">{previewError}</p>{/if}
      {#if diags.length}
        <ul class="diags" aria-label={t("rules.problems")}>
          {#each diags as d, i (i)}
            <li>
              <StatusBadge tone={d.severity === "error" ? "err" : d.severity === "warning" ? "warn" : "info"} label={d.line ? fileLabel(d.file) + ":" + d.line : fileLabel(d.file) || d.code} />
              {#if d.line && d.file}
                <button class="msg" type="button" onclick={() => editors[d.file!]?.reveal(d.line!)}>{catalog.diagMessage(d.code, d.args)}</button>
              {:else}
                <span class="msg">{catalog.diagMessage(d.code, d.args)}</span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      {#if preview}
        <GroupPreview groups={preview.groups} nodes={preview.nodes} rules={preview.rules} />
      {:else if !previewError}
        <p class="muted">{t("common.loading")}</p>
      {/if}
    </section>
  {:else}
    <p class="muted">{t("common.loading")}</p>
  {/if}
</div>

<style>
  .page {
    max-inline-size: 960px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
  }
  .grid {
    display: grid;
    gap: var(--space-4);
  }
  .preview {
    display: grid;
    gap: var(--space-3);
    margin-block-start: var(--space-6);
  }
  .pv-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .pv-src {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .diags {
    display: grid;
    gap: var(--space-1);
    font-size: var(--text-sm);
  }
  .diags li {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }
  .diags .msg {
    border: 0;
    background: none;
    padding: 0;
    color: inherit;
    font: inherit;
    text-align: start;
  }
  .diags button.msg {
    cursor: pointer;
    text-decoration: underline;
    text-decoration-color: var(--border);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
  }
</style>
