import { describe, expect, it } from "vitest";
import { lineDiff } from "./diff";

describe("lineDiff", () => {
  it("相同文本只有上下文行", () => {
    expect(lineDiff("a\nb", "a\nb").every((d) => d.op === " ")).toBe(true);
  });

  it("插入、删除与修改", () => {
    const d = lineDiff("a\nb\nc\nd", "a\nB\nc\nd\ne");
    expect(d).toEqual([
      { op: " ", text: "a" },
      { op: "-", text: "b" },
      { op: "+", text: "B" },
      { op: " ", text: "c" },
      { op: " ", text: "d" },
      { op: "+", text: "e" },
    ]);
  });

  it("应用差异可以还原新文本", () => {
    const a = "x\ny\nz\nw";
    const b = "y\nq\nz";
    const d = lineDiff(a, b);
    expect(d.filter((l) => l.op !== "+").map((l) => l.text).join("\n")).toBe(a);
    expect(d.filter((l) => l.op !== "-").map((l) => l.text).join("\n")).toBe(b);
  });
});
