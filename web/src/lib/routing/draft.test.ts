import { describe, expect, it } from "vitest";
import { addMember, changes, lisKeep, merge, move, reaches, removeMember, setFirst, setParam, setTarget, setType, splitOut, unitOf, type Routing } from "./draft";

const U = (kind: string, f: string) => `https://raw.githubusercontent.com/angwz/domainrouter_v2/release/${kind}/${f}`;

function base(): Routing {
  return {
    groups: [
      { name: "P", type: "select", rule: ["[]A", "[]DIRECT", ".*"] },
      { name: "A", type: "url-test", rule: [".*"], url: "https://www.gstatic.com/generate_204", interval: 300, tolerance: 50 },
      { name: "B", type: "select", rule: ["[]P"] },
    ],
    rulesets: [
      { group: "DIRECT", ruleset: U("clash-domain", "Direct.yaml"), type: "clash-domain", bundle: "Direct" },
      { group: "DIRECT", ruleset: U("clash-ipcidr", "Direct-ipcidr.yaml"), type: "clash-ipcidr", bundle: "Direct" },
      { group: "P", ruleset: U("clash-domain", "AI.yaml"), type: "clash-domain" },
      { group: "P", ruleset: U("clash-classic", "AI-classic.yaml"), type: "clash-classic" },
      { group: "B", ruleset: "[]GEOIP,CN" },
      { group: "P", ruleset: "[]MATCH" },
    ],
  };
}
const names = (r: Routing) => r.rulesets.map((x) => x.ruleset.split("/").pop() + (x.bundle ? "#" + x.bundle : ""));

describe("draft", () => {
  it("合集是一个单元：改走向与移动都作用于整个合集", () => {
    const r = base();
    expect(unitOf(r, 1)).toEqual([0, 1]);
    expect(setTarget(r, 1, "P").rulesets.slice(0, 2).map((x) => x.group)).toEqual(["P", "P"]);
    expect(names(move(r, 0, 2, true)!)).toEqual(["AI.yaml", "Direct.yaml#Direct", "Direct-ipcidr.yaml#Direct", "AI-classic.yaml", "[]GEOIP,CN", "[]MATCH"]);
    expect(r.rulesets[0]!.group).toBe("DIRECT"); // 不修改输入
  });

  it("兜底不能移动，其后不能插入", () => {
    const r = base();
    expect(move(r, 5, 0, false)).toBeNull();
    expect(move(r, 2, 5, true)).toBeNull();
    expect(names(move(r, 2, 5, false)!).at(-2)).toBe("AI.yaml");
  });

  it("合并：成员按 domain、ipcidr、classical 排序，走向随目标；内联规则与兜底不能合并", () => {
    const r = base();
    const m = merge(r, 3, 2)!;
    expect(names(m)).toEqual(["Direct.yaml#Direct", "Direct-ipcidr.yaml#Direct", "AI.yaml#AI", "AI-classic.yaml#AI", "[]GEOIP,CN", "[]MATCH"]);
    const m2 = merge(r, 2, 0)!;
    expect(names(m2)).toEqual(["Direct.yaml#Direct", "AI.yaml#Direct", "Direct-ipcidr.yaml#Direct", "AI-classic.yaml", "[]GEOIP,CN", "[]MATCH"]);
    expect(m2.rulesets[1]!.group).toBe("DIRECT");
    expect(merge(r, 4, 2)).toBeNull();
    expect(merge(r, 2, 5)).toBeNull();
  });

  it("拆出：放在合集之后；只剩一个成员时取消合集", () => {
    const s = splitOut(base(), 0)!;
    expect(names(s).slice(0, 2)).toEqual(["Direct-ipcidr.yaml", "Direct.yaml"]);
    expect(splitOut(base(), 2)).toBeNull();
  });

  it("改类型：参数按类型取舍，切回时取回原值；缺少测速参数时补缺省值", () => {
    const r = base();
    const a = setType(r, "A", "select");
    expect(a.routing.groups[1]).toEqual({ name: "A", type: "select", rule: [".*"] });
    const b = setType(a.routing, "A", "url-test", a.stash);
    expect(b.routing.groups[1]).toMatchObject({ type: "url-test", url: "https://www.gstatic.com/generate_204", interval: 300, tolerance: 50 });
    const c = setType(r, "P", "fallback");
    expect(c.routing.groups[0]).toMatchObject({ type: "fallback", url: "https://www.gstatic.com/generate_204", interval: 300 });
    expect(setParam(r, "A", "tolerance", undefined).groups[1]!.tolerance).toBeUndefined();
  });

  it("成员：加入、设为首选、移除；会形成循环时拒绝", () => {
    const r = base();
    expect(reaches(r, "B", "A")).toBe(true);
    expect(addMember(r, "A", "B")).toBeNull(); // A → B → P → A
    expect(addMember(r, "B", "A")!.groups[2]!.rule).toEqual(["[]P", "[]A"]);
    expect(addMember(r, "P", "A")).toBeNull(); // 已是成员
    expect(setFirst(r, "P", 1).groups[0]!.rule).toEqual(["[]DIRECT", "[]A", ".*"]);
    expect(removeMember(r, "P", 0).groups[0]!.rule).toEqual(["[]DIRECT", ".*"]);
  });

  it("改动列表：一次交换只报一条移动", () => {
    const r = base();
    expect(lisKeep([0, 2, 1, 3]).size).toBe(3);
    expect(lisKeep([3, 0, 1, 2])).toEqual(new Set([1, 2, 3]));
    const d = setTarget(move(r, 2, 0, false)!, 4, "DIRECT");
    const t = setType(d, "A", "fallback").routing;
    expect(changes(r, t).map((c) => c.key + ":" + Object.values(c.params).join("/"))).toEqual(["move:AI", "target:GEOIP,CN/B/DIRECT", "type:A/url-test/fallback"]);
    expect(changes(r, r)).toEqual([]);
    expect(changes(r, splitOut(r, 0)!).map((c) => c.key)).toEqual(["move", "unbundle", "unbundle"]);
  });
});
