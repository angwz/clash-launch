import { describe, expect, it } from "vitest";
import { highlightLine } from "./yaml-highlight";

const kinds = (s: string) => highlightLine(s).map((t) => t.k + ":" + t.s);

describe("highlightLine", () => {
  it("token 拼接后与原行完全一致", () => {
    for (const line of [
      "mixed-port: 7890",
      "  - DOMAIN-SUFFIX,example.com,DIRECT # 注释",
      '  name: "🪐 示例 #1"',
      "proxies: !append",
      "- {name: a, type: ss, server: 192.0.2.1, port: 8388}",
      "#!MANAGED-CONFIG https://example.com/s/x interval=43200",
      "",
      "    ",
    ]) {
      expect(highlightLine(line).map((t) => t.s).join("")).toBe(line);
    }
  });

  it("区分键、数字、布尔、注释与标签", () => {
    expect(kinds("mixed-port: 7890")).toEqual(["key:mixed-port", "pun::", "txt: ", "num:7890"]);
    expect(kinds("allow-lan: false")).toContain("bool:false");
    expect(kinds("rules: !prepend")).toContain("tag:!prepend");
    expect(kinds("a: 'x # y' # c")).toEqual(["key:a", "pun::", "txt: ", "str:'x # y' ", "com:# c"]);
  });
});
