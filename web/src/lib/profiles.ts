// 方案相关的共用操作：新建文档、渲染状态显示。
import { api, listAll, type Profile, type Source, type SourceInput, type Template } from "./api/client";
import { catalog } from "./catalog.svelte";
import { ProfileDoc } from "./profile-doc";

/** 用预设的 defaults 作为表单初值新建方案（第 10 节：预设决定新建方案时的表单初值）。 */
export async function createProfile(name: string, preset: string): Promise<Profile> {
  await catalog.load();
  const meta = catalog.meta!;
  const p = meta.presets.find((x) => x.id === preset) ?? meta.presets[0]!;
  const doc = ProfileDoc.create(name, { pack: meta.pack.id, preset: p.id, core: meta.pack.coreMax }, p.defaults);
  return api.post<Profile>("/api/v1/profiles", { doc: doc.toString(), note: "" });
}

export type RenderTone = "ok" | "err" | "warn" | "muted";

export function renderTone(p: Profile): RenderTone {
  const s = p.render?.status;
  if (s === "ok") return p.render?.diagnostics.some((d) => d.severity === "error") ? "warn" : "ok";
  if (s === "failed") return "err";
  return "muted";
}

// —— 新建配置：从模板或预设开始，可顺手添加节点（反馈 #51、#52）——

export interface NewProfile {
  name: string;
  /** 从模板开始；否则用预设的初值 */
  template?: Template;
  preset?: string;
  /** 订阅地址（单行 http/https）或分享链接、YAML 等节点内容；空表示不添加 */
  nodes?: string;
  /** 节点来源的名称（订阅默认取域名） */
  manualName: string;
}

/** 识别粘贴的节点输入：单行 http(s) 地址是订阅，其余作为手动节点内容（格式由服务器识别）。 */
export function nodeInput(text: string, manualName: string): SourceInput | null {
  const s = text.trim();
  if (!s) return null;
  if (/^https?:\/\/\S+$/.test(s)) {
    let host = s;
    try {
      host = new URL(s).hostname;
    } catch {
      /* 交给服务器校验 */
    }
    return { name: host, kind: "subscription", url: s };
  }
  return { name: manualName, kind: "batch", content: s };
}

/**
 * 新建配置。从模板开始时，模板中引用的、已被删除的节点来源会被去掉（返回 dropped 计数，由调用方提示）。
 * 新添加的节点来源使用管理员设定的默认写法。
 */
export async function createProfileFrom(n: NewProfile): Promise<{ profile: Profile; dropped: number }> {
  await catalog.load();
  const meta = catalog.meta!;
  let doc: ProfileDoc;
  let dropped = 0;
  if (n.template) {
    doc = new ProfileDoc(n.template.doc);
    doc.name = n.name;
    const existing = new Set((await listAll<Source>("/api/v1/sources")).map((s) => s.id));
    const keep = doc.sources.filter((r) => existing.has(r.ref));
    dropped = doc.sources.length - keep.length;
    if (dropped) doc.sources = keep;
  } else {
    const p = meta.presets.find((x) => x.id === n.preset) ?? meta.presets[0]!;
    doc = ProfileDoc.create(n.name, { pack: meta.pack.id, preset: p.id, core: meta.pack.coreMax }, p.defaults);
  }
  const input = n.nodes ? nodeInput(n.nodes, n.manualName) : null;
  if (input) {
    const [src, policy] = await Promise.all([
      api.post<Source>("/api/v1/sources", input),
      api.get<{ default: "inline" | "provider" }>("/api/v1/node-mode"),
    ]);
    doc.sources = [...doc.sources, policy.default === "provider" ? { ref: src.id, mode: "provider" } : { ref: src.id }];
  }
  const profile = await api.post<Profile>("/api/v1/profiles", { doc: doc.toString(), note: "" });
  return { profile, dropped };
}
