// 规则源编辑器（O2，规范第 16 节「规则源」）：CodeMirror 6，toml 与 ini 两种简单的高亮模式，按需加载。
// 也用于节点源内容的大编辑框（反馈 #113）：yaml 模式，内容可能是 Clash YAML、分享链接或 base64，高亮只是辅助。
// 诊断来自后端预览（真实的解析与策略组展开），按行号标在行内；本地只做高亮，不重复实现校验。
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { yaml } from "@codemirror/lang-yaml";
import { HighlightStyle, StreamLanguage, syntaxHighlighting, type StreamParser } from "@codemirror/language";
import { lintGutter, setDiagnostics, type Diagnostic } from "@codemirror/lint";
import { highlightSelectionMatches, searchKeymap } from "@codemirror/search";
import { EditorState, type Extension } from "@codemirror/state";
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers, placeholder } from "@codemirror/view";
import { tags } from "@lezer/highlight";

export type RulesLang = "toml" | "ini" | "yaml";

export interface LineDiagnostic {
  line: number; // 从 1 开始；0 表示无法定位到行（不在行内标出）
  severity: "error" | "warning" | "info";
  message: string;
}

// toml：注释、[[表]] 与 [表]、键、字符串、数字、布尔值。只为可读性着色，不做语法校验。
const tomlParser: StreamParser<{ value: boolean }> = {
  startState: () => ({ value: false }),
  token(stream, state) {
    if (stream.sol()) state.value = false;
    if (stream.eatSpace()) return null;
    if (stream.match("#")) {
      stream.skipToEnd();
      return "comment";
    }
    if (!state.value && stream.match(/^\[\[?[^\]]*\]\]?/)) return "typeName";
    if (!state.value && stream.match(/^[A-Za-z0-9_\-."]+(?=\s*=)/)) return "propertyName";
    if (stream.match("=")) {
      state.value = true;
      return "operator";
    }
    if (stream.match(/^"(?:[^"\\]|\\.)*"?/) || stream.match(/^'[^']*'?/)) return "string";
    if (stream.match(/^(true|false)\b/)) return "bool";
    if (stream.match(/^[+-]?\d[\d_]*(\.\d+)?/)) return "number";
    if (stream.match(/^[[\]{},]/)) return "punctuation";
    stream.next();
    return null;
  },
};

// subconverter 的 ini：; 或 # 注释、[节]、键=值；值里的 ` 是字段分隔符，[] 开头是内联规则
const iniParser: StreamParser<{ value: boolean }> = {
  startState: () => ({ value: false }),
  token(stream, state) {
    if (stream.sol()) {
      state.value = false;
      if (stream.match(/^\s*[;#].*/)) return "comment";
      if (stream.match(/^\s*\[[^\]]*\]\s*$/)) return "typeName";
    }
    if (!state.value) {
      if (stream.match(/^[^=]+(?==)/)) return "propertyName";
      if (stream.match("=")) {
        state.value = true;
        return "operator";
      }
    }
    if (stream.match("`")) return "punctuation";
    if (stream.match(/^\[\][^`,]*/)) return "keyword";
    if (stream.match(/^[^`]+/)) return "string";
    stream.next();
    return null;
  },
};

const highlight = HighlightStyle.define([
  { tag: tags.propertyName, color: "var(--syn-key)" },
  { tag: tags.string, color: "var(--syn-string)" },
  { tag: tags.number, color: "var(--syn-number)" },
  { tag: [tags.bool, tags.keyword], color: "var(--syn-bool)" },
  { tag: tags.comment, color: "var(--syn-comment)", fontStyle: "italic" },
  { tag: tags.typeName, color: "var(--syn-tag)", fontWeight: "600" },
  { tag: [tags.punctuation, tags.operator], color: "var(--syn-punct)" },
]);

const theme = EditorView.theme({
  "&": { fontSize: "13px", backgroundColor: "var(--bg-code)", color: "var(--fg)", border: "1px solid var(--border)", borderRadius: "var(--radius-md)" },
  "&.cm-focused": { outline: "2px solid var(--accent)", outlineOffset: "-1px" },
  ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.6", fontVariantLigatures: "none", minBlockSize: "12em", maxBlockSize: "60vh" },
  ".cm-content": { caretColor: "var(--fg)", paddingBlock: "8px" },
  ".cm-gutters": { backgroundColor: "var(--bg-code)", color: "var(--fg-subtle)", border: "none", borderRadius: "var(--radius-md) 0 0 var(--radius-md)" },
  ".cm-activeLine": { backgroundColor: "color-mix(in srgb, var(--accent) 6%, transparent)" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--fg)" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": { backgroundColor: "var(--accent-soft) !important" },
  ".cm-tooltip": { border: "1px solid var(--border)", backgroundColor: "var(--bg-raised)", color: "var(--fg)", borderRadius: "8px" },
  ".cm-placeholder": { color: "var(--fg-subtle)" },
  ".cm-diagnostic-error": { borderInlineStartColor: "var(--err)" },
  ".cm-diagnostic-warning": { borderInlineStartColor: "var(--warn)" },
});

export interface RulesEditor {
  view: EditorView;
  /** 把后端返回的诊断标在对应的行上。 */
  diagnose(list: LineDiagnostic[]): void;
  /** 滚动到一行并把光标放在行首。 */
  reveal(line: number): void;
  destroy(): void;
}

export function createRulesEditor(opts: { parent: HTMLElement; doc: string; lang: RulesLang; label: string; placeholder: string; onChange: (text: string) => void; fill?: boolean; extensions?: Extension[] }): RulesEditor {
  // 与原始编辑器相同，挂在 Shadow DOM 中：style-mod 在 ShadowRoot 上使用可构造样式表，
  // 挂在 document 上会插入 <style> 元素，被 CSP 的 style-src 'self' 拦下（见 cm.ts）。
  const shadow = opts.parent.shadowRoot ?? opts.parent.attachShadow({ mode: "open" });
  const base = new CSSStyleSheet();
  // fill：撑满宿主元素的高度（大编辑框），内容在编辑器内部滚动
  base.replaceSync(opts.fill ? ":host { display: block; height: 100%; } .cm-editor { height: 100%; } div:has(> .cm-editor) { height: 100%; }" : ":host { display: block; }");
  shadow.adoptedStyleSheets = [base];
  const wrap = document.createElement("div");
  shadow.replaceChildren(wrap);
  const view = new EditorView({
    parent: wrap,
    root: shadow,
    state: EditorState.create({
      doc: opts.doc,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        history(),
        drawSelection(),
        highlightActiveLine(),
        highlightSelectionMatches(),
        opts.lang === "yaml" ? yaml() : StreamLanguage.define(opts.lang === "ini" ? iniParser : tomlParser),
        syntaxHighlighting(highlight),
        lintGutter(),
        placeholder(opts.placeholder),
        keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),
        EditorView.contentAttributes.of({ "aria-label": opts.label, spellcheck: "false" }),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) opts.onChange(u.state.doc.toString());
        }),
        theme,
        ...(opts.extensions ?? []),
      ],
    }),
  });
  return {
    view,
    diagnose(list) {
      const doc = view.state.doc;
      const ds: Diagnostic[] = [];
      for (const d of list) {
        if (d.line < 1 || d.line > doc.lines) continue;
        const line = doc.line(d.line);
        ds.push({ from: line.from, to: Math.max(line.from, line.to), severity: d.severity, message: d.message });
      }
      view.dispatch(setDiagnostics(view.state, ds));
    },
    reveal(lineNo) {
      const doc = view.state.doc;
      if (lineNo < 1 || lineNo > doc.lines) return;
      const line = doc.line(lineNo);
      view.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: "center" }) });
      view.focus();
    },
    destroy: () => view.destroy(),
  };
}
