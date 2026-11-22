// 帮助文档、更新日志与用户协议（R37）：三种语言一一对应，新增一篇时不会漏掉某种语言；
// 每篇都以一级标题开头（作为目录里的标题）。
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const LANGS = ["zh-CN", "en", "ja"];

describe("help", () => {
  it("每种语言的帮助文档篇目相同，且都有一级标题", () => {
    const names = LANGS.map((l) => readdirSync(join(__dirname, l)).filter((f) => f.endsWith(".md")).sort());
    for (const n of names.slice(1)) expect(n).toEqual(names[0]);
    for (const l of LANGS) for (const f of names[0]!) expect(readFileSync(join(__dirname, l, f), "utf8").startsWith("# "), l + "/" + f).toBe(true);
  });

  it("更新日志与用户协议有每种语言的版本", () => {
    for (const l of LANGS) {
      expect(existsSync(join(__dirname, "..", "..", "public", "changelog", l + ".md")), "changelog " + l).toBe(true);
      expect(existsSync(join(__dirname, "..", "..", "..", "internal", "service", "terms", l + ".md")), "terms " + l).toBe(true);
    }
  });
});
