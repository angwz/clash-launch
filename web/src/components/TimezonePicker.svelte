<script lang="ts" module>
  // 浏览器支持的 IANA 时区（不含 UTC 时补上）。只在第一次打开时计算。
  let zonesCache: string[] | null = null;
  export function allZones(): string[] {
    if (zonesCache) return zonesCache;
    let list: string[] = [];
    try {
      list = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.("timeZone") ?? [];
    } catch {
      list = [];
    }
    zonesCache = ["UTC", ...list.filter((z) => z !== "UTC")];
    return zonesCache;
  }

  /** 浏览器当前所在的时区 */
  export function browserZone(): string {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
    } catch {
      return "UTC";
    }
  }

  const offsets = new Map<string, string>();
  /** 时区当前的 UTC 偏移，例如 UTC+08:00 */
  export function offsetOf(zone: string): string {
    let o = offsets.get(zone);
    if (o === undefined) {
      try {
        const p = new Intl.DateTimeFormat("en-US", { timeZone: zone, timeZoneName: "longOffset" }).formatToParts(new Date());
        const v = p.find((x) => x.type === "timeZoneName")?.value ?? "GMT";
        o = v === "GMT" ? "UTC±00:00" : v.replace("GMT", "UTC");
      } catch {
        o = "";
      }
      offsets.set(zone, o);
    }
    return o;
  }

  // 匹配用的规范形式：小写，「/」「_」「-」与空白都视为分隔
  const norm = (s: string) => s.toLowerCase().replace(/[\s/_-]+/g, " ").trim();
</script>

<script lang="ts">
  // 时区选择（反馈 #72）：输入时不区分大小写地匹配时区名的任意部分（城市、地区）或 UTC 偏移，
  // 并把第一个以输入开头的结果自动补全在输入框里（补全部分处于选中状态，继续输入即覆盖）；
  // 回车或点击确认，Esc 放弃。空值表示跟随浏览器。
  import { t } from "../lib/i18n/i18n.svelte";

  let { value, id, onchange }: { value: string; id: string; onchange: (tz: string) => void } = $props();

  let text = $state("");
  let open = $state(false);
  let active = $state(0);
  let input: HTMLInputElement | undefined = $state();
  let typed = $state(""); // 用户实际输入的部分（不含自动补全）

  $effect(() => {
    text = value;
  });

  const FOLLOW = "";
  const results = $derived.by(() => {
    const q = norm(typed);
    const zones = allZones();
    if (!q) return [FOLLOW, ...zones].slice(0, 60);
    const words = q.split(" ");
    const scored: { z: string; s: number }[] = [];
    for (const z of zones) {
      const n = norm(z);
      const off = norm(offsetOf(z));
      if (!words.every((w) => n.includes(w) || off.includes(w))) continue;
      // 排序：整体以输入开头 > 城市名以输入开头 > 其他
      const city = n.slice(n.lastIndexOf(" ") + 1);
      scored.push({ z, s: n.startsWith(q) ? 0 : city.startsWith(q) ? 1 : 2 });
    }
    scored.sort((a, b) => a.s - b.s || a.z.localeCompare(b.z));
    return scored.slice(0, 60).map((x) => x.z);
  });

  function label(z: string): string {
    return z === FOLLOW ? t("settings.tz_follow", { zone: browserZone() }) : z.replaceAll("_", " ");
  }

  function oninput(e: Event & { currentTarget: HTMLInputElement }) {
    const el = e.currentTarget;
    typed = el.value;
    text = el.value;
    open = true;
    active = 0;
    // 只在追加输入、光标在末尾时补全；删除时不补，避免删不掉
    if (!(e as unknown as InputEvent).inputType?.startsWith("insert") || el.selectionStart !== el.value.length || !typed) return;
    const hit = allZones().find((z) => z.toLowerCase().startsWith(typed.toLowerCase()));
    if (hit) {
      text = hit;
      el.value = hit;
      el.setSelectionRange(typed.length, hit.length);
    }
  }

  function commit(z: string) {
    open = false;
    typed = "";
    text = z;
    if (z !== value) onchange(z);
  }

  function exact(s: string): string | undefined {
    const k = s.trim().toLowerCase();
    if (!k) return FOLLOW;
    return allZones().find((z) => z.toLowerCase() === k);
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!open) open = true;
      const n = results.length;
      if (n) active = (active + (e.key === "ArrowDown" ? 1 : n - 1)) % n;
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      const z = open && results[active] !== undefined ? results[active]! : exact(text);
      if (z !== undefined) commit(z);
      return;
    }
    if (e.key === "Escape" && open) {
      e.preventDefault();
      open = false;
      typed = "";
      text = value;
    }
  }

  // 打开列表：从全部时区开始，高亮当前值，选中输入框文字以便直接改写
  function reopen() {
    typed = "";
    open = true;
    active = Math.max(0, results.indexOf(value));
    input?.select();
  }

  function onblur() {
    // 输入恰好是某个时区（不区分大小写）时采用它的规范写法，否则恢复原值
    setTimeout(() => {
      if (!open) return;
      const z = exact(text);
      if (z !== undefined) commit(z);
      else {
        open = false;
        typed = "";
        text = value;
      }
    }, 120);
  }
</script>

<div class="tz">
  <input
    bind:this={input}
    {id}
    class="input ltr"
    role="combobox"
    aria-expanded={open}
    aria-controls={id + "-list"}
    aria-autocomplete="both"
    aria-activedescendant={open && results.length ? `${id}-o${active}` : undefined}
    autocomplete="off"
    spellcheck="false"
    value={text}
    placeholder={t("settings.tz_follow", { zone: browserZone() })}
    {oninput}
    {onkeydown}
    {onblur}
    onfocus={reopen}
    onclick={() => {
      if (!open) reopen(); // 选完后焦点仍在输入框里，再次点击时重新打开列表
    }}
  />
  {#if open}
    <ul id={id + "-list"} class="list" role="listbox">
      {#each results as z, i (z)}
        <li
          id={`${id}-o${i}`}
          role="option"
          aria-selected={i === active}
          class:active={i === active}
          onpointerdown={(e) => {
            e.preventDefault(); // 不让输入框失焦，点击即确认
            commit(z);
          }}
          onpointerenter={() => (active = i)}
        >
          <span class="name">{label(z)}</span>
          <span class="off">{offsetOf(z === FOLLOW ? browserZone() : z)}</span>
        </li>
      {:else}
        <li class="none muted" role="presentation">{t("settings.tz_none")}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .tz {
    position: relative;
  }
  .list {
    position: absolute;
    inset-inline: 0;
    inset-block-start: calc(100% + 4px);
    z-index: 20;
    max-block-size: 280px;
    overflow-y: auto;
    margin: 0;
    padding: 4px;
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    box-shadow: var(--shadow-md);
  }
  li {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    padding-block: 6px;
    padding-inline: var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  li.active {
    background: var(--accent-soft);
  }
  .off {
    color: var(--fg-muted);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .none {
    cursor: default;
  }
</style>
