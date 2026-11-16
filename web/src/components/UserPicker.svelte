<script lang="ts">
  // 选择收件人（反馈 #75、#76）：按用户名前缀联想（不区分大小写），标出对方是否接收消息；
  // 一个都匹配不上时提示「没有这个用户」。方向键选择，回车确认，Esc 收起。
  import Icon from "./Icon.svelte";
  import { api } from "../lib/api/client";
  import { t } from "../lib/i18n/i18n.svelte";

  interface Hit {
    username: string;
    nickname?: string;
    accepts: boolean;
  }

  let {
    value = $bindable(""),
    onselect,
    placeholder,
    id,
  }: { value?: string; onselect: (username: string) => void; placeholder: string; id?: string } = $props();

  const listId = $derived((id ?? "user-picker") + "-list");
  let hits = $state<Hit[] | null>(null);
  let at = $state(0);
  let timer: ReturnType<typeof setTimeout> | undefined;

  // 停止输入 200 毫秒后查询；结果返回时输入已变化则丢弃
  $effect(() => {
    const q = value.trim();
    clearTimeout(timer);
    if (!q) {
      hits = null;
      return;
    }
    timer = setTimeout(async () => {
      try {
        const r = await api.get<{ items: Hit[] }>("/api/v1/users/search", { query: { q } });
        if (value.trim() === q) {
          hits = r.items;
          at = 0;
        }
      } catch {
        hits = null;
      }
    }, 200);
  });

  function pick(name: string) {
    hits = null;
    onselect(name);
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Enter") {
      e.preventDefault();
      const h = hits?.[at];
      if (h) pick(h.username);
      else if (value.trim()) pick(value.trim());
      return;
    }
    if (!hits?.length) return;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      at = (at + (e.key === "ArrowDown" ? 1 : hits.length - 1)) % hits.length;
    } else if (e.key === "Escape") {
      hits = null;
    }
  }
</script>

<div class="picker">
  <input
    {id}
    class="input"
    bind:value
    {onkeydown}
    onblur={() => setTimeout(() => (hits = null), 150)}
    {placeholder}
    aria-label={t("messages.to")}
    autocomplete="off"
    spellcheck="false"
    role="combobox"
    aria-controls={listId}
    aria-expanded={!!hits}
    aria-autocomplete="list"
  />
  {#if value.trim() && hits}
    <ul id={listId} class="hits" role="listbox" aria-label={t("messages.to")}>
      {#each hits as h, i (h.username)}
        <li role="option" aria-selected={i === at} class:on={i === at}>
          <button type="button" onpointerdown={(e) => { e.preventDefault(); pick(h.username); }}>
            <span class="avatar" aria-hidden="true">{h.username.slice(0, 1).toUpperCase()}</span>
            <span class="name">{#if h.nickname}{h.nickname} <span class="muted">@{h.username}</span>{:else}{h.username}{/if}</span>
            <span class="state" class:no={!h.accepts}>{h.accepts ? t("messages.hit_ok") : t("messages.hit_off")}</span>
          </button>
        </li>
      {:else}
        <li class="missing muted" role="presentation"><Icon name="info" size={13} />{t("messages.hit_none", { name: value.trim() })}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    position: relative;
    flex: 1;
  }
  .hits {
    position: absolute;
    z-index: 30;
    inset-inline: 0;
    inset-block-start: calc(100% + 4px);
    margin: 0;
    padding: 4px;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
  }
  .hits button {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    inline-size: 100%;
    padding: 6px var(--space-2);
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--fg);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  li.on button,
  .hits button:hover {
    background: var(--bg-hover);
  }
  .avatar {
    display: grid;
    place-items: center;
    flex: none;
    inline-size: 22px;
    block-size: 22px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: 11px;
    font-weight: 650;
  }
  .name {
    flex: 1;
    font-weight: 560;
  }
  .state {
    color: var(--ok);
    font-size: var(--text-xs);
  }
  .state.no {
    color: var(--fg-muted);
  }
  .missing {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: 6px var(--space-2);
    font-size: var(--text-sm);
  }
</style>
