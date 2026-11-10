// 样式约束（第 15.3、16.5 节）：
// 1. 只用逻辑属性：样式中不出现 left/right（含 margin-left、text-align: right 等）；
// 2. 不用四值的 margin/padding 简写（左右不对称时无法随 RTL 镜像）；
// 3. 深色 token 在「手动选择」与「跟随系统」两处的定义完全一致。
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const SRC = join(__dirname, "..");

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    if (statSync(p).isDirectory()) return files(p);
    return /\.(svelte|css)$/.test(f) ? [p] : [];
  });
}

function styles(file: string): string {
  const s = readFileSync(file, "utf8");
  if (file.endsWith(".css")) return s;
  return [...s.matchAll(/<style>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join("\n");
}

describe("styles", () => {
  it("不使用物理方向的属性与取值", () => {
    const bad: string[] = [];
    for (const f of files(SRC)) {
      styles(f)
        .replace(/\/\*[\s\S]*?\*\//g, "")
        .split("\n")
        .forEach((line, i) => {
          if (/(^|[\s;{-])(left|right)\s*:|:\s*[^;]*\b(left|right)\b/.test(line)) bad.push(`${f}:${i + 1}: ${line.trim()}`);
        });
    }
    expect(bad).toEqual([]);
  });

  it("margin/padding 不使用四值简写", () => {
    const bad: string[] = [];
    for (const f of files(SRC)) {
      for (const m of styles(f).matchAll(/(?:^|[\s;{])(margin|padding)\s*:\s*([^;]+);/g)) {
        const parts = m[2]!.trim().split(/\s+(?![^(]*\))/);
        if (parts.length === 4 && parts[1] !== parts[3]) bad.push(`${f}: ${m[0].trim()}`);
      }
    }
    expect(bad).toEqual([]);
  });

  it("深色 token 的两处定义一致", () => {
    const css = readFileSync(join(SRC, "styles", "tokens.css"), "utf8");
    const block = (re: RegExp) => {
      const m = re.exec(css);
      expect(m, String(re)).not.toBeNull();
      return m![1]!
        .split(";")
        .map((x) => x.trim())
        .filter((x) => x.startsWith("--"))
        .sort();
    };
    const manual = block(/:root\[data-theme="dark"\]\s*\{([\s\S]*?)\n\}/);
    const system = block(/:root:not\(\[data-theme="light"\]\)\s*\{([\s\S]*?)\n {2}\}/);
    expect(system).toEqual(manual);
  });
});
