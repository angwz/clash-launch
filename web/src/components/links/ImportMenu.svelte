<script lang="ts">
  // 导入到客户端（反馈 #74）：一键导入链接（各客户端的 install-config）与下载配置文件。
  // 打开导入链接时浏览器会询问是否用对应的应用打开；没有安装时什么也不会发生，所以每项都写明适用平台。
  import Icon from "../Icon.svelte";
  import Menu from "../Menu.svelte";
  import { CLIENTS, importURL } from "../../lib/clients";
  import { t } from "../../lib/i18n/i18n.svelte";
  import { api } from "../../lib/api/client";

  // track：用于记录「一键导入」的对象（反馈 #76）——自己的配置链接 link、自己的短链接 short，或别人授权给我的短链接 code。
  // 浏览器打开导入链接后本站收不到请求，所以在点击时上报一次，记为 import；客户端随后拉取配置时另记一次。
  let { url, name, ondownload, track }: { url: string; name: string; ondownload?: () => void; track?: { link?: string; short?: string; code?: string } } = $props();

  function recordImport(client: string) {
    if (!track) return;
    void api.post("/api/v1/imports", { ...track, client }).then(() => ondownload?.(), () => undefined);
  }
</script>

<Menu label={t("links.import")} icon="download" text buttonClass="btn sm" align="end">
  {#snippet items(close)}
    <p class="menu-head">{t("links.import_head")}</p>
    {#each CLIENTS as c (c.id)}
      <a role="menuitem" href={importURL(c, url, name)} onclick={() => { recordImport(c.id); close(); }}>
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
