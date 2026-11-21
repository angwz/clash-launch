<script lang="ts">
  // 提醒渠道（需求 R31）：除邮件外，把提醒发到 Webhook、Telegram、Bark、ntfy。测试消息送达后启用，
  // 修改设置回到未验证。服务端不返回令牌、key、密钥与 Webhook 地址；编辑时这些字段留空表示保留原值。
  import { api, type Schemas } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  type Channel = Schemas["Channel"];
  type Kind = Channel["kind"];
  type Form = { name: string; kind: Kind; url: string; token: string; chat: string; key: string; topic: string; secret: string };

  const KINDS: Kind[] = ["webhook", "telegram", "bark", "ntfy"];
  // Bark 与 ntfy 的公共服务地址只是默认值，可以换成自建的服务器
  const DEFAULT_URL: Partial<Record<Kind, string>> = { bark: "https://api.day.app", ntfy: "https://ntfy.sh" };
  const blank = (kind: Kind = "webhook"): Form => ({ name: "", kind, url: DEFAULT_URL[kind] ?? "", token: "", chat: "", key: "", topic: "", secret: "" });

  let items = $state<Channel[]>([]);
  let available = $state(true);
  let max = $state(5);
  let form = $state<Form>(blank());
  let editing = $state<Channel | null>(null);
  let adding = $state(false);
  let busy = $state<string | null>(null);

  async function load() {
    const r = await api.get<{ items: Channel[]; available: boolean; max: number }>("/api/v1/account/channels");
    items = r.items;
    available = r.available;
    max = r.max;
  }
  $effect(() => {
    load().catch((e) => toasts.push("err", errorText(e)));
  });

  function setKind(k: Kind) {
    const keepName = form.name;
    form = { ...blank(k), name: keepName };
  }
  function startAdd() {
    editing = null;
    form = blank();
    adding = true;
  }
  function startEdit(c: Channel) {
    editing = c;
    form = { ...blank(c.kind), name: c.name, url: c.url ?? "", chat: c.chat ?? "", topic: c.topic ?? "" };
    adding = true;
  }
  function close() {
    adding = false;
    editing = null;
  }

  // 只提交当前种类用到的字段
  function payload(): Partial<Form> {
    const f = form;
    const p: Partial<Form> = { name: f.name.trim(), kind: f.kind };
    if (f.kind === "webhook") Object.assign(p, { url: f.url, secret: f.secret });
    if (f.kind === "telegram") Object.assign(p, { token: f.token, chat: f.chat });
    if (f.kind === "bark") Object.assign(p, { url: f.url, key: f.key });
    if (f.kind === "ntfy") Object.assign(p, { url: f.url, topic: f.topic, token: f.token });
    return p;
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    busy = "save";
    try {
      if (editing) await api.put("/api/v1/account/channels/" + editing.id, payload());
      else await api.post("/api/v1/account/channels", payload());
      close();
      await load();
      toasts.push("ok", t("channels.saved"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = null;
    }
  }

  async function test(c: Channel) {
    busy = c.id;
    try {
      await api.post("/api/v1/account/channels/" + c.id + "/test");
      toasts.push("ok", t("channels.test_ok"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = null;
      await load().catch(() => {});
    }
  }

  async function remove(c: Channel) {
    try {
      await api.del("/api/v1/account/channels/" + c.id);
      if (editing?.id === c.id) close();
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  function target(c: Channel): string {
    if (c.kind === "webhook") return c.host ?? "";
    if (c.kind === "telegram") return "chat " + (c.chat ?? "");
    if (c.kind === "ntfy") return (c.topic ?? "") + " · " + (c.url ?? "").replace(/^https:\/\//, "");
    return (c.url ?? "").replace(/^https:\/\//, "");
  }
  const keep = $derived(editing ? t("channels.keep") : "");
  const canSave = $derived.by(() => {
    const f = form;
    const isNew = !editing;
    if (f.kind === "webhook") return !!f.url || !isNew;
    if (f.kind === "telegram") return (!!f.token || !isNew) && !!f.chat;
    if (f.kind === "bark") return !!f.url && (!!f.key || !isNew);
    return !!f.url && !!f.topic;
  });
</script>

<section class="panel box" aria-labelledby="ac-ch">
  <div class="row">
    <div>
      <h2 id="ac-ch">{t("channels.section")}</h2>
      <p class="muted">{t("channels.desc")}</p>
    </div>
    {#if available && !adding && items.length < max}
      <button class="btn" type="button" onclick={startAdd}>{t("channels.add")}</button>
    {/if}
  </div>

  {#if !available}
    <p class="note">{t("channels.unavailable")}</p>
  {:else}
    {#if items.length}
      <ul class="list">
        {#each items as c (c.id)}
          <li>
            <div class="head">
              <strong>{c.name}</strong>
              <span class="kind">{t("channels.kind_" + c.kind)}</span>
              <span class="state" class:on={c.enabled}>{c.enabled ? t("alerts.on") : t("alerts.unverified")}</span>
            </div>
            <p class="mono ltr target">{target(c)}</p>
            {#if c.lastError}
              <p class="err">{t("channels.last_error", { detail: c.lastError.replace(/^source: 推送失败：/, "") })}{#if c.lastAt} · {i18n.date(c.lastAt)}{/if}</p>
            {:else if c.lastAt}
              <p class="hint">{t("channels.last_ok", { time: i18n.date(c.lastAt) })}</p>
            {/if}
            <div class="actions">
              <button class="btn sm primary" type="button" disabled={busy === c.id} onclick={() => void test(c)}>{t("channels.send_test")}</button>
              <button class="btn sm" type="button" onclick={() => startEdit(c)}>{t("alerts.edit")}</button>
              <button class="btn sm ghost danger" type="button" onclick={() => void remove(c)}>{t("common.delete")}</button>
            </div>
          </li>
        {/each}
      </ul>
    {:else if !adding}
      <p class="hint">{t("channels.empty")}</p>
    {/if}

    {#if adding}
      <form class="grid" onsubmit={save} aria-label={editing ? t("channels.edit_title") : t("channels.add")}>
        {#if !editing}
          <div class="kinds" role="radiogroup" aria-label={t("channels.kind")}>
            {#each KINDS as k (k)}
              <button type="button" role="radio" aria-checked={form.kind === k} class="btn sm" class:primary={form.kind === k} onclick={() => setKind(k)}>{t("channels.kind_" + k)}</button>
            {/each}
          </div>
        {/if}
        <div class="two">
          <div>
            <label class="label" for="ch-name">{t("channels.name")}</label>
            <input id="ch-name" class="input" bind:value={form.name} maxlength="40" placeholder={t("channels.kind_" + form.kind)} autocomplete="off" />
          </div>
          {#if form.kind !== "telegram"}
            <div>
              <label class="label" for="ch-url">{form.kind === "webhook" ? t("channels.url") : t("channels.server")}</label>
              <input id="ch-url" class="input mono ltr" type="url" bind:value={form.url} placeholder={form.kind === "webhook" ? (keep || "https://example.com/hook") : ""} spellcheck="false" autocomplete="off" />
            </div>
          {/if}
          {#if form.kind === "telegram"}
            <div>
              <label class="label" for="ch-token">{t("channels.bot_token")}</label>
              <input id="ch-token" class="input mono ltr" type="password" bind:value={form.token} placeholder={keep || "123456:ABC…"} autocomplete="off" />
            </div>
            <div>
              <label class="label" for="ch-chat">{t("channels.chat")}</label>
              <input id="ch-chat" class="input mono ltr" bind:value={form.chat} placeholder="-1001234567890" spellcheck="false" autocomplete="off" />
            </div>
          {:else if form.kind === "bark"}
            <div>
              <label class="label" for="ch-key">{t("channels.device_key")}</label>
              <input id="ch-key" class="input mono ltr" type="password" bind:value={form.key} placeholder={keep} autocomplete="off" />
            </div>
          {:else if form.kind === "ntfy"}
            <div>
              <label class="label" for="ch-topic">{t("channels.topic")}</label>
              <input id="ch-topic" class="input mono ltr" bind:value={form.topic} spellcheck="false" autocomplete="off" />
            </div>
            <div>
              <label class="label" for="ch-ntoken">{t("channels.access_token")}</label>
              <input id="ch-ntoken" class="input mono ltr" type="password" bind:value={form.token} placeholder={keep || t("channels.optional")} autocomplete="off" />
            </div>
          {:else}
            <div>
              <label class="label" for="ch-secret">{t("channels.secret")}</label>
              <input id="ch-secret" class="input mono ltr" type="password" bind:value={form.secret} placeholder={keep || t("channels.optional")} autocomplete="off" />
            </div>
          {/if}
        </div>
        <p class="hint">{t("channels.hint_" + form.kind)} {t("channels.hint")}</p>
        <div class="actions end">
          <button class="btn ghost" type="button" onclick={close}>{t("common.cancel")}</button>
          <button class="btn primary" type="submit" disabled={busy === "save" || !canSave}>{t("common.save")}</button>
        </div>
      </form>
    {/if}
  {/if}
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
  .row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: var(--space-4);
  }
  .note {
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .list {
    display: grid;
    gap: var(--space-2);
  }
  .list li {
    display: grid;
    gap: var(--space-1);
    padding-block: var(--space-3);
    padding-inline: var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
  .head {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .kind {
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .state {
    margin-inline-start: auto;
    padding-inline: 8px;
    border-radius: var(--radius-full);
    background: var(--warn-soft);
    color: var(--warn);
    font-size: var(--text-xs);
    font-weight: 560;
    line-height: 22px;
  }
  .state.on {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .target {
    color: var(--fg-muted);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
  }
  .err {
    color: var(--err);
    font-size: var(--text-sm);
  }
  .grid {
    display: grid;
    gap: var(--space-3);
  }
  .kinds {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-3);
  }
  .actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
    margin-block-start: var(--space-1);
  }
  .actions.end {
    justify-content: flex-end;
  }
</style>
