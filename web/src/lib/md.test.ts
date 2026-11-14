import { describe, expect, it } from "vitest";
import { inline, parse, slug } from "./md";

describe("md", () => {
  it("行内：代码、加粗、链接；不安全的链接降级为文字", () => {
    expect(inline("用 `mixed-port` 和 **加粗**，见 [文档](https://example.com) 与 [x](javascript:alert(1))")).toEqual([
      { k: "text", s: "用 " },
      { k: "code", s: "mixed-port" },
      { k: "text", s: " 和 " },
      { k: "bold", s: "加粗" },
      { k: "text", s: "，见 " },
      { k: "link", s: "文档", href: "https://example.com" },
      { k: "text", s: " 与 " },
      { k: "text", s: "x" },
      { k: "text", s: ")" },
    ]);
  });

  it("块：标题、段落合并、列表、代码块、提示", () => {
    const b = parse("# 快速上手\n\n第一行\n第二行\n\n- a\n- b\n\n1. x\n\n```yaml\nmode: rule\n```\n\n> 注意\n> 续行");
    expect(b.map((x) => x.k)).toEqual(["h", "p", "ul", "ol", "code", "tip"]);
    expect(b[0]).toMatchObject({ level: 1, id: "快速上手" });
    expect(b[1]).toEqual({ k: "p", text: [{ k: "text", s: "第一行 第二行" }] });
    expect(b[2]).toMatchObject({ items: [[{ s: "a" }], [{ s: "b" }]] });
    expect(b[4]).toEqual({ k: "code", lang: "yaml", text: "mode: rule" });
  });

  it("锚点", () => expect(slug("在客户端导入 (OpenClash)")).toBe("在客户端导入-openclash"));
});
