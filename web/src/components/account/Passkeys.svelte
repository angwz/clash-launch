<script lang="ts">
  // 通行密钥：用设备的指纹、面容或 PIN 免密码登录；启用了两步验证时也可以代替验证码。删除需要口令。
  import Icon from "../Icon.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";
  import { isCancelled, passkeysSupported, registerPasskey } from "../../lib/webauthn";

  interface Key {
    id: string;
    name: string;
    createdAt: string;
    lastUsed?: string;
  }

  let keys = $state<Key[]>([]);
  let available = $state(true);
  let name = $state("");
  let busy = $state(false);
  let deleting = $state<string | null>(null);
  let password = $state("");
  const supported = passkeysSupported();

  async function load() {
    const r = await api.get<{ items: Key[]; available: boolean }>("/api/v1/account/passkeys");
    keys = r.items;
    available = r.available;
  }
  $effect(() => {
    load().catch((e) => toasts.push("err", errorText(e)));
  });

  async function add(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      await registerPasskey(name.trim() || t("passkey.default_name"));
      name = "";
      toasts.push("ok", t("passkey.added"));
      await load();
    } catch (err) {
      if (!isCancelled(err)) toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function rename(k: Key, v: string) {
    if (!v || v === k.name) return;
    try {
      await api.patch("/api/v1/account/passkeys/" + k.id, { name: v });
      await load();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function remove(e: SubmitEvent) {
    e.preventDefault();
    try {
      await api.post("/api/v1/account/passkeys/" + deleting + "/delete", { password });
      deleting = null;
      password = "";
      toasts.push("ok", t("passkey.removed"));
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }
</script>

<section class="panel box" aria-labelledby="ac-pk">
  <div>
    <h2 id="ac-pk">{t("passkey.section")}</h2>
    <p class="muted">{t("passkey.desc")}</p>
  </div>

  {#if !available}
    <p class="note">{t("passkey.unavailable")}</p>
  {:else if !supported}
    <p class="note">{t("passkey.unsupported")}</p>
  {:else}
    {#if keys.length}
      <ul class="keys">
        {#each keys as k (k.id)}
          <li>
            <Icon name="shield" size={18} />
            <div class="info">
              <input class="name" value={k.name} aria-label={t("passkey.name")} maxlength="60" onchange={(e) => void rename(k, e.currentTarget.value.trim())} />
              <span class="muted">{t("passkey.meta", { created: i18n.date(k.createdAt), used: k.lastUsed ? i18n.relative(k.lastUsed) : t("passkey.never") })}</span>
            </div>
            <button class="btn sm ghost danger" type="button" onclick={() => { deleting = k.id; password = ""; }}>{t("common.delete")}</button>
          </li>
        {/each}
      </ul>
    {/if}
    {#if deleting}
      <form class="confirm" onsubmit={remove}>
        <label class="label" for="pk-pw">{t("account.current_password")}</label>
        <input id="pk-pw" class="input" type="password" bind:value={password} autocomplete="current-password" />
        <div class="actions">
          <button class="btn ghost" type="button" onclick={() => (deleting = null)}>{t("common.cancel")}</button>
          <button class="btn danger" type="submit" disabled={!password}>{t("common.delete")}</button>
        </div>
      </form>
    {/if}
    <form class="add" onsubmit={add}>
      <input class="input" bind:value={name} maxlength="60" placeholder={t("passkey.name_placeholder")} aria-label={t("passkey.name")} />
      <button class="btn primary" type="submit" disabled={busy}><Icon name="plus" size={15} />{t("passkey.add")}</button>
    </form>
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
  .note {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .keys li {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding-block: var(--space-3);
    border-block-end: 1px solid var(--border);
  }
  .keys :global(.icon) {
    color: var(--accent-fg);
  }
  .info {
    display: grid;
    flex: 1;
    font-size: var(--text-sm);
  }
  .name {
    padding: 0;
    border: 0;
    border-block-end: 1px dashed transparent;
    background: none;
    color: var(--fg);
    font: inherit;
    font-weight: 600;
  }
  .name:hover,
  .name:focus {
    border-block-end-color: var(--border-strong);
    outline: none;
  }
  .add {
    display: flex;
    gap: var(--space-2);
  }
  .add .input {
    flex: 1;
  }
  .confirm {
    display: grid;
    gap: var(--space-2);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
