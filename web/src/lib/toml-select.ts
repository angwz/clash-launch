// groups.toml 中 [custom_groups.select] 的读取与改写（规范第 6.4 节，O2 选择器标签）。
//
// 编辑器把选择条件做成可点选的标签；保存回 TOML 原文时只改动用户改过的键：
// 未改动的键、注释、空行与其他组逐字保留，改动的键保留行尾注释。
// 这里只实现 select 表用到的 TOML 子集（字符串、数组、内联表、数字）；遇到不认识的写法
// （点号键、多行字符串、[custom_groups.select.exclude] 子表等）时整组只读，并说明原因，不猜。

export const LIST_KEYS = ["region", "source", "protocol", "tags"] as const;
export type ListKey = (typeof LIST_KEYS)[number];
export const SORTS = ["source", "name", "rate"] as const;

export interface Conditions {
  region: string[];
  source: string[];
  protocol: string[];
  tags: string[];
  name: string; // 空字符串表示不限
}

export interface Select extends Conditions {
  rateMin: number | null;
  rateMax: number | null;
  sort: string; // 空字符串表示默认（source）
  exclude: Conditions;
}

export interface Group {
  name: string;
  /** 只读的原因：inline（select 写成内联表）| complex（含不支持的写法）；可编辑时为空 */
  readonly: "" | "inline" | "complex";
  detail?: string; // complex 时的具体原因（英文，来自解析器）
}

export const emptyConditions = (): Conditions => ({ region: [], source: [], protocol: [], tags: [], name: "" });
export const emptySelect = (): Select => ({ ...emptyConditions(), rateMin: null, rateMax: null, sort: "", exclude: emptyConditions() });

// —— 行与组的切分 ——

interface Block {
  name: string;
  start: number; // [[custom_groups]] 所在行
  end: number; // 下一个不属于本组的表头所在行（不含）
  inlineSelect: boolean;
  selectHeader: number; // [custom_groups.select] 所在行；没有时为 -1
  selectEnd: number; // select 表结束行（不含）
  subTable: boolean; // 有 [custom_groups.select.*] 子表
}

const headerOf = (line: string): { name: string; array: boolean } | null => {
  const s = line.trim();
  const m = /^(\[\[?)\s*([^\]]+?)\s*\]\]?\s*(#.*)?$/.exec(s);
  if (!m) return null;
  return { name: m[2]!.replace(/\s*\.\s*/g, "."), array: m[1] === "[[" };
};

