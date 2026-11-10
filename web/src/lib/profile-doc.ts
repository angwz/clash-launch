// 方案文档（clash-launch/profile v1）的读写。文档是方案的权威形式（第 4.1 节）。
//
// 前端只做结构操作：读取、设置、删除 form 中的一个路径，调整 sources 的顺序与写法，替换 raw 原文。
// 合并、校验与渲染全部由后端的预览接口完成（第 16.6 节）。
// 使用 yaml 的 Document API：未改动的部分（包括注释与 raw 块）保持原样，避免「打开再保存」产生无关差异。
import { Document, isMap, isScalar, isSeq, parseDocument, Scalar, YAMLMap, YAMLSeq } from "yaml";

export type Mode = "inline" | "provider";

export interface SourceRef {
  ref: string;
  mode?: Mode;
  disabled?: boolean;
}

export interface Target {
  pack: string;
  preset: string;
  core: string;
}

const HEADER = "clash-launch/profile: v1";

export class ProfileDoc {
  readonly doc: Document;
  // 未修改时 toString 返回原文：yaml 的输出会规整注释前的空白，原样返回可避免「打开再保存」产生差异。
  #orig: string;
  #dirty = false;

  constructor(text: string) {
    this.#orig = text;
    this.doc = parseDocument(text, { prettyErrors: true, uniqueKeys: true });
    if (this.doc.errors.length) throw new Error(this.doc.errors[0]!.message);
  }

  static create(name: string, target: Target, form?: Record<string, unknown>): ProfileDoc {
    const d = new ProfileDoc(HEADER + "\n");
    d.name = name;
    const t = d.doc.createNode({ pack: target.pack, preset: target.preset, core: target.core }) as YAMLMap;
    t.flow = true;
    d.doc.set("target", t);
    if (form && Object.keys(form).length) d.doc.set("form", d.doc.createNode(form));
    return d;
  }

  get dirty(): boolean {
    return this.#dirty;
  }

  toString(): string {
    if (!this.#dirty) return this.#orig;
    // lineWidth 0：不折行，长字符串保持在一行（与后端渲染器的约定一致）
    return this.doc.toString({ lineWidth: 0, indent: 2, indentSeq: true, flowCollectionPadding: true });
  }

  get name(): string {
    return String(this.doc.get("name") ?? "");
  }

  set name(v: string) {
    this.#dirty = true;
    this.doc.set("name", v);
  }

  get target(): Target {
    const t = this.doc.get("target");
    const m = isMap(t) ? (t.toJSON() as Partial<Target>) : {};
    return { pack: m.pack ?? "mihomo", preset: m.preset ?? "mihomo", core: String(m.core ?? "") };
  }

  // —— 表单层 ——

  /** 读取 form 中某个路径的值（转换为普通 JS 值）；未设置时返回 undefined。 */
  getForm(path: string[]): unknown {
    const n = this.doc.getIn(["form", ...path], true);
    if (n === undefined || n === null) return undefined;
    if (isScalar(n)) return n.value;
    if (isMap(n) || isSeq(n)) return n.toJSON();
    return n;
  }

  hasForm(path: string[]): boolean {
    return this.doc.hasIn(["form", ...path]);
  }

  /** 设置一个路径；中间的映射按需创建。短的标量列表用流式，便于阅读。 */
  setForm(path: string[], value: unknown): void {
    this.#dirty = true;
    if (!isMap(this.doc.get("form", true))) this.doc.set("form", new YAMLMap());
    const node = this.doc.createNode(value);
    if (node instanceof YAMLSeq && node.items.every((x) => isScalar(x)) && node.items.length <= 8) node.flow = true;
    this.doc.setIn(["form", ...path], node);
  }

  /** 删除一个路径（恢复默认）；删除后留空的上级映射一并删除，form 为空时删除 form。 */
  deleteForm(path: string[]): void {
    this.#dirty = true;
    this.doc.deleteIn(["form", ...path]);
    for (let i = path.length - 1; i >= 0; i--) {
      const parent = this.doc.getIn(["form", ...path.slice(0, i)], true);
      if (isMap(parent) && parent.items.length === 0) {
        if (i === 0) this.doc.delete("form");
        else this.doc.deleteIn(["form", ...path.slice(0, i)]);
      } else break;
    }
  }

  /** form 中已设置的全部叶子路径（映射逐层展开，列表与标量视为叶子）。 */
  formPaths(): string[][] {
    const out: string[][] = [];
    const walk = (n: unknown, path: string[]) => {
      if (isMap(n)) {
        for (const it of n.items) {
          const k = isScalar(it.key) ? String(it.key.value) : String(it.key);
          walk(it.value, [...path, k]);
        }
      } else out.push(path);
    };
    walk(this.doc.get("form", true), []);
    return out.filter((p) => p.length > 0);
  }

  // —— 源 ——

  get sources(): SourceRef[] {
    const s = this.doc.get("sources", true);
    if (!isSeq(s)) return [];
    return (s.toJSON() as SourceRef[]).map((x) => ({ ref: String(x.ref), mode: x.mode, disabled: x.disabled }));
  }

  set sources(list: SourceRef[]) {
    this.#dirty = true;
    if (list.length === 0) {
      this.doc.delete("sources");
      return;
    }
    const seq = new YAMLSeq();
    for (const s of list) {
      const m = this.doc.createNode(clean({ ref: s.ref, mode: s.mode === "provider" ? "provider" : undefined, disabled: s.disabled || undefined })) as YAMLMap;
      m.flow = true;
      seq.items.push(m);
    }
    this.doc.set("sources", seq);
  }

  get rules(): string | undefined {
    const r = this.doc.getIn(["rules", "ref"]);
    return r === undefined || r === null ? undefined : String(r);
  }

  set rules(ref: string | undefined) {
    this.#dirty = true;
    if (!ref) {
      this.doc.delete("rules");
      return;
    }
    const m = this.doc.createNode({ ref }) as YAMLMap;
    m.flow = true;
    this.doc.set("rules", m);
  }

  // —— 输出格式 ——

  /** output.proxies-block：节点用多行块格式（与官方示例一致）；默认每个节点一行。 */
  get proxiesBlock(): boolean {
    return this.doc.getIn(["output", "proxies-block"]) === true;
  }

  set proxiesBlock(v: boolean) {
    this.#dirty = true;
    if (v) {
      if (!this.doc.has("output")) this.doc.set("output", this.doc.createNode({}));
      this.doc.setIn(["output", "proxies-block"], true);
      return;
    }
    if (this.doc.hasIn(["output", "proxies-block"])) this.doc.deleteIn(["output", "proxies-block"]);
  }

  // —— 原始编辑（后备隐藏能源）——

  get raw(): string {
    const r = this.doc.get("raw");
    return typeof r === "string" ? r : "";
  }

  set raw(text: string) {
    this.#dirty = true;
    if (!text.trim()) {
      this.doc.delete("raw");
      return;
    }
    const s = new Scalar(text.endsWith("\n") ? text : text + "\n");
    s.type = Scalar.BLOCK_LITERAL; // 原文保存：块字面量，注释与空行都不变
    this.doc.set("raw", s);
  }

  get hasRaw(): boolean {
    return this.raw.trim() !== "";
  }
}

function clean<T extends Record<string, unknown>>(o: T): Partial<T> {
  const out: Partial<T> = {};
  for (const [k, v] of Object.entries(o)) if (v !== undefined) (out as Record<string, unknown>)[k] = v;
  return out;
}
