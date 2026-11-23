<script lang="ts">
  // 昵称与用户名（需求 R4、R5）。昵称随时可改，可含空格；用户名 180 天改一次，需要当前密码，
  // 旧名保留 180 天不被别人使用。改完更新会话里的用户信息，顶栏与列表立即显示新名字。
  // 头像（需求 R19）：换一个预置的、从全部预置中挑，或上传图片（服务器裁成正方形并缩小）。
  import { api, type User } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { session } from "../../lib/session.svelte";
  import { toasts } from "../../lib/toast.svelte";
  import { PRESETS } from "../../lib/avatars";
  import Avatar from "../Avatar.svelte";

  let picking = $state(false);
  let fileInput: HTMLInputElement | undefined = $state();

  async function setPreset(preset: string) {
    busy = true;
    try {
      const r = await api.put<{ avatar: string }>("/api/v1/account/avatar", { preset });
      if (session.user) session.user = { ...session.user, avatar: r.avatar };
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }
  async function upload(file: File | undefined) {
    if (!file) return;
    busy = true;
    try {
      const r = await api.post<{ avatar: string }>("/api/v1/account/avatar/upload", file);
      if (session.user) session.user = { ...session.user, avatar: r.avatar };
      toasts.push("ok", t("avatar.uploaded"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
      if (fileInput) fileInput.value = "";
    }
  }
  async function suggest() {
    try {
      nickname = (await api.get<{ nickname: string }>("/api/v1/account/nickname/suggest", { query: { lang: i18n.locale } })).nickname;
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  let nickname = $state(session.user?.nickname ?? "");
  let username = $state("");
  let password = $state("");
  let busy = $state(false);

  const nextRename = $derived(session.user?.nextRenameAt ?? "");
  const nickChanged = $derived(nickname.trim() !== (session.user?.nickname ?? ""));
  const renameOk = $derived(!nextRename && username.trim() !== "" && username.trim() !== session.user?.username && password !== "");

  async function saveNickname(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    try {
      session.user = await api.put<User>("/api/v1/account/nickname", { nickname });
      nickname = session.user.nickname ?? "";
      toasts.push("ok", t("account.nickname_saved"));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function rename(e: SubmitEvent) {
    e.preventDefault();
    if (!renameOk) return;
    busy = true;
    try {
      session.user = await api.put<User>("/api/v1/account/username", { username: username.trim(), password });
      username = "";
      password = "";
      toasts.push("ok", t("account.username_saved", { name: session.user.username }));
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }
</script>

<section class="panel box" aria-labelledby="ac-id">
  <h2 id="ac-id">{t("account.identity")}</h2>

  <div class="avatar-row">
    {#if session.user}<Avatar user={session.user} size={64} />{/if}
    <div class="avatar-actions">
      <span class="label">{t("avatar.title")}</span>
      <div class="btns">
        <button class="btn sm" type="button" disabled={busy} onclick={() => void setPreset("")}>{t("avatar.shuffle")}</button>
        <button class="btn sm" type="button" disabled={busy} aria-expanded={picking} onclick={() => (picking = !picking)}>{t("avatar.pick")}</button>
        <button class="btn sm" type="button" disabled={busy} onclick={() => fileInput?.click()}>{t("avatar.upload")}</button>
        <input bind:this={fileInput} class="file" type="file" accept="image/png,image/jpeg,image/gif" aria-label={t("avatar.upload")} onchange={(e) => void upload(e.currentTarget.files?.[0])} />
      </div>
      <p class="hint">{t("avatar.hint")}</p>
    </div>
  </div>
  {#if picking}
    <div class="grid" role="radiogroup" aria-label={t("avatar.pick")}>
      {#each PRESETS as key (key)}
        <button type="button" class="pick" role="radio" aria-checked={session.user?.avatar === key} aria-label={key} disabled={busy} onclick={() => void setPreset(key)}>
          <Avatar user={{ username: "", avatar: key }} size={40} />
        </button>
      {/each}
    </div>
  {/if}

  <form class="row" onsubmit={saveNickname}>
    <div class="field">
      <label class="label" for="ac-nick">{t("account.nickname")}</label>
      <input id="ac-nick" class="input" bind:value={nickname} maxlength="32" placeholder={t("account.nickname_placeholder")} autocomplete="nickname" />
    </div>
    <button class="btn ghost" type="button" onclick={() => void suggest()}>{t("avatar.nick_shuffle")}</button>
    <button class="btn" type="submit" disabled={!nickChanged || busy}>{t("common.save")}</button>
    <!-- 说明放在整行：按钮与输入框底边对齐（反馈 #113） -->
    <p class="hint wide">{t("account.nickname_hint")}</p>
  </form>

  <form class="rename" onsubmit={rename}>
    <div class="field">
      <label class="label" for="ac-user">{t("account.username_new", { name: session.user?.username ?? "" })}</label>
      <input id="ac-user" class="input" bind:value={username} maxlength="32" autocomplete="username" disabled={!!nextRename} />
    </div>
    <div class="field">
      <label class="label" for="ac-user-pw">{t("account.current_password")}</label>
      <input id="ac-user-pw" class="input" type="password" bind:value={password} autocomplete="current-password" disabled={!!nextRename} />
    </div>
    <button class="btn" type="submit" disabled={!renameOk || busy}>{t("account.username_change")}</button>
    <p class="hint wide">
      {#if nextRename}{t("account.username_wait", { when: i18n.date(nextRename) })}{:else}{t("account.username_hint")}{/if}
    </p>
  </form>
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
  .avatar-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }
  .avatar-actions {
    display: grid;
    gap: var(--space-1);
  }
  .btns {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .file {
    display: none;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(48px, 1fr));
    gap: var(--space-2);
  }
  .pick {
    display: grid;
    place-items: center;
    padding: 3px;
    border: 2px solid transparent;
    border-radius: 50%;
    background: none;
    cursor: pointer;
  }
  .pick[aria-checked="true"] {
    border-color: var(--accent);
  }
  .pick:hover {
    background: var(--bg-hover);
  }
  .row,
  .rename {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: var(--space-3);
  }
  .field {
    flex: 1 1 220px;
    display: grid;
    gap: var(--space-1);
  }
  .wide {
    flex-basis: 100%;
    margin: 0;
  }
</style>