function blocks(text: string): Block[] {
  const lines = text.split("\n");
  const out: Block[] = [];
  let cur: Block | null = null;
  let inMain = false; // 是否在组的主表（第一个子表头之前）
  let inSelect = false;
  const close = (at: number) => {
    if (!cur) return;
    if (inSelect) cur.selectEnd = at;
    cur.end = at;
    out.push(cur);
    cur = null;
  };
  lines.forEach((line, i) => {
    const h = headerOf(line);
    if (h) {
      if (h.array && h.name === "custom_groups") {
        close(i);
        cur = { name: "", start: i, end: lines.length, inlineSelect: false, selectHeader: -1, selectEnd: -1, subTable: false };
        inMain = true;
        inSelect = false;
        return;
      }
      if (cur && h.name.startsWith("custom_groups.")) {
        if (inSelect) cur.selectEnd = i;
        inMain = false;
        inSelect = !h.array && h.name === "custom_groups.select";
        if (inSelect) cur.selectHeader = i;
        if (h.name.startsWith("custom_groups.select.")) cur.subTable = true;
        return;
      }
      close(i);
      inMain = inSelect = false;
      return;
    }
    if (!cur || !inMain) return;
    const kv = /^\s*("?)(name|select)\1\s*=\s*(.*)$/.exec(line);
    if (!kv) return;
    if (kv[2] === "select") {
      (cur as Block).inlineSelect = true;
      return;
    }
    try {
      const v = new Parser(kv[3]!).value();
      if (typeof v === "string") (cur as Block).name = v;
    } catch {
      /* 名字写法不认识：该组在列表中显示为空名，由服务器诊断报告 */
    }
  });
  close(lines.length);
  return out;
}

// —— TOML 值的子集解析 ——

type Value = string | number | boolean | Value[] | { [k: string]: Value };

class Parser {
  p = 0;
  constructor(readonly s: string) {}
  fail(msg: string): never {
    throw new Error(msg);
  }
  ws(newlines: boolean) {
    for (;;) {
      const c = this.s[this.p];
      if (c === " " || c === "\t" || c === "\r") this.p++;
      else if (newlines && c === "\n") this.p++;
      else if (newlines && c === "#") while (this.p < this.s.length && this.s[this.p] !== "\n") this.p++;
      else return;
    }
  }
  key(): string {
    const c = this.s[this.p];
    if (c === '"' || c === "'") return this.str();
    const m = /^[A-Za-z0-9_-]+/.exec(this.s.slice(this.p));
    if (!m) this.fail("expected a key");
    this.p += m[0].length;
    return m[0];
  }
  str(): string {
    const q = this.s[this.p]!;
    if (this.s.startsWith(q.repeat(3), this.p)) this.fail("multi-line strings are not supported");
    this.p++;
    let out = "";
    for (;;) {
      const c = this.s[this.p++];
      if (c === undefined || c === "\n") this.fail("unterminated string");
      if (c === q) return out;
      if (c === "\\" && q === '"') {
        const e = this.s[this.p++];
        const simple: Record<string, string> = { b: "\b", t: "\t", n: "\n", f: "\f", r: "\r", '"': '"', "\\": "\\" };
        if (e !== undefined && e in simple) out += simple[e];
        else if (e === "u" || e === "U") {
          const n = e === "u" ? 4 : 8;
          const hex = this.s.slice(this.p, this.p + n);
          if (!/^[0-9A-Fa-f]+$/.test(hex) || hex.length !== n) this.fail("bad escape");
          out += String.fromCodePoint(parseInt(hex, 16));
          this.p += n;
        } else this.fail("bad escape");
      } else out += c;
    }
  }
  value(): Value {
    this.ws(false);
    const c = this.s[this.p];
    if (c === '"' || c === "'") return this.str();
    if (c === "[") {
      this.p++;
      const arr: Value[] = [];
      for (;;) {
        this.ws(true);
        if (this.s[this.p] === "]") {
          this.p++;
          return arr;
        }
        arr.push(this.value());
        this.ws(true);
        if (this.s[this.p] === ",") this.p++;
        else if (this.s[this.p] !== "]") this.fail("expected , or ]");
      }
    }
    if (c === "{") {
      this.p++;
      const obj: { [k: string]: Value } = {};
      this.ws(false);
      if (this.s[this.p] === "}") {
        this.p++;
        return obj;
      }
      for (;;) {
        this.ws(false);
        const k = this.key();
        this.ws(false);
        if (this.s[this.p] === ".") this.fail("dotted keys are not supported");
        if (this.s[this.p++] !== "=") this.fail("expected =");
        obj[k] = this.value();
        this.ws(false);
        const d = this.s[this.p++];
        if (d === "}") return obj;
        if (d !== ",") this.fail("expected , or }");
      }
    }
    const m = /^(true|false|[+-]?[0-9][0-9_]*(\.[0-9_]+)?([eE][+-]?[0-9]+)?)/.exec(this.s.slice(this.p));
    if (!m) this.fail("unsupported value");
    this.p += m[0].length;
    if (m[0] === "true" || m[0] === "false") return m[0] === "true";
    return Number(m[0].replace(/_/g, ""));
  }
}

// —— select 表的条目 ——

interface Entry {
  key: string;
  value: Value;
  from: number; // 在 select 表正文中的起点（行首）
  to: number; // 值结束处（行尾注释之前）
  comment: string; // 行尾注释（含 #），没有时为空
  eol: number; // 该条目最后一行的行尾（换行符位置或文本末尾）
}

function entries(body: string): Entry[] {
  const ps = new Parser(body);
  const out: Entry[] = [];
  for (;;) {
    ps.ws(true);
    if (ps.p >= body.length) return out;
    const from = body.lastIndexOf("\n", ps.p - 1) + 1;
    const key = ps.key();
    ps.ws(false);
    if (body[ps.p] === ".") ps.fail("dotted keys are not supported");
    if (body[ps.p++] !== "=") ps.fail("expected =");
    const value = ps.value();
    const to = ps.p;
    ps.ws(false);
    let comment = "";
    if (body[ps.p] === "#") {
      const e = body.indexOf("\n", ps.p);
      comment = body.slice(ps.p, e < 0 ? body.length : e);
      ps.p += comment.length;
    }
    if (ps.p < body.length && body[ps.p] !== "\n") ps.fail("unexpected text after value");
    out.push({ key, value, from, to, comment, eol: ps.p });
  }
}

const isStrList = (v: Value): v is string[] => Array.isArray(v) && v.every((x) => typeof x === "string");

function conditionsFrom(src: { [k: string]: Value }, into: Conditions, top: boolean) {
  for (const [k, v] of Object.entries(src)) {
    if ((LIST_KEYS as readonly string[]).includes(k)) {
      if (typeof v === "string") into[k as ListKey] = [v];
      else if (isStrList(v)) into[k as ListKey] = [...v];
      else throw new Error(k + " must be a string or an array of strings");
    } else if (k === "name") {
      if (typeof v !== "string") throw new Error("name must be a string");
      into.name = v;
    } else if (!top) throw new Error("unsupported key in exclude: " + k);
  }
}

function selectFrom(es: Entry[]): Select {
  const sel = emptySelect();
  const top: { [k: string]: Value } = {};
  for (const e of es) top[e.key] = e.value;
  conditionsFrom(top, sel, true);
  for (const e of es) {
    if (e.key === "rate") {
      const r = e.value;
      if (typeof r !== "object" || Array.isArray(r)) throw new Error("rate must be a table");
      for (const [k, v] of Object.entries(r)) {
        if (typeof v !== "number" || (k !== "min" && k !== "max")) throw new Error("rate accepts numeric min and max");
        if (k === "min") sel.rateMin = v;
        else sel.rateMax = v;
      }
    } else if (e.key === "sort") {
      if (typeof e.value !== "string") throw new Error("sort must be a string");
      sel.sort = e.value;
    } else if (e.key === "exclude") {
      const x = e.value;
      if (typeof x !== "object" || Array.isArray(x)) throw new Error("exclude must be a table");
      conditionsFrom(x, sel.exclude, false);
    }
  }
  return sel;
}

// —— 序列化 ——

function basic(s: string): string {
  let out = '"';
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (ch === '"' || ch === "\\") out += "\\" + ch;
    else if (ch === "\n") out += "\\n";
    else if (ch === "\t") out += "\\t";
    else if (c < 0x20 || c === 0x7f) out += "\\" + "u" + c.toString(16).padStart(4, "0");
    else out += ch;
  }
  return out + '"';
}
// 正则用字面量字符串（单引号）书写，反斜杠不必转义，与手写习惯一致；含单引号或控制字符时退回基本字符串
const hasCtrl = (s: string) => [...s].some((ch) => {
  const n = ch.codePointAt(0)!;
  return n < 0x20 || n === 0x7f;
});
const regex = (s: string) => (s.includes("'") || hasCtrl(s) ? basic(s) : "'" + s + "'");
const list = (xs: string[]) => "[" + xs.map(basic).join(", ") + "]";

