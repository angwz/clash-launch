import { describe, expect, it } from "vitest";
import { moveItem } from "./sortable";

describe("moveItem", () => {
  it("向后、向前移动，不修改原数组", () => {
    const a = ["a", "b", "c", "d"];
    expect(moveItem(a, 0, 2)).toEqual(["b", "c", "a", "d"]);
    expect(moveItem(a, 3, 1)).toEqual(["a", "d", "b", "c"]);
    expect(moveItem(a, 1, 1)).toEqual(a);
    expect(a).toEqual(["a", "b", "c", "d"]);
  });
});
