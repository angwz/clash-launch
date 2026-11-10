// 方案相关的共用操作：新建文档、渲染状态显示。
import { api, type Profile } from "./api/client";
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