function conditionsText(c: Conditions): string {
  const parts: string[] = [];
  for (const k of LIST_KEYS) if (c[k].length) parts.push(k + " = " + list(c[k]));
  if (c.name) parts.push("name = " + regex(c.name));
  return "{ " + parts.join(", ") + " }";
}

const condEmpty = (c: Conditions) => LIST_KEYS.every((k) => !c[k].length) && !c.name;
const same = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);
const condSame = (a: Conditions, b: Conditions) => LIST_KEYS.every((k) => same(a[k], b[k])) && a.name === b.name;

/** 每个键的目标文本；null 表示该键应不存在。 */
function wanted(sel: Select): Record<string, string | null> {
  const w: Record<string, string | null> = {};
  for (const k of LIST_KEYS) w[k] = sel[k].length ? list(sel[k]) : null;
  w.name = sel.name ? regex(sel.name) : null;
  const r: string[] = [];
  if (sel.rateMin !== null) r.push("min = " + String(sel.rateMin));
  if (sel.rateMax !== null) r.push("max = " + String(sel.rateMax));
  w.rate = r.length ? "{ " + r.join(", ") + " }" : null;
  w.exclude = condEmpty(sel.exclude) ? null : conditionsText(sel.exclude);
  w.sort = sel.sort ? basic(sel.sort) : null;
  return w;
}

/** 键在新旧两份条件中语义是否相同（相同则保留原文）。 */
function unchanged(key: string, a: Select, b: Select): boolean {
  if ((LIST_KEYS as readonly string[]).includes(key)) return same(a[key as ListKey], b[key as ListKey]);
  if (key === "name") return a.name === b.name;
  if (key === "rate") return a.rateMin === b.rateMin && a.rateMax === b.rateMax;
  if (key === "sort") return a.sort === b.sort;
  if (key === "exclude") return condSame(a.exclude, b.exclude);
  return true;
}

const ORDER = ["region", "source", "protocol", "tags", "name", "rate", "exclude", "sort"];

