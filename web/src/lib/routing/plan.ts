// 智能视图（docs/design/routing.md 第 9 节）：按配置的形状选择画法。都是写明阈值的确定规则，不是推断；
// 每一条决定产生一个 why（文案键与参数），显示在地图上方，偏好可以覆盖。同一份配置永远得到同一个 ViewPlan。
import { groupColumns, type Columns, type Graph } from "./layout";

export interface RoutingPrefs {
  rules?: "auto" | "all";
  particles?: "all" | "focus" | "off";
  candidates?: boolean;
  exits?: "auto" | "node" | "region" | "source";
}

/** 相邻且走向相同、显示为一行的规则（只是显示，不改配置）。 */
export interface Run {
  id: string;
  from: number;
  to: number;
  target: string;
  /** sets：RULE-SET（#1）；manual：手写规则（#2） */
  kind: "sets" | "manual";
}

export interface ExitRow {
  id: string;
  kind: "node" | "region" | "source" | "builtin" | "provider";
  /** 显示名：节点名、地区代码、来源名、内置出口名或 provider 名 */
  label: string;
  /** 归并进这一行的出口名 */
  members: string[];
  filter?: string;
  /** 按地区归并时地区的中英文名（来自服务端的地区表） */
  names?: Record<string, string>;
  via?: string;
}

export interface Why {
  key: string;
  params?: Record<string, string | number>;
}

export interface ViewPlan {
  mode: "map" | "list";
  compact: boolean;
  columns: Columns;
  runs: Run[];
  exitMode: "node" | "region" | "source";
  exits: ExitRow[];
  /** 出口名 → 出口行 ID */
  exitRow: Map<string, string>;
  unused: string[];
  bigGroups: Set<string>;
  particleCap: number;
  labelProb: number;
  // 文案键为 routing.why.<key>
  why: Why[];
}

export const THRESHOLDS = {
  foldRules: 30, // #1
  manualRun: 3, // #2
  compactCols: 3, // #3
  bigGroup: 12, // #7
  exitsByNode: 8, // #8：不超过这个数逐个显示
  regionShare: 0.8, // #8：至少这么多节点识别出地区时按地区
  listBelow: 760, // #12
  compactBelow: 1100, // #12
} as const;

const BUILTINS = new Set(["DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"]);

export function planView(g: Graph, width: number, prefs: RoutingPrefs = {}): ViewPlan {
  const why: Why[] = [];
  const columns = groupColumns(g);
  const mode = width < THRESHOLDS.listBelow ? "list" : "map";
  if (mode === "list") why.push({ key: "list" });

  // #1、#2：折叠
  const runs: Run[] = [];
  const n = g.rules.length;
  if (prefs.rules !== "all") {
    const manual = (i: number) => {
      const t = g.rules[i]!.type;
      return t !== "RULE-SET" && t !== "GEOIP" && t !== "MATCH";
    };
    for (let i = 0; i < n; i++) {
      const r = g.rules[i]!;
      let j = i;
      if (n > THRESHOLDS.foldRules && r.type === "RULE-SET") {
        while (j + 1 < n && g.rules[j + 1]!.type === "RULE-SET" && g.rules[j + 1]!.target === r.target) j++;
        if (j > i) runs.push({ id: "seg-" + i, from: i, to: j, target: r.target, kind: "sets" });
      } else if (manual(i)) {
        while (j + 1 < n && manual(j + 1) && g.rules[j + 1]!.target === r.target) j++;
        if (j - i + 1 >= THRESHOLDS.manualRun) runs.push({ id: "seg-" + i, from: i, to: j, target: r.target, kind: "manual" });
        else j = i;
      }
      i = j;
    }
  }
  const hidden = runs.reduce((s, r) => s + r.to - r.from + 1, 0);
  if (runs.length) why.push({ key: "folded", params: { n, hidden, segs: runs.length } });
  else why.push({ key: "rules", params: { n } });

  // #3、#12：紧凑
  const compact = columns.cols.length >= THRESHOLDS.compactCols || width < THRESHOLDS.compactBelow;
  why.push({ key: compact ? "columns_compact" : "columns", params: { groups: g.groups.length, cols: columns.cols.length } });

  // #6：未使用的组
  const unused = g.groups.filter((x) => x.lints.includes("group.unused")).map((x) => x.name);
  if (unused.length) why.push({ key: "unused", params: { n: unused.length } });

  // #7：成员很多的组
  const bigGroups = new Set(g.groups.filter((x) => x.members.length > THRESHOLDS.bigGroup).map((x) => x.name));
  if (bigGroups.size) why.push({ key: "big_groups", params: { n: bigGroups.size, max: THRESHOLDS.bigGroup } });

  // #8：出口归并；#9 provider；#10 链式代理
  const nodes = g.exits.filter((e) => e.kind === "node");
  let exitMode: ViewPlan["exitMode"];
  if (prefs.exits && prefs.exits !== "auto") exitMode = prefs.exits;
  else if (nodes.length <= THRESHOLDS.exitsByNode) exitMode = "node";
  else if (nodes.filter((e) => e.region).length >= nodes.length * THRESHOLDS.regionShare) exitMode = "region";
  else exitMode = "source";
  const exits: ExitRow[] = [];
  const exitRow = new Map<string, string>();
  const rows = new Map<string, ExitRow>();
  for (const e of nodes) {
    let id: string, label: string;
    if (exitMode === "node") (id = "node:" + e.name), (label = e.name);
    else if (exitMode === "region") (id = "region:" + (e.region || "?")), (label = e.region || "?");
    else (id = "source:" + (e.source || "?")), (label = e.source || "?");
    let row = rows.get(id);
    if (!row) {
      row = { id, kind: exitMode, label, members: [] };
      if (exitMode === "node" && e.via) row.via = e.via;
      if (exitMode === "region" && e.regionName) row.names = e.regionName;
      rows.set(id, row);
      exits.push(row);
    }
    row.members.push(e.name);
    exitRow.set(e.name, id);
  }
  if (nodes.length) why.push({ key: "exits_" + exitMode, params: { n: nodes.length, rows: exits.length } });
  for (const e of g.exits) {
    if (e.kind === "provider") {
      const row: ExitRow = { id: "provider:" + e.name, kind: "provider", label: e.name, members: [e.name] };
      if (e.filter) row.filter = e.filter;
      exits.push(row);
      exitRow.set(e.name, row.id);
    }
  }
  if (g.exits.some((e) => e.kind === "provider")) why.push({ key: "providers" });
  // 内置出口放在最后，按 mihomo 的常见顺序
  for (const b of ["DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"]) {
    if (g.exits.some((e) => e.kind === "builtin" && e.name === b)) {
      exits.push({ id: "builtin:" + b, kind: "builtin", label: b, members: [b] });
      exitRow.set(b, "builtin:" + b);
    }
  }
  // 组成员里出现、但出口表中没有的名字（理论上不会发生）：单独一行，避免连线丢失
  for (const grp of g.groups) {
    for (const m of grp.members) {
      if (m.kind !== "group" && !exitRow.has(m.name) && !BUILTINS.has(m.name)) {
        const row: ExitRow = { id: "node:" + m.name, kind: "node", label: m.name, members: [m.name] };
        exits.push(row);
        exitRow.set(m.name, row.id);
      }
    }
  }

  // #11：原始编辑
  if (g.readonlyReason === "raw_override") why.push({ key: "raw_override" });
  else if (g.rules.some((r) => r.origin === "raw") || g.groups.some((x) => x.origin === "raw")) why.push({ key: "raw_rows" });

  // #13：粒子
  const particleCap = Math.min(70, Math.max(30, n * 3));
  const labelProb = n > THRESHOLDS.foldRules ? 0.225 : 0.45;

  return { mode, compact, columns, runs, exitMode, exits, exitRow, unused, bigGroups, particleCap, labelProb, why };
}

