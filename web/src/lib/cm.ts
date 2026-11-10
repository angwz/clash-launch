// 原始编辑器（第 16.4 节）：CodeMirror 6 YAML 模式，按需加载。
// 补全：按缩进推断当前所在的映射路径，列出目录中该路径下的键；值的位置补全合并标签。
// 悬浮：键的标题与摘要。实时报错：YAML 语法错误（语义诊断来自后端预览）。
import { autocompletion, closeBrackets, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { yaml } from "@codemirror/lang-yaml";
import { bracketMatching, HighlightStyle, indentOnInput, syntaxHighlighting } from "@codemirror/language";
import { linter, lintGutter, type Diagnostic } from "@codemirror/lint";
import { highlightSelectionMatches, searchKeymap } from "@codemirror/search";
import { EditorState } from "@codemirror/state";
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, hoverTooltip, keymap, lineNumbers, placeholder } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { parseDocument } from "yaml";
import type { MetaField } from "./catalog.svelte";

export interface RawOptions {
  parent: HTMLElement;
  doc: string;
  sections: MetaField[];
  title: (f: MetaField) => string;
  summary: (f: MetaField) => string;
  tags: { tag: string; info: string }[];
  placeholder: string;
  label: string;
  onChange: (text: string) => void;
}

// 颜色全部引用设计 token，主题切换不需要重建编辑器。
const highlight = HighlightStyle.define([
  { tag: tags.propertyName, color: "var(--syn-key)" },
  { tag: [tags.string, tags.special(tags.string)], color: "var(--syn-string)" },
  { tag: [tags.number, tags.integer, tags.float], color: "var(--syn-number)" },
  { tag: [tags.bool, tags.null, tags.keyword], color: "var(--syn-bool)" },
  { tag: [tags.comment, tags.lineComment], color: "var(--syn-comment)", fontStyle: "italic" },
  { tag: [tags.typeName, tags.labelName, tags.meta], color: "var(--syn-tag)" },
  { tag: [tags.punctuation, tags.separator, tags.squareBracket, tags.brace], color: "var(--syn-punct)" },
]);

