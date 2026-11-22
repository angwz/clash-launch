<script lang="ts">
  // 站内公告的管理（需求 R32）：发布、修改、删除。起止时间按浏览器所在时区输入；结束时间留空表示一直显示到删除。
  // 发布后在线用户即时看到；修改不会让已关闭的用户再次看到。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Announcement {
    id: string;
    text: string;
    level: "info" | "warn";
    startsAt: string;
    endsAt?: string;
    createdBy?: string;
    createdAt: string;
  }
  let items = $state<Announcement[] | null>(null);
  let editing = $state<string | null>(null); // 正在修改的公告 ID；null 为发布新公告
  let text = $state("");
  let level = $state<"info" | "warn">("info");
  let starts = $state(""); // datetime-local
  let ends = $state("");
  let busy = $state(false);

  async function load() {
    try {
      items = (await api.get<{ items: Announcement[] }>("/api/v1/admin/announcements")).items;
      now = Date.now();
    } catch (e) {
      toasts.push("err", errorText(e));
    }
  }
  $effect(() => void load());

  // datetime-local 没有时区：按浏览器时区解释，与输入框显示的一致
  const toLocal = (iso?: string) => {
    if (!iso) return "";
    const d = new Date(iso);
    const p = (n: number) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
  };
  const fromLocal = (v: string) => (v ? new Date(v).toISOString() : undefined);

  function edit(a: Announcement) {
    editing = a.id;
    text = a.text;
    level = a.level;
    starts = toLocal(a.startsAt);
    ends = toLocal(a.endsAt);
  }
  function reset() {
    editing = null;
    text = "";
    level = "info";
    starts = "";
    ends = "";
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    const body = { text, level, startsAt: fromLocal(starts), endsAt: fromLocal(ends) };
    try {
      if (editing) await api.put("/api/v1/admin/announcements/" + editing, body);
      else await api.post("/api/v1/admin/announcements", body);
      toasts.push("ok", editing ? t("common.saved") : t("announce.published"));
      reset();
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    } finally {
      busy = false;
    }
  }

  async function remove(a: Announcement) {
    if (!confirm(t("announce.delete_confirm"))) return;
    try {
      await api.del("/api/v1/admin/announcements/" + a.id);
      if (editing === a.id) reset();
      await load();
    } catch (err) {
      toasts.push("err", errorText(err));
    }
  }

  // 判断状态用的「现在」：每次读取列表时更新（刚发布的公告不会显示为未开始）
  let now = $state(Date.now());
  function phase(a: Announcement): "scheduled" | "active" | "ended" {
    if (new Date(a.startsAt).getTime() > now) return "scheduled";
    if (a.endsAt && new Date(a.endsAt).getTime() <= now) return "ended";
    return "active";
  }
</script>

<section class="panel box" aria-labelledby="a-ann">
  <h2 id="a-ann">{t("announce.title")}</h2>
  <p class="muted">{t("announce.desc")}</p>
  <form class="form" onsubmit={submit}>
    <div class="wide">
      <label class="label" for="ann-text">{t("announce.text")}</label>
      <textarea id="ann-text" class="input" rows="2" maxlength="500" bind:value={text} required></textarea>
    </div>
    <div>
      <label class="label" for="ann-level">{t("announce.level")}</label>
      <select id="ann-level" class="select" bind:value={level}>
        <option value="info">{t("announce.level_info")}</option>
        <option value="warn">{t("announce.level_warn")}</option>
      </select>
    </div>
    <div>
      <label class="label" for="ann-start">{t("announce.starts")}</label>
      <input id="ann-start" class="input" type="datetime-local" bind:value={starts} />
    </div>
    <div>
      <label class="label" for="ann-end">{t("announce.ends")}</label>
      <input id="ann-end" class="input" type="datetime-local" bind:value={ends} />
    </div>
    <p class="hint wide">{t("announce.time_hint")}</p>
    <div class="actions wide">
      {#if editing}<button class="btn ghost" type="button" onclick={reset}>{t("common.cancel")}</button>{/if}
      <button class="btn primary" type="submit" disabled={busy || !text.trim()}>{editing ? t("common.save") : t("announce.publish")}</button>
    </div>
  </form>

  {#if items && items.length}
    <ul class="list">
      {#each items as a (a.id)}
        <li class:editing={editing === a.id}>
          <div class="meta">
            <span class="tag" data-state={phase(a)}>{t("announce.state_" + phase(a))}</span>
            {#if a.level === "warn"}<span class="tag warn">{t("announce.level_warn")}</span>{/if}
            <span class="muted">{i18n.date(a.startsAt)}{#if a.endsAt} – {i18n.date(a.endsAt)}{/if}{#if a.createdBy} · {a.createdBy}{/if}</span>
          </div>
          <p class="text">{a.text}</p>
          <div class="row-actions">
            <button class="btn sm" type="button" onclick={() => edit(a)}>{t("common.edit")}</button>
            <button class="btn sm ghost danger" type="button" onclick={() => void remove(a)}>{t("common.delete")}</button>
          </div>
        </li>
      {/each}
    </ul>
  {:else if items}
    <p class="muted">{t("announce.empty")}</p>
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  .form {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-3);
  }
  .wide {
    grid-column: 1 / -1;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .list {
    display: grid;
    gap: var(--space-2);
  }
  .list li {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }
  .list li.editing {
    border-color: var(--accent);
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }
  .tag {
    padding-inline: 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-active);
    font-size: var(--text-xs);
  }
  .tag[data-state="active"] {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .tag.warn {
    background: var(--warn-soft);
    color: var(--warn);
  }
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .row-actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
