import { describe, expect, it } from "vitest";
import { ordered } from "./prefs.svelte";

describe("ordered", () => {
  const id = (x: string) => x;
  it("没有偏好时保持默认顺序", () => {
    expect(ordered(["a", "b", "c"], id, undefined)).toEqual(["a", "b", "c"]);
  });
  it("按偏好排序，未列出的项按默认顺序排在后面", () => {
    expect(ordered(["a", "b", "c", "d"], id, ["c", "a"])).toEqual(["c", "a", "b", "d"]);
  });
  it("忽略偏好中已不存在的项", () => {
    expect(ordered(["a", "b"], id, ["gone", "b"])).toEqual(["b", "a"]);
  });
});
