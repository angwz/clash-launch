<script lang="ts">
  // 后台任务（需求 R36）：每个任务最近一次的开始时间、耗时、累计次数与失败，以及启动以来的拉取次数。
  // 数据在服务器内存中，重启后从零开始；这里每 30 秒刷新一次（页面不可见时暂停，见 live）。
  import StatusBadge from "../StatusBadge.svelte";
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { live } from "../../lib/live";

  interface Job {
    name: string;
    runs: number;
    failures: number;
    lastStart: string;
    lastMs: number;
    lastError?: string;
    lastErrorAt?: string;
    running: boolean;
  }
  interface Resp {
    jobs: Job[];
    fetches: Record<string, number>;
    since: string;
  }
  const KNOWN = ["fetch", "footprints", "avatars", "stats", "deletions", "prune_audit", "prune_requests", "purge_threads", "purge_backups"];

  let data = $state<Resp | null>(null);
  let error = $state("");
  const load = async () => {
    try {
      data = await api.get<Resp>("/api/v1/admin/jobs");
      error = "";
    } catch (e) {
      error = errorText(e);
    }
  };
  $effect(() => void load());
  $effect(() => live(load, 30_000, []).stop);

  const label = (n: string) => (KNOWN.includes(n) ? t("jobs.name." + n) : n);
  const dur = (ms: number) => (ms < 1000 ? i18n.number(Math.round(ms)) + " ms" : i18n.number(Math.round(ms / 100) / 10) + " s");
</script>

<section class="panel box" aria-labelledby="a-jobs">
  <div class="head">
    <h2 id="a-jobs">{t("jobs.title")}</h2>
    {#if data}<span class="muted since">{t("jobs.since", { when: i18n.relative(data.since) })}</span>{/if}
  </div>
  <p class="muted">{t("jobs.desc")}</p>
  {#if error}<p class="field-error" role="alert">{error}</p>{/if}
  {#if data}
    <!-- 可横向滚动：允许键盘聚焦后用方向键滚动（WCAG 对可滚动区域的推荐做法，与 Users.svelte 相同） -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div class="table-wrap" tabindex="0" role="region" aria-label={t("jobs.title")}>
      <table class="table flush">
        <thead>
          <tr>
            <th>{t("jobs.col_name")}</th>
            <th>{t("jobs.col_last")}</th>
            <th>{t("jobs.col_duration")}</th>
            <th>{t("jobs.col_runs")}</th>
            <th>{t("jobs.col_status")}</th>
          </tr>
        </thead>
        <tbody>
          {#each data.jobs as j (j.name)}
            <tr>
              <td>{label(j.name)}<span class="mono muted id">{j.name}</span></td>
              <td><time datetime={j.lastStart} title={i18n.date(j.lastStart)}>{i18n.relative(j.lastStart)}</time></td>
              <td class="num">{dur(j.lastMs)}</td>
              <td class="num">{t("jobs.runs", { runs: j.runs, failures: j.failures })}</td>
              <td>
                {#if j.running}<StatusBadge tone="info" label={t("jobs.running")} />
                {:else if j.lastError && j.lastErrorAt && j.lastErrorAt >= j.lastStart}<StatusBadge tone="err" label={t("jobs.failed")} /><div class="err">{j.lastError}</div>
                {:else}<StatusBadge tone="ok" label={t("jobs.ok")} />{/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="muted fetch">{t("jobs.fetches", { sok: data.fetches.source_ok ?? 0, sfail: data.fetches.source_failed ?? 0, rok: data.fetches.rule_ok ?? 0, rfail: data.fetches.rule_failed ?? 0 })}</p>
  {/if}
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  /* grid 子项默认的最小宽度是内容宽度，表格的横向滚动容器要能收窄到屏幕宽度 */
  .box > * {
    min-inline-size: 0;
  }
  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  h2 {
    font-size: var(--text-lg);
  }
  .since,
  .fetch {
    font-size: var(--text-sm);
  }
  .id {
    margin-inline-start: var(--space-2);
    font-size: var(--text-xs);
  }
  /* 窄屏上表格在自己的容器里横向滚动，不撑宽页面 */
  .table-wrap {
    overflow-x: auto;
  }
  .num {
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .err {
    margin-block-start: 2px;
    color: var(--err);
    font-size: var(--text-xs);
    overflow-wrap: anywhere;
  }
</style>
