// 站内帮助用的极简 Markdown 解析（反馈 #30）。只支持教程会用到的子集：
//   块：# 到 ### 标题、段落、- 与 1. 列表、``` 代码块、> 提示框
//   行内：`代码`、**加粗**、[文字](链接)
// 输出语法树，由组件逐节点渲染，不经过 innerHTML，因此不需要另做 HTML 转义或净化。
// 内容随前端构建打包，是维护者写的受信任文本；仍只把 http(s) 与站内路径渲染为链接。

export type Inline = { k: "text" | "code" | "bold"; s: string } | { k: "link"; s: string; href: string };

export type Block =
  | { k: "h"; level: 1 | 2 | 3; id: string; text: Inline[] }
  | { k: "p"; text: Inline[] }
  | { k: "ul" | "ol"; items: Inline[][] }
  | { k: "code"; lang: string; text: string }
  | { k: "tip"; text: Inline[] };

const INLINE = /`([^`]+)`|\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/g;

export function inline(src: string): Inline[] {
  const out: Inline[] = [];
  let last = 0;
  for (const m of src.matchAll(INLINE)) {
    if (m.index! > last) out.push({ k: "text", s: src.slice(last, m.index) });
    if (m[1] !== undefined) out.push({ k: "code", s: m[1] });
    else if (m[2] !== undefined) out.push({ k: "bold", s: m[2] });
    else if (/^(https?:\/\/|\/)/.test(m[4]!)) out.push({ k: "link", s: m[3]!, href: m[4]! });
    else out.push({ k: "text", s: m[3]! });
    last = m.index! + m[0].length;
  }
  if (last < src.length) out.push({ k: "text", s: src.slice(last) });
  return out;
}

/** 标题锚点：保留中英文与数字，其余变为连字符。 */
export function slug(s: string): string {
  return s
    .toLowerCase()
    .replace(/[`*[\]()]/g, "")
    .replace(/[^\p{L}\p{N}]+/gu, "-")
    .replace(/^-|-$/g, "");
}

export function parse(src: string): Block[] {
  const lines = src.replace(/\r\n/g, "\n").split("\n");
  const out: Block[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) out.push({ k: "p", text: inline(para.join(" ")) });
    para = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i]!;
    const fence = /^```(\S*)/.exec(l);
    if (fence) {
      flush();
      const body: string[] = [];
      while (++i < lines.length && !lines[i]!.startsWith("```")) body.push(lines[i]!);
      out.push({ k: "code", lang: fence[1] ?? "", text: body.join("\n") });
      continue;
    }
    const h = /^(#{1,3})\s+(.*)$/.exec(l);
    if (h) {
      flush();
      out.push({ k: "h", level: h[1]!.length as 1 | 2 | 3, id: slug(h[2]!), text: inline(h[2]!) });
      continue;
    }
    const li = /^(\s*)(-|\d+\.)\s+(.*)$/.exec(l);
    if (li) {
      flush();
      const kind = li[2] === "-" ? "ul" : "ol";
      const prev = out[out.length - 1];
      const item = inline(li[3]!);
      if (prev && prev.k === kind) prev.items.push(item);
      else out.push({ k: kind, items: [item] });
      continue;
    }
    const q = /^>\s?(.*)$/.exec(l);
    if (q) {
      flush();
      const prev = out[out.length - 1];
      // 连续的 > 行合并为一个提示框
      if (prev && prev.k === "tip" && lines[i - 1]?.startsWith(">")) prev.text.push({ k: "text", s: " " }, ...inline(q[1]!));
      else out.push({ k: "tip", text: inline(q[1]!) });
      continue;
    }
    if (l.trim() === "") flush();
    else para.push(l.trim());
  }
  flush();
  return out;
}
