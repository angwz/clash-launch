// 分流草稿的操作（docs/design/routing.md 第 11.2 节）：都是纯函数，输入一份 routing、返回新的一份，不修改输入。
// routing 的键与 groups.toml、rulesets.toml 相同（另有 bundle），前端只做结构操作；校验与展开由后端完成。

export interface RoutingGroup {
  name: string;
  type: string;
  rule?: string[];
  use?: string[];
  select?: Record<string, unknown>;
  url?: string;
  interval?: number;
  tolerance?: number;
  lazy?: boolean;
  strategy?: string;
  [k: string]: unknown;
}

export interface RoutingRuleset {
  group: string;
  ruleset: string;
  type?: string;
  interval?: number;
  format?: string;
  bundle?: string;
}

export interface Routing {
  from?: { rules?: string; digest?: string; at?: string };
  "ini-node-ops"?: string[];
  groups: RoutingGroup[];
  rulesets: RoutingRuleset[];
  base?: { groups: RoutingGroup[]; rulesets: RoutingRuleset[] };
  checks?: { host: string; via: string }[];
}

export const GROUP_TYPES = ["select", "url-test", "fallback", "load-balance"] as const;
export type GroupType = (typeof GROUP_TYPES)[number];

/** 每种类型使用的参数（mihomo v1.19.34 adapter/outboundgroup）。 */
export const PARAM_KEYS: Record<GroupType, string[]> = {
  select: [],
  "url-test": ["url", "interval", "tolerance", "lazy"],
  fallback: ["url", "interval", "lazy"],
  "load-balance": ["url", "interval", "strategy"],
};

/** 切换到需要测速的类型时补上的缺省值：groups.toml 要求 url-test、fallback、load-balance 写出 url 与 interval。 */
export const DEFAULT_PARAMS: Record<string, unknown> = { url: "https://www.gstatic.com/generate_204", interval: 300 };

const ALL_PARAMS = ["url", "interval", "tolerance", "lazy", "strategy"];

const clone = <T>(x: T): T => structuredClone(x);

export const isInline = (r: RoutingRuleset) => r.ruleset.startsWith("[]");
export const isFinal = (r: RoutingRuleset) => /^\[\](MATCH|FINAL)(,|$)/.test(r.ruleset);

/** 条目 i 所在的合集（相邻且 bundle 相同的一段）；不在合集里时只有它自己。 */
export function unitOf(r: Routing, i: number): number[] {
  const b = r.rulesets[i]?.bundle;
  if (!b) return [i];
  let a = i;
  let z = i;
  while (a > 0 && r.rulesets[a - 1]!.bundle === b) a--;
  while (z + 1 < r.rulesets.length && r.rulesets[z + 1]!.bundle === b) z++;
  return range(a, z);
}

function range(a: number, z: number): number[] {
  return Array.from({ length: z - a + 1 }, (_, k) => a + k);
}

/** 改走向：条目（合集则整个合集）交给 group。 */
export function setTarget(r: Routing, entry: number, group: string): Routing {
  const d = clone(r);
  for (const i of unitOf(r, entry)) d.rulesets[i]!.group = group;
  return d;
}

/**
 * 移动：把条目（合集则整个合集）移到 ref 所在单元之前或之后。兜底（MATCH）不能移动，其后不能插入。
 * 返回 null 表示不允许。
 */
export function move(r: Routing, entry: number, ref: number, after: boolean): Routing | null {
  const unit = unitOf(r, entry);
  const target = unitOf(r, ref);
  if (unit.includes(ref)) return null;
  if (unit.some((i) => isFinal(r.rulesets[i]!))) return null;
  const refUnitFinal = target.some((i) => isFinal(r.rulesets[i]!));
  if (refUnitFinal && after) return null;
  const moving = unit.map((i) => r.rulesets[i]!);
  const rest = r.rulesets.filter((_, i) => !unit.includes(i));
  const anchor = r.rulesets[after ? target[target.length - 1]! : target[0]!]!;
  const at = rest.indexOf(anchor) + (after ? 1 : 0);
  const d = clone(r);
  d.rulesets = clone([...rest.slice(0, at), ...moving, ...rest.slice(at)]);
  return d;
}

const KIND_ORDER = (t?: string) => (t === "clash-domain" ? 0 : t === "clash-ipcidr" ? 1 : t === "clash-classic" ? 2 : 3);

