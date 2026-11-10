// 只读 YAML 的轻量高亮：逐行切分为 token，界面用文本节点渲染（不使用 innerHTML）。
// 只服务于预览显示，不参与任何语义判断；可编辑的场景使用 CodeMirror。
export type Tok = { k: "key" | "str" | "num" | "bool" | "com" | "tag" | "pun" | "txt"; s: string };

const scalarKind = (v: string): Tok["k"] => {
  const s = v.trim();
  if (s === "") return "txt";
  if (/^(true|false|null|~)$/i.test(s)) return "bool";
  if (/^[-+]?(\d[\d_]*)(\.\d+)?([eE][-+]?\d+)?$/.test(s) || /^0x[0-9a-fA-F]+$/.test(s)) return "num";
  if (/^['"]/.test(s)) return "str";
  return "txt";
};

/** 在不处于引号中的位置寻找注释起点（空白后的 #）。 */
function commentAt(line: string): number {
  let q: string | null = null;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (q) {
      if (c === q && !(q === '"' && line[i - 1] === "\\")) q = null;
    } else if (c === '"' || c === "'") q = c;
    else if (c === "#" && (i === 0 || line[i - 1] === " " || line[i - 1] === "\t")) return i;
  }
  return -1;
}

export function highlightLine(line: string): Tok[] {
  const out: Tok[] = [];
  const ci = commentAt(line);
  const body = ci >= 0 ? line.slice(0, ci) : line;
  const comment = ci >= 0 ? line.slice(ci) : "";

  const m = /^(\s*)(- )?((?:[^\s'"#{}[\],:][^:#]*?|'[^']*'|"(?:[^"\\]|\\.)*"))(:)(\s|$)(.*)$/.exec(body);
  if (m) {
    const [, ind, dash, key, colon, sp, rest] = m;
    if (ind) out.push({ k: "txt", s: ind });
    if (dash) out.push({ k: "pun", s: dash });
    out.push({ k: "key", s: key! });
    out.push({ k: "pun", s: colon! });
    if (sp) out.push({ k: "txt", s: sp });
    pushValue(out, rest ?? "");
  } else {
    const d = /^(\s*)(- )(.*)$/.exec(body);
    if (d) {
      if (d[1]) out.push({ k: "txt", s: d[1] });
      out.push({ k: "pun", s: d[2]! });
      pushValue(out, d[3] ?? "");
    } else pushValue(out, body);
  }
  if (comment) out.push({ k: "com", s: comment });
  return out;
}

function pushValue(out: Tok[], v: string): void {
  if (!v) return;
  const tag = /^(![^\s]*)(\s*)(.*)$/.exec(v);
  if (tag) {
    out.push({ k: "tag", s: tag[1]! });
    if (tag[2]) out.push({ k: "txt", s: tag[2] });
    v = tag[3] ?? "";
    if (!v) return;
  }
  if (/^[[{]/.test(v.trim())) {
    // 流式集合：只区分标点与字符串，避免逐项解析
    const parts = v.split(/([[\]{},])/);
    for (const p of parts) {
      if (!p) continue;
      if (/^[[\]{},]$/.test(p)) out.push({ k: "pun", s: p });
      else {
        const kv = /^(\s*)([^:'"]+?)(:\s)(.*)$/.exec(p);
        if (kv) {
          if (kv[1]) out.push({ k: "txt", s: kv[1] });
          out.push({ k: "key", s: kv[2]! });
          out.push({ k: "pun", s: kv[3]! });
          out.push({ k: scalarKind(kv[4]!), s: kv[4]! });
        } else out.push({ k: scalarKind(p), s: p });
      }
    }
    return;
  }
  out.push({ k: scalarKind(v), s: v });
}
