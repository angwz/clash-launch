import { describe, expect, it } from "vitest";
import { yamlRange } from "./yaml-range";

const doc = `mixed-port: 7890
allow-lan: false
dns:
  enable: true
  enhanced-mode: fake-ip
  nameserver:
    - https://example.com/dns-query

  fallback-filter:
    geoip: true
proxies:
  - name: a
`.split("\n");

describe("yamlRange", () => {
  it("顶层标量", () => expect(yamlRange(doc, "allow-lan")).toEqual({ start: 1, end: 2, exact: true }));
  it("嵌套标量", () => expect(yamlRange(doc, "dns.enhanced-mode")).toEqual({ start: 4, end: 5, exact: true }));
  it("列表值包含子行，不含末尾空行", () => expect(yamlRange(doc, "dns.nameserver")).toEqual({ start: 5, end: 7, exact: true }));
  it("映射整体", () => expect(yamlRange(doc, "dns")).toEqual({ start: 2, end: 10, exact: true }));
  it("深层", () => expect(yamlRange(doc, "dns.fallback-filter.geoip")).toEqual({ start: 9, end: 10, exact: true }));
  it("缺失时返回最深祖先", () => expect(yamlRange(doc, "dns.listen")).toEqual({ start: 2, end: 10, exact: false }));
  it("完全缺失", () => expect(yamlRange(doc, "tun.enable")).toBeNull());
  it("不深入列表", () => expect(yamlRange(doc, "proxies.name")?.exact).toBe(false));
});

describe("注释掉的分节", () => {
  it("未启用分节写成注释时仍可定位", () => {
    const lines = ["mixed-port: 7890", "", "# 未启用，不写入下发的配置（disabled, not served）", "# dns:", "#   enable: false", "#   ipv6: true"];
    expect(yamlRange(lines, "dns.ipv6")).toEqual({ start: 5, end: 6, exact: true });
  });
});
