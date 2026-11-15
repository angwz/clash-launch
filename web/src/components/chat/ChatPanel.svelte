<script lang="ts">
  // 站内消息（反馈 #74、#75）：左侧是交流过的人（永久保留，按最近交流时间排序；× 删除放入回收站保存一年），
  // 右侧聊天。输入用户名时联想并标出对方是否存在、是否接收消息。
  // 附件：自己的节点可以「分享」（复制一份，对方可随意修改）或「授权」（对方只能使用，随你更新，可随时收回）；
  // 自己的短链接只能「授权」（为对方单独生成一个新的短链接，使用情况在订阅分析中按好友统计）。
  // 删除对话会收回你们之间的全部授权。会话每 15 秒、列表每 30 秒自动刷新。
  // 反馈 #79：不再是独立页面，而是浮在当前页面之上的窗口（右上角消息图标打开，Esc 或 × 关闭）；
  // 大小随浏览器窗口变化，窗口够宽时左右两栏，否则单栏（用容器查询按窗口自身宽度切换），手机上铺满屏幕。
  import { tick } from "svelte";
  import Icon from "../Icon.svelte";
  import UserPicker from "../UserPicker.svelte";
  import { api, ApiError, listAll, type Source } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { inbox } from "../../lib/inbox.svelte";
  import { live } from "../../lib/live";
  import { prefs } from "../../lib/prefs.svelte";
  import { chat } from "../../lib/chat.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Thread {
    userId: string;
    username: string;
    last: string;
    lastKind: string;
    lastAt: string;
    mine: boolean;
    unread: number;
  }
  interface Deleted {
    userId: string;
    username: string;
    deletedAt: string;
    purgeAt: string;
  }
  interface Attach {
    kind: "nodes" | "subscription" | "grant" | "template";
    note?: string;
    name: string;
    url?: string;
    nodes?: number;
    target?: "source" | "short";
    grantId?: string;
    sourceKind?: string;
    active?: boolean;
  }
  interface Msg {
    id: string;
    mine: boolean;
    text: string;
    attach?: Attach;
    createdAt: string;
    readAt?: string;
    addedSource?: string;
  }
  interface Short {
    id: string;
    code: string;
    note: string;
    grantee?: string;
  }
  /** 待发送的附件 */
  type Pending = { kind: "source"; mode: "share" | "grant"; id: string; name: string; sourceKind: string } | { kind: "short"; id: string; name: string };

  const peer = $derived(chat.peer);

  let threads = $state<Thread[] | null>(null);
  let deleted = $state<Deleted[]>([]);
  let showBin = $state(false);
  let other = $state<{ id: string; username: string } | null>(null);
  let deletedAt = $state<string | null>(null);
  let purgeAt = $state<string | null>(null);
  let msgs = $state<Msg[]>([]); // 正序（旧的在前）
  let next = $state<string | undefined>();
  let missing = $state(false);
  let to = $state("");
  let text = $state("");
  let attach = $state<Pending | null>(null);
  let sources = $state<Source[] | null>(null);
  let shorts = $state<Short[] | null>(null);
  let panel = $state<"" | "emoji" | "attach">("");
  let sending = $state(false);
  let scroller: HTMLDivElement | undefined = $state();
  let input: HTMLTextAreaElement | undefined = $state();

  // 常用 emoji：表情、手势、物品、符号（Unicode 字符，跟随系统字体显示）
  const EMOJI = [..."😀😄😂🤣😊😍🥰😘😎🤔😅😭😡🥳😴🤗🙏👍👎👌✌️🤝👏💪👋🫶❤️🔥✨🎉🎁🚀⚡🌏🌙☀️⭐💡🔒🔑📶📡🛰️💻📱🖥️⌛✅❌⚠️❓💯🆗🆕👀🍻☕🍰🐱🐶".matchAll(/\p{Extended_Pictographic}️?/gu)].map((m) => m[0]);

  async function loadThreads() {
    const [a, b] = await Promise.all([api.get<{ items: Thread[] }>("/api/v1/messages"), api.get<{ items: Deleted[] }>("/api/v1/messages/deleted")]);
    threads = a.items;
    deleted = b.items;
  }

  async function loadThread(reset = true) {
    if (!peer) return;
    try {
      const r = await api.get<{ user: { id: string; username: string }; items: Msg[]; next?: string; deletedAt?: string; purgeAt?: string }>(
        "/api/v1/messages/with/" + encodeURIComponent(peer),
        { query: { cursor: reset ? undefined : next, limit: 50 } },
      );
      missing = false;
      other = r.user;
      deletedAt = r.deletedAt ?? null;
      purgeAt = r.purgeAt ?? null;
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
    deletedAt = null;
    attach = null;
    loadThread().catch((e) => toasts.push("err", errorText(e)));
  });
  $effect(() => live(loadThreads, 30_000).stop);
  $effect(() => (peer ? live(() => loadThread(), 15_000).stop : undefined));

  function open(name: string) {
    to = "";
    chat.show(name);
  }

  // Esc 关闭悬浮窗（弹出的表情或附件面板先收起）
  function onWindowKey(e: KeyboardEvent) {
    if (e.key !== "Escape") return;
    if (panel) panel = "";
    else chat.close();
  }

  // 点击窗口外的页面时关闭（反馈 #80）。用 pointerdown 而不是 click：拖选文字时在窗口内按下、窗口外松开不会误关。
  // 例外：右上角消息图标自己负责开关（否则这里先关、按钮又打开）；提示框里的按钮（如「打开会话」）也不算窗口外。
  let win: HTMLDivElement | undefined = $state();
  // 用 composedPath() 判断：它在事件分发时就固定了。窗口内的元素（如联想列表的选项）可能在 pointerdown 里
  // 被立即移出 DOM，等事件冒泡到 window 时再用 contains() 就会误判为「窗口外」。
  function onWindowPointer(e: PointerEvent) {
    if (!win) return;
    const path = e.composedPath();
    if (path.includes(win)) return;
    if (path.some((n) => n instanceof Element && n.matches(".msg-btn, .toasts"))) return;
    chat.close();
  }

  async function send() {
    if (sending || !peer || (!text.trim() && !attach)) return;
    sending = true;
    try {
      const body: Record<string, string> = { to: peer, text };
      if (attach?.kind === "source") Object.assign(body, { source: attach.id, mode: attach.mode });
      if (attach?.kind === "short") Object.assign(body, { short: attach.id, mode: "grant" });
      await api.post("/api/v1/messages", body);
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
        const [ss, sl] = await Promise.all([listAll<Source>("/api/v1/sources"), api.get<{ items: Short[] }>("/api/v1/short-links")]);
        sources = ss.filter((s) => !(s as Source & { grant?: unknown }).grant); // 只能分享或授权自己的节点
        shorts = sl.items.filter((s) => !s.grantee); // 授权出去的短链接不能再转授
      } catch (e) {
        toasts.push("err", errorText(e));
      }
    }
  }

  function pick(p: Pending) {
    attach = p;
    panel = "";
    input?.focus();
  }

  async function accept(m: Msg) {
    try {
      const r = await api.post<{ kind: string; source: string; name: string }>("/api/v1/messages/" + m.id + "/accept", {});
      toasts.push("ok", r.kind === "template" ? t("messages.added_template", { name: r.name }) : t("messages.added", { name: r.name }));
      await loadThread();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function revoke(m: Msg) {
    if (!m.attach?.grantId) return;
    if (!confirm(m.mine ? t("messages.revoke_confirm", { name: m.attach.name }) : t("messages.remove_confirm", { name: m.attach.name }))) return;
    try {
      await api.post("/api/v1/grants/" + m.attach.grantId + "/revoke", {});
      toasts.push("ok", m.mine ? t("messages.revoked") : t("messages.removed"));
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

  async function removeThread(th: { username: string }) {
    if (!confirm(t("messages.delete_thread_confirm", { name: th.username }))) return;
    try {
      await api.del("/api/v1/messages/with/" + encodeURIComponent(th.username));
      toasts.push("ok", t("messages.thread_deleted", { name: th.username }));
      await loadThreads();
      if (peer.toLowerCase() === th.username.toLowerCase()) await loadThread();
      void inbox.refresh().catch(() => undefined);
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }

  async function restoreThread(name: string) {
    try {
      await api.post("/api/v1/messages/with/" + encodeURIComponent(name) + "/restore", {});
      toasts.push("ok", t("messages.thread_restored", { name }));
      await loadThreads();
      if (peer.toLowerCase() === name.toLowerCase()) await loadThread();
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
  const daysLeft = (iso: string) => Math.max(0, Math.ceil((new Date(iso).getTime() - Date.now()) / 86_400_000));
  // 同一天的消息之间不重复显示日期
  const dayOf = (iso: string) => i18n.date(iso, "long").split(" ")[0];
  const attachIcon = (a: Attach) => (a.kind === "template" ? "layers" : a.kind === "grant" ? (a.target === "short" ? "shrink" : "shield") : a.kind === "subscription" ? "link" : "server");
  function attachLine(a: Attach): string {
    if (a.kind === "subscription") return t("messages.attach_sub", { host: host(a.url) });
    if (a.kind === "nodes") return t("messages.attach_nodes", { n: a.nodes ?? 0 });
    if (a.kind === "template") return a.note ? t("messages.attach_template_note", { note: a.note }) : t("messages.attach_template");
    return a.target === "short" ? t("messages.grant_short") : t("messages.grant_source");
  }
</script>

<svelte:window onkeydown={onWindowKey} onpointerdown={onWindowPointer} />

<div class="window" bind:this={win} role="dialog" aria-modal="false" aria-labelledby="chat-title">
  <header class="win-head">
    <Icon name="message" size={16} />
    <h2 id="chat-title">{t("nav.messages")}</h2>
    {#if inbox.unread}<span class="badge">{inbox.unread}</span>{/if}
    <button class="btn ghost icon close" type="button" onclick={() => chat.close()} aria-label={t("common.close")} title={t("common.close")}><Icon name="x" size={16} /></button>
  </header>
<div class="page" class:has-peer={!!peer}>
  <aside class="threads panel" aria-label={t("messages.threads")}>
    <form class="start" onsubmit={(e) => { e.preventDefault(); if (to.trim()) open(to.trim()); }}>
      <UserPicker bind:value={to} onselect={open} placeholder={t("messages.to_placeholder")} />
      <button class="btn primary icon" type="submit" aria-label={t("messages.start")} title={t("messages.start")}><Icon name="plus" size={15} /></button>
    </form>
    {#if threads && !threads.length && !showBin}
      <p class="muted empty">{t("messages.no_threads")}</p>
    {/if}
    {#if !showBin}
      <ul class="list">
        {#each threads ?? [] as th (th.userId)}
          <li class="th">
            <a href={"/messages/" + encodeURIComponent(th.username)} onclick={(e) => { e.preventDefault(); open(th.username); }} class:active={th.username.toLowerCase() === peer.toLowerCase()} aria-current={th.username.toLowerCase() === peer.toLowerCase() ? "page" : undefined}>
              <span class="avatar" aria-hidden="true">{th.username.slice(0, 1).toUpperCase()}</span>
              <span class="th-main">
                <span class="th-top"><strong>{th.username}</strong><span class="muted when">{i18n.relative(th.lastAt)}</span></span>
                <span class="th-last muted">
                  {#if th.mine}{t("messages.you")}：{/if}{#if th.lastKind !== "text"}<Icon name={th.lastKind === "grant" ? "shield" : th.lastKind === "subscription" ? "link" : "server"} size={12} /> {/if}{th.last}
                </span>
              </span>
              {#if th.unread}<span class="badge">{th.unread}</span>{/if}
            </a>
            <button class="x" type="button" aria-label={t("messages.delete_thread", { name: th.username })} title={t("messages.delete_thread", { name: th.username })} onclick={() => void removeThread(th)}><Icon name="x" size={13} /></button>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="bin-hint muted">{t("messages.bin_hint")}</p>
      <ul class="list">
        {#each deleted as d (d.userId)}
          <li class="th bin">
            <span class="avatar muted-avatar" aria-hidden="true">{d.username.slice(0, 1).toUpperCase()}</span>
            <span class="th-main">
              <strong>{d.username}</strong>
              <span class="muted th-last">{t("messages.purge_in", { n: daysLeft(d.purgeAt) })}</span>
            </span>
            <button class="btn sm" type="button" onclick={() => void restoreThread(d.username)}>{t("messages.restore")}</button>
          </li>
        {:else}
          <li class="muted empty">{t("messages.bin_empty")}</li>
        {/each}
      </ul>
    {/if}
    <button class="bin-toggle" type="button" aria-pressed={showBin} onclick={() => (showBin = !showBin)}>
      <Icon name={showBin ? "message" : "trash"} size={13} />
      {showBin ? t("messages.back_to_threads") : t("messages.bin", { n: deleted.length })}
    </button>
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
        <button class="btn sm" type="button" onclick={() => (chat.peer = "")}>{t("messages.back_to_threads")}</button>
      </div>
    {:else}
      <header class="chat-head">
        <button class="back btn ghost icon" type="button" onclick={() => (chat.peer = "")} aria-label={t("messages.threads")}><Icon name="chevron-right" size={16} /></button>
        <span class="avatar" aria-hidden="true">{(other?.username ?? peer).slice(0, 1).toUpperCase()}</span>
        <strong class="peer">{other?.username ?? peer}</strong>
        {#if other && !deletedAt}
          <button class="btn sm ghost" type="button" onclick={() => void toggleBlock()}>{blocked ? t("messages.unblock") : t("messages.block")}</button>
          {#if threads?.some((x) => x.userId === other!.id)}
            <button class="btn sm ghost danger" type="button" onclick={() => void removeThread(other!)}><Icon name="trash" size={13} />{t("messages.delete_short")}</button>
          {/if}
        {/if}
      </header>

      {#if deletedAt}
        <div class="placeholder">
          <Icon name="trash" size={26} />
          <p>{t("messages.in_bin", { name: other?.username ?? peer })}</p>
          <p class="muted">{t("messages.purge_in", { n: daysLeft(purgeAt ?? deletedAt) })}</p>
          <button class="btn primary sm" type="button" onclick={() => void restoreThread(other?.username ?? peer)}>{t("messages.restore")}</button>
        </div>
      {:else}
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
                  <div class="card" class:revoked={m.attach.kind === "grant" && !m.attach.active}>
                    <Icon name={attachIcon(m.attach)} size={16} />
                    <div class="card-main">
                      <strong>{m.attach.name}</strong>
                      <span class="muted">
                        {attachLine(m.attach)}
                        {#if m.attach.kind === "grant"} · {m.attach.active ? t("messages.grant_active") : t("messages.grant_revoked")}{/if}
                      </span>
                    </div>
                    {#if m.attach.kind === "grant"}
                      {#if m.attach.active}
                        {#if !m.mine}
                          <a class="btn sm ghost" href={m.attach.target === "short" ? "/short-links" : "/sources"}><Icon name="check" size={13} />{m.attach.target === "short" ? t("messages.in_shorts") : t("messages.in_library")}</a>
                        {/if}
                        <button class="btn sm ghost" type="button" onclick={() => void revoke(m)}>{m.mine ? t("messages.revoke") : t("messages.remove")}</button>
                      {/if}
                    {:else if !m.mine}
                      {#if m.addedSource}
                        <a class="btn sm ghost" href={m.attach.kind === "template" ? "/" : "/sources/" + m.addedSource}><Icon name="check" size={13} />{m.attach.kind === "template" ? t("messages.in_templates") : t("messages.in_library")}</a>
                      {:else}
                        <button class="btn sm primary" type="button" onclick={() => void accept(m)}><Icon name="plus" size={13} />{m.attach.kind === "template" ? t("messages.add_template") : t("messages.add")}</button>
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
              <Icon name={attach.kind === "short" ? "shrink" : attach.mode === "grant" ? "shield" : attach.sourceKind === "subscription" ? "link" : "server"} size={13} />
              <span>{attach.kind === "short" ? t("messages.attaching_grant", { name: attach.name }) : attach.mode === "grant" ? t("messages.attaching_grant", { name: attach.name }) : t("messages.attaching", { name: attach.name })}</span>
              <button type="button" aria-label={t("common.remove")} onclick={() => (attach = null)}><Icon name="x" size={12} /></button>
            </div>
          {/if}
          {#if panel === "emoji"}
            <div class="pop emoji" role="listbox" aria-label={t("messages.emoji")}>
              {#each EMOJI as em (em)}<button type="button" role="option" aria-selected="false" onclick={() => insertEmoji(em)}>{em}</button>{/each}
            </div>
          {:else if panel === "attach"}
            <div class="pop sources" aria-label={t("messages.attach")}>
              {#if !sources}<p class="muted">{t("common.loading")}</p>{/if}
              {#if sources}
                <p class="pop-h">{t("nav.sources")}</p>
                {#each sources as s (s.id)}
                  <div class="pick">
                    <Icon name={s.kind === "subscription" ? "link" : "server"} size={14} />
                    <span class="s-name">{s.name}</span>
                    <button class="btn sm ghost" type="button" title={t("messages.share_hint")} onclick={() => pick({ kind: "source", mode: "share", id: s.id, name: s.name, sourceKind: s.kind })}>{t("messages.share")}</button>
                    <button class="btn sm ghost" type="button" title={t("messages.grant_hint")} onclick={() => pick({ kind: "source", mode: "grant", id: s.id, name: s.name, sourceKind: s.kind })}><Icon name="shield" size={12} />{t("messages.grant")}</button>
                  </div>
                {:else}
                  <p class="muted">{t("sources.empty")}</p>
                {/each}
              {/if}
              {#if shorts?.length}
                <p class="pop-h">{t("nav.short_links")}</p>
                {#each shorts as s (s.id)}
                  <div class="pick">
                    <Icon name="shrink" size={14} />
                    <span class="s-name mono">/{s.code}{#if s.note} <span class="muted">· {s.note}</span>{/if}</span>
                    <button class="btn sm ghost" type="button" title={t("messages.grant_short_hint")} onclick={() => pick({ kind: "short", id: s.id, name: s.note || s.code })}><Icon name="shield" size={12} />{t("messages.grant")}</button>
                  </div>
                {/each}
              {/if}
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
    {/if}
  </section>
</div>

</div>
<style>
  /* 悬浮窗：贴在右上角的消息图标下方；宽高随浏览器窗口变化，并留出边距 */
  .window {
    position: fixed;
    z-index: 60;
    inset-block-start: 60px;
    inset-inline-end: 16px;
    display: flex;
    flex-direction: column;
    inline-size: clamp(320px, 62vw, 880px);
    block-size: clamp(420px, calc(100dvh - 84px), 680px);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--bg);
    box-shadow: var(--shadow-lg, 0 16px 48px rgb(0 0 0 / 0.18));
    overflow: hidden;
    container-type: inline-size;
    animation: pop var(--dur-fast, 0.15s) var(--ease, ease-out);
  }
  @keyframes pop {
    from {
      opacity: 0;
      transform: translateY(-6px) scale(0.98);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .window {
      animation: none;
    }
  }
  .win-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: var(--space-2);
    padding-inline: var(--space-4) var(--space-2);
    border-block-end: 1px solid var(--border);
    background: var(--bg-raised);
  }
  .win-head h2 {
    font-size: var(--text-md);
    font-weight: 600;
  }
  .win-head .close {
    margin-inline-start: auto;
  }
  .page {
    flex: 1;
    min-block-size: 0;
    padding: var(--space-3);
    display: grid;
    grid-template-columns: 260px minmax(0, 1fr);
    gap: var(--space-3);
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
  .list {
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
  .s-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  @container (max-width: 640px) {
    .page {
      grid-template-columns: minmax(0, 1fr);
      padding: var(--space-2);
    }
    .page.has-peer .threads,
    .page:not(.has-peer) .chat {
      display: none;
    }
    .back {
      display: inline-flex;
    }
  }
  @media (max-width: 640px) {
    .window {
      inset: 0;
      inline-size: auto;
      block-size: auto;
      border: 0;
      border-radius: 0;
    }
  }
  /* 会话列表：每行右侧的 × 删除（悬停或聚焦时出现） */
  .list {
    flex: 1;
  }
  .th {
    position: relative;
    display: flex;
    align-items: center;
  }
  .threads .th > a {
    flex: 1;
    min-inline-size: 0;
    padding-inline-end: 32px;
  }
  .x {
    position: absolute;
    inset-inline-end: 6px;
    display: inline-grid;
    place-items: center;
    inline-size: 24px;
    block-size: 24px;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--fg-subtle);
    cursor: pointer;
    opacity: 0;
  }
  .th:hover .x,
  .x:focus-visible {
    opacity: 1;
  }
  .x:hover {
    background: var(--bg-hover);
    color: var(--err);
  }
  @media (hover: none) {
    .x {
      opacity: 1;
    }
  }
  .th.bin {
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
  }
  .muted-avatar {
    background: var(--bg-hover);
    color: var(--fg-muted);
  }
  .bin-hint {
    padding: var(--space-3) var(--space-3) 0;
    font-size: var(--text-xs);
  }
  .bin-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4);
    border: 0;
    border-block-start: 1px solid var(--border);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .bin-toggle:hover {
    color: var(--fg);
  }
  /* 附件选择：每个节点可以分享或授权，短链接只能授权 */
  .pop-h {
    padding: var(--space-1) var(--space-1) 2px;
    color: var(--fg-muted);
    font-size: var(--text-xs);
    font-weight: 600;
  }
  .pick {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 3px var(--space-1);
    border-radius: var(--radius-md);
  }
  .pick:hover {
    background: var(--bg-hover);
  }
  .card.revoked {
    opacity: 0.6;
  }
</style>
