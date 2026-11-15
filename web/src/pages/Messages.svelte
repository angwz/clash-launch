<script lang="ts">
  // 站内消息（反馈 #74）：左侧会话列表，右侧聊天。可以插入 emoji，可以把自己的节点（手动节点或订阅）作为附件发出；
  // 对方默认会自动把附件加入节点页（可在设置中关闭，关闭后在消息里手动加入）。
  // 宽屏两栏，窄屏单栏（/messages 显示列表，/messages/<用户名> 显示会话）。
  // 会话页面可见时每 15 秒、列表每 30 秒自动刷新；发送后立即刷新。
  import { tick } from "svelte";
  import Icon from "../components/Icon.svelte";
  import { api, ApiError, listAll, type Source } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { inbox } from "../lib/inbox.svelte";
  import { live } from "../lib/live";
  import { prefs } from "../lib/prefs.svelte";
  import { router } from "../lib/router.svelte";
  import { toasts } from "../lib/toast.svelte";

  interface Thread {
    userId: string;
    username: string;
    last: string;
    lastKind: string;
    lastAt: string;
    mine: boolean;
    unread: number;
  }
  interface Msg {
    id: string;
    mine: boolean;
    text: string;
    attach?: { kind: "nodes" | "subscription"; name: string; url?: string; nodes?: number };
    createdAt: string;
    readAt?: string;
    addedSource?: string;
  }

  let { params }: { params: Record<string, string> } = $props();
  const peer = $derived(params.username ? decodeURIComponent(params.username) : "");

  let threads = $state<Thread[] | null>(null);
  let other = $state<{ id: string; username: string } | null>(null);
  let msgs = $state<Msg[]>([]); // 正序（旧的在前）
  let next = $state<string | undefined>();
  let missing = $state(false);
  let to = $state("");
  let text = $state("");
  let attach = $state<Source | null>(null);
  let sources = $state<Source[] | null>(null);
  let panel = $state<"" | "emoji" | "attach">("");
  let sending = $state(false);
  let scroller: HTMLDivElement | undefined = $state();
  let input: HTMLTextAreaElement | undefined = $state();

  // 常用 emoji：表情、手势、物品、符号（Unicode 字符，跟随系统字体显示）
  const EMOJI = [..."😀😄😂🤣😊😍🥰😘😎🤔😅😭😡🥳😴🤗🙏👍👎👌✌️🤝👏💪👋🫶❤️🔥✨🎉🎁🚀⚡🌏🌙☀️⭐💡🔒🔑📶📡🛰️💻📱🖥️⌛✅❌⚠️❓💯🆗🆕👀🍻☕🍰🐱🐶".matchAll(/\p{Extended_Pictographic}️?/gu)].map((m) => m[0]);

  async function loadThreads() {
    threads = (await api.get<{ items: Thread[] }>("/api/v1/messages")).items;
  }

  async function loadThread(reset = true) {
    if (!peer) return;
    try {
      const r = await api.get<{ user: { id: string; username: string }; items: Msg[]; next?: string }>("/api/v1/messages/with/" + encodeURIComponent(peer), {
        query: { cursor: reset ? undefined : next, limit: 50 },
      });
      missing = false;
      other = r.user;
      const older = [...r.items].reverse();
      const atBottom = !scroller || scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 40;
      msgs = reset ? older : [...older, ...msgs];
      next = r.next;
      if (reset) {
        void inbox.refresh().catch(() => undefined);
        if (atBottom) {
          await tick();
          scroller?.scrollTo({ top: scroller.scrollHeight });
        }
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        missing = true;
        other = null;
        msgs = [];
      } else throw e;
    }
  }

  $effect(() => {
    loadThreads().catch((e) => toasts.push("err", errorText(e)));
  });
  $effect(() => {
    void peer;
    msgs = [];
    other = null;
    attach = null;
    loadThread().catch((e) => toasts.push("err", errorText(e)));
  });
  $effect(() => live(loadThreads, 30_000).stop);
  $effect(() => (peer ? live(() => loadThread(), 15_000).stop : undefined));

  function open(name: string) {
    router.go("/messages/" + encodeURIComponent(name));
  }

  function start(e: SubmitEvent) {
    e.preventDefault();
    const name = to.trim();
    if (name) {
      to = "";
      open(name);
    }
  }

  async function send() {
    if (sending || !peer || (!text.trim() && !attach)) return;
    sending = true;
    try {
      await api.post("/api/v1/messages", { to: peer, text, source: attach?.id ?? "" });
      text = "";
      attach = null;
      panel = "";
      await Promise.all([loadThread(), loadThreads()]);
      await tick();
      scroller?.scrollTo({ top: scroller.scrollHeight, behavior: "smooth" });
      input?.focus();
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      sending = false;
    }
  }

  function onKey(e: KeyboardEvent) {
    // 回车发送，Shift+回车换行；输入法组字时的回车不发送
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      void send();
    }
  }

  function insertEmoji(em: string) {
    const el = input;
    if (!el) {
      text += em;
      return;
    }
    const a = el.selectionStart ?? text.length;
    const b = el.selectionEnd ?? a;
    text = text.slice(0, a) + em + text.slice(b);
    void tick().then(() => {
      el.focus();
      el.setSelectionRange(a + em.length, a + em.length);
    });
  }

  async function togglePanel(p: "emoji" | "attach") {
    panel = panel === p ? "" : p;
    if (p === "attach" && !sources) {
      try {
        sources = await listAll<Source>("/api/v1/sources");
      } catch (e) {
        toasts.push("err", errorText(e));
      }
    }
  }

  async function accept(m: Msg) {
    try {
      const r = await api.post<{ source: string; name: string }>("/api/v1/messages/" + m.id + "/accept", {});
      toasts.push("ok", t("messages.added", { name: r.name }));
      await loadThread();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function hide(m: Msg) {
    try {
      await api.del("/api/v1/messages/" + m.id);
      msgs = msgs.filter((x) => x.id !== m.id);
      void loadThreads();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  const blocked = $derived(!!other && (prefs.data.blocked ?? []).includes(other.id));
  async function toggleBlock() {
    if (!other) return;
    const list = prefs.data.blocked ?? [];
    const nextList = blocked ? list.filter((x) => x !== other!.id) : [...list, other.id];
    try {
      await prefs.save({ ...prefs.data, blocked: nextList.length ? nextList : undefined });
      toasts.push("ok", blocked ? t("messages.unblocked", { name: other.username }) : t("messages.blocked", { name: other.username }));
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  const host = (u?: string) => {
    try {
      return u ? new URL(u).host : "";
    } catch {
      return "";
    }
  };
  // 同一天的消息之间不重复显示日期
  const dayOf = (iso: string) => i18n.date(iso, "long").split(" ")[0];
</script>

<div class="page" class:has-peer={!!peer}>
  <aside class="threads panel" aria-label={t("messages.threads")}>
    <form class="start" onsubmit={start}>
      <input class="input" bind:value={to} placeholder={t("messages.to_placeholder")} aria-label={t("messages.to")} autocomplete="off" />
      <button class="btn primary icon" type="submit" aria-label={t("messages.start")} title={t("messages.start")}><Icon name="plus" size={15} /></button>
    </form>
    {#if threads && !threads.length}
      <p class="muted empty">{t("messages.no_threads")}</p>
    {:else if threads}
      <ul>
        {#each threads as th (th.userId)}
          <li>
            <a href={"/messages/" + encodeURIComponent(th.username)} class:active={th.username.toLowerCase() === peer.toLowerCase()} aria-current={th.username.toLowerCase() === peer.toLowerCase() ? "page" : undefined}>
              <span class="avatar" aria-hidden="true">{th.username.slice(0, 1).toUpperCase()}</span>
              <span class="th-main">
                <span class="th-top"><strong>{th.username}</strong><span class="muted when">{i18n.relative(th.lastAt)}</span></span>
                <span class="th-last muted">
                  {#if th.mine}{t("messages.you")}: {/if}{#if th.lastKind !== "text"}<Icon name={th.lastKind === "subscription" ? "link" : "server"} size={12} /> {/if}{th.last}
                </span>
              </span>
              {#if th.unread}<span class="badge">{th.unread}</span>{/if}
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </aside>

  <section class="chat panel" aria-label={other ? t("messages.with", { name: other.username }) : t("nav.messages")}>
    {#if !peer}
      <div class="placeholder">
        <Icon name="message" size={28} />
        <p>{t("messages.pick")}</p>
        <p class="muted">{t("messages.pick_hint")}</p>
      </div>
    {:else if missing}
      <div class="placeholder">
        <p>{t("messages.no_user", { name: peer })}</p>
        <a class="btn sm" href="/messages">{t("common.close")}</a>
      </div>
    {:else}
      <header class="chat-head">
        <a class="back btn ghost icon" href="/messages" aria-label={t("messages.threads")}><Icon name="chevron-right" size={16} /></a>
        <span class="avatar" aria-hidden="true">{(other?.username ?? peer).slice(0, 1).toUpperCase()}</span>
        <strong class="peer">{other?.username ?? peer}</strong>
        {#if other}
          <button class="btn sm ghost" type="button" onclick={() => void toggleBlock()}>{blocked ? t("messages.unblock") : t("messages.block")}</button>
        {/if}
      </header>

      <div class="scroller" bind:this={scroller} role="log" aria-live="polite">
        {#if next}<button class="btn sm ghost more" type="button" onclick={() => void loadThread(false)}>{t("messages.older")}</button>{/if}
        {#if !msgs.length}<p class="muted hello">{t("messages.say_hi", { name: other?.username ?? peer })}</p>{/if}
        {#each msgs as m, i (m.id)}
          {#if i === 0 || dayOf(msgs[i - 1]!.createdAt) !== dayOf(m.createdAt)}
            <p class="day muted">{dayOf(m.createdAt)}</p>
          {/if}
          <div class="msg" class:mine={m.mine}>
            <div class="bubble">
              {#if m.text}<p class="text">{m.text}</p>{/if}
              {#if m.attach}
                <div class="card">
                  <Icon name={m.attach.kind === "subscription" ? "link" : "server"} size={16} />
                  <div class="card-main">
                    <strong>{m.attach.name}</strong>
                    <span class="muted">{m.attach.kind === "subscription" ? t("messages.attach_sub", { host: host(m.attach.url) }) : t("messages.attach_nodes", { n: m.attach.nodes ?? 0 })}</span>
                  </div>
                  {#if !m.mine}
                    {#if m.addedSource}
                      <a class="btn sm ghost" href={"/sources/" + m.addedSource}><Icon name="check" size={13} />{t("messages.in_library")}</a>
                    {:else}
                      <button class="btn sm primary" type="button" onclick={() => void accept(m)}><Icon name="plus" size={13} />{t("messages.add")}</button>
                    {/if}
                  {/if}
                </div>
              {/if}
            </div>
            <div class="meta muted">
              <time datetime={m.createdAt} title={i18n.date(m.createdAt, "long")}>{i18n.date(m.createdAt).split(" ").slice(-1)[0]}</time>
              {#if m.mine && m.readAt}<span>· {t("messages.read")}</span>{/if}
              <button class="hide" type="button" aria-label={t("messages.hide")} title={t("messages.hide")} onclick={() => void hide(m)}><Icon name="x" size={12} /></button>
            </div>
          </div>
        {/each}
      </div>

      <div class="composer">
        {#if attach}
          <div class="chip">
            <Icon name={attach.kind === "subscription" ? "link" : "server"} size={13} />
            <span>{t("messages.attaching", { name: attach.name })}</span>
            <button type="button" aria-label={t("common.remove")} onclick={() => (attach = null)}><Icon name="x" size={12} /></button>
          </div>
        {/if}
        {#if panel === "emoji"}
          <div class="pop emoji" role="listbox" aria-label={t("messages.emoji")}>
            {#each EMOJI as em (em)}<button type="button" role="option" aria-selected="false" onclick={() => insertEmoji(em)}>{em}</button>{/each}
          </div>
        {:else if panel === "attach"}
          <div class="pop sources" role="listbox" aria-label={t("messages.attach")}>
            {#if !sources}<p class="muted">{t("common.loading")}</p>{:else if !sources.length}<p class="muted">{t("sources.empty")}</p>{/if}
            {#each sources ?? [] as s (s.id)}
              <button type="button" role="option" aria-selected={attach?.id === s.id} onclick={() => { attach = s; panel = ""; input?.focus(); }}>
                <Icon name={s.kind === "subscription" ? "link" : "server"} size={14} />
                <span class="s-name">{s.name}</span>
                <span class="muted">{t("source.kind." + s.kind)}</span>
              </button>
            {/each}
            <p class="hint">{t("messages.attach_hint")}</p>
          </div>
        {/if}
        <div class="row">
          <button class="btn ghost icon" type="button" aria-label={t("messages.emoji")} title={t("messages.emoji")} aria-expanded={panel === "emoji"} onclick={() => void togglePanel("emoji")}><Icon name="smile" size={17} /></button>
          <button class="btn ghost icon" type="button" aria-label={t("messages.attach")} title={t("messages.attach")} aria-expanded={panel === "attach"} onclick={() => void togglePanel("attach")}><Icon name="paperclip" size={17} /></button>
          <textarea bind:this={input} class="textarea" rows="1" bind:value={text} onkeydown={onKey} placeholder={t("messages.placeholder")} aria-label={t("messages.placeholder")} maxlength="2000"></textarea>
          <button class="btn primary" type="button" disabled={sending || (!text.trim() && !attach)} onclick={() => void send()}><Icon name="send" size={15} />{t("messages.send")}</button>
        </div>
      </div>
    {/if}
  </section>
</div>

<style>
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-6);
    display: grid;
    grid-template-columns: 300px minmax(0, 1fr);
    gap: var(--space-4);
    block-size: calc(100dvh - var(--topbar-h, 56px));
    min-block-size: 480px;
  }
  .threads,
  .chat {
    display: flex;
    flex-direction: column;
    min-block-size: 0;
    overflow: hidden;
  }
  .start {
    display: flex;
    gap: var(--space-2);
    padding: var(--space-3);
    border-block-end: 1px solid var(--border);
  }
  .threads ul {
    overflow-y: auto;
    list-style: none;
    margin: 0;
    padding: var(--space-1);
  }
  .threads a {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    color: var(--fg);
  }
  .threads a:hover {
    background: var(--bg-hover);
    text-decoration: none;
  }
  .threads a.active {
    background: var(--bg-active);
  }
  .avatar {
    display: grid;
    place-items: center;
    flex: none;
    inline-size: 32px;
    block-size: 32px;
    border-radius: 50%;
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-weight: 650;
  }
  .th-main {
    display: grid;
    flex: 1;
    min-inline-size: 0;
  }
  .th-top {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
  }
  .when {
    font-size: var(--text-xs);
    white-space: nowrap;
  }
  .th-last {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-sm);
  }
  .badge {
    min-inline-size: 18px;
    padding-inline: 5px;
    border-radius: var(--radius-full);
    background: var(--accent);
    color: var(--fg-on-accent);
    font-size: 11px;
    font-weight: 600;
    line-height: 18px;
    text-align: center;
  }
  .empty {
    padding: var(--space-4);
    font-size: var(--text-sm);
  }
  .placeholder {
    display: grid;
    place-content: center;
    justify-items: center;
    gap: var(--space-2);
    flex: 1;
    padding: var(--space-6);
    color: var(--fg-muted);
    text-align: center;
  }
  .chat-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border-block-end: 1px solid var(--border);
  }
  .peer {
    flex: 1;
  }
  .back {
    display: none;
    transform: scaleX(-1);
  }
  .scroller {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-4);
  }
  .more {
    align-self: center;
  }
  .hello {
    margin: auto;
  }
  .day {
    align-self: center;
    font-size: var(--text-xs);
    margin-block: var(--space-2);
  }
  .msg {
    display: grid;
    justify-items: start;
    max-inline-size: min(560px, 82%);
  }
  .msg.mine {
    justify-items: end;
    align-self: flex-end;
  }
  .bubble {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-radius: 14px;
    border-end-start-radius: 4px;
    background: var(--bg-sunken);
  }
  .mine .bubble {
    border-end-start-radius: 14px;
    border-end-end-radius: 4px;
    background: var(--accent-soft);
  }
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    line-height: 1.55;
  }
  .card {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
  }
  .card-main {
    display: grid;
    min-inline-size: 0;
    font-size: var(--text-sm);
  }
  .card-main strong {
    overflow-wrap: anywhere;
  }
  .meta {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding-inline: var(--space-1);
    font-size: 11px;
  }
  .hide {
    display: inline-grid;
    padding: 2px;
    border: 0;
    background: none;
    color: inherit;
    cursor: pointer;
    opacity: 0;
  }
  .msg:hover .hide,
  .hide:focus-visible {
    opacity: 1;
  }
  .composer {
    position: relative;
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border-block-start: 1px solid var(--border);
  }
  .composer .row {
    display: flex;
    align-items: flex-end;
    gap: var(--space-2);
  }
  .composer .textarea {
    flex: 1;
    min-block-size: 38px;
    max-block-size: 160px;
    resize: none;
    field-sizing: content;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    justify-self: start;
    padding: 3px var(--space-2);
    border-radius: var(--radius-full);
    background: var(--accent-soft);
    color: var(--accent-fg);
    font-size: var(--text-sm);
  }
  .chip button {
    display: inline-grid;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    cursor: pointer;
  }
  .pop {
    padding: var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
    max-block-size: 220px;
    overflow-y: auto;
  }
  .pop.emoji {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(36px, 1fr));
  }
  .pop.emoji button {
    padding: 4px;
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    font-size: 20px;
    line-height: 1.3;
    cursor: pointer;
  }
  .pop.emoji button:hover {
    background: var(--bg-hover);
  }
  .pop.sources {
    display: grid;
    gap: 2px;
  }
  .pop.sources button {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 6px var(--space-2);
    border: 0;
    border-radius: var(--radius-md);
    background: none;
    color: var(--fg);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }
  .pop.sources button:hover,
  .pop.sources button[aria-selected="true"] {
    background: var(--bg-hover);
  }
  .s-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  @media (max-width: 860px) {
    .page {
      grid-template-columns: minmax(0, 1fr);
      padding: var(--space-3);
    }
    .page.has-peer .threads,
    .page:not(.has-peer) .chat {
      display: none;
    }
    .back {
      display: inline-flex;
    }
  }
</style>