/**
 * 折叠后的规则行：段（智能折叠，只是显示）、合集（相邻且 bundle 相同的规则集，决定 0007 第 9 条，始终占一行）
 * 或单条规则。expanded 与命中行所在的段展开；合集不展开，命中其中一条时整行高亮。
 */
export type RuleRow = { kind: "rule"; index: number } | { kind: "seg"; run: Run } | { kind: "bundle"; from: number; to: number; name: string };

export function ruleRows(g: Graph, plan: ViewPlan, expanded: ReadonlySet<string>, hit: readonly number[] = []): RuleRow[] {
  const out: RuleRow[] = [];
  const bundleOf = (i: number) => g.rules[i]?.set?.bundle;
  for (let i = 0; i < g.rules.length; i++) {
    const run = plan.runs.find((r) => r.from === i);
    if (run && !expanded.has(run.id) && !hit.some((h) => h >= run.from && h <= run.to)) {
      out.push({ kind: "seg", run });
      i = run.to;
      continue;
    }
    const b = bundleOf(i);
    if (b) {
      let j = i;
      while (j + 1 < g.rules.length && bundleOf(j + 1) === b && g.rules[j + 1]!.target === g.rules[i]!.target) j++;
      if (j > i) {
        out.push({ kind: "bundle", from: i, to: j, name: b });
        i = j;
        continue;
      }
    }
    out.push({ kind: "rule", index: i });
  }
  return out;
}

/** 方框键：rule:<序号> | bundle:<首条序号> | seg:<段 ID>。 */
export function rowKey(r: RuleRow): string {
  return r.kind === "rule" ? "rule:" + r.index : r.kind === "bundle" ? "bundle:" + r.from : "seg:" + r.run.id;
}

/** 规则序号 → 方框键：折叠段与合集内的规则共用一行。 */
export function ruleBoxOf(rows: readonly RuleRow[]): (index: number) => string {
  const m = new Map<number, string>();
  for (const r of rows) {
    if (r.kind === "rule") m.set(r.index, rowKey(r));
    else if (r.kind === "bundle") for (let i = r.from; i <= r.to; i++) m.set(i, rowKey(r));
    else for (let i = r.run.from; i <= r.run.to; i++) m.set(i, rowKey(r));
  }
  return (i) => m.get(i) ?? "rule:" + i;
}
