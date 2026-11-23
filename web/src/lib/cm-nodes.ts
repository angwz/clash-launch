// 节点源内容的编辑扩展（反馈 #114）：在规则源编辑器（cm-rules.ts）的 YAML 模式上加
// 字段与取值的自动提示、YAML 语法检查与右侧缩略图。语义检查（mihomo 能否加载每个节点）来自后端预览，
// 由 NodesEditorDialog 按行标出，这里不重复实现校验。
//
// 自动提示按光标所在的节点推断上下文：先找到所在的列表项（「- 」开头的行），从中读出 type，
// 再按行内的花括号（{ … } 写法）或缩进（块写法）求出嵌套路径与当前是在写键还是写值。
// 只覆盖常见写法；推断不出时不弹出提示，不会给出错误的候选。
import { autocompletion, closeBrackets, type Completion, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { linter, type Diagnostic } from "@codemirror/lint";
import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { showMinimap } from "@replit/codemirror-minimap";
import { parseDocument } from "yaml";
import type { MetaField } from "./catalog.svelte";

export interface NodeFieldsOptions {
  /** 目录中的 proxies 字段（列表，元素按 type 区分协议） */
  proxies: MetaField | undefined;
  title: (f: MetaField) => string;
  summary: (f: MetaField) => string;
  enumLabel: (f: MetaField, v: string) => string;
  variantTitle: (id: string) => string;
}

export interface Context {
  /** 嵌套路径（不含节点本身），如 ["ws-opts"] */
  path: string[];
  mode: "key" | "value";
  key?: string;
  /** 光标前正在输入的片段 */
  word: string;
  type?: string;
  /** 当前映射里已经写过的键（键位置时用来去重） */
  present: string[];
}

const KEY_RE = /^[A-Za-z0-9_-]*$/;

/** 推断光标处的上下文；推断不出时返回 null。导出供单元测试使用。 */
export function contextAt(text: string, pos: number): Context | null {
  const lines = text.slice(0, pos).split("\n");
  const cur = lines[lines.length - 1]!;
  // 找到所在的列表项
  let itemLine = -1;
  let itemIndent = 0;
  for (let i = lines.length - 1; i >= 0; i--) {
    const m = /^(\s*)- /.exec(lines[i]!);
    if (m) {
      itemLine = i;
      itemIndent = m[1]!.length;
      break;
    }
    // 回溯到缩进不大于列表项的非空行（如 proxies:）即停止
    if (i < lines.length - 1 && lines[i]!.trim() && !/^\s/.test(lines[i]!)) break;
  }
  if (itemLine < 0) return null;
  // 列表项的完整文本（读 type 用）：到下一个同级或更浅的行为止
  const all = text.split("\n");
  const end = all.findIndex((l, i) => i > itemLine && l.trim() !== "" && (l.length - l.trimStart().length <= itemIndent));
  const item = all.slice(itemLine, end < 0 ? all.length : end).join("\n");
  const type = /(?:^|[{,\s])type:\s*["']?([A-Za-z0-9_-]+)/.exec(item.replace(/^\s*- /, " "))?.[1];

  // 行内花括号：只在光标所在行内分析（订阅里常见一行一个节点）
  const lineText = itemLine === lines.length - 1 ? cur.replace(/^(\s*)- /, "$1  ") : cur;
  if (lineText.includes("{")) {
    const stack: (string | null)[] = [];
    const keysAt: string[][] = [];
    let token = "";
    let lastKey: string | undefined;
    let expectKey = false;
    let quote = "";
    for (let i = 0; i < lineText.length; i++) {
      const ch = lineText[i]!;
      if (quote) {
        if (ch === quote) quote = "";
        token += ch;
        continue;
      }
      if (ch === '"' || ch === "'") {
        quote = ch;
        token += ch;
      } else if (ch === "{") {
        stack.push(expectKey ? null : (lastKey ?? null));
        keysAt.push([]);
        expectKey = true;
        token = "";
      } else if (ch === "}") {
        stack.pop();
        keysAt.pop();
        token = "";
        expectKey = false;
      } else if (ch === "[") {
        token = "";
      } else if (ch === ",") {
        expectKey = true;
        token = "";
      } else if (ch === ":" && (i + 1 >= lineText.length || lineText[i + 1] === " ") && expectKey) {
        lastKey = token.trim();
        keysAt[keysAt.length - 1]?.push(lastKey);
        expectKey = false;
        token = "";
      } else token += ch;
    }
    if (!stack.length || quote) return null;
    const path = stack.slice(1).filter((x): x is string => !!x);
    const word = token.trimStart();
    if (expectKey) return KEY_RE.test(word) ? { path, mode: "key", word, type, present: keysAt[keysAt.length - 1] ?? [] } : null;
    return /^[^,{}[\]]*$/.test(word) ? { path, mode: "value", key: lastKey, word: word.trim(), type, present: [] } : null;
  }

  // 块写法：按缩进找父键
  const indentOf = (l: string) => l.length - l.trimStart().length;
  const body = lineText;
  const keyIndent = itemIndent + 2;
  const curIndent = indentOf(body);
  const km = /^\s*([A-Za-z0-9_-]+):\s*(.*)$/.exec(body);
  const path: string[] = [];
  let need = curIndent;
  for (let i = lines.length - 2; i >= itemLine && need > keyIndent; i--) {
    const l = i === itemLine ? lines[i]!.replace(/^(\s*)- /, "$1  ") : lines[i]!;
    if (!l.trim()) continue;
    const ind = indentOf(l);
    if (ind < need) {
      const m = /^\s*([A-Za-z0-9_-]+):\s*$/.exec(l);
      if (!m) return null; // 父级不是「键:」形式（如列表），不提示
      path.unshift(m[1]!);
      need = ind;
    }
  }
  // 同一映射里已写的键
  const present: string[] = [];
  for (let i = itemLine; i < lines.length - 1; i++) {
    const l = i === itemLine ? lines[i]!.replace(/^(\s*)- /, "$1  ") : lines[i]!;
    const m = /^(\s*)([A-Za-z0-9_-]+):/.exec(l);
    if (m && m[1]!.length === curIndent) present.push(m[2]!);
  }
  if (km) {
    if (/[{["']/.test(km[2]!)) return null;
    return { path, mode: "value", key: km[1], word: km[2]!.trim(), type, present: [] };
  }
  const word = body.trim();
  return KEY_RE.test(word) ? { path, mode: "key", word, type, present } : null;
}

/** 某个节点类型在 path 处可写的字段。 */
function fieldsFor(o: NodeFieldsOptions, type: string | undefined, path: string[]): MetaField[] {
  const item = o.proxies?.items;
  const variants = item?.variants ?? [];
  let fields: MetaField[] | undefined;
  if (type) fields = variants.find((v) => v.value === type)?.fields;
  if (!fields) {
    // 还没写 type：只给出所有协议共有的字段
    const first = variants[0]?.fields ?? [];
    fields = first.filter((f) => variants.every((v) => v.fields.some((g) => g.key === f.key)));
  }
  for (const k of path) {
    const f: MetaField | undefined = fields.find((x) => x.key === k);
    fields = f?.fields;
    if (!fields) return [];
  }
  return fields;
}

export function nodeCompletion(o: NodeFieldsOptions) {
  return (ctx: CompletionContext): CompletionResult | null => {
    const c = contextAt(ctx.state.doc.toString(), ctx.pos);
    if (!c) return null;
    if (!ctx.explicit && c.mode === "key" && !c.word) return null;
    const from = ctx.pos - c.word.length;
    if (c.mode === "key") {
      const fields = fieldsFor(o, c.type, c.path).filter((f) => !c.present.includes(f.key));
      if (!fields.length) return null;
      return {
        from,
        validFor: KEY_RE,
        options: fields.map((f) => ({ label: f.key, type: "property", detail: o.title(f), info: o.summary(f) || undefined, apply: f.key + ": " })),
      };
    }
    // 值的位置
    let options: Completion[] = [];
    if (c.path.length === 0 && c.key === "type") {
      options = (o.proxies?.items?.variants ?? []).map((v) => ({ label: v.value, type: "enum", detail: o.variantTitle(v.id) }));
    } else {
      const f = fieldsFor(o, c.type, c.path).find((x) => x.key === c.key);
      if (!f) return null;
      if (f.enum?.length) options = f.enum.map((v) => ({ label: v, type: "enum", detail: o.enumLabel(f, v) || undefined }));
      else if (f.type === "bool") options = ["true", "false"].map((v) => ({ label: v, type: "keyword" }));
      else if (f.examples?.length) options = f.examples.filter((v) => typeof v === "string" || typeof v === "number").map((v) => ({ label: String(v), type: "text" }));
    }
    if (!options.length) return null;
    return { from, validFor: /^[A-Za-z0-9_.-]*$/, options };
  };
}

/** 内容看起来是 YAML（而不是分享链接或 base64）时才做语法检查。 */
export function looksLikeYaml(text: string): boolean {
  const first = text.split("\n").find((l) => l.trim() && !l.trimStart().startsWith("#"));
  return !!first && /^\s*(proxies\s*:|- |\{|[A-Za-z0-9_-]+\s*:)/.test(first) && !/^\s*[a-z0-9]+:\/\//i.test(first);
}

/** YAML 语法错误（与后端解析同一个库的同类错误；语义问题由后端预览给出）。 */
export function yamlErrors(text: string): { from: number; to: number; message: string }[] {
  if (!looksLikeYaml(text)) return [];
  const doc = parseDocument(text, { prettyErrors: false });
  return doc.errors.map((e) => {
    const [from, to] = e.pos;
    return { from: Math.min(from, text.length), to: Math.min(Math.max(to, from + 1), text.length), message: e.message.split("\n")[0]! };
  });
}

export function nodeExtensions(o: NodeFieldsOptions): Extension[] {
  return [
    closeBrackets(),
    autocompletion({ override: [nodeCompletion(o)], icons: false, activateOnTyping: true }),
    linter((view): Diagnostic[] => yamlErrors(view.state.doc.toString()).map((e) => ({ ...e, severity: "error" as const })), { delay: 300 }),
    // 右侧缩略图（类似 VS Code）：按语法着色的色块，叠层显示当前可见区域，可拖动
    showMinimap.compute(["doc"], () => ({
      create: () => ({ dom: document.createElement("div") }),
      displayText: "blocks",
      showOverlay: "always",
    })),
    // 大编辑框里撑满高度，不受规则源编辑器 60vh 上限的限制
    EditorView.theme({
      "&": { height: "100%" },
      ".cm-scroller": { maxBlockSize: "none", height: "100%" },
      ".cm-minimap-gutter": { borderInlineStart: "1px solid var(--border)", backgroundColor: "var(--bg-code)" },
      ".cm-minimap-overlay": { backgroundColor: "var(--fg-subtle)", opacity: "0.12" },
      ".cm-tooltip-autocomplete ul li[aria-selected]": { backgroundColor: "var(--accent-soft)", color: "var(--fg)" },
      ".cm-completionDetail": { marginInlineStart: "8px", color: "var(--fg-muted)", fontStyle: "normal" },
      ".cm-completionInfo": { maxInlineSize: "320px", padding: "6px 8px", fontSize: "12px", lineHeight: "1.5" },
    }),
  ];
}
