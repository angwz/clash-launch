// 强调色（R28）：每种预设在浅色与深色主题下都满足 WCAG AA（4.5:1）——按钮的白字、悬停时的白字、
// 页面与面板底色上的强调文字、浅底徽标上的强调文字；深色两处定义一致；与 accents.ts 的名称一一对应。
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { ACCENTS, SWATCH } from "../lib/accents";

const css = readFileSync(join(__dirname, "accents.css"), "utf8");

function lum(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  const ch = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * ch[0]! + 0.7152 * ch[1]! + 0.0722 * ch[2]!;
}
function contrast(a: string, b: string): number {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p);
  return (x! + 0.05) / (y! + 0.05);
}
function block(selector: string, from = css): Record<string, string> {
  const i = from.indexOf(selector + " {");
  expect(i, selector).toBeGreaterThanOrEqual(0);
  const body = from.slice(i, from.indexOf("}", i));
  return Object.fromEntries([...body.matchAll(/(--[a-z-]+):\s*(#[0-9a-f]{6})/g)].map((m) => [m[1]!, m[2]!]));
}

const LIGHT = { bg: "#f7f7f8", raised: "#ffffff" };
const DARK = { bg: "#111114", raised: "#18181c" };
const presets = ACCENTS.filter((a) => a !== "indigo");

describe("accents", () => {
  for (const a of presets) {
    it(`${a}：浅色主题对比度`, () => {
      const v = block(`:root[data-accent="${a}"]:not([data-theme="dark"])`);
      expect(contrast(v["--accent"]!, "#ffffff")).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-hover"]!, "#ffffff")).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-fg"]!, LIGHT.bg)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-fg"]!, v["--accent-soft"]!)).toBeGreaterThanOrEqual(4.5);
      expect(SWATCH[a]).toBe(v["--accent"]);
    });
    it(`${a}：深色主题对比度，两处定义一致`, () => {
      const v = block(`:root[data-accent="${a}"][data-theme="dark"]`);
      expect(contrast(v["--accent"]!, "#ffffff")).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-hover"]!, "#ffffff")).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-fg"]!, DARK.bg)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-fg"]!, DARK.raised)).toBeGreaterThanOrEqual(4.5);
      expect(contrast(v["--accent-fg"]!, v["--accent-soft"]!)).toBeGreaterThanOrEqual(4.5);
      const media = css.slice(css.indexOf("@media (prefers-color-scheme: dark)"));
      expect(block(`:root[data-accent="${a}"]:not([data-theme="light"])`, media)).toEqual(v);
    });
  }
});
