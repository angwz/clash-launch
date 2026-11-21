<script lang="ts">
  // 订阅分析（反馈 #74）：自己的配置链接与短链接被谁、从哪里、什么时候、多频繁地请求。
  // 筛选条件写在地址栏（?days=&link=&short=），可以从短链接页直接跳到某个短链接的分析。
  // 时间按「设置 → 时区」显示与分桶；没有设置时用浏览器时区。页面可见时每分钟自动刷新。
  import Icon from "../components/Icon.svelte";
  import PageHeader from "../components/PageHeader.svelte";
  import DayBars from "../components/stats/DayBars.svelte";
  import HeatGrid from "../components/stats/HeatGrid.svelte";
  import RankBars from "../components/stats/RankBars.svelte";
  import { api } from "../lib/api/client";
  import { errorText } from "../lib/errors";
  import { i18n, t } from "../lib/i18n/i18n.svelte";
  import { live } from "../lib/live";
  import { flagOf, isLocalIP, regionName } from "../lib/regions";
  import { browserZone } from "../components/TimezonePicker.svelte";

  interface Count {
    key: string;
    label?: string;
    count: number;
  }
  interface IPRow {
    ip: string;
    country: string;
    asn: string;
    org: string;
    client: string;
    count: number;
    ok: number;
    fail: number;
    first: string;
    last: string;
    interval: number;
    new: boolean;
  }
  interface Req {
    at: string;
    ip: string;
    country: string;
    org: string;
    client: string;
    ua: string;
    status: number;
    kind: string;
    linkId: string;
    shortId?: string;
  }
  // 好友经由授权短链接的使用情况（反馈 #75）
  interface Friend {
    userId: string;
    requests: number;
    ok: number;
    fail: number;
    ips: number;
    countries: string[];
    orgs: string[];
    clients: string[];
    shorts: string[];
    daily: number[];
    first: string;
    last: string;
    interval: number;
  }
  interface Data {
    analytics: {
      zone: string;
      totals: { requests: number; ok: number; fail: number; ips: number; countries: number; clients: number };
      days: string[];
      daily: number[];
      heat: number[][];
      countries: Count[];
      orgs: Count[];
      clients: Count[];
      kinds: Count[];
      links: Count[];
      shorts: Count[];
      ips: IPRow[];
      recent: Req[];
      newIps: number;
      peak?: { day: string; count: number };
      friends: Friend[];
    };
    friends: Record<string, string>;
    links: Record<string, { profileId: string; profileName: string; note: string }>;
    shorts: Record<string, string>;
    retentionDays?: number; // 请求明细保留天数（R14c）；0 或缺省表示不清理
  }

  const q = new URLSearchParams(location.search);
  let days = $state(Number(q.get("days")) || 30);
  let link = $state(q.get("link") ?? "");
  let short = $state(q.get("short") ?? "");
  let friend = $state(q.get("friend") ?? "");
  let data = $state<Data | null>(null);
  const retention = $derived(data?.retentionDays ?? 0);
  let error = $state("");
  let ipLimit = $state(20);

  // 实时请求流（R22）：订阅被拉取时页面经 sub 事件重读；与上一次结果相比新出现的请求在列表顶端淡入，
  // 总数变化时数字滚动一下。只在筛选条件不变时比较，切换筛选不算「新请求」。
  let seen = new Set<string>();
  let lastFilter = "";
  let lastTotal = -1;
  let fresh = $state(new Set<string>());
  let rolled = $state(false);
  // 同一毫秒、同一 IP、同一状态的请求可能不止一条：加序号保证键唯一（keyed each 要求）
  function requestKeys(rs: Req[]): string[] {
    const n = new Map<string, number>();
    return rs.map((r) => {
      const k = r.at + "|" + r.ip + "|" + r.status;
      const i = n.get(k) ?? 0;
      n.set(k, i + 1);
      return k + "|" + i;
    });
  }

  async function load() {
    const d = await api.get<Data>("/api/v1/analytics", { query: { days, link: link || undefined, short: short || undefined, friend: friend || undefined, tz: browserZone() } });
    const filter = JSON.stringify([days, link, short, friend]);
    const keys = requestKeys(d.analytics.recent);
    const same = filter === lastFilter && lastTotal >= 0;
    fresh = same ? new Set(keys.filter((k) => !seen.has(k))) : new Set();
    rolled = same && d.analytics.totals.requests > lastTotal;
    seen = new Set(keys);
    lastFilter = filter;
    lastTotal = d.analytics.totals.requests;
    data = d;
    error = "";
  }
  $effect(() => {
    // 筛选变化：更新地址栏（不新增历史记录）并重新载入
    const p = new URLSearchParams();
    if (days !== 30) p.set("days", String(days));
    if (link) p.set("link", link);
    if (short) p.set("short", short);
    if (friend) p.set("friend", friend);
    history.replaceState(history.state, "", location.pathname + (p.size ? "?" + p : ""));
    load().catch((e) => (error = errorText(e)));
  });
  $effect(() => live(load, 60_000, ["sub"]).stop);

  const a = $derived(data?.analytics);
  const dayInts = $derived((a?.days ?? []).map((d) => Number(d.replaceAll("-", ""))));
  const okRate = $derived(a && a.totals.requests ? Math.round((a.totals.ok / a.totals.requests) * 100) : 0);
  const linkLabel = (id: string) => {
    const l = data?.links[id];
    return l ? `${l.profileName} · ${l.note || t("links.untitled")}` : t("analytics.deleted_link");
  };
  // 归属地：有国家时显示旗帜与名称；本机与内网地址显示「本地网络」；其余（数据库里没有这个地址）留空（反馈 #75）
  const region = (c: string, ip = "") => (c ? `${flagOf(c)} ${regionName(c, i18n.intlLocale)}` : isLocalIP(ip) ? t("analytics.local") : "");
  const kindLabel = (k: string) => t("analytics.kind." + (["config", "provider", "export", "short", "import"].includes(k) ? k : "config"));

  // 拉取间隔：约每 N 分钟 / 小时 / 天
  function every(sec: number): string {
    if (!sec) return "—";
    if (sec < 3600) return t("analytics.every_min", { n: Math.max(1, Math.round(sec / 60)) });
    if (sec < 86400) return t("analytics.every_hour", { n: Math.round(sec / 360) / 10 });
    return t("analytics.every_day", { n: Math.round(sec / 8640) / 10 });
  }
  const statusTone = (s: number) => (s < 300 ? "ok" : s === 304 ? "same" : s === 429 ? "warn" : "err");
