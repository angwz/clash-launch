<script lang="ts">
  // 数据保留（需求 R43）：审计日志与订阅请求明细的保留期，网页上的设置优先于环境变量，立即生效；
  // 0 表示永不清理。数据量按需统计（SQLite 上大表的计数需要时间）。
  import { api } from "../../lib/api/client";
  import { errorText } from "../../lib/errors";
  import { i18n, t } from "../../lib/i18n/i18n.svelte";
  import { toasts } from "../../lib/toast.svelte";

  interface Retention {
    auditDays: number;
    requestDays: number;
    auditFromSetting: boolean;
    requestFromSetting: boolean;
    auditDefault: number;
    requestDefault: number;
  }
  interface Usage {
    dialect: string;
    total: number;
    tables: { table: string; rows: number; bytes: number; estimated: boolean }[];
  }

  let r = $state<Retention | null>(null);
  let audit = $state(0);
  let request = $state(0);
  let busy = $state(false);
  let usage = $state<Usage | null>(null);
  let counting = $state(false);

  const apply = (x: Retention) => {
    r = x;
    audit = x.auditDays;
    request = x.requestDays;
  };
  $effect(() => {
    api.get<Retention>("/api/v1/admin/retention").then(apply, (e) => toasts.push("err", errorText(e)));
  });

  async function save(body: { auditDays: number | null; requestDays: number | null }) {
    busy = true;
    try {
      apply(await api.put<Retention>("/api/v1/admin/retention", body));
      toasts.push("ok", t("common.saved"));
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      busy = false;
    }
  }
  async function count() {
    counting = true;
    try {
      usage = await api.get<Usage>("/api/v1/admin/usage");
    } catch (e) {
      toasts.push("err", errorText(e));
    } finally {
      counting = false;
    }
  }
  const size = (b: number) => (b < 0 ? "—" : b < 1 << 20 ? i18n.number(Math.round(b / 1024)) + " KiB" : i18n.number(Math.round((b / (1 << 20)) * 10) / 10) + " MiB");
  const keep = (n: number) => (n === 0 ? t("retention.forever") : t("retention.n_days", { n }));
  const rows = $derived((usage?.tables ?? []).filter((x) => x.rows > 0).sort((a, b) => b.rows - a.rows));
</script>

<section class="panel box" aria-labelledby="a-ret">
  <h2 id="a-ret">{t("retention.title")}</h2>
  <p class="muted">{t("retention.desc")}</p>
  {#if r}
    <form class="grid" onsubmit={(e) => { e.preventDefault(); void save({ auditDays: audit, requestDays: request }); }}>
      <div>
        <label class="label" for="ret-audit">{t("retention.audit")}</label>
        <input id="ret-audit" class="input num" type="number" min="0" max="3650" bind:value={audit} />
        <p class="hint">{r.auditFromSetting ? t("retention.from_setting", { n: keep(r.auditDefault) }) : t("retention.from_env", { n: keep(r.auditDefault) })}</p>
      </div>
      <div>
        <label class="label" for="ret-req">{t("retention.requests")}</label>
        <input id="ret-req" class="input num" type="number" min="0" max="3650" bind:value={request} />
        <p class="hint">{r.requestFromSetting ? t("retention.from_setting", { n: keep(r.requestDefault) }) : t("retention.from_env", { n: keep(r.requestDefault) })}</p>
      </div>
      <p class="hint wide">{t("retention.zero")}</p>
      <div class="actions wide">
        {#if r.auditFromSetting || r.requestFromSetting}<button class="btn ghost" type="button" disabled={busy} onclick={() => void save({ auditDays: null, requestDays: null })}>{t("retention.reset")}</button>{/if}
        <button class="btn primary" type="submit" disabled={busy}>{t("common.save")}</button>
      </div>
    </form>
  {/if}

  <div class="usage">
    <div class="uhead">
      <h3>{t("retention.usage")}</h3>
      <button class="btn sm" type="button" disabled={counting} onclick={() => void count()}>{counting ? t("common.loading") : t("retention.count")}</button>
    </div>
    {#if usage}
      <p class="muted">{t("retention.total", { size: size(usage.total), db: usage.dialect === "postgres" ? "PostgreSQL" : "SQLite" })}{#if usage.dialect === "postgres"} · {t("retention.estimated")}{/if}</p>
      <!-- 可横向滚动：允许键盘聚焦后用方向键滚动（WCAG 对可滚动区域的推荐做法） -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="table-wrap" tabindex="0" role="region" aria-label={t("retention.usage")}>
        <table class="table flush">
          <thead><tr><th>{t("retention.col_table")}</th><th class="n">{t("retention.col_rows")}</th>{#if usage.dialect === "postgres"}<th class="n">{t("retention.col_size")}</th>{/if}</tr></thead>
          <tbody>
            {#each rows as x (x.table)}
              <tr><td class="mono">{x.table}</td><td class="n">{x.estimated ? "≈ " : ""}{i18n.number(x.rows)}</td>{#if usage.dialect === "postgres"}<td class="n">{size(x.bytes)}</td>{/if}</tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</section>

<style>
  .box {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-5);
    margin-block-end: var(--space-4);
  }
  .box > * {
    min-inline-size: 0;
  }
  h2 {
    font-size: var(--text-lg);
  }
  h3 {
    font-size: var(--text-md);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-3);
  }
  .wide {
    grid-column: 1 / -1;
  }
  .num {
    max-inline-size: 140px;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
  .usage {
    display: grid;
    gap: var(--space-2);
    padding-block-start: var(--space-3);
    border-block-start: 1px solid var(--border);
  }
  .uhead {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .table-wrap {
    overflow-x: auto;
  }
  .n {
    text-align: end;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
</style>
