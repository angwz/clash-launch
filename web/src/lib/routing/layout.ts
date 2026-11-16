// 分流地图的布局（docs/design/routing.md 第 8 节）：纯函数，不读 DOM，可单测。
// 组按「默认路径」分列：每个组沿第一个成员走到出口的步数决定它离出口多远；
// 默认路径上的连线因此总是从左到右，只当备份的组不会挤进入口列。
import type { Schemas } from "../api/client";

export type Graph = Schemas["RoutingGraph"];
export type Rule = Schemas["RoutingRule"];
export type Group = Schemas["RoutingGroup"];
export type Exit = Schemas["RoutingExit"];
export type Trace = Schemas["RoutingTrace"];

export interface Columns {
  /** 每一列的组，列内已排序 */
  cols: Group[][];
  /** 组名 → 列号（从 1 开始） */
  col: Map<string, number>;
  /** 组名 → 沿默认成员到出口的步数（出口为 0） */
  steps: Map<string, number>;
  /** 成环的组（内核会拒绝这样的配置；这里只是不陷入死循环） */
  cycles: string[];
}

export function groupIndex(g: Graph): Map<string, Group> {
  return new Map(g.groups.map((x) => [x.name, x]));
}

/** 8.1 + 8.2 + 8.3：分列与列内顺序。 */
export function groupColumns(g: Graph): Columns {
  const groups = groupIndex(g);
  const steps = new Map<string, number>();
  const cycles: string[] = [];
  const visiting = new Set<string>();
  const stepOf = (name: string): number => {
    const grp = groups.get(name);
    if (!grp) return 0; // 出口
    const known = steps.get(name);
    if (known !== undefined) return known;
    if (visiting.has(name)) {
      cycles.push(name);
      return 1;
    }
    visiting.add(name);
    const first = grp.members[0];
    const s = 1 + (first ? stepOf(first.name) : 0);
    visiting.delete(name);
    steps.set(name, s);
    return s;
  };
  for (const x of g.groups) stepOf(x.name);

  const defaultChild = new Set(g.groups.map((x) => x.members[0]).filter((m) => m && m.kind === "group").map((m) => m!.name));
  const hasParent = new Set(g.groups.flatMap((x) => x.members.filter((m) => m.kind === "group").map((m) => m.name)));
  const ruleUsed = new Set(g.rules.map((r) => r.target));
  const entry = (name: string) => !hasParent.has(name) || (ruleUsed.has(name) && !defaultChild.has(name));
  const C = Math.max(1, ...steps.values());
  const col = new Map<string, number>();
  for (const x of g.groups) col.set(x.name, entry(x.name) ? 1 : C - (steps.get(x.name) ?? 1) + 1);

  // 列内顺序：最早被哪条规则或哪个组引用；候选成员排在默认成员之后
  const key = new Map<string, number>();
  g.rules.forEach((r, i) => {
    if (groups.has(r.target) && !key.has(r.target)) key.set(r.target, i);
  });
  const byCol = [...g.groups].sort((a, b) => col.get(a.name)! - col.get(b.name)!);
  byCol.forEach((x, gi) => {
    if (!key.has(x.name)) key.set(x.name, 100000 + gi); // 未被引用的排在最后
    x.members.forEach((m, mi) => {
      if (m.kind !== "group" || col.get(m.name)! <= col.get(x.name)!) return;
      const k = key.get(x.name)! + (mi + 1) * 0.001 + (mi === 0 ? 0 : 0.5);
      if (!key.has(m.name) || k < key.get(m.name)!) key.set(m.name, k);
    });
  });
  const cols: Group[][] = [];
  for (let c = 1; c <= C; c++) cols.push(g.groups.filter((x) => col.get(x.name) === c).sort((a, b) => key.get(a.name)! - key.get(b.name)!));
  // 去掉空列（如只有入口列与直连节点的组时中间没有组），列号随之重排
  const kept = cols.filter((c) => c.length);
  kept.forEach((c, i) => c.forEach((x) => col.set(x.name, i + 1)));
  return { cols: kept, col, steps, cycles };
}

export interface Hop {
  /** 地图上的方框键：group:<名> 或 exit:<出口行 ID> */
  box: string;
  name: string;
}

export type FlowKind = "proxy" | "direct" | "reject" | "neutral";

/** 沿默认路径（每层第一个成员）从规则目标走到出口。exitRow 把出口名映射到出口行（节点可能按地区归并）。 */
export function defaultPath(g: Graph, target: string, exitRow: (name: string) => string): { hops: Hop[]; kind: FlowKind } {
  const groups = groupIndex(g);
  const hops: Hop[] = [];
  const seen = new Set<string>();
  let name = target;
  for (;;) {
    const grp = groups.get(name);
    if (!grp) break;
    if (seen.has(name)) return { hops, kind: "neutral" };
    seen.add(name);
    hops.push({ box: "group:" + name, name });
    const first = grp.members[0];
    if (!first) return { hops, kind: "neutral" };
    name = first.name;
  }
  hops.push({ box: "exit:" + exitRow(name), name });
  return { hops, kind: flowKind(name) };
}