/** 规则集的显示名：文件名去掉扩展名；内联规则取原文。 */
export function rulesetName(x: RoutingRuleset): string {
  if (isInline(x)) return x.ruleset.slice(2);
  const file = x.ruleset.split(/[?#]/)[0]!.split("/").pop() || x.ruleset;
  return file.replace(/\.[^.]+$/, "");
}

/**
 * 合并（决定 0007 第 9 条）：把 src 所在单元并入 dst 所在单元，成为同一个合集；走向随 dst。
 * 成员按 domain → ipcidr → classical 排序；合集名取 dst 的合集名或 dst 的规则集名。两者都是规则集、都不是兜底时才允许。
 */
export function merge(r: Routing, src: number, dst: number): Routing | null {
  const a = unitOf(r, src);
  const b = unitOf(r, dst);
  if (a.includes(dst)) return null;
  const all = [...b, ...a].map((i) => r.rulesets[i]!); // 同类时目标原有成员在前（sort 是稳定的）
  if (all.some((x) => isInline(x) || isFinal(x))) return null;
  const head = r.rulesets[b[0]!]!;
  const name = head.bundle || rulesetName(head).replace(/-(ipcidr|classic)$/, "");
  const members = clone(all)
    .map((x) => ({ ...x, group: head.group, bundle: name }))
    .sort((p, q) => KIND_ORDER(p.type) - KIND_ORDER(q.type));
  const rest = r.rulesets.filter((_, i) => !a.includes(i) && !b.includes(i));
  const at = rest.length ? countBefore(r, b[0]!, [...a, ...b]) : 0;
  const d = clone(r);
  d.rulesets = clone([...rest.slice(0, at), ...members, ...rest.slice(at)]);
  return d;
}

function countBefore(r: Routing, pos: number, excluded: number[]): number {
  let n = 0;
  for (let i = 0; i < pos; i++) if (!excluded.includes(i)) n++;
  return n;
}

/** 拆出：条目离开合集，放在合集之后；合集只剩一个成员时取消合集。 */
export function splitOut(r: Routing, entry: number): Routing | null {
  const unit = unitOf(r, entry);
  if (unit.length < 2) return null;
  const d = clone(r);
  const item = { ...d.rulesets[entry]! };
  delete item.bundle;
  const rest = d.rulesets.filter((_, i) => i !== entry);
  const lastOfUnit = unit[unit.length - 1]! - (unit[unit.length - 1]! > entry ? 1 : 0);
  rest.splice(lastOfUnit + 1, 0, item);
  const left = unit.filter((i) => i !== entry).map((i) => (i > entry ? i - 1 : i));
  if (left.length === 1) delete rest[left[0]!]!.bundle;
  d.rulesets = rest;
  return d;
}

function groupIndex(r: Routing, name: string): number {
  return r.groups.findIndex((g) => g.name === name);
}

/**
 * 改组类型。只保留新类型用得到的参数；用不到的放进 stash，切回原类型时取回（第 11.2 节）。
 * 需要测速的类型缺少 url 或 interval 时补上缺省值。
 */
export function setType(r: Routing, name: string, type: GroupType, stash: Record<string, unknown> = {}): { routing: Routing; stash: Record<string, unknown> } {
  const d = clone(r);
  const g = d.groups[groupIndex(d, name)];
  if (!g) return { routing: r, stash };
  const keep = PARAM_KEYS[type];
  const nextStash = { ...stash };
  for (const k of ALL_PARAMS) {
    if (k in g && !keep.includes(k)) {
      nextStash[k] = g[k];
      delete g[k];
    }
  }
  for (const k of keep) {
    if (!(k in g) && k in nextStash) {
      g[k] = nextStash[k];
      delete nextStash[k];
    }
    if (!(k in g) && k in DEFAULT_PARAMS) g[k] = DEFAULT_PARAMS[k];
  }
  g.type = type;
  return { routing: d, stash: nextStash };
}

/** 设置参数；value 为 undefined 时删除（回到 mihomo 的缺省值）。 */
export function setParam(r: Routing, name: string, key: string, value: unknown): Routing {
  const d = clone(r);
  const g = d.groups[groupIndex(d, name)];
  if (!g) return r;
  if (value === undefined || value === "") delete g[key];
  else g[key] = value;
  return d;
}

/** group 是否经由成员（[]名称）到达 target（包括自己）。 */
export function reaches(r: Routing, group: string, target: string, seen = new Set<string>()): boolean {
  if (group === target) return true;
  if (seen.has(group)) return false;
  seen.add(group);
  const g = r.groups.find((x) => x.name === group);
  for (const e of g?.rule ?? []) if (e.startsWith("[]") && reaches(r, e.slice(2), target, seen)) return true;
  return false;
}

/** 加入成员：在 rule 末尾追加 []名称。会形成循环、或已经是成员时返回 null。 */
export function addMember(r: Routing, group: string, member: string): Routing | null {
  const gi = groupIndex(r, group);
  if (gi < 0) return null;
  if (r.groups.some((x) => x.name === member) && reaches(r, member, group)) return null; // 循环：mihomo 不接受
  const entry = "[]" + member;
  if (r.groups[gi]!.rule?.includes(entry)) return null;
  const d = clone(r);
  const g = d.groups[gi]!;
  g.rule = [...(g.rule ?? []), entry];
  return d;
}

/** 设为首选：rule 中第 idx 条移到最前（成为默认成员）。 */
export function setFirst(r: Routing, group: string, idx: number): Routing {
  const d = clone(r);
  const g = d.groups[groupIndex(d, group)];
  if (!g?.rule || idx <= 0 || idx >= g.rule.length) return r;
  const [x] = g.rule.splice(idx, 1);
  g.rule.unshift(x!);
  return d;
}

/** 移除成员：删除 rule 中第 idx 条。 */
export function removeMember(r: Routing, group: string, idx: number): Routing {
  const d = clone(r);
  const g = d.groups[groupIndex(d, group)];
  if (!g?.rule || idx < 0 || idx >= g.rule.length) return r;
  g.rule.splice(idx, 1);
  return d;
}

// —— 改动列表 ——

export interface Change {
  /** 文案键 routing.change.<key> */
  key: "target" | "move" | "bundle" | "unbundle" | "added" | "removed" | "type" | "param" | "members" | "group_added" | "group_removed" | "check_added" | "check_removed" | "upstream";
  params: Record<string, string>;
}

/** 最长递增子序列：在其中的元素视为没有移动，其余报告为「移动过」（一次交换只报一条）。 */
export function lisKeep(seq: number[]): Set<number> {
  const tails: number[] = [];
  const prev: number[] = new Array(seq.length).fill(-1);
  seq.forEach((v, i) => {
    let lo = 0;
    let hi = tails.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (seq[tails[mid]!]! < v) lo = mid + 1;
      else hi = mid;
    }
    if (lo > 0) prev[i] = tails[lo - 1]!;
    tails[lo] = i;
  });
  const keep = new Set<number>();
  let k = tails.length ? tails[tails.length - 1]! : -1;
  while (k >= 0) {
    keep.add(k);
    k = prev[k]!;
  }
  return keep;
}

const key = (x: RoutingRuleset) => x.ruleset;

/** applied 到 draft 的改动，按规则在前、组在后。 */
export function changes(applied: Routing, draft: Routing): Change[] {
  const out: Change[] = [];
  const before = new Map(applied.rulesets.map((x, i) => [key(x), { x, i }]));
  const after = new Map(draft.rulesets.map((x, i) => [key(x), { x, i }]));
  // 移动：共同条目在草稿中的顺序里，不在最长递增子序列中的视为移动过
  const common = draft.rulesets.filter((x) => before.has(key(x)));
  const keep = lisKeep(common.map((x) => before.get(key(x))!.i));
  common.forEach((x, j) => {
    if (!keep.has(j)) out.push({ key: "move", params: { name: rulesetName(x) } });
  });
  for (const x of draft.rulesets) {
    const b = before.get(key(x));
    if (!b) {
      out.push({ key: "added", params: { name: rulesetName(x), to: x.group } });
      continue;
    }
    if (b.x.group !== x.group) out.push({ key: "target", params: { name: rulesetName(x), from: b.x.group, to: x.group } });
    if ((b.x.bundle ?? "") !== (x.bundle ?? "")) {
      out.push(x.bundle ? { key: "bundle", params: { name: rulesetName(x), bundle: x.bundle } } : { key: "unbundle", params: { name: rulesetName(x), bundle: b.x.bundle ?? "" } });
    }
  }
  for (const x of applied.rulesets) if (!after.has(key(x))) out.push({ key: "removed", params: { name: rulesetName(x) } });

  const gb = new Map(applied.groups.map((g) => [g.name, g]));
  const ga = new Map(draft.groups.map((g) => [g.name, g]));
  for (const g of draft.groups) {
    const b = gb.get(g.name);
    if (!b) {
      out.push({ key: "group_added", params: { name: g.name } });
      continue;
    }
    if (b.type !== g.type) out.push({ key: "type", params: { name: g.name, from: b.type, to: g.type } });
    for (const k of ALL_PARAMS) {
      if (b.type === g.type && JSON.stringify(b[k]) !== JSON.stringify(g[k])) out.push({ key: "param", params: { name: g.name, param: k, value: g[k] === undefined ? "—" : String(g[k]) } });
    }
    if (JSON.stringify(b.rule ?? []) !== JSON.stringify(g.rule ?? [])) out.push({ key: "members", params: { name: g.name } });
  }
  for (const g of applied.groups) if (!ga.has(g.name)) out.push({ key: "group_removed", params: { name: g.name } });
  return out;
}

// —— 上游同步与期望（第 12 节）——

export interface UpstreamChange {
  id: string;
  kind: "group" | "ruleset";
  op: "add" | "remove" | "modify" | "move";
  key: string;
  field?: string;
  upstream?: unknown;
  conflict: boolean;
}

export interface Upstream {
  digest: string;
  groups: Record<string, unknown>[];
  rulesets: Record<string, unknown>[];
}

/**
 * 采纳上游的变化：accepted 中的每一处写入 routing；之后 base 一律更新为上游的当前内容、摘要随之更新——
 * 没有采纳的就是忽略（第 12 节）。at 由调用方给出（纯函数不读当前时间）。
 */
export function applyUpstream(r: Routing, up: Upstream, accepted: UpstreamChange[], at: string): Routing {
  const d = clone(r);
  const upGroups = up.groups as unknown as RoutingGroup[];
  const upSets = up.rulesets as unknown as RoutingRuleset[];
  // 上游里排在 key 前面、且在 list 中存在的最近一项之后的位置；没有时放在最前
  const after = <T>(list: T[], upList: T[], keyOf: (x: T) => string, key: string): number => {
    const i = upList.findIndex((x) => keyOf(x) === key);
    for (let j = i - 1; j >= 0; j--) {
      const at = list.findIndex((x) => keyOf(x) === keyOf(upList[j]!));
      if (at >= 0) return at + 1;
    }
    return 0;
  };
  for (const c of accepted) {
    if (c.kind === "ruleset") {
      const idx = d.rulesets.findIndex((x) => x.ruleset === c.key);
      const fromUp = upSets.find((x) => x.ruleset === c.key);
      if (c.op === "remove" && idx >= 0) d.rulesets.splice(idx, 1);
      else if ((c.op === "add" || c.op === "move") && fromUp) {
        const item = idx >= 0 ? d.rulesets.splice(idx, 1)[0]! : clone(fromUp);
        let pos = after(d.rulesets, upSets, (x) => x.ruleset, c.key);
        // 兜底规则之后不能插入
        const fin = d.rulesets.findIndex(isFinal);
        if (fin >= 0 && pos > fin && !isFinal(item)) pos = fin;
        d.rulesets.splice(pos, 0, item);
      } else if (c.op === "modify" && idx >= 0 && c.field) setField(d.rulesets[idx] as unknown as Record<string, unknown>, c.field, c.upstream);
    } else {
      const idx = d.groups.findIndex((x) => x.name === c.key);
      const fromUp = upGroups.find((x) => x.name === c.key);
      if (c.op === "remove" && idx >= 0) d.groups.splice(idx, 1);
      else if (c.op === "add" && fromUp && idx < 0) d.groups.splice(after(d.groups, upGroups, (x) => x.name, c.key), 0, clone(fromUp));
      else if (c.op === "modify" && idx >= 0 && c.field) setField(d.groups[idx]!, c.field, c.upstream);
    }
  }
  d.base = { groups: clone(upGroups), rulesets: clone(upSets) };
  d.from = { ...d.from, digest: up.digest, at };
  return d;
}

function setField(o: Record<string, unknown>, field: string, value: unknown) {
  if (value === undefined || value === null) delete o[field];
  else o[field] = clone(value);
}

/** 固定为期望：追踪 host 时路径上必须出现 via。已有同一 host 的期望时替换。 */
export function addCheck(r: Routing, host: string, via: string): Routing {
  const d = clone(r);
  d.checks = [...(d.checks ?? []).filter((c) => c.host !== host), { host, via }];
  return d;
}

export function removeCheck(r: Routing, host: string): Routing {
  const d = clone(r);
  d.checks = (d.checks ?? []).filter((c) => c.host !== host);
  if (!d.checks.length) delete d.checks;
  return d;
}

/** 期望与上游摘要的改动（changes 只比较组与规则集）。 */
export function metaChanges(applied: Routing, draft: Routing): Change[] {
  const out: Change[] = [];
  const before = new Map((applied.checks ?? []).map((c) => [c.host, c.via]));
  const after = new Map((draft.checks ?? []).map((c) => [c.host, c.via]));
  for (const [h, v] of after) if (before.get(h) !== v) out.push({ key: "check_added", params: { host: h, via: v } });
  for (const [h] of before) if (!after.has(h)) out.push({ key: "check_removed", params: { host: h } });
  if ((applied.from?.digest ?? "") !== (draft.from?.digest ?? "")) out.push({ key: "upstream", params: {} });
  return out;
}
