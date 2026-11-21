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
  "admin.mfa_policy_": ["off", "admins", "all"],
  "check.": ["idle", "running", "passed", "failed", "unavailable", "error"],
  "account.logout_in_": ["1h", "1d", "3d", "7d"],
  "theme.": ["system", "light", "dark"],
  "role.": ["admin", "user"],
  "render.": ["ok", "failed", "pending"],
  "source.kind.": ["single", "batch", "subscription", "file"],
  "source.kind_hint.": ["batch", "subscription"],
  "region.why.": ["flag", "keyword", "route", "manual", "rule", "unknown"],
  "region.kind.": ["flag", "keyword", "route", "manual", "rule", "unknown"],
  "severity.": ["error", "warning", "info"],
  "kernel.": ["passed", "failed", "unavailable", "timeout"],
  "invite.": ["active", "used", "expired", "revoked"],
  "palette.kind.": ["page", "field", "command"],
  "mode.": ["inline", "provider"],
  "stats.page.": ["home", "welcome", "profile", "profile-links", "profile-history", "sources", "source", "rules", "rule", "account", "admin", "help", "settings", "analytics", "short-links", "messages", "message-thread"],
  "analytics.kind.": ["config", "provider", "export", "short", "import"],
  "analytics.range_": ["7", "30", "90", "365"],
  "short.state.": ["active", "disabled", "expired"],
  "profiles.nodes_kind.": ["subscription", "batch"],
  "settings.field_click.": ["card", "text", "button"],
  "settings.field_click_hint.": ["card", "text", "button"],
  "empty_groups.": ["prune", "fallback", "keep"],
  "empty_groups.hint.": ["prune", "fallback", "keep"],
  // 与后端写入的审计动作一致（components/AuditLog.svelte）
  "audit.action.": AUDIT_ACTIONS,
  // 与 internal/auth.BuiltinQuestions 一致
  "question.builtin.": ["0", "1", "2", "3", "4", "5", "6", "7", "8", "9"],
  // 与 internal/engine/build.go 中 provider 模式无法表达的整理设置一致
  "proc.setting.": ["flags", "rename", "keepInfo", "keepDuplicates", "overrides", "sort"],
  // 与 internal/catalog.Guide.Basis 一致
  "guide.basis.": ["official", "practice"],
  "guide.basis_hint.": ["official", "practice"],
  "guide.suggest.": ["official", "practice"],
  // 讲解图层的五步（components/routing/ExplainLayer.svelte）
  "routing.explain.title_": ["1", "2", "3", "4", "5"],
  "routing.explain.body_": ["1", "2", "3", "4", "5"],
  // 分流页（components/routing/）：偏好取值、组类型（只有官方四种）、无法判断的原因（internal/match 与 service）、体检类别（catalog/routing/health.yaml）
  // 智能视图的原因（lib/routing/plan.ts）与内置出口的显示名（lib/routing/labels.ts）
  "routing.why.": ["list", "rules", "folded", "columns", "columns_compact", "unused", "big_groups", "exits_node", "exits_region", "exits_source", "providers", "raw_override", "raw_rows"],
  "routing.exit.": ["direct", "reject", "reject_drop", "pass", "compatible"],
  "routing.seg_": ["sets", "manual"],
  // 分流编辑（components/routing/DetailPanel.svelte、DraftBar.svelte）
  "routing.change.": ["target", "move", "bundle", "unbundle", "added", "removed", "type", "param", "members", "group_added", "group_removed", "check_added", "check_removed", "upstream"],
  // 上游同步（components/routing/UpstreamDialog.svelte）：种类 × 操作
  "routing.upstream.": ["group", "ruleset"].flatMap((k) => ["add", "remove", "modify", "move"].map((o) => k + "_" + o)),
  "routing.lint.": ["single_member", "tolerance_zero", "lazy_stale", "latency_only", "round_robin", "http_probe"],
  "routing.param.": ["url", "interval", "tolerance", "lazy", "strategy"],
  "routing.strategy.": ["consistent-hashing", "round-robin", "sticky-sessions"],
  "routing.type_info.": ["select", "url-test", "fallback", "load-balance"].flatMap((x) => [x + "_tip", x + "_fit", x + "_avoid"]),
  "routing.pref.rules_": ["auto", "all"],
  "routing.pref.particles_": ["all", "focus", "off"],
  "routing.type.": ["select", "url-test", "fallback", "load-balance"],
  "routing.type_char.": ["select", "url-test", "fallback", "load-balance"],
  "routing.reason.": ["logic-ruleset", "sub-rule", "needs-process", "needs-connection", "no-content", "geodata", "parse", "pass", "no-target"],
  "routing.health.category.": ["ai", "search", "video", "social", "dev", "platform", "game", "shop", "cn", "ads"],
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

  // R54：中英文之间的细缝交给 CSS text-autospace，文案里不手写空格，避免「空格 + 细缝」叠加、各处宽窄不一
  it("中文消息里中文与英文、数字之间不手写空格", () => {
    const cjk = "[" + String.fromCharCode(0x4e00) + "-" + String.fromCharCode(0x9fff) + "]";
    const re = new RegExp(cjk + " [A-Za-z0-9{]|[A-Za-z0-9}] " + cjk);
    const bad = Object.entries(zh).filter(([, v]) => re.test(v)).map(([k]) => k);
    expect(bad).toEqual([]);
  });

  it("每条消息都是合法的 ICU MessageFormat", () => {
    for (const [lang, msgs] of [["en", en], ["zh-CN", zh]] as const) {
      for (const [k, v] of Object.entries(msgs)) {
        expect(() => new IntlMessageFormat(v, lang), `${lang} ${k}`).not.toThrow();
      }
    }
  });
});