</script>

<div class="page">
  <PageHeader title={t("nav.analytics")} subtitle={t("analytics.subtitle")} />

  <div class="filters" role="group" aria-label={t("analytics.filters")}>
    <div class="seg" role="radiogroup" aria-label={t("analytics.range")}>
      {#each [7, 30, 90, 365] as d (d)}
        <button type="button" role="radio" aria-checked={days === d} class:on={days === d} onclick={() => (days = d)}>{t("analytics.range_" + d)}</button>
      {/each}
    </div>
    <select class="select" aria-label={t("analytics.link")} bind:value={link}>
      <option value="">{t("analytics.all_links")}</option>
      {#each Object.entries(data?.links ?? {}) as [id] (id)}<option value={id}>{linkLabel(id)}</option>{/each}
    </select>
    <select class="select" aria-label={t("analytics.short")} bind:value={short}>
      <option value="">{t("analytics.all_shorts")}</option>
      {#each Object.entries(data?.shorts ?? {}) as [id, code] (id)}<option value={id}>/{code}</option>{/each}
    </select>
    {#if Object.keys(data?.friends ?? {}).length}
      <select class="select" aria-label={t("analytics.friend")} bind:value={friend}>
        <option value="">{t("analytics.all_people")}</option>
        {#each Object.entries(data?.friends ?? {}) as [id, name] (id)}<option value={id}>{t("analytics.friend_only", { name })}</option>{/each}
      </select>
    {/if}
    {#if a}<span class="zone muted"><Icon name="globe" size={13} />{a.zone}</span>{/if}
  </div>

  {#if error}
    <p class="field-error" role="alert">{error}</p>
  {:else if !a}
    <p class="muted">{t("common.loading")}</p>
  {:else if !a.totals.requests}
    <div class="empty panel">
      <p>{t("analytics.empty")}</p>
      <p class="muted">{t("analytics.empty_hint")}</p>
    </div>
  {:else}
    {#if retention > 0 && days > retention}<p class="hint warn-note">{t("analytics.retention", { n: retention })}</p>{/if}

    <dl class="tiles">
      <div class="tile panel">
        <dt>{t("analytics.requests")}</dt>
        <dd>{#key a.totals.requests}<strong class:roll={rolled}>{i18n.number(a.totals.requests)}</strong>{/key}{#if a.peak}<span class="muted">{t("analytics.peak", { day: a.peak.day.slice(5), n: a.peak.count })}</span>{/if}</dd>
      </div>
      <div class="tile panel">
        <dt>{t("analytics.success")}</dt>
        <dd><strong>{okRate}%</strong><span class="muted">{t("analytics.failed_n", { n: a.totals.fail })}</span></dd>
      </div>
      <div class="tile panel">
        <dt>{t("analytics.ips")}</dt>
        <dd><strong>{i18n.number(a.totals.ips)}</strong><span class="muted">{t("analytics.countries_n", { n: a.totals.countries })}</span></dd>
      </div>
      <div class="tile panel" class:alert={a.newIps > 0}>
        <dt>{t("analytics.new_ips")}</dt>
        <dd><strong>{a.newIps}</strong><span class="muted">{t("analytics.new_ips_hint")}</span></dd>
      </div>
    </dl>

    <section class="panel box">
      <h2>{t("analytics.daily")}</h2>
      <DayBars days={dayInts} values={a.daily} label={t("analytics.daily")} />
    </section>

    <section class="panel box">
      <h2>{t("analytics.when")}</h2>
      <p class="hint">{t("analytics.when_hint")}</p>
      <HeatGrid heat={a.heat} />
    </section>

    <div class="ranks">
      <section class="panel box">
        <h2>{t("analytics.regions")}</h2>
        <RankBars items={a.countries.map((x) => ({ label: region(x.key), count: x.count }))} empty={t("analytics.no_geo")} />
      </section>
      <section class="panel box">
        <h2>{t("analytics.orgs")}</h2>
        <RankBars items={a.orgs.map((x) => ({ label: `${x.label || "?"} · ${x.key}`, count: x.count }))} empty={t("analytics.no_geo")} />
      </section>
      <section class="panel box">
        <h2>{t("stats.top_clients")}</h2>
        <RankBars items={a.clients.map((x) => ({ label: x.key, count: x.count, mono: true }))} empty={t("stats.none")} />
      </section>
      <section class="panel box">
        <h2>{t("analytics.via")}</h2>
        <RankBars
          items={[
            ...a.kinds.map((x) => ({ label: kindLabel(x.key), count: x.count })),
          ]}
          empty={t("stats.none")}
        />
        {#if a.shorts.length}
          <h3>{t("nav.short_links")}</h3>
          <RankBars items={a.shorts.map((x) => ({ label: "/" + (data?.shorts[x.key] ?? "?"), count: x.count, mono: true }))} empty={t("stats.none")} />
        {/if}
      </section>
    </div>

    {#if a.friends.length}
      <!-- 好友的使用情况：经由授权给好友的短链接的请求，逐项列出（反馈 #75） -->
      <section class="panel box">
        <h2>{t("analytics.friends")}</h2>
        <p class="hint">{t("analytics.friends_hint")}</p>
        <div class="wrap">
          <table class="table flush">
            <thead>
              <tr><th>{t("analytics.friend")}</th><th>{t("stats.col_count")}</th><th>IP</th><th>{t("analytics.region")}</th><th>{t("analytics.org")}</th><th>{t("stats.col_client")}</th><th>{t("analytics.interval")}</th><th>{t("stats.col_first")}</th><th>{t("stats.col_last")}</th><th>{t("nav.short_links")}</th><th></th></tr>
            </thead>
            <tbody>
              {#each a.friends as f (f.userId)}
                <tr>
                  <td><strong>{data?.friends[f.userId] ?? "?"}</strong></td>
                  <td class="num">{i18n.number(f.requests)}{#if f.fail} <span class="bad">({t("analytics.fail_n", { n: f.fail })})</span>{/if}</td>
                  <td class="num">{f.ips}</td>
                  <td>{f.countries.map((c) => flagOf(c) + " " + regionName(c, i18n.intlLocale)).join("、") || ""}</td>
                  <td class="org" title={f.orgs.join(" · ")}>{f.orgs.join("、") || ""}</td>
                  <td class="mono">{f.clients.join(", ") || "—"}</td>
                  <td>{every(f.interval)}</td>
                  <td>{i18n.date(f.first)}</td>
                  <td>{i18n.relative(f.last)}</td>
                  <td class="mono">{f.shorts.map((id) => "/" + (data?.shorts[id] ?? "?")).join(" ")}</td>
                  <td><button class="btn sm ghost" type="button" onclick={() => (friend = f.userId)}>{t("analytics.only_this")}</button></td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </section>
    {/if}

    <section class="panel box">
      <h2>{t("analytics.ip_detail")}</h2>
      <p class="hint">{t("analytics.ip_hint")}</p>
      <div class="wrap">
        <table class="table flush">
          <thead>
            <tr><th>IP</th><th>{t("analytics.region")}</th><th>{t("analytics.org")}</th><th>{t("stats.col_client")}</th><th>{t("stats.col_count")}</th><th>{t("analytics.interval")}</th><th>{t("stats.col_first")}</th><th>{t("stats.col_last")}</th></tr>
          </thead>
          <tbody>
            {#each a.ips.slice(0, ipLimit) as x (x.ip)}
              <tr>
                <td class="mono ltr">{x.ip}{#if x.new} <span class="new-tag">{t("analytics.new")}</span>{/if}</td>
                <td class:muted={!x.country}>{region(x.country, x.ip)}</td>
                <td class="org" title={x.asn}>{x.org || "—"}</td>
                <td class="mono">{x.client || "—"}</td>
                <td class="num">{i18n.number(x.count)}{#if x.fail} <span class="bad">({t("analytics.fail_n", { n: x.fail })})</span>{/if}</td>
                <td>{every(x.interval)}</td>
                <td>{i18n.date(x.first)}</td>
                <td>{i18n.relative(x.last)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if a.ips.length > ipLimit}<button class="btn sm" type="button" onclick={() => (ipLimit += 50)}>{t("links.load_more")}</button>{/if}
    </section>

    <section class="panel box">
      <h2>{t("analytics.recent")}</h2>
      <div class="wrap">
        <table class="table flush">
          <thead>
            <tr><th>{t("links.col_time")}</th><th>IP</th><th>{t("analytics.region")}</th><th>{t("stats.col_client")}</th><th>{t("links.col_status")}</th><th>{t("analytics.through")}</th></tr>
          </thead>
          <tbody>
            {#each requestKeys(a.recent) as k, i (k)}
              {@const r = a.recent[i]!}
              <tr title={r.ua} class:fresh={fresh.has(k)}>
                <td>{i18n.date(r.at, "long")}</td>
                <td class="mono ltr">{r.ip}</td>
                <td class:muted={!r.country}>{region(r.country, r.ip)}</td>
                <td class="mono">{r.client || "—"}</td>
                <td><span class="code" data-tone={statusTone(r.status)}>{r.status}</span></td>
                <td>{r.shortId ? "/" + (data?.shorts[r.shortId] ?? "?") : linkLabel(r.linkId)}{#if r.kind !== "config"} · {kindLabel(r.kind)}{/if}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>
  {/if}
</div>

<style>
  /* 实时请求流：只改 opacity 与 transform，交给合成线程；系统要求减少动态效果时不播放 */
  @media (prefers-reduced-motion: no-preference) {
    tr.fresh {
      animation: req-in 1.2s ease-out;
    }
    strong.roll {
      display: inline-block;
      animation: count-roll 0.45s ease-out;
    }
  }
  @keyframes req-in {
    from {
      opacity: 0;
      transform: translateY(-6px);
    }
    40% {
      opacity: 1;
      transform: none;
    }
  }
  @keyframes count-roll {
    from {
      opacity: 0;
      transform: translateY(40%);
    }
  }
  /* 新请求的底色在 3 秒内褪去：颜色渐变不算动态效果，减少动态效果时也保留 */
  tr.fresh td {
    animation: req-glow 3s ease-out;
  }
  @keyframes req-glow {
    from {
      background: var(--accent-soft);
    }
  }
  .page {
    max-inline-size: 1120px;
    margin-inline: auto;
    padding: var(--space-8) var(--space-6);
    display: grid;
    gap: var(--space-4);
    align-content: start;
  }
  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }
  .filters .select {
    inline-size: auto;
    min-inline-size: 180px;
  }
  .seg {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--bg-sunken);
  }
  .seg button {
    padding: 4px var(--space-3);
    border: 0;
    border-radius: calc(var(--radius-md) - 2px);
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .seg button.on {
    background: var(--bg-raised);
    color: var(--fg);
    box-shadow: var(--shadow-sm);
  }
  .zone {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-inline-start: auto;
    font-size: var(--text-xs);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-3);
    margin: 0;
  }
  .tile {
    padding: var(--space-4) var(--space-5);
  }
  .tile.alert {
    border-color: color-mix(in srgb, var(--warn) 50%, var(--border));
  }
  .tile dt {
    color: var(--fg-muted);
    font-size: var(--text-sm);
  }
  .tile dd {
    display: grid;
    margin: 0;
  }
  .tile strong {
    font-size: 26px;
    font-weight: 650;
    font-variant-numeric: tabular-nums;
  }
  .tile .muted {
    font-size: var(--text-xs);
  }
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
  }
  h2 {
    font-size: var(--text-md);
    font-weight: 600;
  }
  h3 {
    margin-block-start: var(--space-2);
    font-size: var(--text-sm);
    color: var(--fg-muted);
  }
  .ranks {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: var(--space-4);
  }
  .wrap {
    position: relative;
    overflow-x: auto;
  }
  .num {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .org {
    max-inline-size: 220px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .bad {
    color: var(--err);
  }
  .new-tag {
    padding-inline: 5px;
    border-radius: var(--radius-full);
    background: color-mix(in srgb, var(--warn) 18%, transparent);
    color: var(--warn);
    font-family: var(--font-sans);
    font-size: 11px;
  }
  .code[data-tone="ok"] {
    color: var(--ok);
  }
  .code[data-tone="warn"] {
    color: var(--warn);
  }
  .code[data-tone="err"] {
    color: var(--err);
  }
  .empty {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-6);
  }
  .warn-note {
    color: var(--warn);
  }
  @media (max-width: 767px) {
    .page {
      padding: var(--space-5) var(--space-4);
    }
    .zone {
      margin-inline-start: 0;
    }
  }
</style>
