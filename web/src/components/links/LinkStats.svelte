<script lang="ts" module>
  export interface IPRow {
    ip: string;
    first: string;
    last: string;
    ok: number;
    fail: number;
    client: string;
  }
  export interface LinkStat {
    id: string;
    total: number;
    first?: string;
    last?: string;
    series: number[];
    ips: IPRow[];
  }
</script>

<script lang="ts">
  // 一条配置链接的请求统计（反馈 #67）：近 90 天趋势；展开后是按 IP 的汇总（安全检查：
  // 哪些地址成功拉取过、多少次）与逐条请求记录。只有配置的所有者能看到。
  import DayBars from "../stats/DayBars.svelte";
  import Icon from "../Icon.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Req {
    at: string;
    ip: string;
    ua: string;
    client: string;
    status: number;
    bytes: number;
    kind: string;
  }

  let { stat, days, profileId }: { stat: LinkStat; days: number[]; profileId: string } = $props();

  let open = $state(false);
  let tab = $state<"ips" | "log">("ips");
  let reqs = $state<Req[]>([]);
  let next = $state<string | undefined>();
  let loading = $state(false);

  async function loadMore(reset = false) {
    loading = true;
    try {
      const r = await api.get<{ items: Req[]; next?: string }>(`/api/v1/profiles/${profileId}/links/${stat.id}/requests`, {
        query: { cursor: reset ? undefined : next, limit: 50 },
      });
      reqs = reset ? r.items : [...reqs, ...r.items];
      next = r.next;
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      loading = false;
    }
  }

  function toggle() {
    open = !open;
    if (open && !reqs.length) void loadMore(true);
  }

  const statusTone = (s: number) => (s < 300 ? "ok" : s === 304 ? "same" : s === 429 ? "warn" : "err");
  const kb = (n: number) => (n ? (n / 1024).toFixed(n < 10240 ? 1 : 0) + " KiB" : "—");
</script>

<div class="lstats">
  <div class="summary">
    <span><strong>{i18n.number(stat.total)}</strong> {t("links.requests_total")}</span>
    {#if stat.first}<span class="muted">{t("links.first_request", { when: i18n.date(stat.first) })}</span>{/if}
    <span class="muted">{t("links.ip_count", { n: stat.ips.length })}</span>
    <button type="button" class="linkish" aria-expanded={open} onclick={toggle}>
      {t("links.details")}<Icon name={open ? "chevron-down" : "chevron-right"} size={13} />
    </button>
  </div>
  <DayBars {days} values={stat.series} label={t("links.requests_daily")} height={64} />

  {#if open}
    <div class="detail">
      <div class="tabs" role="tablist">
        <button role="tab" type="button" aria-selected={tab === "ips"} onclick={() => (tab = "ips")}>{t("links.tab_ips")}</button>
        <button role="tab" type="button" aria-selected={tab === "log"} onclick={() => (tab = "log")}>{t("links.tab_log")}</button>
      </div>
      {#if tab === "ips"}
        <p class="hint">{t("links.ips_hint")}</p>
        {#if stat.ips.length}
          <div class="wrap">
            <table class="table flush">
              <thead><tr><th>IP</th><th>{t("links.col_ok")}</th><th>{t("links.col_fail")}</th><th>{t("stats.col_first")}</th><th>{t("stats.col_last")}</th><th>{t("stats.col_client")}</th></tr></thead>
              <tbody>
                {#each stat.ips as x (x.ip)}
                  <tr>
                    <td class="mono ltr">{x.ip}</td>
                    <td class="num">{i18n.number(x.ok)}</td>
                    <td class="num" class:bad={x.fail > 0}>{i18n.number(x.fail)}</td>
                    <td>{i18n.date(x.first)}</td>
                    <td>{i18n.relative(x.last)}</td>
                    <td class="mono">{x.client || "—"}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {:else}
          <p class="muted">{t("links.never_used")}</p>
        {/if}
      {:else}
        {#if reqs.length}
          <div class="wrap">
            <table class="table flush">
              <thead><tr><th>{t("links.col_time")}</th><th>IP</th><th>{t("links.col_status")}</th><th>{t("stats.col_client")}</th><th>{t("links.col_size")}</th></tr></thead>
              <tbody>
                {#each reqs as q, i (i)}
                  <tr title={q.ua}>
                    <td>{i18n.date(q.at)}</td>
                    <td class="mono ltr">{q.ip}</td>
                    <td><span class="code" data-tone={statusTone(q.status)}>{q.status}</span>{#if q.kind === "provider"}<span class="muted kind">{t("links.kind_provider")}</span>{:else if q.kind === "export"}<span class="muted kind">{t("links.kind_export")}</span>{/if}</td>
                    <td class="mono">{q.client || "—"}</td>
                    <td class="num">{kb(q.bytes)}</td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
          {#if next}<button class="btn sm" type="button" disabled={loading} onclick={() => void loadMore()}>{t("links.load_more")}</button>{/if}
        {:else if !loading}
          <p class="muted">{t("links.never_used")}</p>
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .lstats {
    display: grid;
    gap: var(--space-2);
    margin-block-start: var(--space-2);
  }
  .summary {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }
  .summary strong {
    font-size: var(--text-md);
    font-variant-numeric: tabular-nums;
  }
  .linkish {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-fg);
    font: inherit;
    cursor: pointer;
  }
  .detail {
    display: grid;
    gap: var(--space-2);
  }
  .tabs {
    display: flex;
    gap: var(--space-4);
    border-block-end: 1px solid var(--border);
  }
  .tabs button {
    padding-block: var(--space-1);
    padding-inline: 0;
    border: 0;
    border-block-end: 2px solid transparent;
    background: none;
    color: var(--fg-muted);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .tabs button[aria-selected="true"] {
    border-block-end-color: var(--accent);
    color: var(--fg);
  }
  .wrap {
    position: relative;
    overflow-x: auto;
  }
  .num {
    font-variant-numeric: tabular-nums;
  }
  .bad {
    color: var(--err);
  }
  .kind {
    margin-inline-start: var(--space-2);
  }
  .code {
    font-variant-numeric: tabular-nums;
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
</style>
