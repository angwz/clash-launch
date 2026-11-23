<script lang="ts">
  // 个人访问令牌（需求 R25）：供脚本调用 API。明文只在创建后显示一次；吊销立即生效。
  // 令牌能访问的范围由服务端限定（节点源、规则源、配置、链接、统计等），这里只负责生成与管理。
  import { copyAndTell } from "../../lib/clipboard";
  import Icon from "../Icon.svelte";
  import { api, type Schemas } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  type Token = Schemas["AccessToken"];
  const DAYS = [30, 90, 365, 0] as const;

  let items = $state<Token[]>([]);
  let name = $state("");
  let scope = $state<"read" | "write">("read");
  let days = $state<(typeof DAYS)[number]>(90);
  let created = $state<Token | null>(null);
  let revoking = $state<string | null>(null);
  let busy = $state(false);

  async function load() {
    items = (await api.get<{ items: Token[] }>("/api/v1/account/tokens")).items;
  }
  $effect(() => {
    load().catch((e) => toasts.push("err", errorText(e)));
  });

  async function create(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      created = await api.post<Token>("/api/v1/account/tokens", { name: name.trim(), scope, days });
      name = "";
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function revoke(id: string) {
    try {
      await api.del("/api/v1/account/tokens/" + id);
      revoking = null;
      if (created?.id === id) created = null;
      await load();
      toasts.push("ok", t("tokens.revoked"));
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  async function copy() {
    await copyAndTell(created?.token ?? "");
  }

  const example = $derived(created ? "curl -H 'Authorization: Bearer " + created.token + "' " + location.origin + "/api/v1/profiles" : "");
</script>

<section class="panel box" aria-labelledby="ac-tok">
  <div>
    <h2 id="ac-tok">{t("tokens.section")}</h2>
    <p class="muted">{t("tokens.desc")}</p>
  </div>

  {#if created}
    <div class="once" role="status">
      <p><strong>{t("tokens.once_title")}</strong> {t("tokens.once_hint")}</p>
      <code class="tok mono ltr">{created.token}</code>
      <div class="actions">
        <button class="btn sm" type="button" onclick={() => void copy()}><Icon name="copy" size={14} />{t("common.copy")}</button>
        <button class="btn sm ghost" type="button" onclick={() => (created = null)}>{t("tokens.once_done")}</button>
      </div>
      <p class="hint">{t("tokens.example")}</p>
      <pre class="ex mono ltr">{example}</pre>
    </div>
  {/if}

  {#if items.length}
    <ul class="list">
      {#each items as x (x.id)}
        <li>
          <span class="info">
            <strong>{x.name}</strong>
            <span class="muted">
              {x.scope === "write" ? t("tokens.scope_write") : t("tokens.scope_read")}
              · {x.expiresAt ? t("tokens.expires", { when: i18n.date(x.expiresAt) }) : t("tokens.no_expiry")}
              · {x.lastUsedAt ? t("tokens.last_used", { when: i18n.relative(x.lastUsedAt) }) : t("tokens.never_used")}
            </span>
          </span>
          {#if revoking === x.id}
            <button class="btn sm ghost" type="button" onclick={() => (revoking = null)}>{t("common.cancel")}</button>
            <button class="btn sm danger" type="button" onclick={() => void revoke(x.id)}>{t("tokens.revoke_confirm")}</button>
          {:else}
            <button class="btn sm ghost danger" type="button" aria-label={t("tokens.revoke") + " " + x.name} onclick={() => (revoking = x.id)}>{t("tokens.revoke")}</button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  <form class="new" onsubmit={create}>
    <div class="field">
      <label class="label" for="tok-name">{t("tokens.name")}</label>
      <input id="tok-name" class="input" bind:value={name} maxlength="64" placeholder={t("tokens.name_placeholder")} />
    </div>
    <div class="field">
      <label class="label" for="tok-scope">{t("tokens.scope")}</label>
      <select id="tok-scope" class="select" bind:value={scope}>
        <option value="read">{t("tokens.scope_read")}</option>
        <option value="write">{t("tokens.scope_write")}</option>
      </select>
    </div>
    <div class="field">
      <label class="label" for="tok-days">{t("tokens.validity")}</label>
      <select id="tok-days" class="select" bind:value={days}>
        {#each DAYS as d (d)}<option value={d}>{d ? t("tokens.days", { n: d }) : t("tokens.no_expiry")}</option>{/each}
      </select>
    </div>
    <button class="btn primary" type="submit" disabled={busy || !name.trim()}><Icon name="plus" size={15} />{t("tokens.create")}</button>
  </form>
  <p class="hint">{t("tokens.scope_hint")}</p>
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .once {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--warn-soft);
  }
  .tok,
  .ex {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--bg-raised);
    font-size: 12.5px;
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }
  .list li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: var(--space-3);
    border-block-end: 1px solid var(--border);
  }
  .info {
    display: grid;
    flex: 1;
    min-inline-size: 0;
    font-size: var(--text-sm);
  }
  .new {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--space-3);
  }
  .field {
    display: grid;
    gap: var(--space-1);
    flex: 1 1 160px;
  }
  .actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
