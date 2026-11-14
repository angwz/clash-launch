<script lang="ts">
  // 调整菜单顺序（反馈 #37）：主菜单与内核设置分节各一列，用上移 / 下移按钮排序（键盘可用），
  // 保存到用户偏好；「恢复默认」清空对应列表。
  import Dialog from "./Dialog.svelte";
  import Icon from "./Icon.svelte";
  import { errorText } from "../lib/errors";
  import { t } from "../lib/i18n/i18n.svelte";
  import { prefs } from "../lib/prefs.svelte";
  import { toasts } from "../lib/toast.svelte";

  interface Item {
    id: string;
    label: string;
  }

  let {
    open = $bindable(false),
    nav,
    sections,
  }: { open?: boolean; nav: Item[]; sections: Item[] } = $props();

  // 打开时从当前（已排序的）菜单复制一份草稿
  let lists = $state<{ nav: Item[]; sections: Item[] }>({ nav: [], sections: [] });
  let saving = $state(false);
  $effect(() => {
    if (open) lists = { nav: [...nav], sections: [...sections] };
  });

  function move(which: "nav" | "sections", i: number, d: -1 | 1) {
    const l = [...lists[which]];
    const j = i + d;
    if (j < 0 || j >= l.length) return;
    [l[i], l[j]] = [l[j]!, l[i]!];
    lists[which] = l;
    // 焦点跟随被移动的项，便于连续按键
    queueMicrotask(() => document.getElementById(`ord-${which}-${j}-${d < 0 ? "up" : "down"}`)?.focus());
  }

  async function save(reset = false) {
    saving = true;
    try {
      await prefs.save(reset ? {} : { nav: lists.nav.map((x) => x.id), sections: lists.sections.map((x) => x.id) });
      toasts.push("ok", reset ? t("order.reset_done") : t("order.saved"));
      open = false;
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      saving = false;
    }
  }
</script>

<Dialog bind:open title={t("order.title")} wide>
  <p class="muted">{t("order.desc")}</p>
  <div class="cols">
    {#each [["nav", t("order.nav")], ["sections", t("order.sections")]] as const as [which, title] (which)}
      <section aria-label={title}>
        <h3>{title}</h3>
        <ol>
          {#each lists[which] as x, i (x.id)}
            <li>
              <span class="n mono">{i + 1}</span>
              <span class="label">{x.label}</span>
              <button id={`ord-${which}-${i}-up`} class="btn ghost icon sm" type="button" aria-label={t("editor.move_up") + " " + x.label} disabled={i === 0} onclick={() => move(which, i, -1)}><Icon name="arrow-up" size={14} /></button>
              <button id={`ord-${which}-${i}-down`} class="btn ghost icon sm" type="button" aria-label={t("editor.move_down") + " " + x.label} disabled={i === lists[which].length - 1} onclick={() => move(which, i, 1)}><Icon name="arrow-down" size={14} /></button>
            </li>
          {/each}
        </ol>
      </section>
    {/each}
  </div>
  {#snippet footer()}
    <button class="btn ghost" type="button" disabled={saving} onclick={() => void save(true)}>{t("order.reset")}</button>
    <span class="spacer"></span>
    <button class="btn" type="button" onclick={() => (open = false)}>{t("common.cancel")}</button>
    <button class="btn primary" type="button" disabled={saving} onclick={() => void save()}>{t("common.save")}</button>
  {/snippet}
</Dialog>

<style>
  .cols {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-5);
    margin-block-start: var(--space-3);
  }
  h3 {
    margin-block-end: var(--space-2);
    font-size: var(--text-sm);
    color: var(--fg-muted);
  }
  ol {
    display: grid;
    gap: 2px;
    list-style: none;
    padding: 0;
    margin: 0;
  }
  li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding-block: 2px;
    padding-inline: var(--space-2);
    border-radius: var(--radius-md);
  }
  li:hover {
    background: var(--bg-hover);
  }
  .n {
    inline-size: 1.5em;
    color: var(--fg-subtle);
    font-size: var(--text-xs);
  }
  .label {
    flex: 1;
    min-inline-size: 0;
  }
  .spacer {
    flex: 1;
  }
</style>