export function flowKind(exitName: string): FlowKind {
  if (exitName === "DIRECT") return "direct";
  if (exitName === "REJECT" || exitName === "REJECT-DROP") return "reject";
  if (exitName === "PASS" || exitName === "COMPATIBLE") return "neutral";
  return "proxy";
}

export interface Pt {
  x: number;
  y: number;
}

/** 8.4：三次贝塞尔，水平切线；目标在同列或左侧时从列间空隙绕行。 */
export function curveD(a: Pt, b: Pt): string {
  const f = (n: number) => n.toFixed(1);
  if (b.x > a.x + 8) {
    const mx = (a.x + b.x) / 2;
    return `M${f(a.x)} ${f(a.y)} C ${f(mx)} ${f(a.y)}, ${f(mx)} ${f(b.y)}, ${f(b.x)} ${f(b.y)}`;
  }
  const out = 26;
  return `M${f(a.x)} ${f(a.y)} C ${f(a.x + out)} ${f(a.y)}, ${f(b.x - out)} ${f(b.y)}, ${f(b.x)} ${f(b.y)}`;
}

/** 方框相对地图左上角的位置。 */
export interface Box {
  l: number;
  r: number;
  t: number;
  b: number;
  y: number;
}

export interface Link {
  key: string;
  d: string;
  /** default：默认成员；alt：测速决定（url-test 的全部成员）；cand：候选；rule：规则到目标 */
  kind: "rule" | "default" | "alt" | "cand";
}

export interface LinkInput {
  graph: Graph;
  boxes: Map<string, Box>;
  /** 规则序号 → 方框键（折叠段内的规则共用段的方框） */
  ruleBox: (index: number) => string;
  exitRow: (name: string) => string;
  /** 总是画出候选成员（偏好 candidates）或当前聚焦的组 */
  showCandidates: (group: string) => boolean;
  /** 成员太多的组只画默认成员（第 9 节 #7） */
  bigGroups: Set<string>;
}

/** 计算全部连线。入度 ≥ 4 的方框改为先到左侧 18px 的汇聚点（第 9 节 #5）。 */
export function links(inp: LinkInput): Link[] {
  const { graph, boxes } = inp;
  const groups = groupIndex(graph);
  const boxOf = (name: string) => (groups.has(name) ? "group:" + name : "exit:" + inp.exitRow(name));
  const raw: { key: string; from: string; to: string; kind: Link["kind"] }[] = [];
  const seen = new Set<string>();
  for (const r of graph.rules) {
    const from = inp.ruleBox(r.index);
    const to = boxOf(r.target);
    const key = from + ">" + to;
    if (seen.has(key)) continue; // 折叠段内走向相同的规则共用一根线
    seen.add(key);
    raw.push({ key, from, to, kind: "rule" });
  }
  for (const grp of graph.groups) {
    const from = "group:" + grp.name;
    grp.members.forEach((m, i) => {
      const equal = grp.type === "url-test" || grp.type === "load-balance";
      const isDefault = i === 0 || equal;
      if (inp.bigGroups.has(grp.name) && i > 0 && !inp.showCandidates(grp.name)) return;
      if (!isDefault && !inp.showCandidates(grp.name)) return;
      const to = boxOf(m.name);
      const key = from + ">" + to;
      if (seen.has(key)) return; // 多个节点归并到同一出口行时只画一根
      seen.add(key);
      raw.push({ key, from, to, kind: grp.type === "url-test" ? "alt" : isDefault ? "default" : "cand" });
    });
  }
  const indeg = new Map<string, number>();
  for (const x of raw) indeg.set(x.to, (indeg.get(x.to) ?? 0) + 1);
  const out: Link[] = [];
  for (const x of raw) {
    const a = boxes.get(x.from);
    const b = boxes.get(x.to);
    if (!a || !b) continue;
    const start = { x: a.r, y: a.y };
    if ((indeg.get(x.to) ?? 0) >= 4 && b.l > a.r + 30) {
      const join = { x: b.l - 18, y: b.y };
      out.push({ key: x.key, kind: x.kind, d: curveD(start, join) + ` L ${b.l.toFixed(1)} ${b.y.toFixed(1)}` });
    } else {
      out.push({ key: x.key, kind: x.kind, d: curveD(start, { x: b.l, y: b.y }) });
    }
  }
  return out;
}