const theme = EditorView.theme({
  "&": { height: "100%", fontSize: "13px", backgroundColor: "var(--bg-code)", color: "var(--fg)" },
  ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.6", fontVariantLigatures: "none" },
  ".cm-content": { caretColor: "var(--fg)", paddingBlock: "8px" },
  ".cm-gutters": { backgroundColor: "var(--bg-code)", color: "var(--fg-subtle)", border: "none" },
  ".cm-activeLine": { backgroundColor: "color-mix(in srgb, var(--accent) 6%, transparent)" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--fg)" },
  "&.cm-focused": { outline: "none" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": { backgroundColor: "var(--accent-soft) !important" },
  ".cm-tooltip": { border: "1px solid var(--border)", backgroundColor: "var(--bg-raised)", color: "var(--fg)", borderRadius: "8px", boxShadow: "var(--shadow-md)" },
  ".cm-tooltip-autocomplete > ul > li[aria-selected]": { backgroundColor: "var(--bg-active)", color: "var(--fg)" },
  ".cm-completionDetail": { color: "var(--fg-subtle)", fontStyle: "normal", marginInlineStart: "8px" },
  ".cm-hover": { padding: "8px 10px", maxWidth: "380px", fontFamily: "var(--font-sans)", fontSize: "13px" },
  ".cm-hover strong": { display: "block", marginBottom: "2px" },
  ".cm-placeholder": { color: "var(--fg-subtle)" },
  ".cm-diagnostic-error": { borderInlineStartColor: "var(--err)" },
});

/** 根据光标之前的行推断映射路径：逐行向上找缩进更小的「键:」行。列表项（- ）中的键不再深入。 */
export function pathAt(text: string, pos: number): { path: string[]; indent: number } {
  const before = text.slice(0, pos);
  const lines = before.split("\n");
  const cur = lines[lines.length - 1] ?? "";
  let indent = cur.length - cur.trimStart().length;
  const path: string[] = [];
  for (let i = lines.length - 2; i >= 0 && indent > 0; i--) {
    const l = lines[i]!;
    if (!l.trim() || l.trimStart().startsWith("#")) continue;
    const ind = l.length - l.trimStart().length;
    if (ind >= indent) continue;
    const m = /^\s*(- )?([^\s:#'"][^:#]*?):(\s|$)/.exec(l);
    if (!m) break;
    if (m[1]) return { path: [], indent: -1 }; // 列表元素内部：不提供键补全
    path.unshift(m[2]!.trim());
    indent = ind;
  }
  return { path, indent };
}

function fieldsAt(sections: MetaField[], path: string[]): MetaField[] | undefined {
  let fields: MetaField[] | undefined = sections;
  for (const k of path) {
    const f: MetaField | undefined = fields?.find((x) => x.key === k);
    if (!f || f.type !== "object") return undefined;
    fields = f.fields;
  }
  return fields;
}

export function createRawEditor(o: RawOptions): EditorView {
  const keyCompletion = (ctx: CompletionContext): CompletionResult | null => {
    const line = ctx.state.doc.lineAt(ctx.pos);
    const head = line.text.slice(0, ctx.pos - line.from);
    // 值的位置：补全合并标签
    const tagm = /:\s+(!\S*)$/.exec(head);
    if (tagm) {
      return {
        from: ctx.pos - tagm[1]!.length,
        options: o.tags.map((x) => ({ label: x.tag, type: "keyword", info: x.info })),
      };
    }
    const keym = /^(\s*)([\w.-]*)$/.exec(head);
    if (!keym || (!ctx.explicit && keym[2] === "")) return null;
    const { path, indent } = pathAt(ctx.state.doc.toString(), ctx.pos);
    if (indent < 0) return null;
    const fields = fieldsAt(o.sections, path);
    if (!fields) return null;
    return {
      from: ctx.pos - keym[2]!.length,
      options: fields.map((f) => ({ label: f.key, type: "property", detail: o.title(f), info: o.summary(f) || undefined, apply: f.key + ": " })),
      validFor: /^[\w.-]*$/,
    };
  };

  const hover = hoverTooltip((view, pos) => {
    const line = view.state.doc.lineAt(pos);
    const m = /^(\s*)(- )?([\w.-]+):/.exec(line.text);
    if (!m) return null;
    const start = line.from + m[1]!.length + (m[2]?.length ?? 0);
    const end = start + m[3]!.length;
    if (pos < start || pos > end) return null;
    const { path, indent } = pathAt(view.state.doc.toString(), start);
    if (indent < 0) return null;
    const f = fieldsAt(o.sections, path)?.find((x) => x.key === m[3]);
    if (!f) return null;
    return {
      pos: start,
      end,
      above: true,
      create() {
        const dom = document.createElement("div");
        dom.className = "cm-hover";
        const b = document.createElement("strong");
        b.textContent = o.title(f);
        const p = document.createElement("span");
        p.textContent = o.summary(f);
        dom.append(b, p);
        return { dom };
      },
    };
  });

  const syntax = linter((view) => {
    // 合并标签（!append 等）对 yaml 库来说是未知标签，只产生警告；这里只报告语法错误与重复键。
    const doc = parseDocument(view.state.doc.toString(), { uniqueKeys: true, prettyErrors: false });
    const out: Diagnostic[] = [];
    for (const e of doc.errors) {
      const [from, to] = e.pos;
      out.push({ from: Math.min(from, view.state.doc.length), to: Math.min(Math.max(to, from + 1), view.state.doc.length), severity: "error", message: e.message.split("\n")[0]! });
    }
    return out;
  }, { delay: 300 });

  // 挂在 Shadow DOM 中：style-mod 只在 ShadowRoot 上使用可构造样式表（adoptedStyleSheets），
  // 挂在 document 上会插入 <style> 元素，被 CSP 的 style-src 'self' 拦下。设计 token（CSS 变量）可以穿过影子边界继承。
  const shadow = o.parent.shadowRoot ?? o.parent.attachShadow({ mode: "open" });
  const base = new CSSStyleSheet();
  base.replaceSync(":host { display: block; height: 100%; } .wrap { height: 100%; }");
  shadow.adoptedStyleSheets = [base];
  const wrap = document.createElement("div");
  wrap.className = "wrap";
  shadow.replaceChildren(wrap);

  return new EditorView({
    parent: wrap,
    root: shadow,
    state: EditorState.create({
      doc: o.doc,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        history(),
        drawSelection(),
        indentOnInput(),
        bracketMatching(),
        closeBrackets(),
        highlightActiveLine(),
        highlightSelectionMatches(),
        yaml(),
        syntaxHighlighting(highlight),
        autocompletion({ override: [keyCompletion], icons: false }),
        hover,
        syntax,
        lintGutter(),
        placeholder(o.placeholder),
        EditorState.tabSize.of(2),
        EditorView.contentAttributes.of({ "aria-label": o.label, dir: "ltr" }),
        keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap, ...searchKeymap]),
        theme,
        EditorView.updateListener.of((u) => {
          if (u.docChanged) o.onChange(u.state.doc.toString());
        }),
      ],
    }),
  });
}

/** 把光标移到某个顶层键所在的行（从分节菜单进入时定位到该分节）。 */
export function revealKey(view: EditorView, key: string): void {
  const text = view.state.doc.toString();
  const re = new RegExp("^" + key.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + ":", "m");
  const m = re.exec(text);
  const pos = m ? m.index : text.length;
  view.dispatch({ selection: { anchor: pos }, scrollIntoView: true });
  view.focus();
}
