<script lang="ts">
  // 用户名 + 口令 + 确认口令。口令规则由后端判定（最短 10 个字符、常见口令表）；这里只做长度与一致性的即时提示。
  import { t } from "../lib/i18n/i18n.svelte";

  let {
    username = $bindable(""),
    password = $bindable(""),
    valid = $bindable(false),
  }: { username?: string; password?: string; valid?: boolean } = $props();

  let confirm = $state("");
  const tooShort = $derived(password.length > 0 && [...password].length < 10);
  const mismatch = $derived(confirm.length > 0 && confirm !== password);
  $effect(() => {
    valid = username.length >= 3 && [...password].length >= 10 && confirm === password;
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