// —— 对外接口 ——

/** 列出 groups.toml 中的组及其是否可编辑。 */
export function listGroups(text: string): Group[] {
  return blocks(text).map((b) => {
    if (b.inlineSelect) return { name: b.name, readonly: "inline" };
    if (b.subTable) return { name: b.name, readonly: "complex", detail: "[custom_groups.select.*] sub-tables" };
    try {
      if (b.selectHeader >= 0) selectFrom(entries(body(text, b)));
      return { name: b.name, readonly: "" };
    } catch (e) {
      return { name: b.name, readonly: "complex", detail: (e as Error).message };
    }
  });
}

function body(text: string, b: Block): string {
  return text
    .split("\n")
    .slice(b.selectHeader + 1, b.selectEnd)
    .join("\n");
}

/** 读取第 i 个组的选择条件；没有 select 表时返回空条件。只读的组抛出错误。 */
export function readSelect(text: string, i: number): Select {
  const b = blocks(text)[i];
  if (!b) throw new Error("no such group");
  if (b.inlineSelect || b.subTable) throw new Error("read-only group");
  return b.selectHeader >= 0 ? selectFrom(entries(body(text, b))) : emptySelect();
}

/**
 * 把第 i 个组的选择条件改为 sel，返回新文本。只改动与原条件不同的键：
 * 改动的键原位替换（多行数组收为一行，保留行尾注释）；删除的键连同所在行删除；
 * 新增的键追加在最后一个条目之后。没有 select 表时在组末尾新建；条件全部清空时连表头一起删除
 * （空的 select 表会选中全部节点，与「不限」的本意不同）。
 */
export function writeSelect(text: string, i: number, sel: Select): string {
  const b = blocks(text)[i];
  if (!b) throw new Error("no such group");
  if (b.inlineSelect || b.subTable) throw new Error("read-only group");
  const lines = text.split("\n");
  const w = wanted(sel);
  const anyWanted = Object.values(w).some((v) => v !== null);

  if (b.selectHeader < 0) {
    if (!anyWanted) return text;
    // 组末尾的空行留在新表之后，与原来的组间距一致
    let at = b.end;
    while (at > b.start + 1 && lines[at - 1]!.trim() === "") at--;
    const add = ["", "[custom_groups.select]", ...ORDER.filter((k) => w[k] !== null).map((k) => k + " = " + w[k])];
    lines.splice(at, 0, ...add);
    return lines.join("\n");
  }

  const src = body(text, b);
  const es = entries(src);
  const old = selectFrom(es);
  // 按行重建 select 表正文：条目之外的行（注释、空行）原样保留
  const bl = src.split("\n");
  const lineAt = (off: number) => src.slice(0, off).split("\n").length - 1;
  const spans = es.map((e) => ({ e, a: lineAt(e.from), z: lineAt(e.eol) }));
  const added = ORDER.filter((k) => !es.some((e) => e.key === k) && w[k] !== null).map((k) => k + " = " + w[k]);
  const out: string[] = [];
  if (!es.length) out.push(...added);
  let n = 0; // 保留下来的条目数
  for (let li = 0; li < bl.length; li++) {
    const sp = spans.find((s) => s.a === li);
    if (!sp) {
      out.push(bl[li]!);
      continue;
    }
    const { e } = sp;
    li = sp.z;
    const v = w[e.key];
    if (!ORDER.includes(e.key) || unchanged(e.key, old, sel)) {
      out.push(...bl.slice(sp.a, sp.z + 1));
      n++;
    } else if (v !== null && v !== undefined) {
      const indent = /^[ \t]*/.exec(bl[sp.a]!)![0];
      out.push(indent + e.key + " = " + v + (e.comment ? " " + e.comment : ""));
      n++;
    }
    if (sp === spans[spans.length - 1]) out.push(...added);
  }
  n += added.length;
  const head = lines.slice(0, b.selectHeader);
  const rest = lines.slice(b.selectEnd);
  if (n === 0) {
    // 条件全部清空：删除表头；正文只剩空行时一并删除，表头前的空行也去掉
    const keep = out.some((l) => l.trim() !== "") ? out : [];
    if (!keep.length && head.length && head[head.length - 1]!.trim() === "") head.pop();
    if (!keep.length && out.length && out[out.length - 1]!.trim() === "" && head.length) keep.push("");
    return [...head, ...keep, ...rest].join("\n");
  }
  return [...head, lines[b.selectHeader]!, ...out, ...rest].join("\n");
}
