<script lang="ts">
  // 导入到客户端（反馈 #74）：一键导入链接（各客户端的 install-config）与下载配置文件。
  // 打开导入链接时浏览器会询问是否用对应的应用打开；没有安装时什么也不会发生，所以每项都写明适用平台。
  import Icon from "../Icon.svelte";
  import Menu from "../Menu.svelte";
  import { CLIENTS, importURL } from "../../lib/clients";
  import { t } from "../../lib/i18n/i18n.svelte";

  let { url, name, ondownload }: { url: string; name: string; ondownload?: () => void } = $props();
</script>

<Menu label={t("links.import")} icon="download" text buttonClass="btn sm" align="end">
  {#snippet items(close)}
    <p class="menu-head">{t("links.import_head")}</p>
    {#each CLIENTS as c (c.id)}
      <a role="menuitem" href={importURL(c, url, name)} onclick={() => close()}>
        <span class="name">{c.name}</span>
        <span class="plat">{c.platforms}</span>
      </a>
    {/each}
    <hr />
    <a role="menuitem" href={url + (url.includes("?") ? "&" : "?") + "download=1"} download title={t("links.export_hint")} onclick={() => { ondownload?.(); close(); }}>
      <Icon name="file" size={14} /><span class="name">{t("links.export")}</span>
    </a>
  {/snippet}
</Menu>

<style>
  .menu-head {
    padding: var(--space-1) var(--space-3) var(--space-2);
    color: var(--fg-muted);
    font-size: var(--text-xs);
  }
  .name {
    flex: 1;
  }
  .plat {
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
</style>
