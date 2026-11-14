import { describe, expect, it } from "vitest";
import { byHeat, heat } from "./field-order";

describe("按热度排序", () => {
  const items = ["dns.ipv6", "dns.enhanced-mode", "dns.nameserver", "dns.fallback"];
  it("热度合计不足时保持默认顺序", () => {
    expect(byHeat(items, (x) => x, { "dns.nameserver": 5 })).toEqual(items);
  });
  it("按热度从高到低，相同时保持原顺序", () => {
    const scores = { "dns.nameserver": 20, "dns.fallback": 20, "dns.ipv6": 1 };
    expect(byHeat(items, (x) => x, scores)).toEqual(["dns.nameserver", "dns.fallback", "dns.ipv6", "dns.enhanced-mode"]);
  });
  it("对象字段的热度包含子字段", () => {
    expect(heat("tun", { "tun.stack": 3, "tun.auto-route": 4, "tunnels": 9 })).toBe(7);
  });
});
