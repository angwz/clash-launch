import { describe, expect, it } from "vitest";
import { yamlSnippet } from "./yaml-snippet";

describe("yamlSnippet", () => {
  it("嵌套标量", () => expect(yamlSnippet(["dns", "enhanced-mode"], "fake-ip")).toBe("dns:\n  enhanced-mode: fake-ip"));
  it("布尔与数字", () => expect(yamlSnippet(["unified-delay"], true)).toBe("unified-delay: true"));
  it("会被误读的字符串加引号", () => {
    expect(yamlSnippet(["a"], "true")).toBe('a: "true"');
    expect(yamlSnippet(["a"], "*.lan")).toBe('a: "*.lan"');
  });
  it("标量列表写成块式", () => expect(yamlSnippet(["dns", "nameserver"], ["https://example.com/dns-query", "192.0.2.1"])).toBe("dns:\n  nameserver:\n    - https://example.com/dns-query\n    - 192.0.2.1"));
});

describe("yamlSnippet 数字形状", () => {
  it("像数字的字符串加引号，IP 不加", () => {
    expect(yamlSnippet(["a"], "1.5")).toBe('a: "1.5"');
    expect(yamlSnippet(["a"], "7890")).toBe('a: "7890"');
    expect(yamlSnippet(["a"], "192.0.2.1")).toBe("a: 192.0.2.1");
  });
});
