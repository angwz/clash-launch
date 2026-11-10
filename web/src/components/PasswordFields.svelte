<script lang="ts">
  // 用户名 + 口令 + 确认口令。口令规则由后端判定（至少 5 个字符、至少 2 类字符、常见口令表）；
  // 这里只做即时提示（lib/password.ts）。
  import { t } from "../lib/i18n/i18n.svelte";
  import { passwordOk } from "../lib/password";

  let {
    username = $bindable(""),
    password = $bindable(""),
    valid = $bindable(false),
  }: { username?: string; password?: string; valid?: boolean } = $props();

  let confirm = $state("");
  const strong = $derived(passwordOk(password));
  const tooShort = $derived(password.length > 0 && !strong);
  const mismatch = $derived(confirm.length > 0 && confirm !== password);
  $effect(() => {
    valid = username.trim().length >= 1 && strong && confirm === password;
  });
</script>

<div>
  <label class="label" for="pf-u">{t("field.username")}</label>
  <input id="pf-u" class="input" bind:value={username} autocomplete="username" required autocapitalize="off" spellcheck="false" maxlength="32" />
  <p class="hint">{t("register.username_hint")}</p>
</div>
<div>
  <label class="label" for="pf-p">{t("field.password")}</label>
  <input id="pf-p" class="input" type="password" bind:value={password} autocomplete="new-password" required aria-invalid={tooShort} aria-describedby="pf-p-hint" />
  <p class="hint" class:field-error={tooShort} id="pf-p-hint">{t("register.password_hint")}</p>
</div>
<div>
  <label class="label" for="pf-c">{t("field.password_confirm")}</label>
  <input id="pf-c" class="input" type="password" bind:value={confirm} autocomplete="new-password" required aria-invalid={mismatch} />
  {#if mismatch}<p class="field-error">{t("register.mismatch")}</p>{/if}
</div>
