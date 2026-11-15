<script lang="ts" module>
  // 常用时区排在前面（反馈 #73），其余按字母。排序综合两点：
  //   1. 需要代理工具的用户集中在哪里：中国大陆远多于其他地区，其次是港澳台、新加坡等华人聚居地，
  //      以及网络管控较多的俄罗斯、伊朗、土耳其、越南等；
  //   2. 国家体量与网络用户规模：美、日、英、德、韩、印等。
  // 每个国家只列一个代表时区（美国按东、西、中部人口列三个）。这是编辑判断，不是统计数据。
  // 全部使用 Go 与浏览器都接受的 IANA 名称；浏览器列表里的旧别名（如 Asia/Calcutta）合并到这里，不重复显示。
  const COMMON = [
    "Asia/Shanghai", "Asia/Hong_Kong", "Asia/Taipei", "Asia/Macau", "Asia/Singapore", "Asia/Tokyo",
    "America/New_York", "America/Los_Angeles", "Europe/London", "Asia/Seoul", "Europe/Moscow", "Europe/Berlin",
    "Asia/Tehran", "Europe/Istanbul", "Asia/Kuala_Lumpur", "Australia/Sydney", "America/Chicago", "Asia/Dubai",
    "Europe/Paris", "America/Toronto", "Asia/Kolkata", "Asia/Ho_Chi_Minh", "Asia/Bangkok", "Asia/Jakarta",
    "Asia/Manila", "Europe/Amsterdam", "Asia/Riyadh", "Asia/Karachi", "America/Sao_Paulo", "UTC",
  ];
  const ALIASES = new Set(["Asia/Calcutta", "Asia/Saigon", "Asia/Macao", "Asia/Istanbul", "Etc/UTC", "Etc/GMT", "GMT", "UCT", "Universal", "Zulu", "PRC", "ROC", "ROK", "Japan", "Hongkong", "Singapore", "Turkey", "Iran", "W-SU"]);
  let zonesCache: string[] | null = null;
  /** 常用时区在前，其余为浏览器支持的 IANA 时区按字母排列。只在第一次打开时计算。 */
  export function allZones(): string[] {
    if (zonesCache) return zonesCache;
    let list: string[] = [];
    try {
      list = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.("timeZone") ?? [];
    } catch {
      list = [];
    }
    const common = new Set(COMMON);
    zonesCache = [...COMMON, ...list.filter((z) => !common.has(z) && !ALIASES.has(z)).sort()];
    return zonesCache;
  }
  /** 常用时区的个数：列表在这之后显示分隔 */
  export const commonCount = COMMON.length;

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
    if (!q) return [FOLLOW, ...zones]; // 不输入时列出全部：常用在前，之后按字母
    const words = q.split(" ");
    const scored: { z: string; s: number; i: number }[] = [];
    for (const [i, z] of zones.entries()) {
      const n = norm(z);
      const off = norm(offsetOf(z));
      if (!words.every((w) => n.includes(w) || off.includes(w))) continue;
      // 排序：整体以输入开头 > 城市名以输入开头 > 其他
      const city = n.slice(n.lastIndexOf(" ") + 1);
      scored.push({ z, i, s: n.startsWith(q) ? 0 : city.startsWith(q) ? 1 : 2 });
    }
    // 同一匹配档内按列表顺序：常用时区在前
    scored.sort((a, b) => a.s - b.s || a.i - b.i);
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
          class:others={!typed && i === commonCount + 1}
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
  /* 常用时区与其余时区之间的分隔 */
  li.others {
    margin-block-start: 4px;
    border-block-start: 1px solid var(--border);
    border-start-start-radius: 0;
    border-start-end-radius: 0;
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
