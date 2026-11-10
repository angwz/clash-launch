// 消息完整性：代码中用到的每个键在两种语言中都存在；两种语言的键集合一致；每条消息都是合法的 ICU MessageFormat。
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { IntlMessageFormat } from "intl-messageformat";
import { describe, expect, it } from "vitest";
import { AUDIT_ACTIONS } from "../../components/AuditLog.svelte";
import en from "./messages/en.json";
import zh from "./messages/zh-CN.json";

const SRC = join(__dirname, "..", "..");

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    if (statSync(p).isDirectory()) return files(p);
    return /\.(svelte|ts)$/.test(f) && !f.endsWith(".test.ts") ? [p] : [];
  });
}

// 以前缀拼接的动态键：列出每个前缀可能的取值（与后端枚举、组件状态一致）
const dynamic: Record<string, string[]> = {
  "check.": ["idle", "running", "passed", "failed", "unavailable", "error"],
  "theme.": ["system", "light", "dark"],
  "role.": ["admin", "user"],
  "render.": ["ok", "failed", "pending"],
  "source.kind.": ["single", "batch", "subscription", "file"],
  "source.kind_hint.": ["single", "batch", "subscription", "file"],
  "region.why.": ["flag", "keyword", "route", "manual", "rule", "unknown"],
  "region.kind.": ["flag", "keyword", "route", "manual", "rule", "unknown"],
  "severity.": ["error", "warning", "info"],
  "kernel.": ["passed", "failed", "unavailable", "timeout"],
  "invite.": ["active", "used", "expired"],
  "palette.kind.": ["page", "field", "command"],
  // 与后端写入的审计动作一致（components/AuditLog.svelte）
  "audit.action.": AUDIT_ACTIONS,
  // 与 internal/auth.BuiltinQuestions 一致
  "question.builtin.": ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"],
  // 与 internal/engine/build.go 中 provider 模式无法表达的整理设置一致
  "proc.setting.": ["flags", "rename", "keepInfo", "keepDuplicates", "overrides", "sort"],
};

describe("messages", () => {
  const used = new Set<string>();
  const prefixes = new Set<string>();
  for (const f of files(SRC)) {
    const s = readFileSync(f, "utf8");
    for (const m of s.matchAll(/\bt\("([a-z_.0-9]+)"\s*([,)+])/g)) {
      if (m[2] === "+") prefixes.add(m[1]!);
      else used.add(m[1]!);
    }
    // errors.ts 中的映射表
    for (const m of s.matchAll(/:\s*"(error\.[a-z_]+)"/g)) used.add(m[1]!);
  }
  for (const p of prefixes) {
    it(`动态前缀 ${p} 已登记`, () => expect(dynamic[p], p).toBeDefined());
    for (const v of dynamic[p] ?? []) used.add(p + v);
  }

  it("两种语言的键集合一致", () => {
    expect(Object.keys(zh).sort()).toEqual(Object.keys(en).sort());
  });

  it("代码中用到的键都存在", () => {
    const missing = [...used].filter((k) => !(k in en) || !(k in zh));
    expect(missing).toEqual([]);
  });

  it("没有未使用的键", () => {
    const unused = Object.keys(en).filter((k) => !used.has(k));
    expect(unused).toEqual([]);
  });

  it("每条消息都是合法的 ICU MessageFormat", () => {
    for (const [lang, msgs] of [["en", en], ["zh-CN", zh]] as const) {
      for (const [k, v] of Object.entries(msgs)) {
        expect(() => new IntlMessageFormat(v, lang), `${lang} ${k}`).not.toThrow();
      }
    }
  });
});
