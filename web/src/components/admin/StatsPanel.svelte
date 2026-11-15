<script lang="ts">
  // 全站统计（反馈 #66、#67）：总数从启用统计起永久累计；趋势为近 90 天；
  // 「令牌无效的请求来源」用于发现有人在试探配置链接。
  import DayBars from "../stats/DayBars.svelte";
  import RankBars from "../stats/RankBars.svelte";
  import { api } from "../../lib/api/client";
  import { catalog } from "../../lib/catalog.svelte";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { live } from "../../lib/live";

  interface IP {
    ip: string;
    first: string;
    last: string;
    ok: number;
    fail: number;
    client: string;
  }
  interface AdminStats {
    days: number[];
    totals: Record<string, number>;
    series: Record<string, number[]>;
    pages: { key: string; count: number }[];
    clients: { key: string; count: number }[];
    fields: { key: string; count: number }[];
    probes: IP[];
  }

  let data = $state<AdminStats | null>(null);
  let error = $state("");
  $effect(() => {
    void Promise.all([api.get<AdminStats>("/api/v1/admin/stats"), catalog.load()])
      .then(([d]) => (data = d))
      .catch((e) => (error = errorText(e)));
  });
  // 自动刷新（反馈 #73）：页面可见时每分钟刷新
  $effect(() => live(async () => (data = await api.get<AdminStats>("/api/v1/admin/stats")), 60_000).stop);

  const last30 = (xs: number[] | undefined) => (xs ?? []).slice(-30).reduce((a, b) => a + b, 0);
  // 页面类型显示为菜单里的名称；没有对应名称的保留原样
  const pageLabel = (k: string) => {
    const s = t("stats.page." + k);
    return s === "stats.page." + k ? k : s;
  };
  const fieldLabel = (path: string) => {
    const f = catalog.field(path);
    return f ? `${catalog.title(f)} · ${path}` : path;
  };
</script>

<section class="panel box" aria-labelledby="a-stats">
  <h2 id="a-stats">{t("stats.title")}</h2>
  {#if error}
    <p class="field-error" role="alert">{error}</p>
  {:else if !data}
    <p class="muted">{t("common.loading")}</p>
  {:else}
    <dl class="tiles">
      {#each [["visit", t("stats.visits")], ["sub_all", t("stats.requests")], ["login", t("stats.logins")], ["sub_fail", t("stats.failed")]] as const as [k, label] (k)}
        <div class="tile">
          <dt>{label}</dt>
          <dd><strong>{(data.totals[k] ?? 0).toLocaleString(i18n.intlLocale)}</strong><span class="muted">{t("stats.last30", { n: last30(data.series[k]).toLocaleString(i18n.intlLocale) })}</span></dd>
        </div>
      {/each}
    </dl>
    <p class="hint">{t("stats.since_hint")}</p>

    <div class="charts">
      <div>
        <h3>{t("stats.visits_daily")}</h3>
        <DayBars days={data.days} values={data.series.visit ?? []} label={t("stats.visits_daily")} />
      </div>
      <div>
        <h3>{t("stats.requests_daily")}</h3>
        <DayBars days={data.days} values={data.series.sub_all ?? []} label={t("stats.requests_daily")} />
      </div>
      <div>
        <h3>{t("stats.people_daily")}</h3>
        <DayBars days={data.days} values={data.series.uv ?? []} label={t("stats.people_daily")} />
      </div>
    </div>

    <div class="ranks">
      <div>
        <h3>{t("stats.top_pages")}</h3>
        <RankBars items={data.pages.map((x) => ({ label: pageLabel(x.key), count: x.count }))} empty={t("stats.none")} />
      </div>
      <div>
        <h3>{t("stats.top_clients")}</h3>
        <RankBars items={data.clients.map((x) => ({ label: x.key, count: x.count, mono: true }))} empty={t("stats.none")} />
      </div>
      <div>
        <h3>{t("stats.top_fields")}</h3>
        <RankBars items={data.fields.map((x) => ({ label: fieldLabel(x.key), count: x.count }))} empty={t("stats.none")} />
        <p class="hint">{t("stats.fields_hint")}</p>
      </div>
    </div>

    <h3>{t("stats.probes")}</h3>
    <p class="hint">{t("stats.probes_hint")}</p>
    {#if data.probes.length}
      <div class="wrap">
        <table class="table flush">
          <thead><tr><th>IP</th><th>{t("stats.col_count")}</th><th>{t("stats.col_first")}</th><th>{t("stats.col_last")}</th><th>{t("stats.col_client")}</th></tr></thead>
          <tbody>
            {#each data.probes as p (p.ip)}
              <tr>
                <td class="mono ltr">{p.ip}</td>
                <td class="num">{(p.ok + p.fail).toLocaleString(i18n.intlLocale)}</td>
                <td>{i18n.date(p.first)}</td>
                <td>{i18n.relative(p.last)}</td>
                <td class="mono">{p.client || "—"}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="muted">{t("stats.no_probes")}</p>
    {/if}
  {/if}
</section>

<style>
  .box {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  h2 {
    font-size: var(--text-lg);
  }
  h3 {
    margin-block: var(--space-2) var(--space-1);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--fg-muted);
  }
  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-3);
    margin: 0;
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
    letter-spacing: -0.01em;
  }
  .tile .muted {
    font-size: var(--text-xs);
  }
  .charts,
  .ranks {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: var(--space-5);
  }
  .wrap {
    position: relative;
    overflow-x: auto;
  }
  .num {
    font-variant-numeric: tabular-nums;
  }
</style>
