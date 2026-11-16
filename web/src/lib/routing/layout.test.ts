import { describe, expect, it } from "vitest";
import { curveD, defaultPath, groupColumns, links, type Box, type Graph } from "./layout";
import tiny from "./fixtures/tiny.json";
import domainrouter from "./fixtures/domainrouter.json";
import acl4ssr from "./fixtures/acl4ssr.json";
import provider from "./fixtures/provider.json";
import rawOverride from "./fixtures/raw-override.json";
import deep8 from "./fixtures/deep8.json";

const fixtures = { tiny, domainrouter, acl4ssr, provider, rawOverride, deep8 } as unknown as Record<string, Graph>;

describe("groupColumns", () => {
  for (const [name, g] of Object.entries(fixtures)) {
    it(`${name}：默认路径上的连线总是从左到右，每个组恰好在一列`, () => {
      const { cols, col, cycles } = groupColumns(g);
      expect(cycles).toEqual([]);
      expect(cols.flat().length).toBe(g.groups.length);
      for (const grp of g.groups) {
        const first = grp.members[0];
        if (first?.kind === "group") expect(col.get(first.name)!, `${grp.name} → ${first.name}`).toBeGreaterThan(col.get(grp.name)!);
      }
    });
  }

  it("domainrouter：23 个组按默认路径分 5 列，规则直接使用的业务组在入口列", () => {
    const { cols } = groupColumns(fixtures.domainrouter!);
    expect(cols.map((c) => c.length)).toEqual([13, 1, 3, 3, 3]);
    expect(cols[0]!.map((g) => g.name)).toContain("🐳 漏网の鱼");
    expect(cols[4]!.map((g) => g.name)).toEqual(["🎇 香港高速", "🏞️ 美利坚高速", "🗾 日本高速"]);
  });

  it("deep8：8 层各占一列", () => {
    expect(groupColumns(fixtures.deep8!).cols.map((c) => c.map((g) => g.name))).toEqual([["L1"], ["L2"], ["L3"], ["L4"], ["L5"], ["L6"], ["L7"], ["L8"]]);
  });

  it("成环时不陷入死循环，并报告出来", () => {
    const g = {
      ...fixtures.tiny!,
      groups: [
        { name: "A", type: "select", members: [{ name: "B", kind: "group" }], params: {}, origin: "rules", lints: [] },
        { name: "B", type: "select", members: [{ name: "A", kind: "group" }], params: {}, origin: "rules", lints: [] },
      ],
    } as Graph;
    expect(groupColumns(g).cycles.length).toBeGreaterThan(0);
  });
});

describe("defaultPath", () => {
  it("沿每层第一个成员走到出口，并给出流向", () => {
    const g = fixtures.deep8!;
    const p = defaultPath(g, "L1", (n) => "node:" + n);
    expect(p.hops.map((h) => h.box)).toEqual(["group:L1", "group:L2", "group:L3", "group:L4", "group:L5", "group:L6", "group:L7", "group:L8", "exit:node:🇭🇰 香港 01"]);
    expect(p.kind).toBe("proxy");
    expect(defaultPath(g, "DIRECT", (n) => "builtin:" + n).kind).toBe("direct");
    expect(defaultPath(fixtures.tiny!, "REJECT", (n) => "builtin:" + n).kind).toBe("reject");
  });
});

describe("links", () => {
  const box = (l: number, y: number): Box => ({ l, r: l + 100, t: y - 10, b: y + 10, y });
  it("候选成员只在需要时画", () => {
    const g = fixtures.tiny!;
    const boxes = new Map<string, Box>([
      ["rule:0", box(0, 10)],
      ["rule:1", box(0, 40)],
      ["rule:2", box(0, 70)],
      ["rule:3", box(0, 100)],
      ["group:PROXY", box(200, 100)],
      ["exit:node:🇭🇰 香港 01", box(400, 10)],
      ["exit:node:🇯🇵 日本 02", box(400, 40)],
      ["exit:node:🇺🇸 美国 03", box(400, 70)],
      ["exit:builtin:DIRECT", box(400, 100)],
      ["exit:builtin:REJECT", box(400, 130)],
    ]);
    const base = {
      graph: g,
      boxes,
      ruleBox: (i: number) => "rule:" + i,
      exitRow: (n: string) => (n === "DIRECT" || n === "REJECT" ? "builtin:" : "node:") + n,
      bigGroups: new Set<string>(),
    };
    const few = links({ ...base, showCandidates: () => false });
    expect(few.filter((l) => l.kind === "cand")).toEqual([]);
    expect(few.filter((l) => l.kind === "default").map((l) => l.key)).toEqual(["group:PROXY>exit:builtin:DIRECT"]);
    const all = links({ ...base, showCandidates: () => true });
    expect(all.filter((l) => l.kind === "cand").length).toBe(3);
  });

  it("入度 ≥ 4 时先到方框左侧 18px 的汇聚点", () => {
    const g = fixtures.tiny!;
    const boxes = new Map<string, Box>();
    for (let i = 0; i < 4; i++) boxes.set("rule:" + i, box(0, 10 + i * 30));
    boxes.set("exit:builtin:DIRECT", box(400, 50));
    const all = links({ graph: { ...g, rules: g.rules.map((r) => ({ ...r, target: "DIRECT" })), groups: [] }, boxes, ruleBox: (i) => "rule:" + i, exitRow: (n) => "builtin:" + n, showCandidates: () => false, bigGroups: new Set() });
    expect(all.length).toBe(4);
    for (const l of all) expect(l.d.endsWith(" L 400.0 50.0")).toBe(true);
  });

  it("curveD：同列或向左时从列间空隙绕行", () => {
    expect(curveD({ x: 0, y: 0 }, { x: 100, y: 50 })).toBe("M0.0 0.0 C 50.0 0.0, 50.0 50.0, 100.0 50.0");
    expect(curveD({ x: 100, y: 0 }, { x: 50, y: 50 })).toBe("M100.0 0.0 C 126.0 0.0, 24.0 50.0, 50.0 50.0");
  });
});
