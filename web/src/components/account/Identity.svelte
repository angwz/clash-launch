<script lang="ts">
  // 昵称与用户名（需求 R4、R5）。昵称随时可改，可含空格；用户名 180 天改一次，需要当前密码，
  // 旧名保留 180 天不被别人使用。改完更新会话里的用户信息，顶栏与列表立即显示新名字。
  import { api, type User } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { session } from "../../lib/session.svelte";
  import { toasts } from "../../lib/toast.svelte";

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

  <form class="row" onsubmit={saveNickname}>
    <div class="field">
      <label class="label" for="ac-nick">{t("account.nickname")}</label>
      <input id="ac-nick" class="input" bind:value={nickname} maxlength="32" placeholder={t("account.nickname_placeholder")} autocomplete="nickname" />
      <p class="hint">{t("account.nickname_hint")}</p>
    </div>
    <button class="btn" type="submit" disabled={!nickChanged || busy}>{t("common.save")}</button>
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
  .row .hint {
    margin: 0;
  }
  .wide {
    flex-basis: 100%;
    margin: 0;
  }
  .row .btn,
  .rename .btn {
    margin-block-end: 0;
  }
  .row {
    align-items: flex-start;
  }
  .row .btn {
    margin-block-start: 22px;
  }
</style>
