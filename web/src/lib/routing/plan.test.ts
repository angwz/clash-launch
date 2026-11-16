import { describe, expect, it } from "vitest";
import { planView, ruleBoxOf, ruleRows } from "./plan";
import type { Graph } from "./layout";
import tiny from "./fixtures/tiny.json";
import domainrouter from "./fixtures/domainrouter.json";
import acl4ssr from "./fixtures/acl4ssr.json";
import provider from "./fixtures/provider.json";
import rawOverride from "./fixtures/raw-override.json";
import deep8 from "./fixtures/deep8.json";

const F = { tiny, domainrouter, acl4ssr, provider, rawOverride, deep8 } as unknown as Record<string, Graph>;
const keys = (g: Graph, w = 1440, prefs = {}) => planView(g, w, prefs).why.map((x) => x.key);

describe("planView：每个夹具的画法与原因", () => {
  it("tiny：逐条显示、1 列、节点逐个、标出原始编辑的行", () => {
    const p = planView(F.tiny!, 1440);
    expect(p.mode).toBe("map");
    expect(p.compact).toBe(false);
    expect(p.runs).toEqual([]);
    expect(p.exits.map((e) => e.id)).toEqual(["node:🇭🇰 香港 01", "node:🇯🇵 日本 02", "node:🇺🇸 美国 03", "builtin:DIRECT", "builtin:REJECT"]);
    expect(keys(F.tiny!)).toEqual(["rules", "columns", "exits_node", "raw_rows"]);
    expect(p.particleCap).toBe(30);
  });

  it("domainrouter：66 条规则中相邻同走向的 59 条折叠为 21 段，5 列紧凑", () => {
    const p = planView(F.domainrouter!, 1440);
    expect(p.runs.length).toBe(21);
    expect(p.runs[0]).toEqual({ id: "seg-0", from: 0, to: 2, target: "DIRECT", kind: "sets" });
    expect(p.why[0]).toEqual({ key: "folded", params: { n: 66, hidden: 59, segs: 21 } });
    expect(p.compact).toBe(true);
    expect(p.particleCap).toBe(70);
    expect(p.labelProb).toBe(0.225);
    // 偏好「全部规则」时不折叠
    expect(planView(F.domainrouter!, 1440, { rules: "all" }).runs).toEqual([]);
  });

  it("acl4ssr：未使用的组单独列出，provider 出口单独一行", () => {
    const p = planView(F.acl4ssr!, 1440);
    expect(p.unused).toEqual(["🇭🇰 香港"]);
    expect(keys(F.acl4ssr!)).toEqual(["rules", "columns_compact", "unused", "exits_node", "providers"]);
    expect(p.exits.find((e) => e.kind === "provider")!.label).toBe("机场 B");
  });

  it("provider：交给客户端的订阅只显示 provider 出口", () => {
    expect(planView(F.provider!, 1440).exits.map((e) => e.id)).toEqual(["provider:机场 B", "builtin:DIRECT"]);
  });

  it("raw-override：原始编辑整段替换时说明原因", () => {
    expect(keys(F.rawOverride!)).toContain("raw_override");
  });

  it("deep8：8 列一律紧凑", () => {
    const p = planView(F.deep8!, 1440);
    expect(p.columns.cols.length).toBe(8);
    expect(p.compact).toBe(true);
  });

  it("视口：< 760 为列表模式，760–1100 一律紧凑", () => {
    expect(planView(F.tiny!, 700).mode).toBe("list");
    expect(keys(F.tiny!, 700)[0]).toBe("list");
    expect(planView(F.tiny!, 900).compact).toBe(true);
  });

  it("出口归并：节点多且大多识别出地区时按地区，否则按来源；偏好优先", () => {
    const exits = Array.from({ length: 10 }, (_, i) => ({ name: "n" + i, kind: "node", region: i < 9 ? (i % 2 ? "HK" : "JP") : undefined, source: "S" + (i % 3) }));
    const many = { ...F.tiny!, exits: [...exits, { name: "DIRECT", kind: "builtin" }] } as Graph;
    expect(planView(many, 1440).exitMode).toBe("region");
    expect(
      planView(many, 1440)
        .exits.filter((e) => e.kind === "region")
        .map((e) => [e.label, e.members.length]),
    ).toEqual([
      ["JP", 5],
      ["HK", 4],
      ["?", 1],
    ]);
    const unknown = { ...many, exits: many.exits.map((e, i) => (i < 5 ? { ...e, region: undefined } : e)) } as Graph;
    expect(planView(unknown, 1440).exitMode).toBe("source");
    expect(planView(many, 1440, { exits: "node" }).exitMode).toBe("node");
  });

  it("手写规则：连续 3 条以上走向相同时折叠", () => {
    const rule = (i: number, type: string, target: string) => ({ index: i, raw: `${type},x${i}.example.com,${target}`, type, target, origin: "rules", flags: [] });
    const g = { ...F.tiny!, rules: [rule(0, "DOMAIN", "A"), rule(1, "DOMAIN-SUFFIX", "A"), rule(2, "DOMAIN", "A"), rule(3, "DOMAIN", "B"), rule(4, "MATCH", "B")] } as Graph;
    expect(planView(g, 1440).runs).toEqual([{ id: "seg-0", from: 0, to: 2, target: "A", kind: "manual" }]);
  });
});

describe("ruleRows", () => {
  it("折叠段在展开或追踪命中其中一行时展开；段内规则共用段的方框", () => {
    const g = F.domainrouter!;
    const p = planView(g, 1440);
    const rows = ruleRows(g, p, new Set());
    expect(rows.length).toBe(66 - 59 + 21);
    expect(ruleBoxOf(rows)(1)).toBe("seg:seg-0");
    expect(ruleRows(g, p, new Set(["seg-0"]))[0]).toEqual({ kind: "rule", index: 0 });
    expect(ruleRows(g, p, new Set(), [4])[1]).toEqual({ kind: "rule", index: 3 });
  });
});
